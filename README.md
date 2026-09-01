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

# Next
- parse querier struct to get schema.Schema
