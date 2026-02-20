# EVALUATE Action

The **EVALUATE** action inspects the outcome of a previously executed task or variable values and compares them against declared conditions. It is the primary mechanism for assertions, validating complex JSON structures, and implementing conditional logic (if/else) in FlowK.

## Usage

```json
{
  "id": "check_response",
  "action": "EVALUATE",
  "if_conditions": [
    {
      "left": "${from.task:http_request.result$.response.status_code}",
      "operation": "=",
      "right": 200
    },
    {
      "left": "${from.task:http_request.result$.response.body.tags}",
      "operation": "CONTAINS",
      "right": "verified"
    }
  ],
  "then": {
    "continue": "Validation passed"
  },
  "else": {
    "exit": "Validation failed"
  }
}
```

## Operands

### Left Operand (`left`)
The `left` field specifies the value to inspect. It typically references a task result, metadata, or a variable, but can also be a literal value (useful when checking if a literal exists in a collection).

*   **Task Result (JSON Path)**: Use `result$` followed by a JsonPath expression.
    *   `${from.task:my_task.result$.response.body.users[0].id}`
    *   `${from.task:my_task.result$.items[?(@.active == true)]}`
*   **Task Metadata**:
    *   `${from.task:my_task.status}` (e.g., "completed", "failed")
    *   `${from.task:my_task.success}` (true/false)
*   **Variables**:
    *   `${my_variable}`

### Right Operand (`right`)
The `right` field specifies the expected value. It can be:
*   **Literal**: `"success"`, `""` (empty string), `200`, `true`, `["a", "b"]`, `{"key": "value"}`.
*   **Reference**: Another task result or variable (e.g., `${expected_status}`).

## Supported Operations

### Equality

| Operator | Description | Example |
| :--- | :--- | :--- |
| `=` | Checks types and values for equality. | `status = 200` |
| `!=` | Checks if values are not equal. | `status != 500` |

### Numeric Comparison

| Operator | Description | Example |
| :--- | :--- | :--- |
| `>` | Greater Than | `count > 0` |
| `<` | Less Than | `latency < 500` |
| `>=` | Greater or Equal | `retries >= 3` |
| `<=` | Less or Equal | `score <= 10` |

### String Operations

| Operator | Description | Example |
| :--- | :--- | :--- |
| `STARTS_WITH` | Prefix check | `url STARTS_WITH "https://"` |
| `ENDS_WITH` | Suffix check | `file ENDS_WITH ".json"` |
| `MATCHES` | Regular Expression match | `version MATCHES "^v[0-9]+\."` |
| `CONTAINS` | Substring check | `log CONTAINS "error"` |
| `NOT_CONTAINS` | Negated substring check | `log NOT_CONTAINS "panic"` |

### Collection Operations

These operators apply when one operand is an Array/Slice.

| Operator | Description | Logic | Example |
| :--- | :--- | :--- | :--- |
| `IN` | Item in List | Left value exists in Right list | `"US" IN ["US", "UK"]` |
| `NOT_IN` | Item Not in List | Left value is NOT in Right list | `"IT" NOT_IN ["US", "UK"]` |
| `CONTAINS` | List contains Item | Left list contains Right value | `tags CONTAINS "admin"` |
| `NOT_CONTAINS` | List doesn't contain | Left list does NOT contain Right value | `errors NOT_CONTAINS "timeout"` |

> [!TIP]
> **Checking Literals in Lists**:
> Prefer `CONTAINS`/`NOT_CONTAINS` with the **list field as the left operand**.
> *   ✅ `tags CONTAINS "admin"`
> *   ❌ `"admin" IN tags` (Avoid if possible)

## Boolean Logic
*   **AND Logic**: All conditions defined in `if_conditions` must evaluate to `true` for the action to succeed (execute `then`).
*   **Failure**: If **any** condition fails, the action executes the `else` branch.

## Flow Control
Based on the evaluation result, you can control the flow execution using the `then` and `else` blocks:
*   `"continue": "reason"`: Log a message and proceed to the next task.
*   `"exit": "reason"`: Terminate the entire flow immediately with a failure status.
*   `"break": "reason"`: Break out of a `FOR` loop (if inside one).
*   `"gototask": "task_id"`: Jump to a specific task.
*   `"sleep": seconds`: Wait before proceeding.
