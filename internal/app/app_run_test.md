# Functional Overview

`app_run_test.go` runs the application orchestrator against miniature environment and flow definitions to ensure branching logic, task selection, and summary logging behave as expected. The tests simulate real execution paths without requiring external services.

# Technical Implementation Details

* **Logger stub:** `bufferLogger` collects log entries in a thread-safe slice so tests can inspect output ordering and content.
* **Task selection:** `TestRunBeginsFromSpecifiedTask` confirms that supplying `begin-from-task` skips earlier tasks and that their status remains `not started` in the summary log. `TestRunFailsWhenBeginTaskNotFound` ensures a missing ID produces an error.
* **Branch actions:** `TestRunEvaluateBranchActions` constructs a flow with evaluate tasks that trigger continue, sleep, and goto behaviours. The test asserts that sleep actions log messages, skipped tasks remain `not started`, and goto jumps land on the correct task.
* **Exit handling:** `TestRunEvaluateExitStopsFlow` verifies that an evaluate task with `exit` stops the flow immediately and logs the exit message, leaving subsequent tasks untouched.
* **Variables integration:** Additional integration tests are planned to exercise VARIABLE tasks and confirm that declared values are expanded into later payloads during orchestration.
* **Fixture helpers:**
  * `writeEnvironmentAndFlow` writes a minimal environment and flow definition along with the JSON schema for schema validation.
  * `writeSchemaFile` copies the project schema next to temporary flows, mirroring production requirements.
* **Runtime utilities:** Tests use `context.WithTimeout` to bound execution time, and the `runtime`/`filepath` packages to locate the schema file relative to the test source.
