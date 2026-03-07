import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import useFlowStore from '../state/flowStore';
import { FlowDefinition } from '../types/flow';

type FlowGroup = {
  dir: string;
  flows: FlowDefinition[];
};

const sortBySource = (left: FlowDefinition, right: FlowDefinition): number => {
  const leftPath = left.sourceName ?? left.id;
  const rightPath = right.sourceName ?? right.id;
  return leftPath.localeCompare(rightPath);
};

function FlowListPage() {
  const navigate = useNavigate();
  const { t } = useTranslation();
  const flows = useFlowStore((state) => state.flows);
  const flowsRootDir = useFlowStore((state) => state.flowsRootDir);
  const loadError = useFlowStore((state) => state.loadError);
  const loadFlows = useFlowStore((state) => state.loadFlows);
  const selectFlow = useFlowStore((state) => state.selectFlow);
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());

  useEffect(() => {
    void loadFlows();
    selectFlow(); // Clear active flow and subflows menu
  }, [loadFlows, selectFlow]);

  const groupedFlows = useMemo<FlowGroup[]>(() => {
    const groups = new Map<string, FlowDefinition[]>();
    flows.forEach((flow) => {
      const dir = flow.sourceDir?.trim() || '.';
      const current = groups.get(dir) ?? [];
      current.push(flow);
      groups.set(dir, current);
    });

    return Array.from(groups.entries())
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([dir, items]) => ({
        dir,
        flows: [...items].sort(sortBySource)
      }));
  }, [flows]);

  return (
    <div>
      <div className="flow-list__header">
        <div>
          <h2>{t('flowList.title')}</h2>
          <p className="flow-list__description">{t('flowList.description')}</p>
          {loadError ? (
            <p className="flow-list__error">{t('flowList.loadError', { message: loadError })}</p>
          ) : null}
        </div>
      </div>
      {groupedFlows.length === 0 ? (
        <p className="flow-list__empty">
          {t('flowList.empty', { path: flowsRootDir ?? t('flowList.unknownPath') })}
        </p>
      ) : (
        <div className="flow-groups">
          {groupedFlows.map((group, index) => {
            const isExpanded = expandedGroups.has(group.dir);
            const groupPathLabel = group.dir === '.' ? t('flowList.rootGroup') : group.dir;
            return (
              <section key={group.dir} className="flow-group">
                <button
                  type="button"
                  className={`flow-group__header ${isExpanded ? 'flow-group__header--expanded' : ''}`}
                  aria-expanded={isExpanded}
                  aria-controls={`flow-group-panel-${index}`}
                  aria-label={
                    isExpanded
                      ? t('flowList.collapseGroup', { path: groupPathLabel })
                      : t('flowList.expandGroup', { path: groupPathLabel })
                  }
                  onClick={() =>
                    setExpandedGroups((previous) => {
                      const next = new Set(previous);
                      if (next.has(group.dir)) {
                        next.delete(group.dir);
                      } else {
                        next.add(group.dir);
                      }
                      return next;
                    })
                  }
                >
                  <span className="flow-group__title">
                    <span className="flow-group__chevron" aria-hidden>
                      {isExpanded ? '-' : '+'}
                    </span>
                    <span className="flow-group__path">{groupPathLabel}</span>
                  </span>
                  <span className="flow-group__count">
                    {t('flowList.groupCount', { count: group.flows.length })}
                  </span>
                </button>
                {isExpanded ? (
                  <div id={`flow-group-panel-${index}`} className="flow-list">
                    {group.flows.map((flow) => (
                      <article key={`${flow.sourceName ?? flow.id}`} className="flow-card">
                        <header>
                          <h3>{flow.name ?? flow.id}</h3>
                          <p className="flow-card__description">{flow.description}</p>
                        </header>
                        <div className="flow-card__meta">
                          <span>{t('flowList.taskCount', { count: flow.tasks.length })}</span>
                          <span className="flow-card__path">{flow.sourceName ?? flow.id}</span>
                        </div>
                        <div className="flow-card__footer">
                          <button
                            className="button"
                            onClick={() =>
                              navigate(
                                `/flows/${encodeURIComponent(flow.id)}${
                                  flow.sourceName
                                    ? `?source=${encodeURIComponent(flow.sourceName)}`
                                    : ''
                                }`
                              )
                            }
                          >
                            {t('buttons.open')}
                          </button>
                        </div>
                      </article>
                    ))}
                  </div>
                ) : null}
              </section>
            );
          })}
        </div>
      )}
    </div>
  );
}

export default FlowListPage;
