import { ChangeEvent, useCallback, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import useFlowStore from '../state/flowStore';
import { uploadFlowDefinition } from '../api/client';
import useToastStore from '../state/toastStore';

function FlowListPage() {
  const navigate = useNavigate();
  const flows = useFlowStore((state) => state.flows);
  const loadFlows = useFlowStore((state) => state.loadFlows);
  const selectFlow = useFlowStore((state) => state.selectFlow);
  const importedFlowIds = useFlowStore((state) => state.importedFlowIds);
  const removeImportedFlow = useFlowStore((state) => state.removeImportedFlow);
  const importFlow = useFlowStore((state) => state.importFlow);
  const { t } = useTranslation();
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const addToast = useToastStore((state) => state.addToast);

  useEffect(() => {
    loadFlows();
    selectFlow(); // Clear active flow and subflows menu
  }, [loadFlows, selectFlow]);

  const handleImportClick = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  const handleFileChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0];
      if (!file) {
        return;
      }

      const reader = new FileReader();

      reader.onload = async (loadEvent) => {
        try {
          const contents = loadEvent.target?.result;
          if (typeof contents !== 'string') {
            throw new Error(t('appShell.feedback.invalidFile'));
          }

          const importedFlow = await uploadFlowDefinition(contents, file.name);
          importFlow({ ...importedFlow, sourceFileName: file.name });
          addToast({
            type: 'success',
            message: t('appShell.feedback.success', { flowId: importedFlow.id }),
            timeoutMs: 3500
          });
        } catch (error) {
          const message =
            error instanceof Error
              ? error.message
              : t('appShell.feedback.unknown');
          console.error('Failed to import flow. Details:', message);
          console.error(error);
          addToast({
            type: 'error',
            message: t('appShell.feedback.error', { message }),
            timeoutMs: 6000
          });
        } finally {
          event.target.value = '';
        }
      };

      reader.onerror = () => {
        addToast({
          type: 'error',
          message: t('appShell.feedback.readError'),
          timeoutMs: 6000
        });
        event.target.value = '';
      };

      reader.readAsText(file);
    },
    [importFlow, t]
  );

  return (
    <div>
      <div className="flow-list__header">
        <div>
          <h2>{t('flowList.title')}</h2>
          <p className="flow-list__description">{t('flowList.description')}</p>
        </div>
        <div className="flow-list__actions">
          <input
            ref={fileInputRef}
            type="file"
            accept="application/json"
            onChange={handleFileChange}
            style={{ display: 'none' }}
          />
          <button className="button button--secondary" onClick={handleImportClick}>
            {t('buttons.importFlow')}
          </button>
        </div>
      </div>
      <div className="flow-list">
        {flows.length === 0 ? (
          <p className="flow-list__empty">{t('flowList.empty')}</p>
        ) : (
          flows.map((flow) => (
            <article key={flow.id} className="flow-card">
              <header>
                <h3>{flow.id}</h3>
                <p className="flow-card__description">{flow.description}</p>
              </header>
              <div className="flow-card__meta">
                <span>{t('flowList.taskCount', { count: flow.tasks.length })}</span>
              </div>
              <div className="flow-card__footer">
                {importedFlowIds.includes(flow.id) ? (
                  <button
                    className="button button--danger"
                    onClick={() => removeImportedFlow(flow.id)}
                  >
                    {t('buttons.delete')}
                  </button>
                ) : null}
                <button className="button" onClick={() => navigate(`/flows/${flow.id}`)}>
                  {t('buttons.open')}
                </button>
              </div>
            </article>
          ))
        )}
      </div>
    </div>
  );
}

export default FlowListPage;
