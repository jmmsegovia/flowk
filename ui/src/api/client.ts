import { CombinedSchema, FlowDefinition, TaskDefinition } from '../types/flow';
import { ActionsGuide } from '../types/actionsGuide';
import { FlowEvent } from '../types/run';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '';

const withBase = (path: string): string => {
  if (!API_BASE_URL) {
    return path;
  }
  return `${API_BASE_URL}${path}`;
};

const parseJSON = async <T>(response: Response): Promise<T> => {
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with status ${response.status}`);
  }
  return (await response.json()) as T;
};

type RawFlowResponse = {
  id: string;
  description: string;
  imports?: string[];
  tasks: RawTaskResponse[];
};

type RawTaskResponse = {
  id: string;
  description?: string;
  action: string;
  flowId?: string;
  status?: string;
  success?: boolean;
  startedAt?: string;
  finishedAt?: string;
  durationSeconds?: number;
  resultType?: string;
  result?: unknown;
  fields?: Record<string, unknown>;
};

const mapTask = (task: RawTaskResponse): TaskDefinition => {
  const base: TaskDefinition = {
    id: task.id,
    description: task.description,
    action: task.action,
    flowId: task.flowId,
    status: task.status,
    success: task.success,
    startedAt: task.startedAt,
    finishedAt: task.finishedAt,
    durationSeconds: task.durationSeconds,
    result: task.result,
    resultType: task.resultType,
    logs: [],
    fields: task.fields ?? {},
  };

  if (task.fields) {
    Object.entries(task.fields).forEach(([key, value]) => {
      if (!(key in base)) {
        // Preserve task-specific properties for inspectors and builders
        base[key] = value;
      }
    });
  }

  return base;
};

const mapFlowResponse = (data: RawFlowResponse): FlowDefinition => ({
  id: data.id,
  description: data.description,
  imports: data.imports ?? [],
  tasks: data.tasks.map(mapTask),
});

export const fetchFlowDefinition = async (): Promise<FlowDefinition | null> => {
  const response = await fetch(withBase('/api/flow'));
  if (response.status === 404 || response.status === 204) {
    return null;
  }
  const data = await parseJSON<RawFlowResponse>(response);
  return mapFlowResponse(data);
};

export const fetchFlowNotes = async (): Promise<string | null> => {
  const response = await fetch(withBase('/api/flow/notes'));
  if (response.status === 404) {
    return null;
  }
  const data = await parseJSON<{ markdown?: string }>(response);
  return typeof data.markdown === 'string' ? data.markdown : '';
};

export const uploadFlowDefinition = async (payload: string, filename?: string): Promise<FlowDefinition> => {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (filename?.trim()) {
    headers['X-Flow-Filename'] = filename.trim();
  }
  const response = await fetch(withBase('/api/flow'), {
    method: 'POST',
    headers,
    body: payload,
  });
  const data = await parseJSON<RawFlowResponse>(response);
  return mapFlowResponse(data);
};

export const fetchSchema = async (): Promise<CombinedSchema> => {
  const response = await fetch(withBase('/api/schema'));
  return parseJSON<CombinedSchema>(response);
};

export const fetchActionsGuide = async (): Promise<ActionsGuide> => {
  const response = await fetch(withBase('/api/actions/guide'));
  return parseJSON<ActionsGuide>(response);
};

export const createEventSource = (): EventSource => {
  return new EventSource(withBase('/api/run/events'));
};

type RunRequestOptions = {
  taskId?: string;
  beginFromTask?: string;
  flowId?: string;
  subtaskId?: string;
  resumeFromTaskId?: string;
};

const buildRunPayload = (options?: RunRequestOptions): Record<string, string> | undefined => {
  if (!options) {
    return undefined;
  }
  const payload: Record<string, string> = {};
  if (options.taskId?.trim()) {
    payload.taskId = options.taskId.trim();
  }
  if (options.beginFromTask?.trim()) {
    payload.beginFromTask = options.beginFromTask.trim();
  }
  if (options.flowId?.trim()) {
    payload.flowId = options.flowId.trim();
  }
  if (options.subtaskId?.trim()) {
    payload.subtaskId = options.subtaskId.trim();
  }
  if (options.resumeFromTaskId?.trim()) {
    payload.resumeFromTaskId = options.resumeFromTaskId.trim();
  }
  return Object.keys(payload).length ? payload : undefined;
};

export const requestFlowRun = async (options?: RunRequestOptions): Promise<void> => {
  const payload = buildRunPayload(options);
  const response = await fetch(withBase('/api/run'), {
    method: 'POST',
    headers: payload ? { 'Content-Type': 'application/json' } : undefined,
    body: payload ? JSON.stringify(payload) : undefined,
  });
  if (!response.ok) {
    const message = await response.text();
    try {
      const parsed = JSON.parse(message) as { error?: string };
      if (parsed?.error) {
        throw new Error(parsed.error);
      }
    } catch {
      if (message.trim()) {
        throw new Error(message);
      }
    }
    throw new Error(`Request failed with status ${response.status}`);
  }
};

export const requestFlowStop = async (): Promise<void> => {
  const response = await fetch(withBase('/api/run/stop'), {
    method: 'POST',
  });
  if (!response.ok) {
    const message = await response.text();
    try {
      const parsed = JSON.parse(message) as { error?: string };
      if (parsed?.error) {
        throw new Error(parsed.error);
      }
    } catch {
      if (message.trim()) {
        throw new Error(message);
      }
    }
    throw new Error(`Request failed with status ${response.status}`);
  }
};

export const requestStopAtTask = async (taskId?: string): Promise<void> => {
  const payload = { taskId: taskId?.trim() ?? '' };
  const response = await fetch(withBase('/api/run/stop-at'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const message = await response.text();
    try {
      const parsed = JSON.parse(message) as { error?: string };
      if (parsed?.error) {
        throw new Error(parsed.error);
      }
    } catch {
      if (message.trim()) {
        throw new Error(message);
      }
    }
    throw new Error(`Request failed with status ${response.status}`);
  }
};

export const requestCloseFlow = async (flowId?: string): Promise<void> => {
  const payload = { flowId: flowId?.trim() ?? '' };
  const response = await fetch(withBase('/api/ui/close-flow'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const message = await response.text();
    try {
      const parsed = JSON.parse(message) as { error?: string };
      if (parsed?.error) {
        throw new Error(parsed.error);
      }
    } catch {
      if (message.trim()) {
        throw new Error(message);
      }
    }
    throw new Error(`Request failed with status ${response.status}`);
  }
};

export type { FlowEvent };
