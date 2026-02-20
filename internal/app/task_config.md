# Functional Overview

Legacy decoding responsibilities previously hosted in `task_config.go` now live within the individual action packages. Each action registers itself with `internal/actions/registry`, exposes its payload validation, and executes using the shared execution context.

# Technical Implementation Details

* **Per-action decoding:** Cassandra, Sleep, HTTP, Kubernetes, Evaluate, Print, and Variables packages own their respective payload structs and validation logic. Moving the code alongside the executors keeps invariants close to where they are enforced.
* **Registry integration:** Every package exposes an `init` function that calls `registry.Register`, providing an implementation of the `Action` interface. The interface accepts the raw payload plus the execution context, allowing actions to decode, validate, and run in one step.
* **Execution context:** `app.RunContext` builds a `registry.ExecutionContext` before invoking an action. The structure clones the current task, the full task slice, the shared variables map, and the task logger. After a successful run, the context synchronises updated variables back into the application state.
* **Flow control:** Actions can return optional `Control` directives (e.g. `JumpToTaskID`, `Exit`) in the `registry.Result`. The runner interprets these flags to implement branch-specific behaviour without hard-coding action names.
* **Payload expansion:** Variable interpolation lives in `internal/shared/expansion`, keeping expansion logic reusable for both the runner and action decoders that need it (e.g. HTTP body files).
