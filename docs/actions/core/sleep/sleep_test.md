# Functional Overview

`sleep_test.go` verifies that the Sleep action waits for the requested duration, returns immediately for zero-length sleeps, honours context cancellation, and rejects negative inputs.

# Technical Implementation Details

* **Elapsed time measurement:** `TestExecuteWaitsForDuration` records the time before invoking `Execute` and asserts that at least 45 milliseconds have elapsed when requesting a 0.05 second pause. It also checks the returned result is a `float64` with the same value and that the reported type is `flow.ResultTypeFloat`.
* **Zero duration:** `TestExecuteImmediateForZero` ensures a zero-second sleep responds instantly while still reporting the correct result value and type.
* **Cancellation:** `TestExecuteReturnsErrorOnCancellation` creates a cancelled context via `context.WithCancel` and confirms the function returns the sentinel error `context.Canceled`.
* **Validation:** `TestExecuteRejectsNegativeSeconds` checks that negative durations cause an error, validating the guard clause in the implementation.
