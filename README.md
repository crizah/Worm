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
- initial version of postgres inspector (untested)

# Next
- test the postgres inspector
- work on opposite version next (schema -> postgres)


# trade off
- not supporting indexes on functions like lower(email) etc, if they are a part of a composite index, that index will be rebuilt without it (wrong, but out of scope for now)
