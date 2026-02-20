# Core Actions

Core actions provide the fundamental building blocks for controlling flow execution, managing state, and debugging.

## PRINT

Prints messages to the console or UI logs. Combining static text with variable values.

### Action: `PRINT`

| Property | Type | Description |
| :--- | :--- | :--- |
| `entries` | Array | **Required**. List of message objects to print. |

#### Entry Object
| Property | Description |
| :--- | :--- |
| `message` | Static string to print. |
| `variable` | Name of a variable to print. |
| `taskId` | ID of a task to inspect (requires `field`). |
| `field` | Field path to extract from `taskId`. |
| `value` | Literal value to print. |

### Example
```json
{
  "id": "log_output",
  "action": "PRINT",
  "entries": [
    { "message": "Processing complete for user: " },
    { "variable": "user_id" }
  ]
}
```

---

## VARIABLES

Defines or updates variables in the flow's context.

### Action: `VARIABLES`

| Property | Type | Description |
| :--- | :--- | :--- |
| `vars` | Array | **Required**. List of variable definitions. |

#### Variable Definition
| Property | Type | Description |
| :--- | :--- | :--- |
| `name` | String | **Required**. Name of the variable. |
| `type` | String | **Required**. One of `string`, `number`, `bool`, `array`, `object`, `secret`. |
| `value` | Any | Literal value to assign. |
| `operation` | Object | Dynamic operation (see below). |

#### Operation Object
| Property | Description |
| :--- | :--- |
| `operator` | Operation type (e.g., specific to the type). |
| `variable` | Source variable for the operation. |

### Example
```json
{
  "id": "init_vars",
  "action": "VARIABLES",
  "vars": [
    { "name": "count", "type": "number", "value": 0 },
    { "name": "api_key", "type": "secret", "value": "12345-secret" }
  ]
}
```

---

## SLEEP

Pauses the execution for a specified amount of time.

### Action: `SLEEP`

| Property | Type | Description |
| :--- | :--- | :--- |
| `seconds` | Number | **Required**. Duration to sleep in seconds. |

### Example
```json
{
  "id": "wait_for_service",
  "action": "SLEEP",
  "seconds": 5.5
}
```

---

## EVALUATE

Conditionally executes logic within a task (often used as a logic gate or branching mechanism within a linear flow).

### Action: `EVALUATE`

| Property | Type | Description |
| :--- | :--- | :--- |
| `if_conditions` | Array | **Required**. List of conditions to evaluate. |
| `then` | Object | **Required**. Directives to execute if conditions are met. |
| `else` | Object | **Required**. Directives to execute if conditions are NOT met. |

#### Condition Object
| Property | Description |
| :--- | :--- |
| `left` | Left operand (value or variable ref). |
| `operation` | Comparator: `=`, `!=`, `>`, `<`, `CONTAINS`, etc. |
| `right` | Right operand (string literals may be empty, e.g. `""`). |

### Example
```json
{
  "id": "check_status",
  "action": "EVALUATE",
  "if_conditions": [
    { "left": "${http_status}", "operation": "=", "right": 200 }
  ],
  "then": { "continue": "true" },
  "else": { "exit": "true" }
}
```

---

## PARALLEL

Executes a list of child tasks concurrently.

### Action: `PARALLEL`

| Property | Type | Description |
| :--- | :--- | :--- |
| `tasks` | Array | **Required**. List of Task objects to run. |
| `fail_fast` | Boolean | If true, stops all other tasks if one fails. |
| `merge_strategy` | String | `last_write_wins` or `fail_on_conflict`. |

### Example
```json
{
  "id": "run_checks",
  "action": "PARALLEL",
  "tasks": [
    { "id": "check_a", "action": "HTTP_REQUEST", ... },
    { "id": "check_b", "action": "HTTP_REQUEST", ... }
  ]
}
```

---

## FOR

Iterates over a range of numbers or a list.

### Action: `FOR`

| Property | Type | Description |
| :--- | :--- | :--- |
| `variable` | String | **Required**. Name of the loop variable. |
| `tasks` | Array | **Required**. List of tasks to execute per iteration. |
| `values` | Array | List of string values to iterate over. |
| `initial` | Number | Start number (for numeric loops). |
| `condition` | Object | Condition to stop the loop (e.g., iterator < 10). |
| `step` | Number | Increment size. |

### Example (List)
```json
{
  "id": "process_files",
  "action": "FOR",
  "variable": "filename",
  "values": ["file1.txt", "file2.txt"],
  "tasks": [ ... ]
}
```
