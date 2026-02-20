# Functional Overview

`print.go` implements the **PRINT** action. It evaluates a list of entries that can mix static text with runtime data (variables or prior task results) and forwards human-readable messages to the shared logger. The action also returns a structured JSON array capturing every rendered entry so subsequent tasks can consume the same information programmatically.

# Technical Implementation Details

* **Payload structure:**
  * `Payload` holds the ordered slice of `Entry` definitions supplied in the flow file.
  * Each `Entry` optionally carries a `message` prefix and references either a flow variable (`variable`) or a completed task (`taskId` plus optional `field`).
* **Validation:** `Payload.Validate` ensures at least one entry is present and delegates to `Entry.Validate`, which forbids mixing `variable` and `taskId` in the same item and requires a `message` when no data source is provided.
* **Execution flow:**
  * `Execute` validates the payload, iterates over every entry, resolves the referenced value, and logs a formatted string (`prefix: value` when a data source is present).
  * Variable lookups read from the runner context. Secrets are masked as `****` to avoid leaking sensitive information.
  * Task lookups require the referenced task to be completed. Field extraction relies on `evaluate.ResolveFieldValue`, providing identical semantics (metadata fields and `result$` JSONPath support) across actions.
* **Result formatting:** Each resolved entry becomes a `ResultEntry` with the final message prefix and the resolved value (masked when necessary). Complex values are marshalled to JSON only for display purposes; the returned value preserves the underlying Go type for downstream reuse.
* **Helpers:**
  * `formatValue` prepares readable log output by JSON-encoding slices/maps when possible and falling back to `fmt.Sprintf` for primitive types.
  * `findTask` mirrors existing helper logic in other actions, scanning the slice of prior tasks by identifier.
