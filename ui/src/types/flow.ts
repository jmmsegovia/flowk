export interface TaskDefinition extends Record<string, unknown> {
  id: string;
  description?: string;
  action: string;
  flowId?: string;
  operation?: string;
  status?: string;
  startedAt?: string;
  finishedAt?: string;
  result?: unknown;
  success?: boolean;
  durationSeconds?: number;
  logs?: string[];
  fields?: Record<string, unknown>;
  children?: TaskDefinition[];
}

export interface FlowDefinition {
  id: string;
  description: string;
  imports?: string[];
  tasks: TaskDefinition[];
  sourceFileName?: string;
}

export interface FlowImport {
  id: string;
  name: string;
  path: string;
  flowId?: string;
  firstTaskId?: string;
}

export interface SchemaFragment {
  action: string;
  schema: Record<string, unknown>;
}

export interface CombinedSchema {
  version: string;
  schema: Record<string, unknown>;
}
