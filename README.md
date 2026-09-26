- querier (queries the db to get the native schema)
- parser (converts the normalised schems into language native schema)

HLD:
postgreql db   ---querier---> schema.Schema ------emitter----> Sqlite
               <----emitter-               <-----querier-------

once the schema migration itself is in place , for the data migration:

- order the schema by FK constraints (resuse the same fk dependency function we have in place probably so do the parent tables first)
- convert column values (postgres true/false becoumes 1/0 in sqlite etc)

HLD for ^^ 

follow a normaliser pattern, just like the schema migrater
choose normalised data types, so we can build the same language specific encoder -> canon -> decoder pipeline with the actual data
then, each column/data batch gets normalised first, then emitted into desired type



- use batch processing, read n rows, maintain pagination, write those n rows
- MAKE IT RESUMABLE, shouldnt loose data midway through (probably an sqlite table for that)

# done so far:
- querier that JUST qieries the postgres connection
- inspector for postgres->schema
- emitter (schema->sqlite migration file)
- schema migrator

# working on:
cleaning up the data migrater code and makin the global function

# TODO:
- have the order of tables, index columns and other shit be in the state db, so we dont have to rerun querier to get schema on every migrater resume
- do the cli 
- CONTEXT
- make the demo application
- have a worker pool and get rid of the n+1 queries with em
- make the backfill function work concurrently 

design for that:
- we need to change sort tables to be in levels, level 1: all tables with degrees 0, level 2: all dependedt tables in order
- process the level 0 tables concurrently 
- for the level 1 table,s concurrency can be done only within the table itself 
- instead of tracking the pagination with a rolling cursor, get the range of the pk/unique columns, and split her up n ways
- have each go routine work on each chunk
- change the state table to store with chunk_id (make sure the actual chunk size never changes. i.e: chunk id always maps to the same range of rows in the given snapshot)
- have a pool of *sql.Tx 
- the resume plan stays as is 
- note on this: sqlite doesnt support mutiple writes, so the parellalism would need to work in 2 chunks, always have the reads concurrent, but depending on the target, the writes can or cannot be concurrent


- flag to the user if a table doesnt have a unique index, or dont have a unique index where all columns are non nullable, fail this in the inspecter stage itself

# Next
- add resume to migrater (establish connection again and load everything back into memeory)
- increase limit for reads, but fir writes (especially for sqlite), fit to the limitations of that db (999 for sqlite)


# missed/gaps:
- certain postgres types
- check constraints
- comments
- reserved keywords in postgres that i havent accounted for yet (and their sqlite counterparts)
`	"current_timestamp":       "CURRENT_TIMESTAMP",
	"current_date":            "CURRENT_DATE",
	"current_time":            "CURRENT_TIME",
	"localtime":               "localtime",
	"localtimestamp":          "localtimestamp",
	"current_user": "NULL", // SQLite does not have users/roles
	"session_user": "NULL",
	"system_user":  "NULL",
	"current_role": "NULL",
`


# trade off
- not supporting indexes on functions like lower(email) etc, if they are a part of a composite index, that index will be rebuilt without it (wrong, but out of scope for now)
- a lot of loss, for example sqlite does have uuid, boolean etc, and also, does have some functions which sqlite doesnt support

- for certain indexes with predicates, only supporting = predicates right now, not >=, <= etc, (also include eg like LIKE, IN(), NOT IN() etc)

- not supporting the entire migration if a table doesnt have a unqiue index and if they do have unique indexes, but none of them not null



# scope for later
right now, the migrater is purely a one time thing. i.e, it will wipe your db, run the schema migrationa nd transfer all you data
later on, we can make this guy track state, the target connection doesnt need to be wiped, track its state and only migrate the diff, like alembic, but thats scope for later (never)

- write sqlite specific functions for postgres functions that dont have a sqlite counterpart

### references:
https://www.citusdata.com/blog/2016/03/30/five-ways-to-paginate/

### cdc model:
- we create a snapshot of the db
- we perform paginated reads on this snapshot 
      - for tables with unique indexes, we use that index for pagination
      - is the table doesnt have a unique index, what is wrong with u
      - we can either just migrate the entire table in one go, but no resume available simce we have nothing to store in pagination
      - we CAN use ctid, which is the physical row allocater, we can then do a `SELECT ctid, * FROM t WHERE ctid > $1 ORDER BY ctid LIMIT 500;` 
       problem with this too is that you have to do everythin in one transaction or the ctid can change,
      AND we cant have resume becasue of the volatile nature of ctid, so we have to either write the entire table in one go, or risk duplicate elements on a crash and resume (which wont even get filetered out becasue there ints any unique index)
      so, i think for this, we just flag, if any table doesnt have a unqiue constraint, we skip the entire migration
      - also, if you tables unique index doesnt have a not null constraint, kill urself, no service4u, becasue the pagination of >col will always be true

     - for composite unqiue indexes, we compare on all the values 
     `WHERE (col1, col2) > ($1, $2) ORDER BY col1, col2 LIMIT N`
     - we store the last read column/columns ids/values in out state table, per batch, after writing to the target db
     - on resume, we pick up from there itself
- after everything is comitted to target db from that current snapshot, we start streaming (create a new replConn or reuse existing, and track state via the lns)
- for resume, if we were to resume the backfill itself, just start reading from live db, using the per table checkpoints in the stateDb
- if we want to resume streaming, we use the other table in the stateDb, that tracks the lsn
