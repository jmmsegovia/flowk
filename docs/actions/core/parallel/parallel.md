# Functional Overview

`parallel.go` implements the **PARALLEL** action. It runs nested tasks concurrently and then merges their results and variables back into the parent execution context.

# Variable merge behavior

- `merge_strategy: "last_write_wins"` (default) overwrites variables in merge order.
- `merge_strategy: "fail_on_conflict"` fails the action if two tasks set the same variable to different values.
- `merge_order` controls the merge sequence; tasks not listed are merged afterward in declaration order.

When `fail_fast` is `true`, the action cancels remaining tasks as soon as one fails.

# Result payload

The action returns `flow.ResultTypeJSON` with an object keyed by task id. Each entry includes:

- `result`: the subtask result (if successful)
- `type`: the result type string
- `error`: error string (if the subtask failed)

# Example

```json
{
  "id": "parallel.queries",
  "action": "PARALLEL",
  "fail_fast": false,
  "merge_strategy": "last_write_wins",
  "tasks": [
    {
      "id": "query.one",
      "action": "HTTP_REQUEST",
      "protocol": "HTTPS",
      "method": "GET",
      "url": "https://example.org/api/one"
    },
    {
      "id": "query.two",
      "action": "HTTP_REQUEST",
      "protocol": "HTTPS",
      "method": "GET",
      "url": "https://example.org/api/two"
    }
  ]
}
```
