export interface TaskDefinition extends Record<string, unknown> {
  id: string;
  name?: string;
  description?: string;
  action: string;
  flowId?: string;
  raw?: Record<string, unknown>;
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
  name?: string;
  description: string;
  isSubflow?: boolean;
  sourceName?: string;
  sourceDir?: string;
  imports?: string[];
  flowNames?: Record<string, string>;
  tasks: TaskDefinition[];
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
