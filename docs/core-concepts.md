# Core Concepts

Understanding the core building blocks of FlowK is essential for creating robust automation flows.

## Flows

A **Flow** is the top-level unit of execution in FlowK. It is defined in a JSON file and consists of a sequence of tasks.

### Structure of a Flow

```json
{
  "id": "my-payment-flow",
  "description": "Processes payments and updates database",
  "imports": [
    "./subflows/payment_gateway.json",
    "./subflows/notifications.json"
  ],
  "tasks": [ ... ],
  "on_error_flow": "error_handler_flow",
  "finally_flow": "cleanup_flow",
  "finally_task": "notify_finished"
}
```

- **id**: Unique identifier for the flow.
- **imports**: List of other flow files to include. This is how subflows are defined. Paths are resolved relative to the main flow file. Imported tasks are prepended in import order.
  For cross-platform compatibility (Linux/macOS/Windows), prefer relative paths like `./subflows/...` and `../shared/...`. Forward slashes are supported on Windows.
- **tasks**: Ordered array of tasks (including tasks from imported subflows).
- **on_error_flow**: Flow ID to run immediately if any task fails (must exist in the main flow or imports).
- **finally_flow**: Flow ID to run after the main flow finishes (success or failure).
- **finally_task**: Task ID to run after the main flow finishes (success or failure).

## Tasks

A **Task** is a single unit of work. Every task must have an `id` and an `action`.

```json
{
  "id": "check_service_health",
  "description": "Pings the API to check availability",
  "action": "HTTP_REQUEST",
  "protocol": "HTTP",
  "method": "GET",
  "url": "http://api.example.com/health"
}
```

### Common Task Properties
- **id**: Unique ID within the flow.
- **action**: The type of operation (e.g., `HTTP_REQUEST`, `SHELL`, `DB_MYSQL_OPERATION`).
- **description**: Human-readable explanation.
- Some control actions (e.g., `PARALLEL`, `FOR`) include a nested `tasks` array. Nested tasks follow the same structure.

## Variables

Variables allow you to pass data between tasks and subflows. They are referenced using `${variable_name}` syntax.

### Defining Variables
Use the `VARIABLES` action to define or update variables.

```json
{
  "id": "set_env",
  "action": "VARIABLES",
  "vars": [
    { "name": "environment", "type": "string", "value": "production" },
    { "name": "retry_count", "type": "number", "value": 3 }
  ]
}
```

### Using Variables
```json
{
  "id": "deploy",
  "action": "SHELL",
  "command": "./deploy.sh ${environment}"
}
```

### Task Results as Variables
You can access results from previous tasks using `${from.task:TASK_ID}`.
`from.task` placeholders are resolved during payload expansion for all actions, so you can use them anywhere a string value is accepted (headers, bodies, args, etc.).
If you need to preserve non-string types or build complex values, capture the result first with a `VARIABLES` task and reference the variable instead.

```json
{
  "id": "get_user",
  "action": "HTTP_REQUEST",
  "protocol": "HTTPS",
  "method": "GET",
  "url": "https://api.example.com/users/123"
},
{
  "id": "log_user",
  "action": "PRINT",
  "entries": [
    { "message": "User ID is: " },
    { "value": "${from.task:get_user.body.id}" }
  ]
}
```

## Control Flow

### Subflows
Subflows are regular flow JSON files referenced in `imports`. They are expanded before execution, and their tasks run as part of the full task list. Each subflow keeps its own flow ID for logging and for targeting `on_error_flow` / `finally_flow`.

```json
{
  "id": "payments.flow",
  "description": "Parent flow that imports payment subflows",
  "imports": ["./subflows/payment_gateway.json"],
  "tasks": [ ... ],
  "on_error_flow": "payment_gateway"
}
```

### Parallel Execution
Run multiple tasks concurrently using the `PARALLEL` action.

```json
{
  "id": "run_checks",
  "action": "PARALLEL",
  "tasks": [
    { "id": "check_db", "action": "DB_MYSQL_OPERATION", ... },
    { "id": "check_api", "action": "HTTP_REQUEST", ... }
  ]
}
```

### Loops
Iterate over a list or numeric range using `FOR`.

```json
{
  "id": "process_items",
  "action": "FOR",
  "variable": "item",
  "values": ["item-a", "item-b", "item-c"],
  "tasks": [
    {
      "id": "process_single",
      "action": "PRINT",
      "entries": [
        { "message": "Processing " },
        { "variable": "item" }
      ]
    }
  ]
}
```

## Error Handling

FlowK provides robust mechanisms to handle failures:

1.  **Flow Level**: `on_error_flow` defines a specific rescue flow (e.g., send alerts) that triggers on any unhandled failure.
2.  **Cleanup**: `finally_flow` and `finally_task` ensure critical cleanup steps (e.g., deleting temporary files, closing connections) always run.

## AI-Assisted Development

FlowK is built with Large Language Models (LLMs) in mind. While you can write flows by hand, the platform provides a comprehensive **Context Guide** designed specifically for LLMs.

This guide includes:
- The full JSON schema for flows and tasks.
- Detailed definitions of all available actions (required fields, optional fields, operations).
- Usage examples for every action.
- Best practices for structuring flows.

### How to get the Context Guide
When you run the FlowK UI, this guide is generated dynamically based on the current version of the code (ensuring it's always up-to-date).

1.  Start FlowK with the UI:
    ```bash
    ./bin/flowk run -serve-ui -flow ./flows/my_flow.json
    ```
2.  The guide is available at the API endpoint: `http://localhost:8080/api/actions/guide`

**Workflow:**
1.  Download the guide content.
2.  Paste it into your favorite LLM (ChatGPT, Claude, etc.).
3.  Prompt the LLM: *"Using the context above, create a FlowK flow that checks if a website is up, and if not, sends a notification."*
4.  The LLM will generate valid, schema-compliant JSON flow definitions.
