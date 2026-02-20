import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { TaskDefinition } from '../../types/flow';
import JsonViewer from './JsonViewer';

interface TaskInspectorProps {
  task: TaskDefinition;
}

const parseMaybeJSON = (value: string): { prefix: string; json: string } | null => {
  const trimmed = value.trim();
  if (!trimmed) {
    return null;
  }
  const firstJsonIdx = (() => {
    const brace = trimmed.indexOf('{');
    const bracket = trimmed.indexOf('[');
    if (brace === -1 && bracket === -1) {
      return -1;
    }
    if (brace === -1) return bracket;
    if (bracket === -1) return brace;
    return Math.min(brace, bracket);
  })();

  if (firstJsonIdx === -1) {
    return null;
  }

  const jsonCandidate = trimmed.slice(firstJsonIdx);
  try {
    const parsed = JSON.parse(jsonCandidate);
    const formatted = JSON.stringify(parsed, null, 2);
    const prefix = trimmed.slice(0, firstJsonIdx).trimEnd();
    return { prefix, json: formatted };
  } catch {
    return null;
  }
};

const normalizeJsonViewerData = (value: unknown): Record<string, unknown> | unknown[] => {
  if (Array.isArray(value)) {
    return value;
  }
  if (value && typeof value === 'object') {
    return value as Record<string, unknown>;
  }
  return { value };
};

function TaskInspector({ task }: TaskInspectorProps) {
  const { t } = useTranslation();

  const taskDefinitionData = useMemo(() => {
    // Extract only definition-related fields
    const { status, success, startedAt, finishedAt, durationSeconds, result, resultType, logs, ...definition } = task;
    return definition;
  }, [task]);

  const formattedResult =
    task.result === undefined || task.result === null
      ? ''
      : typeof task.result === 'string'
        ? task.result
        : JSON.stringify(task.result, null, 2);

  const resultJsonViewerData = useMemo(() => {
    if (task.result === undefined || task.result === null) {
      return null;
    }
    if (typeof task.result === 'object') {
      return normalizeJsonViewerData(task.result);
    }
    if (typeof task.result === 'string') {
      const trimmed = task.result.trim();
      if (!trimmed) {
        return null;
      }
      try {
        const parsed = JSON.parse(trimmed);
        return normalizeJsonViewerData(parsed);
      } catch {
        return null;
      }
    }
    return null;
  }, [task.result]);

  const statusLabel =
    typeof task.status === 'string' && task.status.trim() ? task.status : t('taskInspector.statusUnknown');
  const normalizedStatus = statusLabel.toLowerCase();
  const statusTone =
    normalizedStatus.includes('fail') || normalizedStatus.includes('error')
      ? 'danger'
      : normalizedStatus.includes('progress') || normalizedStatus.includes('running')
        ? 'info'
        : normalizedStatus.includes('complete')
          ? 'success'
          : 'neutral';

  const hasRuntimeData =
    (typeof task.status === 'string' && task.status.trim().length > 0) ||
    task.success !== undefined ||
    typeof task.startedAt === 'string' ||
    typeof task.finishedAt === 'string' ||
    task.durationSeconds !== undefined ||
    typeof task.resultType === 'string' ||
    task.result !== undefined;

  return (
    <section className="task-inspector">
      <header className="task-inspector__header">
        <div>
          <p className="task-inspector__subtitle">{t('taskInspector.title')}</p>
          <h3 className="task-inspector__title">
            {typeof task.description === 'string' && task.description.trim()
              ? task.description
              : task.id}
          </h3>
        </div>
        <span className={`task-inspector__status task-inspector__status--${statusTone}`}>
          {statusLabel}
        </span>
      </header>

      <section className="task-inspector__section">
        <div className="task-inspector__section-header">
          <div>
            <h4>{t('taskInspector.definitionTitle')}</h4>
          </div>
        </div>
        <div className="task-inspector__grid task-inspector__grid--compact">
          <div className="task-inspector__field">
            <label htmlFor="task-id">{t('taskInspector.fields.id')}</label>
            <input
              id="task-id"
              name="id"
              value={task.id}
              readOnly
              className="task-inspector__control task-inspector__control--readonly"
            />
          </div>
          <div className="task-inspector__field task-inspector__field--full">
            <label htmlFor="task-description">{t('taskInspector.fields.description')}</label>
            <textarea
              id="task-description"
              name="description"
              value={typeof task.description === 'string' ? task.description : ''}
              readOnly
              className="task-inspector__control task-inspector__control--readonly"
              rows={3}
            />
          </div>
          <div className="task-inspector__field">
            <label htmlFor="task-action">{t('taskInspector.fields.action')}</label>
            <input
              id="task-action"
              name="action"
              value={task.action as string}
              readOnly
              className="task-inspector__control task-inspector__control--readonly"
            />
          </div>
          <div className="task-inspector__field">
            <label htmlFor="task-operation">{t('taskInspector.fields.operation')}</label>
            <input
              id="task-operation"
              name="operation"
              value={typeof task.operation === 'string' ? task.operation : ''}
              readOnly
              className="task-inspector__control task-inspector__control--readonly"
            />
          </div>
        </div>
        <details className="task-inspector__details">
          <summary className="task-inspector__details-summary">{t('taskInspector.definitionJson')}</summary>
          <JsonViewer data={taskDefinitionData} defaultInspectDepth={2} rootName="task" />
        </details>
      </section>

      <section className="task-inspector__section">
        <div className="task-inspector__section-header">
          <div>
            <h4>{t('taskInspector.runtimeTitle')}</h4>
          </div>
        </div>
        {hasRuntimeData ? (
          <>
            <div className="task-inspector__grid task-inspector__grid--compact">
              <div className="task-inspector__field">
                <label htmlFor="task-status">{t('taskInspector.fields.status')}</label>
                <input
                  id="task-status"
                  name="status"
                  value={typeof task.status === 'string' ? task.status : ''}
                  readOnly
                  className="task-inspector__control task-inspector__control--readonly"
                />
              </div>
              <div className="task-inspector__field">
                <label htmlFor="task-success">{t('taskInspector.fields.success')}</label>
                <input
                  id="task-success"
                  name="success"
                  value={task.success === undefined ? '' : String(task.success)}
                  readOnly
                  className="task-inspector__control task-inspector__control--readonly"
                />
              </div>
              <div className="task-inspector__field">
                <label htmlFor="task-result-type">{t('taskInspector.fields.resultType')}</label>
                <input
                  id="task-result-type"
                  name="resultType"
                  value={typeof task.resultType === 'string' ? task.resultType : ''}
                  readOnly
                  className="task-inspector__control task-inspector__control--readonly"
                />
              </div>
              <div className="task-inspector__field">
                <label htmlFor="task-started">{t('taskInspector.fields.startedAt')}</label>
                <input
                  id="task-started"
                  name="startedAt"
                  value={typeof task.startedAt === 'string' ? task.startedAt : ''}
                  readOnly
                  className="task-inspector__control task-inspector__control--readonly"
                />
              </div>
              <div className="task-inspector__field">
                <label htmlFor="task-finished">{t('taskInspector.fields.finishedAt')}</label>
                <input
                  id="task-finished"
                  name="finishedAt"
                  value={typeof task.finishedAt === 'string' ? task.finishedAt : ''}
                  readOnly
                  className="task-inspector__control task-inspector__control--readonly"
                />
              </div>
              <div className="task-inspector__field">
                <label htmlFor="task-duration">{t('taskInspector.fields.durationSeconds')}</label>
                <input
                  id="task-duration"
                  name="durationSeconds"
                  value={task.durationSeconds === undefined ? '' : String(task.durationSeconds)}
                  readOnly
                  className="task-inspector__control task-inspector__control--readonly"
                />
              </div>
            </div>
            <div className="task-inspector__section-header">
              <div>
                <p className="task-inspector__subtitle">{t('taskInspector.resultTitle')}</p>
                <h4>{t('taskInspector.resultSubtitle')}</h4>
              </div>
            </div>
            {resultJsonViewerData ? (
              <div className="task-inspector__result-viewer">
                <JsonViewer data={resultJsonViewerData} defaultInspectDepth={2} rootName="result" />
              </div>
            ) : formattedResult ? (
              <textarea
                id="task-result"
                name="result"
                value={formattedResult}
                readOnly
                rows={6}
                className="task-inspector__control task-inspector__control--mono"
              />
            ) : (
              <p className="task-inspector__empty">{t('taskInspector.resultEmpty')}</p>
            )}
          </>
        ) : (
          <p className="task-inspector__empty">{t('taskInspector.runtimeEmpty')}</p>
        )}
      </section>

      <section className="task-inspector__section">
        <div className="task-inspector__section-header">
          <div>
            <p className="task-inspector__subtitle">{t('taskInspector.logsTitle')}</p>
            <h4>{t('taskInspector.logsTitle')}</h4>
          </div>
        </div>
        {!task.logs || task.logs.length === 0 ? (
          <p className="task-inspector__empty">{t('taskInspector.logsEmpty')}</p>
        ) : (
          <ul className="task-inspector__logs">
            {task.logs.map((logEntry, index) => {
              const formatted = parseMaybeJSON(logEntry);
              const tone =
                logEntry.includes('ERROR') || logEntry.includes('error')
                  ? 'danger'
                  : logEntry.includes('SUCCESS') || logEntry.includes('success')
                    ? 'success'
                    : undefined;
              return (
                <li key={`${task.id}-log-${index}`} className={tone ? `task-inspector__log-${tone}` : undefined}>
                  {formatted ? (
                    <>
                      {formatted.prefix ? <span>{formatted.prefix} </span> : null}
                      <pre className="task-inspector__code task-inspector__code--inline">{formatted.json}</pre>
                    </>
                  ) : (
                    logEntry
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </section>
  );
}

export default TaskInspector;
