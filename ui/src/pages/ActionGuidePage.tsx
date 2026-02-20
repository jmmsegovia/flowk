import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { fetchActionsGuide } from '../api/client';
import {
  ActionDocumentation,
  ActionsGuide,
  FieldDocumentation,
  OperationDocumentation
} from '../types/actionsGuide';

type ActionCategory =
  | 'auth'
  | 'core'
  | 'db'
  | 'infra'
  | 'network'
  | 'security'
  | 'storage'
  | 'system'
  | 'other';

const ACTION_CATEGORY_ORDER: ActionCategory[] = [
  'auth',
  'core',
  'db',
  'infra',
  'network',
  'security',
  'storage',
  'system',
  'other'
];

const ACTION_CATEGORY_MAP: Record<string, ActionCategory> = {
  GMAIL: 'auth',
  OAUTH2: 'auth',
  EVALUATE: 'core',
  FOR: 'core',
  PARALLEL: 'core',
  PRINT: 'core',
  SLEEP: 'core',
  VARIABLES: 'core',
  DB_CASSANDRA_OPERATION: 'db',
  DB_MYSQL_OPERATION: 'db',
  DB_POSTGRES_OPERATION: 'db',
  KUBERNETES: 'infra',
  HELM: 'infra',
  HTTP_REQUEST: 'network',
  SSH: 'network',
  TELNET: 'network',
  PGP: 'security',
  GCLOUD_STORAGE: 'storage',
  SHELL: 'system',
  DOCKER: 'system',
  BASE64: 'system'
};

const actionCategoryFor = (name: string): ActionCategory => {
  const key = name.trim().toUpperCase();
  if (!key) {
    return 'other';
  }
  return ACTION_CATEGORY_MAP[key] ?? 'other';
};

function ActionGuidePage() {
  const { t } = useTranslation(undefined, { lng: 'en' });
  const navigate = useNavigate();
  const [guide, setGuide] = useState<ActionsGuide | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const groupedActions = useMemo(() => {
    if (!guide?.actions) {
      return [];
    }

    const groups = new Map<ActionCategory, ActionDocumentation[]>();
    for (const action of guide.actions) {
      const category = actionCategoryFor(action.name);
      const existing = groups.get(category);
      if (existing) {
        existing.push(action);
      } else {
        groups.set(category, [action]);
      }
    }

    return ACTION_CATEGORY_ORDER.filter((category) => groups.has(category)).map((category) => {
      const actions = groups.get(category) ?? [];
      actions.sort((left, right) => left.name.localeCompare(right.name));
      return { category, actions };
    });
  }, [guide?.actions]);

  useEffect(() => {
    const load = async () => {
      try {
        const response = await fetchActionsGuide();
        setGuide(response);
      } catch (err) {
        const message = err instanceof Error ? err.message : t('actionGuide.error');
        setError(message);
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, [t]);

  const formattedDate = useMemo(() => {
    if (!guide?.generatedAt) {
      return '';
    }
    const timestamp = new Date(guide.generatedAt);
    return timestamp.toLocaleString();
  }, [guide?.generatedAt]);

  const renderFieldList = (fields: FieldDocumentation[]) => {
    if (!fields || fields.length === 0) {
      return <p className="text-muted">{t('actionGuide.none')}</p>;
    }

    return (
      <ul className="action-card__fields">
        {fields.map((field) => (
          <li key={field.name}>
            <strong>{field.name}</strong>
            {field.description ? <span className="text-muted"> — {field.description}</span> : null}
          </li>
        ))}
      </ul>
    );
  };

  const renderOperations = (operations?: OperationDocumentation[]) => {
    if (!operations || operations.length === 0) {
      return <p className="text-muted">{t('actionGuide.noOperations')}</p>;
    }

    return (
      <ul className="action-card__operations">
        {operations.map((op, idx) => (
          <li key={`${op.name}-${idx}`} className="action-card__operation">
            <div className="action-card__operation-header">
              <span className="badge badge--muted">{op.name}</span>
              {op.note ? <span className="text-muted">{op.note}</span> : null}
            </div>
            {op.required?.length ? renderFieldList(op.required) : null}
            {op.example ? <pre className="code-block">{op.example}</pre> : null}
          </li>
        ))}
      </ul>
    );
  };

  const renderAllowedValues = (action: ActionDocumentation) => {
    if (!action.allowedValues || action.allowedValues.length === 0) {
      return null;
    }

    return (
      <div className="action-card__section">
        <h5>{t('actionGuide.allowedValues')}</h5>
        <ul className="action-card__allowed">
          {action.allowedValues.map((entry) => (
            <li key={entry.field}>
              <strong>{entry.field}</strong>
              <span className="text-muted"> — {entry.values.join(', ')}</span>
            </li>
          ))}
        </ul>
      </div>
    );
  };

  const downloadGuide = () => {
    if (!guide?.markdown) {
      return;
    }

    const blob = new Blob([guide.markdown], { type: 'text/markdown' });
    const url = URL.createObjectURL(blob);
    const datePrefix = guide.generatedAt ? guide.generatedAt.slice(0, 10) : 'guide';
    const link = document.createElement('a');
    link.href = url;
    link.download = `flowk-actions-${datePrefix}.md`;
    link.click();
    URL.revokeObjectURL(url);
  };

  if (loading) {
    return (
      <div className="action-guide">
        <p className="text-muted">{t('actionGuide.loading')}</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="action-guide">
        <p className="error-text">{t('actionGuide.errorWithMessage', { message: error })}</p>
      </div>
    );
  }

  if (!guide) {
    return null;
  }

  const handleActionSelect = (name: string) => {
    navigate(`/actions/guide/${encodeURIComponent(name)}`);
  };

  const actionAnchorId = (name: string) =>
    `action-${name
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, '-')
      .replace(/^-+|-+$/g, '')}`;

  return (
    <div className="action-guide">
      <header className="action-guide__header">
        <div>
          <h2>{t('actionGuide.title')}</h2>
          <p className="text-muted">{t('actionGuide.description')}</p>
          {formattedDate ? (
            <p className="action-guide__meta">{t('actionGuide.generated', { date: formattedDate })}</p>
          ) : null}
        </div>
        <div className="action-guide__actions">
          <button
            className="button button--secondary"
            onClick={downloadGuide}
            disabled={!guide.markdown}
          >
            {t('actionGuide.download')}
          </button>
        </div>
      </header>

      <section className="action-guide__primer">
        <div>
          <h3>{t('actionGuide.primerTitle')}</h3>
          <pre className="code-block">{guide.primer}</pre>
        </div>
      </section>

      <nav className="action-guide__toc" aria-label={t('actionGuide.indexTitle')}>
        <h3>{t('actionGuide.indexTitle')}</h3>
        <div className="action-guide__toc-groups">
          {groupedActions.map((group) => (
            <div className="action-guide__toc-group" key={group.category}>
              <h4 className="action-guide__toc-group-title">
                {t(`actionGuide.categories.${group.category}`)}
              </h4>
              <div className="action-guide__toc-list">
                {group.actions.map((action) => (
                  <a
                    key={action.name}
                    className="action-guide__toc-link"
                    href={`#${actionAnchorId(action.name)}`}
                  >
                    {action.name}
                  </a>
                ))}
              </div>
            </div>
          ))}
        </div>
      </nav>

      <div className="action-guide__groups">
        {groupedActions.map((group) => (
          <section className="action-guide__group" key={group.category}>
            <header className="action-guide__group-header">
              <h3>{t(`actionGuide.categories.${group.category}`)}</h3>
              <span className="action-guide__group-count">
                {t('actionGuide.groupCount', { count: group.actions.length })}
              </span>
            </header>
            <div className="action-guide__grid">
              {group.actions.map((action) => (
                <article
                  className="action-card action-card--clickable"
                  key={action.name}
                  id={actionAnchorId(action.name)}
                  onClick={() => handleActionSelect(action.name)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault();
                      handleActionSelect(action.name);
                    }
                  }}
                  role="button"
                  tabIndex={0}
                >
                  <div className="action-card__header">
                    <span className="badge">{action.name}</span>
                  </div>
                  <div className="action-card__section">
                    <h5>{t('actionGuide.required')}</h5>
                    {renderFieldList(action.required)}
                  </div>
                  <div className="action-card__section">
                    <h5>{t('actionGuide.optional')}</h5>
                    {renderFieldList(action.optional)}
                  </div>
                  {renderAllowedValues(action)}
                  <div className="action-card__section">
                    <h5>{t('actionGuide.operations')}</h5>
                    {renderOperations(action.operations)}
                  </div>
                  {action.example ? (
                    <div className="action-card__section">
                      <h5>{t('actionGuide.example')}</h5>
                      <pre className="code-block">{action.example}</pre>
                    </div>
                  ) : null}
                </article>
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}

export default ActionGuidePage;
