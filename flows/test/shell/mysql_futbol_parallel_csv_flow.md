# mysql_futbol_parallel_csv_flow

Detailed Description
MySQL flow with PARALLEL and EVALUATE loading football data from CSV (Docker included). Imports 4 subflow(s): mysql_futbol_parallel_csv_setup, mysql_futbol_parallel_csv_load, mysql_futbol_parallel_csv_queries, mysql_futbol_parallel_csv_cleanup. Primary actions: MySQL operations, Docker container management, assertions and conditional logic, parallel execution, shell command execution, controlled waits, variable configuration.

Requirements
- Docker installed and daemon running.
- Reachable MySQL instance and valid credentials.
- Shell access and any binaries invoked by the flow's commands.
