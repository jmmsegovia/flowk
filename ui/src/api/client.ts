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
  name?: string;
  description: string;
  isSubflow?: boolean;
  sourceName?: string;
  sourceDir?: string;
  imports?: string[] | null;
  flowNames?: Record<string, string> | null;
  tasks?: RawTaskResponse[] | null;
};

type RawFlowListResponse = {
  flows?: RawFlowResponse[] | null;
  rootDir?: string;
};

type RawTaskResponse = {
  id: string;
  name?: string;
  description?: string;
  action: string;
  flowId?: string;
  raw?: Record<string, unknown>;
  status?: string;
  success?: boolean;
  startedAt?: string;
  finishedAt?: string;
  durationSeconds?: number;
  resultType?: string;
  result?: unknown;
  fields?: Record<string, unknown>;
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null;

const isRawTaskResponse = (value: unknown): value is RawTaskResponse =>
  isRecord(value) &&
  typeof value.id === 'string' &&
  typeof value.action === 'string';

const isRawFlowResponse = (value: unknown): value is RawFlowResponse =>
  isRecord(value) &&
  typeof value.id === 'string' &&
  typeof value.description === 'string';

export type FlowLayoutSnapshot = {
  version: number;
  viewport?: { x: number; y: number; zoom: number };
  nodes: Record<string, { x: number; y: number }>;
};

const mapTask = (task: RawTaskResponse): TaskDefinition => {
  const base: TaskDefinition = {
    id: task.id,
    name: task.name ?? task.id,
    description: task.description,
    action: task.action,
    flowId: task.flowId,
    raw: task.raw,
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
  name: data.name ?? data.id,
  description: data.description,
  isSubflow: data.isSubflow,
  sourceName: data.sourceName,
  sourceDir: data.sourceDir,
  imports: Array.isArray(data.imports) ? data.imports : [],
  flowNames: data.flowNames ?? undefined,
  tasks: Array.isArray(data.tasks) ? data.tasks.filter(isRawTaskResponse).map(mapTask) : [],
});

export type AvailableFlowsResponse = {
  flows: FlowDefinition[];
  rootDir?: string;
};

export const fetchAvailableFlows = async (): Promise<AvailableFlowsResponse> => {
  const response = await fetch(withBase('/api/flows'));
  const data = await parseJSON<RawFlowListResponse>(response);
  const items = Array.isArray(data.flows) ? data.flows.filter(isRawFlowResponse) : [];
  return {
    flows: items.map(mapFlowResponse),
    rootDir: typeof data.rootDir === 'string' ? data.rootDir : undefined
  };
};

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

export const openFlowDefinition = async (sourceName: string): Promise<FlowDefinition> => {
  const response = await fetch(withBase('/api/flows/open'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sourceName }),
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

export const fetchFlowLayout = async (
  flowId: string,
  sourceName?: string
): Promise<FlowLayoutSnapshot | null> => {
  const trimmed = flowId?.trim();
  if (!trimmed) {
    return null;
  }
  const params = new URLSearchParams({ flowId: trimmed });
  if (sourceName?.trim()) {
    params.set('sourceName', sourceName.trim());
  }
  const response = await fetch(withBase(`/api/ui/layout?${params.toString()}`));
  if (response.status === 404) {
    return null;
  }
  return parseJSON<FlowLayoutSnapshot>(response);
};

export const saveFlowLayout = async (
  flowId: string,
  sourceName: string | undefined,
  snapshot: FlowLayoutSnapshot
): Promise<void> => {
  const trimmed = flowId?.trim();
  if (!trimmed) {
    return;
  }
  const payload = {
    flowId: trimmed,
    sourceName: sourceName?.trim() || undefined,
    snapshot
  };
  const response = await fetch(withBase('/api/ui/layout'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload)
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

export const deleteFlowLayout = async (flowId: string, sourceName?: string): Promise<void> => {
  const trimmed = flowId?.trim();
  if (!trimmed) {
    return;
  }
  const params = new URLSearchParams({ flowId: trimmed });
  if (sourceName?.trim()) {
    params.set('sourceName', sourceName.trim());
  }
  const response = await fetch(withBase(`/api/ui/layout?${params.toString()}`), {
    method: 'DELETE'
  });
  if (response.status === 404) {
    return;
  }
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
