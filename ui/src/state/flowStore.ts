import { create } from 'zustand';
import {
  fetchFlowDefinition,
  fetchSchema,
  createEventSource,
  FlowEvent,
  requestFlowRun,
  requestFlowStop,
  requestStopAtTask,
  requestCloseFlow,
  uploadFlowDefinition
} from '../api/client';
import { CombinedSchema, FlowDefinition, FlowImport, TaskDefinition } from '../types/flow';
import { extractForChildren, extractParallelChildren, isSubtaskId } from '../utils/flowUtils';

interface FlowState {
  flows: FlowDefinition[];
  importedFlowIds: string[];
  activeFlow?: FlowDefinition;
  importsTree: FlowImport[];
  schema?: CombinedSchema;
  isFlowRunning: boolean;
  lastRunFinished: boolean;
  lastRunError: string | null;
  lastRunAt?: string;
  resumePending: boolean;
  stopAtTaskId?: string;
  focusTaskId?: string;
  loadError: string | null;
  loadFlows: () => Promise<void>;
  selectFlow: (id?: string) => void;
  openFlow: (id: string) => Promise<void>;
  updateTask: (taskId: string, patch: Partial<TaskDefinition>) => void;
  addTask: (task: TaskDefinition) => void;
  setSchema: (schema: CombinedSchema) => void;
  importFlow: (flow: FlowDefinition) => void;
  removeImportedFlow: (id: string) => void;
  connectToRunStream: () => () => void;
  triggerRun: () => Promise<void>;
  triggerTaskRun: (taskId: string) => Promise<void>;
  triggerResume: (taskId: string) => Promise<void>;
  triggerStop: () => Promise<void>;
  setStopAtTask: (taskId?: string) => Promise<void>;
  focusOnTask: (taskId?: string) => void;
}

type TaskModifier = (task: TaskDefinition) => TaskDefinition;

type ModifyResult = {
  tasks: TaskDefinition[];
  changed: boolean;
};

const STORAGE_KEY = 'flowk.importedFlows';

const isFlowDefinition = (value: unknown): value is FlowDefinition => {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as FlowDefinition;
  return typeof candidate.id === 'string' && Array.isArray(candidate.tasks);
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null;

const readPersistedFlows = (): FlowDefinition[] => {
  if (typeof window === 'undefined') {
    return [];
  }

  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return [];
    }
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed.filter(isFlowDefinition);
  } catch (error) {
    console.warn('No se pudieron cargar los flows importados.', error);
    return [];
  }
};

const writePersistedFlows = (flows: FlowDefinition[]) => {
  if (typeof window === 'undefined') {
    return;
  }

  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(flows));
  } catch (error) {
    console.warn('No se pudieron guardar los flows importados.', error);
  }
};

const runtimeTaskKeys = new Set([
  'status',
  'startedAt',
  'finishedAt',
  'durationSeconds',
  'success',
  'result',
  'resultType',
  'logs',
  'flowId',
  'fields',
  'children'
]);

const resetRuntimeState = (task: TaskDefinition): TaskDefinition => {
  const base: TaskDefinition = {
    ...task,
    status: undefined,
    success: undefined,
    startedAt: undefined,
    finishedAt: undefined,
    durationSeconds: undefined,
    result: undefined,
    resultType: undefined,
    logs: [],
  };

  if (task.children?.length) {
    base.children = task.children.map(resetRuntimeState);
  }

  const rawTasks = (task as any).tasks;
  if (Array.isArray(rawTasks) && rawTasks.length > 0) {
    base.tasks = rawTasks.map(resetRuntimeState);
  }

  if (task.fields) {
    const nextFields: Record<string, unknown> = { ...task.fields };
    if (Array.isArray(task.fields.tasks)) {
      nextFields.tasks = task.fields.tasks.map((t: any) => resetRuntimeState(t));
    }
    base.fields = nextFields;
  }

  return base;
};

const resetFlowRuntime = (flow: FlowDefinition): FlowDefinition => ({
  ...flow,
  tasks: flow.tasks.map(resetRuntimeState)
});

const sanitizeTaskForUpload = (task: TaskDefinition): Record<string, unknown> => {
  const sanitized: Record<string, unknown> = {};

  Object.entries(task).forEach(([key, value]) => {
    if (runtimeTaskKeys.has(key) || value === undefined) {
      return;
    }
    if (key === 'tasks' && Array.isArray(value)) {
      sanitized.tasks = value.map((item) =>
        isRecord(item) ? sanitizeTaskForUpload(item as TaskDefinition) : item
      );
      return;
    }
    sanitized[key] = value;
  });

  return sanitized;
};

const sanitizeFlowForUpload = (flow: FlowDefinition): FlowDefinition => ({
  id: flow.id,
  description: flow.description,
  imports: flow.imports ?? [],
  tasks: flow.tasks
    .filter((task) => !task.flowId || task.flowId === flow.id)
    .map((task) => sanitizeTaskForUpload(task) as TaskDefinition)
});

const mergeFlows = (baseFlows: FlowDefinition[], importedFlows: FlowDefinition[]): FlowDefinition[] => {
  const byId = new Map<string, FlowDefinition>();
  baseFlows.forEach((flow) => byId.set(flow.id, flow));
  importedFlows.forEach((flow) => byId.set(flow.id, flow));
  return Array.from(byId.values());
};

const collectFirstTaskIds = (tasks: TaskDefinition[]): Map<string, string> => {
  const map = new Map<string, string>();
  const visit = (list: TaskDefinition[]) => {
    list.forEach((task) => {
      if (task.flowId && !map.has(task.flowId)) {
        map.set(task.flowId, task.id);
      }
      if (task.children?.length) {
        visit(task.children);
      }

      const parallelChildren = extractParallelChildren(task);
      if (parallelChildren.length > 0) {
        visit(parallelChildren);
      }

      const forChildren = extractForChildren(task);
      if (forChildren.length > 0) {
        visit(forChildren);
      }
    });
  };
  visit(tasks);
  return map;
};

const normalizeIdentifier = (value: string): string => value.toLowerCase().replace(/[^a-z0-9]+/g, '');

const matchFlowIdForImport = (path: string, candidates: Set<string>): string | undefined => {
  if (!candidates.size) {
    return undefined;
  }
  const filenameWithExt = path.split(/[/\\]/).pop();
  const filename = filenameWithExt ? filenameWithExt.replace(/\.json$/i, '') : undefined;
  if (!filename) {
    return undefined;
  }
  const normalized = filename.toLowerCase();
  const normalizedLoose = normalizeIdentifier(normalized);

  const matchesEnd = (flowId: string, needle: string) => {
    const normalizedFlow = flowId.toLowerCase();
    return normalizedFlow.endsWith(needle);
  };

  const matchesInclude = (flowId: string, needle: string) => {
    const normalizedFlow = flowId.toLowerCase();
    return normalizedFlow.includes(needle);
  };

  const matchesIncludeReverse = (flowId: string, needle: string) => {
    const normalizedFlow = flowId.toLowerCase();
    return needle.includes(normalizedFlow);
  };

  const matchesLoose = (flowId: string, needle: string) => {
    if (!needle) {
      return false;
    }
    const normalizedFlow = normalizeIdentifier(flowId);
    return normalizedFlow.endsWith(needle) || normalizedFlow.includes(needle);
  };

  const matchesLooseReverse = (flowId: string, needle: string) => {
    if (!needle) {
      return false;
    }
    const normalizedFlow = normalizeIdentifier(flowId);
    return normalizedFlow && needle.includes(normalizedFlow);
  };

  for (const flowId of candidates) {
    if (matchesEnd(flowId, normalized)) {
      return flowId;
    }
  }

  for (const flowId of candidates) {
    if (matchesInclude(flowId, normalized)) {
      return flowId;
    }
  }

  for (const flowId of candidates) {
    if (matchesIncludeReverse(flowId, normalized)) {
      return flowId;
    }
  }

  for (const flowId of candidates) {
    if (matchesLoose(flowId, normalizedLoose)) {
      return flowId;
    }
  }

  for (const flowId of candidates) {
    if (matchesLooseReverse(flowId, normalizedLoose)) {
      return flowId;
    }
  }

  // Try matching without _sf suffix if present
  if (normalized.endsWith('_sf')) {
    const withoutSf = normalized.slice(0, -3);
    for (const flowId of candidates) {
      if (matchesEnd(flowId, withoutSf) || matchesInclude(flowId, withoutSf)) {
        return flowId;
      }
    }
    const withoutSfLoose = normalizeIdentifier(withoutSf);
    for (const flowId of candidates) {
      if (matchesLoose(flowId, withoutSfLoose)) {
        return flowId;
      }
    }
  }

  return undefined;
};

const buildImportNodes = (flow: FlowDefinition | undefined): FlowImport[] => {
  if (!flow) {
    return [];
  }

  const firstTaskMap = collectFirstTaskIds(flow.tasks ?? []);
  const knownFlows = new Set(
    Array.from(firstTaskMap.keys()).filter((flowId) => flowId && flowId !== flow.id)
  );
  const unmatched = new Set(knownFlows);
  const imports: FlowImport[] = [];

  const assignFlow = (flowId: string, label?: string, path?: string) => {
    const firstTaskId = firstTaskMap.get(flowId);
    if (!firstTaskId) {
      return;
    }
    imports.push({
      id: flowId,
      name: label ?? flowId,
      path: path ?? label ?? flowId,
      flowId,
      firstTaskId
    });
    unmatched.delete(flowId);
  };

  const matchedFlows = new Set<string>();
  flow.imports?.forEach((path, index) => {
    const normalizedPath = path.replace(/\\/g, '/');
    const displayName = normalizedPath.split('/').pop()?.replace(/\.json$/i, '') ?? normalizedPath;
    const flowId = matchFlowIdForImport(normalizedPath, knownFlows);
    if (flowId) {
      assignFlow(flowId, displayName, normalizedPath);
      matchedFlows.add(flowId);
    } else {
      imports.push({
        id: `${flow.id}-import-${index}`,
        name: displayName,
        path: normalizedPath
      });
    }
  });

  Array.from(unmatched)
    .sort()
    .forEach((flowId) => {
      if (matchedFlows.has(flowId)) {
        return;
      }
      assignFlow(flowId);
    });

  return imports;
};

const modifyTaskTree = (tasks: TaskDefinition[], taskId: string, modifier: TaskModifier): ModifyResult => {
  let changed = false;

  const updated = tasks.map((task) => {
    if (task.id === taskId) {
      changed = true;
      return modifier(task);
    }

    if (task.children?.length) {
      const nested = modifyTaskTree(task.children, taskId, modifier);
      if (nested.changed) {
        changed = true;
        return { ...task, children: nested.tasks };
      }
    }

    const parallelChildren = extractParallelChildren(task);
    if (parallelChildren.length > 0) {
      const nested = modifyTaskTree(parallelChildren, taskId, modifier);
      if (nested.changed) {
        changed = true;
        // We need to update the source of truth for these children
        // For PARALLEL, they are in 'tasks' or 'fields.tasks'
        // Since we can't easily patch the exact location without more complex logic,
        // and extractParallelChildren returns a normalized list,
        // we might need a way to map back.
        // However, for runtime updates (status, logs), we usually just need to find the task in the tree.
        // But wait, modifyTaskTree returns a new list of tasks.
        // If we just return the modified task, we need to know WHERE to put it back.

        // The current implementation of extractParallelChildren returns COPIES with normalized IDs.
        // If we modify these copies, we need to write them back to the original structure.
        // This is tricky because the original structure is untyped 'tasks' array or 'fields.tasks'.

        // Let's look at how we can update the original 'tasks' array if it exists.
        const originalTasks = (task as any).tasks;
        if (Array.isArray(originalTasks)) {
          const updatedChildren = originalTasks.map((child: any, index: number) => {
            // We need to match the child to the one we modified.
            // The ID generation logic in extractParallelChildren must match.
            const id = typeof child.id === 'string' && child.id.trim().length > 0 ? child.id : `${task.id}-parallel-${index + 1}`;
            const modifiedChild = nested.tasks.find(t => t.id === id);
            return modifiedChild ? { ...child, ...modifiedChild } : child;
          });
          return { ...task, tasks: updatedChildren };
        }

        // Also check fields.tasks
        const fieldsTasks = task.fields?.tasks;
        if (Array.isArray(fieldsTasks)) {
          const updatedChildren = fieldsTasks.map((child: any, index: number) => {
            const id = typeof child.id === 'string' && child.id.trim().length > 0 ? child.id : `${task.id}-parallel-${index + 1}`;
            const modifiedChild = nested.tasks.find(t => t.id === id);
            return modifiedChild ? { ...child, ...modifiedChild } : child;
          });
          return { ...task, fields: { ...task.fields, tasks: updatedChildren } };
        }
      }
    }

    const forChildren = extractForChildren(task);
    if (forChildren.length > 0) {
      const nested = modifyTaskTree(forChildren, taskId, modifier);
      if (nested.changed) {
        changed = true;
        // Same logic for FOR loops
        const originalTasks = (task as any).tasks;
        if (Array.isArray(originalTasks)) {
          const updatedChildren = originalTasks.map((child: any, index: number) => {
            const id = typeof child.id === 'string' && child.id.trim().length > 0 ? child.id : `${task.id}-for-${index + 1}`;
            const modifiedChild = nested.tasks.find(t => t.id === id);
            return modifiedChild ? { ...child, ...modifiedChild } : child;
          });
          return { ...task, tasks: updatedChildren };
        }

        const fieldsTasks = task.fields?.tasks;
        if (Array.isArray(fieldsTasks)) {
          const updatedChildren = fieldsTasks.map((child: any, index: number) => {
            const id = typeof child.id === 'string' && child.id.trim().length > 0 ? child.id : `${task.id}-for-${index + 1}`;
            const modifiedChild = nested.tasks.find(t => t.id === id);
            return modifiedChild ? { ...child, ...modifiedChild } : child;
          });
          return { ...task, fields: { ...task.fields, tasks: updatedChildren } };
        }
      }
    }

    return task;
  });

  if (!changed) {
    return { tasks, changed: false };
  }

  return { tasks: updated, changed: true };
};

const applyTaskUpdate = (
  state: FlowState,
  taskId: string,
  modifier: TaskModifier
): Partial<FlowState> => {
  if (!state.activeFlow) {
    return {};
  }

  const { tasks, changed } = modifyTaskTree(state.activeFlow.tasks, taskId, modifier);
  if (!changed) {
    return {};
  }

  const updatedFlow = { ...state.activeFlow, tasks };
  const flows = state.flows.map((flow) => (flow.id === updatedFlow.id ? updatedFlow : flow));

  return { activeFlow: updatedFlow, flows };
};

const normalizeStatus = (type: FlowEvent['type'], snapshotStatus?: string, success?: boolean): string | undefined => {
  if (type === 'task_failed') {
    return 'failed';
  }

  if (snapshotStatus) {
    if (snapshotStatus === 'completed' && success === false) {
      return 'failed';
    }
    return snapshotStatus;
  }

  if (type === 'task_completed') {
    return success === false ? 'failed' : 'completed';
  }

  if (type === 'task_started') {
    return 'in progress';
  }

  return undefined;
};

const useFlowStore = create<FlowState>((set, get) => ({
  flows: [],
  importedFlowIds: [],
  activeFlow: undefined,
  importsTree: [],
  schema: undefined,
  isFlowRunning: false,
  lastRunFinished: false,
  lastRunError: null,
  lastRunAt: undefined,
  resumePending: false,
  stopAtTaskId: undefined,
  focusTaskId: undefined,
  loadError: null,
  loadFlows: async () => {
    const [flow, schema] = await Promise.all([fetchFlowDefinition(), fetchSchema()]);
    const importedFlows = readPersistedFlows();
    const flows = mergeFlows(flow ? [resetFlowRuntime(flow)] : [], importedFlows.map(resetFlowRuntime));
    set({
      flows,
      importedFlowIds: importedFlows.map((item) => item.id),
      schema
    });
  },
  selectFlow: (id?: string) => {
    if (!id) {
      const active = get().activeFlow;
      if (active) {
        const reset = resetFlowRuntime(active);
        const persistedFlows = readPersistedFlows();
        const nextPersisted = persistedFlows.map((item) => (item.id === reset.id ? reset : item));
        writePersistedFlows(nextPersisted);
        set((state) => ({
          activeFlow: undefined,
          importsTree: [],
          focusTaskId: undefined,
          stopAtTaskId: undefined,
          flows: state.flows.map((item) => (item.id === reset.id ? reset : item))
        }));
        void requestCloseFlow(reset.id);
      } else {
        set({ activeFlow: undefined, importsTree: [], focusTaskId: undefined, stopAtTaskId: undefined });
      }
      void requestStopAtTask('');
      return;
    }
    const flow = get().flows.find((item) => item.id === id);
    if (flow) {
      set({ activeFlow: flow, importsTree: buildImportNodes(flow), focusTaskId: undefined, stopAtTaskId: undefined });
      void requestStopAtTask('');
    }
  },
  openFlow: async (id: string) => {
    const flow = get().flows.find((item) => item.id === id);
    if (!flow) {
      set({ activeFlow: undefined, importsTree: [], focusTaskId: undefined, stopAtTaskId: undefined });
      void requestStopAtTask('');
      return;
    }

    const resetFlow = resetFlowRuntime(flow);
    const sourceFileName = flow.sourceFileName;
    set((state) => ({
      activeFlow: resetFlow,
      flows: state.flows.map((item) => (item.id === resetFlow.id ? resetFlow : item)),
      importsTree: buildImportNodes(resetFlow),
      focusTaskId: undefined,
      stopAtTaskId: undefined,
      loadError: null
    }));
    void requestStopAtTask('');

    try {
      const payload = JSON.stringify(sanitizeFlowForUpload(resetFlow));
      const uploaded = await uploadFlowDefinition(payload, sourceFileName);
      const normalized = resetFlowRuntime(uploaded);
      const isPersisted = get().importedFlowIds.includes(uploaded.id);

      set((state) => ({
        activeFlow: normalized,
        flows: state.flows.map((item) => (item.id === normalized.id ? normalized : item)),
        importsTree: buildImportNodes(normalized)
      }));

      if (isPersisted) {
        const persistedFlows = readPersistedFlows();
        const persistedIndex = persistedFlows.findIndex((item) => item.id === uploaded.id);
        const nextPersisted =
          persistedIndex >= 0
            ? persistedFlows.map((item) => (item.id === normalized.id ? normalized : item))
            : [...persistedFlows, normalized];
        writePersistedFlows(nextPersisted);
      }
    } catch (error) {
      console.warn('No se pudo activar el flujo en el backend.', error);
      let errorMessage = 'Unknown error loading flow';
      if (error instanceof Error) {
        errorMessage = error.message;
      } else if (typeof error === 'string') {
        errorMessage = error;
      }

      // Try to parse JSON error message if possible
      try {
        if (errorMessage.startsWith('{') || errorMessage.startsWith('[')) {
          // In the error log provided by user: Error: {"error":"..."}
          // The client.ts throws Error(message). Message might be JSON.
          // But usually it's "Error: <json>" string if caught as error.message?
          // client.ts: throw new Error(message || ...)
          // If message is '{"error":...}', then error.message is '{"error":...}'
          const parsed = JSON.parse(errorMessage);
          if (parsed && typeof parsed === 'object' && 'error' in parsed) {
            errorMessage = parsed.error;
          }
        }
      } catch (e) {
        // ignore parsing error
      }

      // Also handle the case where the error message itself is prefixed with "Error: " by the browser console or something,
      // though error.message should be clean.
      // But looking at the user screenshot calls: Error: {"error": ...}

      set({ loadError: errorMessage });
    }
  },
  updateTask: (taskId: string, patch: Partial<TaskDefinition>) => {
    set((state) => applyTaskUpdate(state, taskId, (task) => ({ ...task, ...patch })));
  },
  addTask: (task: TaskDefinition) => {
    const { activeFlow } = get();
    if (!activeFlow) {
      return;
    }

    const updatedFlow: FlowDefinition = { ...activeFlow, tasks: [...activeFlow.tasks, task] };
    set((state) => ({
      activeFlow: updatedFlow,
      flows: state.flows.map((item) => (item.id === updatedFlow.id ? updatedFlow : item))
    }));
  },
  setSchema: (schema: CombinedSchema) => set({ schema }),
  importFlow: (flow: FlowDefinition) => {
    const normalized = resetFlowRuntime(flow);
    const persistedFlows = readPersistedFlows();
    const persistedIndex = persistedFlows.findIndex((item) => item.id === normalized.id);
    const nextPersisted =
      persistedIndex >= 0
        ? persistedFlows.map((item) => (item.id === normalized.id ? normalized : item))
        : [...persistedFlows, normalized];
    writePersistedFlows(nextPersisted);

    set((state) => {
      const existingIndex = state.flows.findIndex((item) => item.id === normalized.id);
      const flows =
        existingIndex >= 0
          ? state.flows.map((item) => (item.id === normalized.id ? normalized : item))
          : [...state.flows, normalized];

      return {
        flows,
        importedFlowIds: nextPersisted.map((item) => item.id)
      };
    });
  },
  removeImportedFlow: (id: string) => {
    const persistedFlows = readPersistedFlows();
    if (!persistedFlows.some((flow) => flow.id === id)) {
      return;
    }
    const nextPersisted = persistedFlows.filter((flow) => flow.id !== id);
    writePersistedFlows(nextPersisted);

    // Notify backend to close the flow
    void requestCloseFlow(id);

    set((state) => {
      const isActiveRemoved = state.activeFlow?.id === id;
      return {
        flows: state.flows.filter((flow) => flow.id !== id),
        importedFlowIds: state.importedFlowIds.filter((flowId) => flowId !== id),
        activeFlow: isActiveRemoved ? undefined : state.activeFlow,
        importsTree: isActiveRemoved ? [] : state.importsTree,
        focusTaskId: isActiveRemoved ? undefined : state.focusTaskId
      };
    });
  },
  connectToRunStream: () => {
    const eventSource = createEventSource();
    const handleFlowLoaded = (event: MessageEvent<string>) => {
      try {
        const payload: FlowEvent = JSON.parse(event.data);
        set((state) => {
          if (state.resumePending) {
            return { resumePending: false };
          }
          const targetFlowId = payload.flowId;
          const updateFlow = (flow: FlowDefinition): FlowDefinition => ({
            ...flow,
            tasks: flow.tasks.map(resetRuntimeState)
          });

          let updatedActive = state.activeFlow;
          if (state.activeFlow && (!targetFlowId || state.activeFlow.id === targetFlowId)) {
            updatedActive = updateFlow(state.activeFlow);
          }

          const updatedFlows = state.flows.map((flow) => {
            if (!targetFlowId || flow.id === targetFlowId) {
              return updateFlow(flow);
            }
            return flow;
          });

          return {
            activeFlow: updatedActive,
            flows: updatedFlows
          };
        });
      } catch (error) {
        console.error('Error processing flow loaded event', error);
      }
    };

    const handleFlowStarted = (event: MessageEvent<string>) => {
      try {
        JSON.parse(event.data) as FlowEvent;
        set({ isFlowRunning: true, lastRunFinished: false, lastRunError: null, resumePending: false });
      } catch (error) {
        console.error('Error processing flow start event', error);
      }
    };

    const handleFlowFinished = (event: MessageEvent<string>) => {
      try {
        const payload: FlowEvent = JSON.parse(event.data);
        const runError = payload.error?.trim();
        set({
          isFlowRunning: false,
          lastRunFinished: true,
          lastRunError: runError && runError.length > 0 ? runError : null,
          lastRunAt: payload.timestamp
        });
      } catch (error) {
        console.error('Error processing flow finish event', error);
      }
    };

    const applySnapshot = (event: FlowEvent, messageOverride?: string) => {
      const snapshot = event.task;
      if (!snapshot) {
        return;
      }

      const status = normalizeStatus(event.type, snapshot.status, snapshot.success);
      const normalizedStatus =
        typeof status === 'string'
          ? status.toLowerCase()
          : typeof snapshot.status === 'string'
            ? snapshot.status.toLowerCase()
            : '';
      const isTerminalStatus =
        normalizedStatus === 'completed' ||
        normalizedStatus === 'succeeded' ||
        normalizedStatus === 'failed';

      set((state) =>
        applyTaskUpdate(state, snapshot.id, (task) => {
          const logEntry = messageOverride ?? event.error;
          const nextLogs =
            logEntry && logEntry.trim()
              ? [...(task.logs ?? []), logEntry]
              : task.logs ?? [];

          return {
            ...task,
            status: status ?? task.status,
            success: isTerminalStatus ? snapshot.success ?? task.success : undefined,
            startedAt: snapshot.startTimestamp ?? task.startedAt,
            finishedAt: snapshot.endTimestamp ?? task.finishedAt,
            durationSeconds: snapshot.durationSeconds ?? task.durationSeconds,
            result: snapshot.result ?? task.result,
            resultType: snapshot.resultType ?? task.resultType,
            logs: nextLogs,
          };
        })
      );
    };

    const handleEvent = (type: FlowEvent['type']) => (event: MessageEvent<string>) => {
      try {
        const payload: FlowEvent = JSON.parse(event.data);
        applySnapshot(payload);
      } catch (error) {
        console.error('Error processing run event', error);
      }
    };

    const handleLog = (event: MessageEvent<string>) => {
      try {
        const payload: FlowEvent = JSON.parse(event.data);
        if (!payload.task || !payload.message) {
          return;
        }
        applySnapshot(payload, payload.message);
      } catch (error) {
        console.error('Error processing log event', error);
      }
    };

    eventSource.addEventListener('task_started', handleEvent('task_started'));
    eventSource.addEventListener('task_completed', handleEvent('task_completed'));
    eventSource.addEventListener('task_failed', handleEvent('task_failed'));
    eventSource.addEventListener('task_log', handleLog);
    eventSource.addEventListener('flow_loaded', handleFlowLoaded);
    eventSource.addEventListener('flow_started', handleFlowStarted);
    eventSource.addEventListener('flow_finished', handleFlowFinished);

    eventSource.addEventListener('error', () => {
      eventSource.close();
    });

    return () => {
      eventSource.removeEventListener('flow_loaded', handleFlowLoaded);
      eventSource.removeEventListener('flow_started', handleFlowStarted);
      eventSource.removeEventListener('flow_finished', handleFlowFinished);
      eventSource.close();
    };
  },
  triggerRun: async () => {
    set({ resumePending: false });
    await requestFlowRun();
  },
  triggerTaskRun: async (taskId: string) => {
    const trimmed = taskId?.trim();
    if (!trimmed) {
      return;
    }
    set({ resumePending: false });
    const activeFlow = get().activeFlow;
    const isTopLevel =
      activeFlow?.tasks?.some((task) => task.id === trimmed) ?? false;

    if (!isTopLevel && activeFlow && isSubtaskId(activeFlow.tasks, trimmed)) {
      await requestFlowRun({ subtaskId: trimmed });
      return;
    }

    await requestFlowRun({ taskId: trimmed });
  },
  triggerResume: async (taskId: string) => {
    const trimmed = taskId?.trim();
    if (!trimmed) {
      return;
    }
    const activeFlow = get().activeFlow;
    const isSubtask = activeFlow ? isSubtaskId(activeFlow.tasks, trimmed) : false;
    if (isSubtask) {
      return;
    }
    set({ resumePending: true });
    try {
      await requestFlowRun({ resumeFromTaskId: trimmed });
    } catch (error) {
      set({ resumePending: false });
      throw error;
    }
  },
  triggerStop: async () => {
    await requestFlowStop();
  },
  setStopAtTask: async (taskId?: string) => {
    const trimmed = taskId?.trim();
    await requestStopAtTask(trimmed);
    set({ stopAtTaskId: trimmed || undefined });
  },
  focusOnTask: (taskId) => set({ focusTaskId: taskId ?? undefined })
}));

export default useFlowStore;
