import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { TaskDefinition } from '../types/flow';
import { fetchFlowNotes } from '../api/client';
import useFlowStore from '../state/flowStore';
import FlowCanvas from '../components/flow/FlowCanvas';
import TaskInspector from '../components/flow/TaskInspector';
import ExecutionTimeline from '../components/flow/ExecutionTimeline';
import FlowControls from '../components/flow/FlowControls';

import { extractForChildren, extractParallelChildren, isSubtaskId } from '../utils/flowUtils';

const findTaskById = (tasks: TaskDefinition[], taskId: string): TaskDefinition | undefined => {
  for (const task of tasks) {
    if (task.id === taskId) {
      return task;
    }

    // Check standard children
    if (task.children) {
      const nested = findTaskById(task.children, taskId);
      if (nested) {
        return nested;
      }
    }

    // Check PARALLEL children
    const parallelChildren = extractParallelChildren(task);
    if (parallelChildren.length > 0) {
      const nested = findTaskById(parallelChildren, taskId);
      if (nested) {
        return nested;
      }
    }

    // Check FOR children
    const forChildren = extractForChildren(task);
    if (forChildren.length > 0) {
      const nested = findTaskById(forChildren, taskId);
      if (nested) {
        return nested;
      }
    }
  }
  return undefined;
};

function FlowBuilderPage() {
  const { flowId } = useParams();
  const openFlow = useFlowStore((state) => state.openFlow);
  const loadFlows = useFlowStore((state) => state.loadFlows);
  const connectToRunStream = useFlowStore((state) => state.connectToRunStream);
  const flowsCount = useFlowStore((state) => state.flows.length);
  const activeFlow = useFlowStore((state) => state.activeFlow);
  const triggerRun = useFlowStore((state) => state.triggerRun);
  const triggerTaskRun = useFlowStore((state) => state.triggerTaskRun);
  const triggerResume = useFlowStore((state) => state.triggerResume);
  const triggerStop = useFlowStore((state) => state.triggerStop);
  const setStopAtTask = useFlowStore((state) => state.setStopAtTask);
  const isFlowRunning = useFlowStore((state) => state.isFlowRunning);
  const lastRunFinished = useFlowStore((state) => state.lastRunFinished);
  const focusTaskId = useFlowStore((state) => state.focusTaskId);
  const requestTaskFocus = useFlowStore((state) => state.focusOnTask);
  const stopAtTaskId = useFlowStore((state) => state.stopAtTaskId);
  const loadError = useFlowStore((state) => state.loadError);
  const [selectedTask, setSelectedTask] = useState<TaskDefinition | undefined>(activeFlow?.tasks[0]);
  const [activePanel, setActivePanel] = useState<'inspector' | 'execution' | 'notes'>('inspector');
  const [runPending, setRunPending] = useState(false);
  const [taskRunPending, setTaskRunPending] = useState(false);
  const [resumePending, setResumePending] = useState(false);
  const [stopPending, setStopPending] = useState(false);
  const [stopAtPending, setStopAtPending] = useState(false);
  const [runError, setRunError] = useState<string | null>(null);
  const [flowNotes, setFlowNotes] = useState<string | null>(null);
  const { t } = useTranslation();
  const hasFlowNotes = flowNotes !== null;

  const markdownComponents = useMemo(
    () => ({
      pre: ({ children }: { children?: ReactNode }) => <pre className="code-block">{children}</pre>,
      code: ({ inline, children }: { inline?: boolean; children?: ReactNode }) =>
        inline ? <code className="inline-code">{children}</code> : <code>{children}</code>
    }),
    []
  );

  useEffect(() => {
    if (flowsCount === 0) {
      void loadFlows();
    }
  }, [flowsCount, loadFlows]);

  useEffect(() => {
    let isActive = true;

    const load = async () => {
      setFlowNotes(null);
      if (!flowId) {
        return;
      }
      await openFlow(flowId);
      if (!isActive) {
        return;
      }
      try {
        const notes = await fetchFlowNotes();
        if (isActive) {
          setFlowNotes(notes);
        }
      } catch {
        if (isActive) {
          setFlowNotes(null);
        }
      }
    };

    void load();
    return () => {
      isActive = false;
    };
  }, [flowId, openFlow]);

  useEffect(() => {
    setSelectedTask(activeFlow?.tasks[0]);
  }, [activeFlow]);

  useEffect(() => {
    if (!hasFlowNotes && activePanel === 'notes') {
      setActivePanel('inspector');
    }
  }, [hasFlowNotes, activePanel]);

  useEffect(() => {
    const disconnect = connectToRunStream();
    return () => {
      disconnect();
    };
  }, [connectToRunStream]);

  useEffect(() => {
    if (focusTaskId && activeFlow) {
      const task = findTaskById(activeFlow.tasks, focusTaskId);
      if (task) {
        setSelectedTask(task);
      }
    }
  }, [focusTaskId, activeFlow]);

  useEffect(() => {
    if (!selectedTask || !activeFlow) {
      return;
    }
    const refreshed = findTaskById(activeFlow.tasks, selectedTask.id);
    if (refreshed && refreshed !== selectedTask) {
      setSelectedTask(refreshed);
    }
  }, [activeFlow, selectedTask]);

  const handleTaskSelect = (taskId: string) => {
    if (activeFlow) {
      setSelectedTask(findTaskById(activeFlow.tasks, taskId));
    }
  };

  const handleRunFlow = async () => {
    setRunError(null);
    setRunPending(true);
    try {
      await triggerRun();
    } catch (error) {
      if (error instanceof Error) {
        setRunError(error.message);
      } else {
        setRunError(t('flowBuilder.runError'));
      }
    } finally {
      setRunPending(false);
    }
  };

  const handleRunTask = async () => {
    if (!selectedTask) {
      return;
    }
    setRunError(null);
    setTaskRunPending(true);
    try {
      await triggerTaskRun(selectedTask.id);
    } catch (error) {
      if (error instanceof Error) {
        setRunError(error.message);
      } else {
        setRunError(t('flowBuilder.runError'));
      }
    } finally {
      setTaskRunPending(false);
    }
  };

  const handleResume = async () => {
    if (!selectedTask) {
      return;
    }
    setRunError(null);
    setResumePending(true);
    try {
      await triggerResume(selectedTask.id);
    } catch (error) {
      if (error instanceof Error) {
        setRunError(error.message);
      } else {
        setRunError(t('flowBuilder.resumeError'));
      }
    } finally {
      setResumePending(false);
    }
  };

  const handleStop = async () => {
    setRunError(null);
    setStopPending(true);
    try {
      await triggerStop();
    } catch (error) {
      if (error instanceof Error) {
        setRunError(error.message);
      } else {
        setRunError(t('flowBuilder.stopError'));
      }
    } finally {
      setStopPending(false);
    }
  };

  const isTaskResumeEligible = selectedTask?.status === 'completed' || selectedTask?.status === 'failed';
  const isSubtask = selectedTask && activeFlow ? isSubtaskId(activeFlow.tasks, selectedTask.id) : false;
  const canResume = !isFlowRunning && lastRunFinished && isTaskResumeEligible && !isSubtask;
  const canStopAtTask = Boolean(selectedTask) && !isSubtask;
  const isStopAtTaskActive = Boolean(selectedTask && stopAtTaskId === selectedTask.id);

  const handleStopAtTask = async () => {
    if (!selectedTask || !canStopAtTask) {
      return;
    }
    setRunError(null);
    setStopAtPending(true);
    try {
      const nextTaskId = stopAtTaskId === selectedTask.id ? undefined : selectedTask.id;
      await setStopAtTask(nextTaskId);
    } catch (error) {
      if (error instanceof Error) {
        setRunError(error.message);
      } else {
        setRunError(t('flowBuilder.stopAtError'));
      }
    } finally {
      setStopAtPending(false);
    }
  };

  const [panelWidth, setPanelWidth] = useState(380);
  const [isResizing, setIsResizing] = useState(false);

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isResizing) return;

      // Calculate new width based on window width minus mouse position
      // We subtract a small buffer for the sidebar if needed, but simple calculation is:
      // window.innerWidth - e.clientX
      // But we need to account for the right edge.
      // Actually, since the panel is on the right, width = window.innerWidth - e.clientX
      const newWidth = window.innerWidth - e.clientX;

      // Clamp between 380 and 760
      if (newWidth >= 380 && newWidth <= 760) {
        setPanelWidth(newWidth);
      }
    };

    const handleMouseUp = () => {
      setIsResizing(false);
      document.body.style.cursor = 'default';
    };

    if (isResizing) {
      window.addEventListener('mousemove', handleMouseMove);
      window.addEventListener('mouseup', handleMouseUp);
      document.body.style.cursor = 'col-resize';
    }

    return () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
      document.body.style.cursor = 'default';
    };
  }, [isResizing]);

  const startResizing = () => {
    setIsResizing(true);
  };

  if (!activeFlow) {
    return (
      <div className="flow-builder-page">
        <header className="flow-builder-header">
          <div>
            <h2>{t('flowBuilder.noFlowTitle')}</h2>
            <p className="flow-builder-header__description">{t('flowBuilder.noFlowDescription')}</p>
            {loadError ? (
              <div style={{
                marginTop: '1rem',
                padding: '1rem',
                backgroundColor: '#fee2e2',
                border: '1px solid #ef4444',
                borderRadius: '0.375rem',
                color: '#b91c1c'
              }}>
                <h3 style={{ margin: '0 0 0.5rem 0', fontSize: '1rem', fontWeight: 600 }}>Error loading flow</h3>
                <p style={{ margin: 0, whiteSpace: 'pre-wrap', fontFamily: 'monospace', fontSize: '0.875rem' }}>
                  {loadError}
                </p>
              </div>
            ) : null}
          </div>
        </header>
      </div>
    );
  }

  return (
    <div className="flow-builder-page">
      <header className="flow-builder-header">
        <div className="flow-builder-header__meta">
          <h2>{activeFlow.id}</h2>
          <p className="flow-builder-header__description">{activeFlow.description}</p>
        </div>
        <div className="flow-builder-header__actions">
          <div className="flow-builder-header__actions">
            <FlowControls
              onRun={handleRunFlow}
              onStopAtTask={handleStopAtTask}
              onRunTask={handleRunTask}
              onStop={handleStop}
              onResume={handleResume}
              isFlowRunning={isFlowRunning}
              runPending={runPending}
              taskRunPending={taskRunPending}
              stopPending={stopPending}
              resumePending={resumePending}
              stopAtTaskPending={stopAtPending}
              canRunTask={!!selectedTask}
              canResume={canResume}
              canStopAtTask={canStopAtTask}
              stopAtTaskActive={isStopAtTaskActive}
            />
          </div>
        </div>
        {runError ? (
          <p style={{ color: '#dc2626', marginTop: '0.5rem' }}>{runError}</p>
        ) : null}
      </header>
      <div
        className="flow-builder-layout"
        style={{ gridTemplateColumns: `1fr auto ${panelWidth}px` }}
      >
        <section className="flow-builder-canvas">
          <FlowCanvas
            flow={activeFlow}
            onTaskSelect={handleTaskSelect}
            selectedTaskId={selectedTask?.id}
            stopAtTaskId={stopAtTaskId}
            focusTaskId={focusTaskId}
            onTaskFocusHandled={() => requestTaskFocus(undefined)}
          />
        </section>

        <div className="resize-handle" onMouseDown={startResizing} />

        <aside className="form-panel">
          <div className="panel-tabs">
            <button
              onClick={() => setActivePanel('inspector')}
              className={activePanel === 'inspector' ? 'active' : ''}
            >
              {t('flowBuilder.tabs.inspector')}
            </button>
            <button
              onClick={() => setActivePanel('execution')}
              className={activePanel === 'execution' ? 'active' : ''}
            >
              {t('flowBuilder.tabs.execution')}
            </button>
            {hasFlowNotes ? (
              <button
                onClick={() => setActivePanel('notes')}
                className={activePanel === 'notes' ? 'active' : ''}
              >
                {t('flowBuilder.tabs.notes')}
              </button>
            ) : null}
          </div>
          {activePanel === 'inspector' && selectedTask ? <TaskInspector task={selectedTask} /> : null}
          {activePanel === 'execution' ? <ExecutionTimeline flow={activeFlow} /> : null}
          {activePanel === 'notes' && hasFlowNotes ? (
            <div className="flow-notes__markdown">
              <ReactMarkdown components={markdownComponents} remarkPlugins={[remarkGfm]}>
                {flowNotes ?? ''}
              </ReactMarkdown>
            </div>
          ) : null}
        </aside>
      </div>
    </div>
  );
}

export default FlowBuilderPage;
