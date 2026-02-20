# Functional Overview

`forloop.go` implements the **FOR** action. It executes a list of nested tasks multiple times, either by iterating over a numeric range or by iterating over a list of values. The current iteration value is exposed through the configured `variable` name and can be interpolated using `${variable}` inside nested tasks.

# Loop modes

## Values mode

Provide a `values` array of strings. Each iteration assigns the next value to the loop variable.

## Numeric mode

Provide `initial`, `condition`, and `step` to define a counter loop. The loop runs while `condition` remains true, then increments the counter by `step` after each iteration.

> Note: `values` mode and numeric mode are mutually exclusive.

# Result payload

The action returns `flow.ResultTypeJSON` with an array of iteration summaries. Each entry includes:

- `index`: iteration index (0-based).
- `counter`: the numeric counter (numeric loops only).
- `value`: the current value (values loops only).
- `tasks`: array of nested task outcomes (`task_id`, `result`, `result_type`, optional `control`, optional `error`).

If `require_break` is `true` and the loop exits without a `break` control, the action fails.

# Example (values loop)

```json
{
  "id": "loop.players",
  "action": "FOR",
  "variable": "player",
  "values": ["Ada", "Linus"],
  "tasks": [
    {
      "id": "print.player",
      "action": "PRINT",
      "entries": [
        { "message": "Player", "value": "${player}" }
      ]
    }
  ]
}
```
