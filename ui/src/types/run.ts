export type FlowEventType =
  | 'flow_loaded'
  | 'flow_started'
  | 'flow_finished'
  | 'task_started'
  | 'task_completed'
  | 'task_failed'
  | 'task_log';

export interface TaskSnapshot {
  id: string;
  flowId?: string;
  description?: string;
  action: string;
  status?: string;
  success?: boolean;
  startTimestamp?: string;
  endTimestamp?: string;
  durationSeconds?: number;
  resultType?: string;
  result?: unknown;
}

export interface FlowEvent {
  type: FlowEventType;
  timestamp: string;
  flowId?: string;
  task?: TaskSnapshot;
  message?: string;
  error?: string;
}
