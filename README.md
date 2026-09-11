- querier (queries the db to get the native schema)
- parser (converts the normalised schems into language native schema)

HLD:
postgreql db   ---querier---> schema.Schema ------emitter----> Sqlite
               <----emitter-               <-----querier-------

once the schema migration itself is in place , for the data migration:
- order the schema by FK constraints
- convert column values (postgres true/false becoumes 1/0 in sqlite etc)
- use batch processing, read n rows, maintain pagination, write those n rows
- MAKE IT RESUMABLE, shouldnt loose data midway through (probably an sqlite table for that)
- verification
- locks need ot be maintained


# done so far:
- querier that JUST qieries the postgres connection
- inspector for postgres->schema
- emitter (schema->sqlite migration file)

# working on:
supporting
- partial indexes/ones with where clause eg: 
`CREATE INDEX idx_scores_team_leaderboard ON employee_scores(team_id, user_id, points_awarded) WHERE superseded = false;`


# Next
- write sqlite specific functions for postgres functions that dont have a sqlite counterpart
- sqlite schema migrater (needs to be in order of fks)

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


the querier already has the right fields, the inspecter and the emitter need it


# trade off
- not supporting indexes on functions like lower(email) etc, if they are a part of a composite index, that index will be rebuilt without it (wrong, but out of scope for now)
- a lot of loss, for example sqlite does have uuid, boolean etc, and also, does have some functions like gen_random_uuid(), so is the postgres table has 
`id UUID PRIMARY KEY DEFAULT gen_random_uuid()` 
it will just translate to 
`id TEXT PRIMARY KEY` and the user will have to change their code to now generate a uuid before inserting rows manually

- for certain indexes with predicates, only supporting = predicates right now, not >=, <= etc, (also include eg like LIKE, IN(), NOT IN() etc)


# scope for later
right now, the migrater is purely a one time thing. i.e, it will wipe your db, run the schema migrationa nd transfer all you data
later on, we can make this guy track state, the target connection doesnt need to be wiped, track its state and only migrate the diff, like alembic, but thats scope for later (never)
