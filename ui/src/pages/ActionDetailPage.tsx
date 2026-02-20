import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Link, useParams } from 'react-router-dom';
import { fetchActionsGuide } from '../api/client';
import { ActionDocumentation, ActionsGuide } from '../types/actionsGuide';

function ActionDetailPage() {
  const { t } = useTranslation(undefined, { lng: 'en' });
  const { actionName } = useParams();
  const [guide, setGuide] = useState<ActionsGuide | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const markdownComponents = useMemo(
    () => ({
      pre: ({ children }: { children?: ReactNode }) => <pre className="code-block">{children}</pre>,
      code: ({ inline, children }: { inline?: boolean; children?: ReactNode }) =>
        inline ? <code className="inline-code">{children}</code> : <code>{children}</code>
    }),
    []
  );

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

  const action = useMemo<ActionDocumentation | null>(() => {
    if (!guide?.actions || !actionName) {
      return null;
    }
    const normalized = actionName.toUpperCase();
    return guide.actions.find((candidate) => candidate.name.toUpperCase() === normalized) ?? null;
  }, [guide?.actions, actionName]);

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

  if (!action) {
    return (
      <div className="action-guide">
        <Link className="action-detail__back" to="/actions/guide">
          {t('actionGuide.back')}
        </Link>
        <p className="text-muted">{t('actionGuide.notFound')}</p>
      </div>
    );
  }

  return (
    <div className="action-detail">
      <header className="action-detail__header">
        <div>
          <Link className="action-detail__back" to="/actions/guide">
            {t('actionGuide.back')}
          </Link>
          <h2>{action.name}</h2>
          <p className="text-muted">{t('actionGuide.detailDescription')}</p>
        </div>
      </header>
      {action.helpMarkdown ? (
        <div className="action-detail__markdown">
          <ReactMarkdown components={markdownComponents} remarkPlugins={[remarkGfm]}>
            {action.helpMarkdown}
          </ReactMarkdown>
        </div>
      ) : (
        <p className="text-muted">{t('actionGuide.noHelp')}</p>
      )}
    </div>
  );
}

export default ActionDetailPage;
