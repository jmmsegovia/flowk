# Functional Overview

`sleep.go` defines the **SLEEP** action which pauses the workflow for a configurable number of seconds. It respects cancellation so that long waits can be aborted when the overall flow terminates early. The action reports the sleep duration as its result for downstream tasks.

# Technical Implementation Details

* **Inputs:** `Execute` receives a `context.Context`, the requested duration in seconds (as a `float64` to match JSON decoding), and an optional logger.
  Variable interpolation happens prior to invocation, so flow authors can declare delays using `${}` placeholders that resolve to numeric values.
* **Validation:** Negative durations are rejected with an error, protecting the workflow from misconfigured definitions.
* **Duration handling:** The number of seconds is converted into a `time.Duration` by multiplying by `time.Second`. Durations less than or equal to zero trigger an immediate return that still reports the configured number of seconds and the `flow.ResultTypeFloat` type.
* **Logging:** When a logger is provided, `Printf` is used to emit a message describing the sleep length using `%.2f` formatting for readability.
* **Timer management:**
  * A `time.NewTimer` is created for positive durations and stopped with `defer timer.Stop()` to release resources.
  * A `select` statement waits on either `ctx.Done()`—returning the context error if cancellation happens first—or the timer's channel, in which case the action returns the sleep length and declares its type as `flow.ResultTypeFloat`.
