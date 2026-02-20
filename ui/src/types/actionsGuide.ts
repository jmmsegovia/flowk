export interface FieldDocumentation {
  name: string;
  description?: string;
}

export interface OperationDocumentation {
  name: string;
  required: FieldDocumentation[];
  note?: string;
  example?: string;
}

export interface AllowedValue {
  field: string;
  values: string[];
}

export interface ActionDocumentation {
  name: string;
  required: FieldDocumentation[];
  optional: FieldDocumentation[];
  operations?: OperationDocumentation[];
  allowedValues?: AllowedValue[];
  example?: string;
  helpMarkdown?: string;
}

export interface ActionsGuide {
  primer: string;
  generatedAt: string;
  actions: ActionDocumentation[];
  markdown?: string;
}
