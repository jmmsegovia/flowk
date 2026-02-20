# Functional Overview

`flow.go` models flow definitions and tasks. It loads a JSON flow file, ensures each task has the required metadata, and prepares task structures for execution, including initial status values and raw payload retention for later decoding by specific actions.

# Technical Implementation Details

* **Type definitions:**
  * `ResultType` enumerates the supported data types (`bool`, `string`, `int`, `float`, `json`) that tasks can expose to subsequent steps.
  * `Definition` represents the overall flow with a description and ordered task list. It can optionally declare `OnErrorTask`, the task that should execute after the first failure instead of aborting immediately.
  * `TaskStatus` enumerates lifecycle states such as `not started`, `in progress`, `paused`, and `completed`.
  * `Task` captures identifiers, descriptive text, action name, runtime metadata (timestamps, duration, success flag, result, result type), and the raw JSON payload.
* **Custom JSON unmarshalling:** `(*Task).UnmarshalJSON` extracts the ID, description, and action while storing the entire raw payload in `Payload`. This allows action-specific decoders to access the original JSON body later and enables variable expansion to operate on the untouched JSON document before type-specific decoding occurs.
* **Flow loading:**
  * `LoadDefinition` reads a file from disk, validates it against the embedded JSON schema (`values.schema.json`) via `validateDefinitionAgainstSchema`, and unmarshals it into a `Definition` struct.
  * During post-processing, the function iterates over tasks to enforce non-empty IDs, descriptions, and actions, tracking duplicates using a map of IDs to indices. Each task's initial status is set to `TaskStatusNotStarted`.
* **Error reporting:** Validation errors include the task index and field name for clear diagnostics (e.g., duplicated IDs reference the original index).
* **Dependencies:** The file relies only on the standard library (`encoding/json`, `fmt`, `os`, `strings`, `time`) plus internal validation helpers, keeping the core data model lightweight.
