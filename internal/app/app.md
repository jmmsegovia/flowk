# Functional Overview

`app.go` orchestrates the execution of a flow. It loads environment and flow definitions, iterates through tasks, runs the corresponding action implementation, expands declared variables, and manages branching logic for conditional tasks. The function records execution metadata (start/end time, duration, success) and logs a summary once the flow completes.

# Technical Implementation Details

* **Run lifecycle:**
  * `Run` accepts a context, file paths for the environment and flow, a logger, and optional task selectors (`startTaskID`, `singleTaskID`).
  * It uses `config.LoadEnvironment` and `flow.LoadDefinition` to obtain configuration data.
  * When a flow declares `OnErrorTask`, the runner jumps directly to that task after the first failure, executes it, and then surfaces the original error to the caller.
  * `logFlowSummary` runs after execution so a summary of each task is printed regardless of success or failure.
  * A `RunContext` initialises a `Vars` map that stores flow-scoped variables created by VARIABLES tasks.
* **Task selection:**
  * If `singleTaskID` is provided, only that task runs (`startIdx`/`endIdx` are adjusted). When `begin-from-task` is set, execution starts at the specified task and proceeds sequentially.
  * Helper functions `findTaskIndexByID` and `findTaskByID` scan the task slice, returning indices or pointers, or signalling errors when tasks are missing.
* **Task execution:**
  * Each iteration marks the task as `in progress`, records timestamps (`StartTimestamp`, `EndTimestamp`), and logs the description.
  * `registry.Lookup` resolves the concrete action registered by each package. The retrieved implementation receives a cloned execution context (task pointer, tasks slice, variables map, logger) and returns a `registry.Result` with the value, result type, and optional flow-control directives.
  * Errors returned by the action are wrapped with the task index for context; a missing registration yields an “unsupported action” error.
* **Task payload expansion:** Before dispatching, `internal/shared/expansion` helpers walk the JSON payload, interpolating `${var}` placeholders from the shared variables map so every action receives fully materialised configuration. Evaluate payloads use a specialised variant that preserves conditional expressions.
* **Evaluate branching:**
  * The evaluate action now encapsulates branch selection and logging, emitting control directives (goto/exit) via the returned `registry.Result`.
  * The runner interprets these directives, adjusting the loop index or terminating execution when requested.
* **Result tracking:** On success, the task's `Result` and `ResultType` fields are populated so later tasks—including VARIABLES steps that read JSON results—can inspect previous outputs. Failures clear the result and propagate an error, aborting the run.
* **Summary logging:** `logFlowSummary` iterates over all tasks and prints their status, success flag, result type, and a stringified result (JSON is marshalled to preserve structure).
