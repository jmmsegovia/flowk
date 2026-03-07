# UI Guide

FlowK includes an optional web interface that provides a rich visual experience for monitoring and debugging your automation flows.

## Accessing the UI

To start the UI, include the `-serve-ui` flag when running a flow:

```bash
./bin/flowk run -serve-ui
```

By default, the UI is accessible at `http://localhost:8080`.
The **Available flows** page scans the configured `flows_dir` recursively and groups flows by folder path.
Imported subflows are hidden from that top-level list, and files marked with `"is_subflow": true` are also excluded.

## Features

### 1. Execution Controls
The header controls let you start, stop, and resume runs. Some buttons act on the entire flow and others act on the selected task in the canvas.

![Execution Controls](../brand/ui_controls.jpg)

- **Run flow (Play)**: starts a full flow run. Disabled while a run is in progress.
- **Stop at task (Pause)**: toggles the selected task as a stop point; the run stops right after that task completes. Applies only to top-level tasks (not subtasks).
- **Stop flow (Stop)**: requests the current run to stop; the flow finishes after the current task completes.
- **Run task (PlayCircle)**: runs only the selected task (or a subtask if you select one inside a block).
- **Resume from task (FastForward)**: resumes the flow starting at the selected task after a prior run has finished; available only when the task has completed (success or failure) and is not a subtask.
- **Save layout (Save)**: saves the current canvas layout manually.
- **Reset layout (Rotate)**: deletes the saved layout for the current flow and resets the canvas.
- **Auto-save layout (Toggle)**: enables/disables automatic layout saving while you drag nodes or pan/zoom.

### 2. Flow Visualization
The main view renders your flow as an interactive graph.
- **Nodes**: Represent tasks.
- **Edges**: Show the dependency and execution order.
- **Subflows**: Nested flows are encapsulated in draggable groups.
- **FOR/PARALLEL**: Group containers wrap their child tasks and can be dragged as a unit.
- **Status Indicators**: Tasks change color real-time based on status (Pending: Grey, Running: Blue, Success: Green, Error: Red).

![Flow Visualization](../brand/screenshot1.jpg)

### 3. Task Inspector
Clicking on any task node opens the **Inspector Panel** on the right.
- **Definition**: View the raw JSON configuration of the task.
- **Input/Output**: See exactly what inputs the task received and what output it produced.
- **Logs**: View task-specific logs.

### 4. Real-Time Execution Log
The bottom panel acts as a unified terminal, streaming logs from all tasks as they execute. You can filter these logs by log level or search for specific keywords.

![Run Details](../brand/Untitled 2.jpg)

### 5. Variables Explorer
View the current state of all variables in the flow context. This is crucial for debugging data passing issues between tasks.

## Tips & Tricks
- **Zoom/Pan**: Use your mouse wheel or trackpad to zoom in/out of large flows.
- **Auto-Focus**: The UI will automatically center on the currently running task if you enable "Follow Execution".
- **Layout Persistence**: Node positions and viewport are saved under FlowK's config directory (the folder containing `config.yaml`, in `ui/layouts`). If you run with `-config`, layouts are stored next to that config file. Delete those files to reset.
- **Layout Controls**: Use the header controls to save the layout manually, toggle auto-save, or reset the saved layout for the current flow.


## API Contract (OpenAPI)

When FlowK runs with `-serve-ui`, the backend now exposes an OpenAPI document for the UI/server contract at:

- `http://localhost:8080/api/openapi.json`

This file can be used to generate API clients for future integrations (for example, additional web or automation clients beyond the bundled UI).
