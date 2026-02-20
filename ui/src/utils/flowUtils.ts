import { TaskDefinition } from '../types/flow';

export type EvaluateBranchName = 'then' | 'else';

export const isRecord = (value: unknown): value is Record<string, unknown> =>
    typeof value === 'object' && value !== null;

export const getStringField = (value: unknown): string | undefined => {
    if (typeof value !== 'string') {
        return undefined;
    }
    const trimmed = value.trim();
    return trimmed.length > 0 ? trimmed : undefined;
};

export const isForTask = (task: TaskDefinition): boolean =>
    typeof task.action === 'string' && task.action.trim().toUpperCase() === 'FOR';

export const isParallelTask = (task: TaskDefinition): boolean =>
    typeof task.action === 'string' && task.action.trim().toUpperCase() === 'PARALLEL';

export const isEvaluateTask = (task: TaskDefinition): boolean =>
    typeof task.action === 'string' && task.action.trim().toUpperCase() === 'EVALUATE';

export const extractEvaluateBranchInfo = (
    task: TaskDefinition,
    branch: EvaluateBranchName
): { goto?: string; continues: boolean } => {
    const rawBranch = task[branch];
    if (!isRecord(rawBranch)) {
        return { continues: true };
    }

    const goto =
        getStringField(rawBranch.gototaskid) ??
        getStringField(rawBranch.gototask);
    const hasExit = Boolean(getStringField(rawBranch.exit));
    const hasBreak = Boolean(getStringField(rawBranch.break));

    return {
        goto,
        continues: !goto && !hasExit && !hasBreak
    };
};

export const extractDeclaredChildTasks = (task: TaskDefinition): unknown[] => {
    const direct = (task as Record<string, unknown>).tasks;
    if (Array.isArray(direct)) {
        return direct;
    }
    const fromFields = task.fields?.tasks;
    if (Array.isArray(fromFields)) {
        return fromFields;
    }
    return [];
};

export const extractParallelChildren = (task: TaskDefinition): TaskDefinition[] => {
    if (!isParallelTask(task)) {
        return [];
    }
    const candidateTasks = extractDeclaredChildTasks(task);

    return candidateTasks
        .map((raw, index) => {
            if (!isRecord(raw)) {
                return null;
            }
            const id =
                typeof raw.id === 'string' && raw.id.trim().length > 0
                    ? raw.id
                    : `${task.id}-parallel-${index + 1}`;
            const action =
                typeof raw.action === 'string' && raw.action.trim().length > 0
                    ? raw.action
                    : 'TASK';
            const description =
                typeof raw.description === 'string' && raw.description.trim().length > 0
                    ? raw.description
                    : id;
            const normalized = raw as TaskDefinition;
            const operation =
                typeof raw.operation === 'string' && raw.operation.trim().length > 0
                    ? raw.operation
                    : typeof normalized.operation === 'string'
                        ? normalized.operation
                        : undefined;

            const normalizedTask: TaskDefinition = {
                ...normalized,
                id,
                action,
                description
            };

            if (operation !== undefined) {
                normalizedTask.operation = operation;
            }

            return normalizedTask;
        })
        .filter(Boolean) as TaskDefinition[];
};

export const extractForChildren = (task: TaskDefinition): TaskDefinition[] => {
    if (!isForTask(task)) {
        return [];
    }

    const candidateTasks = extractDeclaredChildTasks(task);

    return candidateTasks
        .map((raw, index) => {
            if (!isRecord(raw)) {
                return null;
            }
            const id =
                typeof raw.id === 'string' && raw.id.trim().length > 0
                    ? raw.id
                    : `${task.id}-for-${index + 1}`;
            const action =
                typeof raw.action === 'string' && raw.action.trim().length > 0
                    ? raw.action
                    : 'TASK';
            const description =
                typeof raw.description === 'string' && raw.description.trim().length > 0
                    ? raw.description
                    : id;
            const normalized = raw as TaskDefinition;
            const operation =
                typeof raw.operation === 'string' && raw.operation.trim().length > 0
                    ? raw.operation
                    : typeof normalized.operation === 'string'
                        ? normalized.operation
                        : undefined;

            const normalizedTask: TaskDefinition = {
                ...normalized,
                id,
                action,
                description
            };

            if (operation !== undefined) {
                normalizedTask.operation = operation;
            }

            return normalizedTask;
        })
        .filter(Boolean) as TaskDefinition[];
};

export const isSubtaskId = (tasks: TaskDefinition[], targetId: string): boolean => {
    const trimmed = typeof targetId === 'string' ? targetId.trim() : '';
    if (!trimmed) {
        return false;
    }

    const visit = (list: TaskDefinition[]): boolean => {
        for (const task of list) {
            const parallelChildren = extractParallelChildren(task);
            const forChildren = extractForChildren(task);
            const compositeChildren = [...parallelChildren, ...forChildren];

            if (compositeChildren.some((child) => child.id === trimmed)) {
                return true;
            }

            if (compositeChildren.length > 0 && visit(compositeChildren)) {
                return true;
            }

            if (task.children?.length && visit(task.children)) {
                return true;
            }
        }
        return false;
    };

    return visit(tasks);
};
