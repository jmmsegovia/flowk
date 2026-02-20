import { CSSProperties, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { FlowDefinition, TaskDefinition } from '../../types/flow';
import { extractForChildren, extractParallelChildren } from '../../utils/flowUtils';

interface ExecutionTimelineProps {
  flow: FlowDefinition;
}

type TimelineItem = TaskDefinition & { depth: number; order: number };

const parseTimestamp = (value?: string): number | null => {
  if (!value) {
    return null;
  }
  const ms = Date.parse(value);
  return Number.isFinite(ms) ? ms : null;
};

const buildTimelineItems = (tasks: TaskDefinition[]): TimelineItem[] => {
  const items: TimelineItem[] = [];
  const orderRef = { value: 0 };

  const visit = (list: TaskDefinition[], depth: number) => {
    list.forEach((task) => {
      items.push({ ...task, depth, order: orderRef.value++ });

      const nested = [
        ...(task.children ?? []),
        ...extractForChildren(task),
        ...extractParallelChildren(task)
      ];

      if (nested.length) {
        visit(nested, depth + 1);
      }
    });
  };

  visit(tasks, 0);
  return items;
};

const getStatusTone = (task: TaskDefinition, fallback: string): 'success' | 'danger' | 'info' | 'neutral' => {
  const statusLabel = fallback.toLowerCase();
  if (task.success === false || statusLabel.includes('fail') || statusLabel.includes('error')) {
    return 'danger';
  }
  if (statusLabel.includes('progress') || statusLabel.includes('running')) {
    return 'info';
  }
  if (statusLabel.includes('complete') || statusLabel.includes('success')) {
    return 'success';
  }
  return 'neutral';
};

function ExecutionTimeline({ flow }: ExecutionTimelineProps) {
  const { t } = useTranslation();

  const timeline = useMemo(() => {
    const rawItems = buildTimelineItems(flow.tasks ?? []);
    const withTimes = rawItems.map((item) => ({
      item,
      startMs: parseTimestamp(item.startedAt),
      endMs: parseTimestamp(item.finishedAt)
    }));

    const hasAnyTime = withTimes.some(({ startMs, endMs }) => startMs !== null || endMs !== null);
    if (!hasAnyTime) {
      return { items: rawItems, sortedByTime: false };
    }

    const sorted = withTimes
      .map(({ item, startMs, endMs }) => ({
        item,
        sortKey: startMs ?? endMs ?? Number.POSITIVE_INFINITY
      }))
      .sort((a, b) => {
        if (a.sortKey !== b.sortKey) {
          return a.sortKey - b.sortKey;
        }
        return a.item.order - b.item.order;
      })
      .map(({ item }) => ({ ...item, depth: 0 }));

    return { items: sorted, sortedByTime: true };
  }, [flow.tasks]);

  return (
    <div className="execution-timeline">
      <div className="execution-timeline__header">
        <h3 className="execution-timeline__title">{t('timeline.title')}</h3>
        {timeline.sortedByTime ? (
          <span className="execution-timeline__note">{t('timeline.sortedByTime')}</span>
        ) : null}
      </div>
      <ul className="execution-timeline__list">
        {timeline.items.map((task) => {
          const statusLabel =
            typeof task.status === 'string' && task.status.trim()
              ? task.status
              : t('timeline.statusFallback');
          const statusTone = getStatusTone(task, statusLabel);
          const timeParts = [];
          if (task.startedAt) {
            timeParts.push(`${t('timeline.startLabel')}: ${task.startedAt}`);
          }
          if (task.finishedAt) {
            timeParts.push(`${t('timeline.endLabel')}: ${task.finishedAt}`);
          }
          const timeLine = timeParts.length ? timeParts.join(' \u00b7 ') : t('timeline.noTime');

          return (
            <li
              key={`timeline-${task.order}-${task.id}`}
              className="execution-timeline__item"
              style={{ '--depth': task.depth } as CSSProperties}
            >
              <article className="execution-timeline__card">
                <header className="execution-timeline__card-header">
                  <div className="execution-timeline__heading">
                    <span className="execution-timeline__name">
                      {typeof task.description === 'string' && task.description.trim()
                        ? task.description
                        : task.id}
                    </span>
                    <span className={`execution-timeline__status execution-timeline__status--${statusTone}`}>
                      {statusLabel}
                    </span>
                  </div>
                  {task.success !== undefined ? (
                    <span
                      className={`execution-timeline__success execution-timeline__success--${task.success ? 'true' : 'false'}`}
                    >
                      {t('timeline.success')}: {String(task.success)}
                    </span>
                  ) : null}
                </header>
                <div className="execution-timeline__meta">
                  <span className="execution-timeline__meta-item">
                    {t('timeline.action')}: {task.action}
                  </span>
                  {task.operation ? (
                    <span className="execution-timeline__meta-item">
                      {t('timeline.operation')}: {task.operation}
                    </span>
                  ) : null}
                  {task.durationSeconds !== undefined ? (
                    <span className="execution-timeline__meta-item">
                      {t('timeline.duration')}: {task.durationSeconds}
                    </span>
                  ) : null}
                </div>
                <footer className="execution-timeline__footer">{timeLine}</footer>
              </article>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

export default ExecutionTimeline;
