# Worm

Worm moves a live PostgreSQL database into SQLite without taking the source offline. It copies a snapshot of your data, then keeps following the changes that happen while the copy runs, so the two databases end up in sync.

It started because I was paying for RDS when SQLite would have been fine. Plenty of tools already do this faster and better than Worm does, but oh well

This README doubles as a walkthrough of how the pieces work, so it explains the ideas as well as the commands.

- [The idea](#the-idea)
- [Quick start](#quick-start)
- [Commands](#commands)
- [How a migration runs](#how-a-migration-runs)
- [Normalisation](#why-we-normalise)
- [Backfill](#backfill)
- [Concurrency](#concurrency)
- [The state db](#the-state-db)
- [Streaming](#streaming)
- [Resuming and resetting](#resuming-and-resetting)
- [Limits and trade-offs](#limits-and-trade-offs)
- [TODO](#todo)

## The idea

How do you move a database that's still being written to?

The simple way: stop the app, dump everything, load it, start the app again. That's downtime, and it grows with the size of your data.

Worm uses **Change Data Capture (CDC)** instead:

1. Take a **snapshot** of the source. It's a frozen, consistent view of the data at one moment.
2. **Backfill**: copy that snapshot into the target.
3. **Stream**: replay every change that happened on the source *after* the snapshot, until the target has caught up.

The snapshot gives you a clean starting point. The stream covers everything after it. The hard part is making sure the two meet exactly, with no gap and no overlap. Postgres has features for exactly this.

### Postgres features Worm relies on

**Write-ahead log (WAL).** Before Postgres changes a table, it writes the change to a log. It does this for crash recovery, but the log is also a full record of every insert, update and delete.

**Logical replication slot.** Postgres normally deletes old WAL once it no longer needs it. A slot is a bookmark that says "keep everything after this point until I say I've read it." While the slot exists, the changes since the bookmark can't disappear. The cost is that an abandoned slot makes Postgres hold WAL forever and fill your disk. That's why there's a `migrate-reset` command.

**LSN (log sequence number).** A position in the WAL. When you create a slot, Postgres tells you its starting LSN. That's the clean cutoff: everything before it is in the snapshot, everything after it comes from the stream.

**Exported snapshot.** When Worm creates the slot, Postgres can also hand back a snapshot name that matches that exact LSN. Worm reads all its backfill data through that snapshot (`SET TRANSACTION SNAPSHOT '...'`). So the data you copy and the point where you start streaming line up exactly.

**Publication.** A list of tables whose changes you want. Worm creates one called `worm_pub` for all the tables it migrates.

## Quick start

You need Go. The demo also needs Docker and `psql`.

### Try it on your own database

1. Make sure your Postgres is set up for logical replication (see the note below).
2. Create a `.env` file in the project root:

   ```bash
   SOURCE_DB=POSTGRESQL
   TARGET_DB=SQLITE
   SOURCE_CONN_STR=<your conn string>
   TARGET_CONN_STR=<your conn string>
   ```

   The target path must end in `.db`. `SOURCE_DB` and `TARGET_DB` have to match their connection strings, or Worm exits with a mismatch error.

3. Run the commands in order:

   ```bash
   go build -o worm .  # build her
   
   ./worm migrate-schema    # creates the tables in SQLite (wipes the target first)
   ./worm migrate-data      # backfills, then keeps streaming. Ctrl+C when you're done
   ./worm migrate-resume    # pick up where you left off, and sync again later
   ```

> **Note: Postgres needs `wal_level=logical`.**
> Replication slots, publications and change streaming only work when Postgres writes enough detail into the WAL. That's controlled by the `wal_level` setting. The default is `replica`, which is too low, and `migrate-data` fails when it tries to create the slot.
>
> Check yours with `SHOW wal_level;`. To change it, set `wal_level = logical` in `postgresql.conf` and **restart** Postgres, because a reload isn't enough. In Docker, pass it on the command line:
>
> ```yaml
> command: ["postgres", "-c", "wal_level=logical"]
> ```
>
> Managed services have their own switch. On RDS, set the parameter `rds.logical_replication = 1` and reboot.
>
> The user in your connection string also needs the `REPLICATION` attribute (superusers already have it) and permission to create publications on the database.

### Try the demo

This starts a throwaway Postgres in Docker, already set up for logical replication and loaded with a sample schema. You need `psql` installed.

```bash
make dev-up        # starts Postgres and loads the sample schema
make seed          # adds some rows

mkdir -p .data
cat > .env <<'EOF'
SOURCE_DB=POSTGRESQL
TARGET_DB=SQLITE
SOURCE_CONN_STR="postgres://postgres:password@localhost:5432/worm_dev?sslmode=disable"
TARGET_CONN_STR="./.data/yay.db"
EOF

go build -o worm .

./worm migrate-schema
./worm migrate-data      # Ctrl+C once it's streaming
```

Now give the stream something to do. Add more rows to the source, then resume:

```bash
make seed
./worm migrate-resume
make counts                # compares row counts between source and target, table by table
```

To start over, run `./worm migrate-reset`.

## Commands

| Command | What it does |
| --- | --- |
| `migrate-schema` | Reads the source schema, normalises it, and creates the equivalent tables in the target. **This wipes the target's tables first.** It also writes the migration plan into the state db. |
| `migrate-data` | Creates the slot and snapshot, backfills every table, then starts streaming. |
| `migrate-resume` | Continues an interrupted backfill, then streams from the last saved LSN. Safe to run again later to catch up on new changes. |
| `migrate-reset` | Drops Worm's replication slot and publication on the source, wipes the target tables, and clears the state db. |

The commands have to run in order: `migrate-schema`, then `migrate-data`, then `migrate-resume` as often as you like. Worm tracks a "stage" in the state db and refuses to run a command out of order.

`migrate-reset` exists because slots and publications don't clean themselves up. Run it before starting a fresh migration against the same databases, and don't forget it, or the source keeps piling up WAL.

## How a migration runs

```
 migrate-schema                                        migrate-data / migrate-resume
 ──────────────                                        ─────────────────────────────
 Postgres ──querier──▶ rows from catalogs              create publication + slot ──▶ LSN + snapshot
          ──inspector─▶ schema.Schema (normalised)             │
          ──emitter───▶ .sql migration file                    ▼
          ──schema-migrator─▶ SQLite tables             backfill: read snapshot ─▶ write queue ─▶ SQLite
                                                               │
                                                               ▼
                                                        stream: read WAL from LSN ─▶ SQLite
```

Each step has its own package:

| Package | Job |
| --- | --- |
| `querier` | Runs the raw catalog queries against the source and returns plain rows. It knows nothing about schemas. |
| `inspect` | Turns those rows into the normalised `schema.Schema`. |
| `emmit` | Turns a `schema.Schema` into target-specific SQL, and sorts tables by foreign key dependency. |
| `schema-migrator` | Runs the SQL file against the target, in one transaction. |
| `data-migrator` | Reads from the source: snapshot, backfill, stream. |
| `data-writer` | Writes to the target. |
| `cli` | The commands above, and the plan saved in the state db. |

## Why we normalise

The straightforward way to build a migrator is one converter per pair: Postgres→SQLite, Postgres→MySQL, SQLite→Postgres, and so on. With *m* sources and *n* targets that's *m × n* converters.

Worm converts everything into a middle form first. Each source only has to learn to produce that form (*m* pieces of code), and each target only has to learn to consume it (*n* pieces). Adding a database means writing one side, not one converter per existing database.

The middle form lives in `schema/`. For the schema it's `Schema`, `Table`, `Column`, `Index` and `ForeignKey`, with a small set of types: bool, integer, uuid, time, text, json, enum. For data it's the `Batch`: a table name, column names, and rows of plain Go values.

Postgres has a lot of types, and SQLite has very few. So going from one to the other loses information, and the middle form is where the loss gets decided in one place. Currently:

| Postgres | Normalised | SQLite |
| --- | --- | --- |
| `boolean` | bool | `INTEGER` 0/1 with a `CHECK (col IN (0, 1))` |
| `integer`, `bigint`, ... | integer | `INTEGER` |
| `uuid` | uuid | `TEXT` |
| `timestamptz`, ... | time | `TEXT` (RFC 3339) |
| `json`, `jsonb` | json | `TEXT` |
| `enum` | enum | `TEXT` with a `CHECK (col IN ('a', 'b', ...))` |
| `text`, `varchar`, ... | text | `TEXT` |

The same applies to default values. `now()` becomes `CURRENT_TIMESTAMP`, and functions SQLite doesn't have (like `gen_random_uuid()`) are mapped or dropped. See `schema/dialects.go` and `emmit/sqlite.go` for the tables.

Data goes through the same idea. When rows are read from Postgres they're *decoded* into the normalised value (the source side). When they're written to SQLite they're *encoded* into what SQLite wants (the target side). For example, `true` becomes `1`.

## Backfill

Backfill copies the snapshot into the target. A few decisions go into it.

### Order tables by foreign keys

If `orders` has a foreign key to `customers`, `customers` has to be copied first, or the insert fails. Worm does a topological sort of the tables (Kahn's algorithm): tables that depend on nothing go first, then the tables that only depend on those, and so on. Tables in a cycle can't be ordered and are left at the end.

The sort also groups the tables into **levels**. Level 0 holds tables with no dependencies, level 1 holds tables that only depend on level 0, and so on. The concurrency model below uses the levels.

### Read in pages with keyset pagination

Reading a big table in one query would use too much memory, and a crash would lose all progress. So Worm reads it in pages.

The obvious way to page is `LIMIT 500 OFFSET 1000`, but it gets slower the deeper you go, because the database still has to walk past all the skipped rows. Instead, Worm uses **keyset pagination**: remember the last key you saw, and ask for rows after it.

```sql
SELECT * FROM orders WHERE id > $1 ORDER BY id LIMIT 500;
```

With a composite key, compare the whole tuple:

```sql
SELECT * FROM t WHERE (a, b) > ($1, $2) ORDER BY a, b LIMIT 500;
```

This costs the same on page 1 as on page 10,000, and "the last key I saw" is something we can save to disk. Further reading: [Five ways to paginate in Postgres](https://www.citusdata.com/blog/2016/03/30/five-ways-to-paginate/).

### Which key to paginate on

Pagination needs a column (or columns) that is unique and never NULL. Worm picks the primary key first, then falls back to a unique index whose columns are all `NOT NULL`.

If a table has neither, Worm refuses to migrate. This is deliberate. Without a unique key there's nothing to resume from, and a crash-and-retry would silently write duplicate rows (nothing would reject them). A unique index that allows NULLs doesn't help either: `col > NULL` is never true, so pagination would stop dead.

I couldn't find a nice way to chunk tables with no key. Postgres's `ctid` (physical row location) can change under you and isn't stable across a restart. If you've solved this, I'd like to hear about it.

### Batch size

Reads default to 500 rows per batch. Writes to SQLite are limited by its bound-parameter cap (999 in older builds). The writer has to keep `rows × columns` under that.

## Concurrency

Concurrency applies to **backfill only**. It's where all the time goes.

### Reads: chunk each table and fan out

Instead of walking one table with a single moving cursor, Worm splits each table into **chunks** and gives each chunk to a goroutine.

1. Look up the min and max of the pagination key.
2. Split that range into fixed-size chunks. Each chunk gets a number, its `chunk_id`.
3. A pool of workers picks up chunks. Each worker reads its chunk (still page by page, with keyset pagination inside the chunk).

Two rules keep this safe:

- **The chunk size never changes**, so `chunk_id` always maps to the same range of keys in a given snapshot. That's what makes resume work: "chunk 7 is half done" only means something if chunk 7 always covers the same rows.
- **Every worker reads from the same snapshot.** Each worker gets its own `*sql.Tx` from a pool, and each one runs `SET TRANSACTION SNAPSHOT` with the exported snapshot id. They all see exactly the same data, as of the slot's LSN.

The foreign key levels decide what runs at the same time:

- **Level 0:** these tables don't depend on anything, so they all run in parallel.
- **Later levels:** tables run one after another, and the parallelism comes from chunks *within* the table. A child table never starts before its parents are done.

### Writes: one queue, one writer for SQLite

Reading is parallel on any source. Writing depends on the target.

SQLite allows only one writer at a time. If many goroutines tried to write to it, they'd just fight over the lock and fail with `database is locked`. So the pipeline is split in two:

```
 chunk workers (many)                    writer (one, for SQLite)
 ┌──────────┐
 │ chunk 0  │──┐
 ├──────────┤  │     ┌─────────────┐     ┌────────────────────────────┐
 │ chunk 1  │──┼────▶│ write queue │───▶ │ 1. write batch to target   │
 ├──────────┤  │     └─────────────┘     │ 2. commit                  │
 │ chunk 2  │──┘                         │ 3. update the state db     │
 └──────────┘                            │ 4. ack back to the worker  │
                                         └────────────────────────────┘
```

Workers decode their rows into normalised batches and push them on the queue. The writer takes them one at a time, encodes them for SQLite, and writes. This split works because of the normalised middle layer: the read side and the write side only meet at the `Batch`.

The write side is concurrent in design too. For a target that allows parallel writes (Postgres, once it's supported), you'd run several writers on the same queue. Only SQLite gets the single-writer version.

### Ack after write

The order inside the writer matters:

1. Write the batch to the target and **commit**.
2. Only then, record the progress in the state db.
3. Only then, ack the worker so it moves to the next page.

Progress is never recorded before the data is safely in the target. If you press Ctrl+C anywhere in that sequence:

- Killed before the commit: the batch isn't in the target and isn't in the state db. On resume it's read again. Nothing is lost.
- Killed after the commit but before the state update: the batch is in the target but the state db doesn't know. On resume it's read and written *again*. This is safe because the insert is `INSERT OR IGNORE`, so rows that already exist are skipped. The key is unique, so duplicates can't happen.

So the state db may lag behind the target, but it never runs ahead of it. That's all a resume needs.

Writes also run under their own timeout, detached from the cancelled context. So when you press Ctrl+C, the batch in flight can still finish and be checkpointed instead of being torn in half.

## The state db

Worm keeps its own bookkeeping in a small SQLite file, `./.data/state.db`. It's separate from the target. All the tables start with `capture_`.

**`capture_stage`**: which command may run next (1 = schema done, 2 = data started, 3 = resumed). Commands claim a stage with a single atomic `UPDATE ... WHERE stage IN (...)`, so two runs can't both start at once, and running commands out of order fails with a clear message.

**`capture_plan_tables`**: for each table, its position in the FK-sorted order and the index used for pagination (name, columns, whether unique). Saved at `migrate-schema` time so `migrate-data` and `migrate-resume` don't have to re-inspect the source.

**`capture_plan_columns`**: for each column, its normalised type (and enum name and values). The decoders and encoders use this to know what to do with each value.

**`capture_snapshot_state`**: one row: the slot name, the exported snapshot id, and the LSN. The LSN starts as the slot's starting point and moves forward as the stream commits, so it also serves as the stream's checkpoint.

**`capture_batch_state`**: one row per **chunk**, primary key `(table_name, chunk_id)`:

| Column | Meaning |
| --- | --- |
| `table_name`, `chunk_id` | Which chunk. |
| `range_start`, `range_end` | The key range this chunk covers. Fixed for the life of the migration. |
| `status` | `pending`, `in_progress` or `done`. |
| `last_index_values` | JSON of the last key written, e.g. `["a1b2..."]` or `[3, "x"]` for a composite key. This is the keyset cursor. |
| `rows_done`, `total_rows` | Progress, for chunk and table totals. |
| `updated_at` | Last write. |

A table is done when all of its chunks are `done`.

On resume, a chunk with status `done` is skipped. A chunk that's `in_progress` restarts from `last_index_values`, i.e. `WHERE key > last_index_values AND key <= range_end`. A chunk that's `pending` starts from `range_start`.

Notes:

- Because the checkpoint is written only after the target commit (see [Ack after write](#ack-after-write)), `last_index_values` can be behind the target but never ahead.
- The state db is plain SQLite and lives next to your target. You can open it with `sqlite3 .data/state.db` and see exactly where a migration is.

## Streaming

When every chunk of every table is `done`, the backfill is complete and streaming begins.

Streaming is **sequential, on purpose**. Order is the whole point of a change log: if a row is inserted, updated and then deleted, replaying those out of order gives the wrong result. So one loop reads changes in WAL order and applies them one at a time.

It works like this:

1. Read the saved LSN from `capture_snapshot_state`.
2. Open a replication connection and run `START_REPLICATION` on the slot from that LSN, using the `pgoutput` plugin and the `worm_pub` publication.
3. Postgres sends messages. A `Relation` message describes a table's columns (Worm caches these). `Insert`, `Update` and `Delete` messages carry the changed rows, and a `Commit` marks the end of a transaction.
4. Each change is decoded into a normalised `Batch`, then written to the target as an insert (`INSERT OR IGNORE`), an update, or a delete, found by key.
5. On each `Commit`, Worm saves that commit's LSN to the state db.
6. Every 10 seconds it sends Postgres a status update ("I've processed up to here"), which lets Postgres release old WAL.

Updates and deletes need to know the row's *old* key (the key might have changed). Postgres sends the old key when it changes, and Worm falls back to the new row's key when it doesn't.

The stream keeps running until you stop it. Run `migrate-resume` any time later to catch up on whatever changed in between.

## Resuming and resetting

**Resuming** is built from everything above. Press Ctrl+C at any time: Worm catches the signal, finishes the write in flight, saves progress, and exits with a note to run `migrate-resume`. Running it reads the state db, skips finished chunks, restarts unfinished ones from their last checkpoint, and then streams from the saved LSN.

On resume the backfill reads from the live source, because the old snapshot died with the old process. That's fine: the rows are inserted with `INSERT OR IGNORE`, and anything that changed since the snapshot is also in the WAL, which the stream replays afterwards. The slot guarantees that part hasn't been thrown away.

**Resetting** (`migrate-reset`) is how you start over, or clean up. It drops the slot and publication on the source, wipes the target tables, and empties the state db. Stop any running `migrate-*` first, or the slot is still active and Postgres won't drop it.

## Limits and trade-offs

Worm is a learning project. Know what it doesn't do:

- **Postgres → SQLite only** for now (see the TODO).
- **It's a one-shot migration.** `migrate-schema` wipes the target. It doesn't diff against what's already there.
- **Every table needs a primary key or a unique `NOT NULL` index.** Otherwise Worm refuses to start. See [Which key to paginate on](#which-key-to-paginate-on).
- **Type loss is real.** SQLite has no UUID, boolean or timestamp types, so those become `TEXT` or `INTEGER` with `CHECK` constraints. Some Postgres functions have no SQLite equivalent.
- **Indexes on expressions** (like `lower(email)`) aren't supported. If one is part of a composite index, the index is rebuilt without it, which is wrong but out of scope for now.
- **Partial indexes** only support `=` predicates, not `>=`, `LIKE`, `IN` and so on.
- **Not covered yet:** some Postgres types, check constraints, comments, and some reserved keywords.
- Unchanged TOASTed values in a streamed update arrive as "unchanged" and are written as `NULL`.

## TODO

Right now only one side of each pair exists, which is why only Postgres → SQLite works. Each missing counterpart is a separate piece of work.

**Directions**
- [x] Postgres → SQLite
- [ ] Postgres → Postgres
- [ ] SQLite → SQLite
- [ ] SQLite → Postgres

**Source side** (reads a database into the normalised form)
- [x] Postgres querier
- [x] Postgres inspector
- [x] Postgres data migrator (snapshot, backfill, stream)
- [ ] SQLite querier
- [ ] SQLite inspector
- [ ] SQLite data migrator (backfill, and change capture for the stream)

**Target side** (writes the normalised form into a database)
- [x] SQLite emitter (schema → SQL)
- [x] SQLite schema migrator
- [x] SQLite data writer
- [ ] Postgres emitter
- [ ] Postgres schema migrator
- [ ] Postgres data writer (this one can use several concurrent writers)
