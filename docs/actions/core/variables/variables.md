# Functional Overview

`variables.go` implements the **VARIABLES** action. It declares flow-scoped variables, optionally resolves values from previous task results, and stores the resolved values so subsequent tasks can reference them using `${name}` placeholders.

# Technical Implementation Details

* **Payload validation:**
  * `Payload.Validate` enforces that the optional scope is either empty or `flow`, that the declaration list is non-empty, and that variable names are unique within the payload.
  * `VariableConfig.Validate` checks name formatting (alphanumeric, underscores, dashes, and dots), ensures the declared type is supported (`string`, `number`, `bool`, `array`, `object`, `secret`, or `proxy`), and verifies that any arithmetic `operation` block targets `number` variables with a supported operator.
* **Execution flow:**
  * `Execute` revalidates the payload, honours the `overwrite` flag, and prevents redeclarations within the same task. When an `operation` is provided it fetches the current value of the target variable, resolves the referenced operand variable, and applies the requested arithmetic (add, subtract, multiply, divide) before storing the updated number.
  * Proxy variables (`type: "proxy"`) are normalised into `map[string]string` entries so that downstream actions, such as SHELL, can materialise HTTP/HTTPS/NO proxy environment variables.
  * Each value passes through `resolveValue`, which interprets `${from.task:<id>.<jsonpath>}` placeholders by locating the referenced task, verifying it completed successfully with a JSON result, and applying the JSONPath expression via `github.com/PaesslerAG/jsonpath`.
  * `coerceValue` converts the resolved value into the requested type, handling strings, numbers, booleans, arrays, objects, and masking `secret` values in execution summaries.
* **Result exposure:** The function returns a map of declared variables along with the `flow.ResultTypeJSON` identifier, enabling logs to show which variables were defined while still hiding secret contents.
* **Helper utilities:**
  * `normalizeJSONPath` and `normalizeJSONContainer` adapt JSONPath expressions and task results into a format the jsonpath library can evaluate reliably.
  * `findTask` navigates the flow's task list by ID, ensuring references only succeed when the target task has finished and produced JSON data.
