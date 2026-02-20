# Functional Overview

`main_test.go` ensures the command-line argument parser handles the different supported flag formats and error paths. The tests validate combinations of positional arguments, flag ordering, inline values, mutual exclusivity rules, and failure reporting.

# Technical Implementation Details

* **Testing framework:** Uses Go's built-in `testing` package. Each test calls `parseArgs` directly to inspect the parsed values and error states.
* **Supported flag patterns:**
  * `TestParseArgsSupportsFlagsInAnyOrder` confirms flags can be supplied in any sequence and that `-begin-from-task` clears when `-run-task` is absent.
  * `TestParseArgsSupportsValuesAfterEquals` checks that empty placeholders like `-flow=` cause the parser to read the next argument as the value, matching GNU-style flag behaviour.
  * `TestParseArgsSupportsPositionalArguments` verifies positional fallbacks populate the required flow path when the flag is omitted.
* **Error handling assertions:**
  * `TestParseArgsUnexpectedArguments` ensures extra positionals yield explicit errors rather than `flag.ErrHelp`.
  * `TestParseArgsRunTaskConflictsWithBeginFromTask` enforces the mutual exclusivity constraint between `-run-task` and `-begin-from-task`.
  * `TestParseRunArgsRunSubtaskConflictsWithBeginFromTask` enforces the mutual exclusivity constraint between `-run-subtask` and `-begin-from-task`.
* **Specific flag behaviour:**
  * `TestParseArgsRunTask` confirms that the dedicated `-run-task` flag targets a single task and suppresses the `beginFromTask` output field.
  * `TestParseRunArgsRunSubtask` confirms that the dedicated `-run-subtask` flag targets a single subtask.
* **String containment checks:** The tests use `strings.Contains` to check error messages, ensuring the parser presents actionable text to end users.
