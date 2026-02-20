import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import { Handle, NodeProps, Position } from 'reactflow';
import { TaskNodeData } from './FlowCanvas';
import TaskTypeIcon, { getVariantForAction } from './TaskTypeIcon';

function TaskNode({ data }: NodeProps<TaskNodeData>) {
  const { t } = useTranslation();
  const { task, isSelected, isStopTarget, nestedChildCount, nestedChildLabel } = data;
  const title = typeof task.description === 'string' && task.description.trim() ? task.description : task.id;

  const normalizedStatus = typeof task.status === 'string' ? task.status.toLowerCase() : '';
  const isRunning = normalizedStatus === 'in progress' || normalizedStatus === 'running';
  const isFailed = normalizedStatus === 'failed' || (task.success === false && normalizedStatus !== '' && normalizedStatus !== 'not started');
  const isSucceeded = task.success === true || normalizedStatus === 'completed' || normalizedStatus === 'succeeded';

  // Get visual variant based on action
  const variant = getVariantForAction(task.action);

  // Status Classes
  let statusClass = 'task-node__status-pill--pending';
  let statusText = normalizedStatus || t('taskNode.statusFallback');

  if (isRunning) {
    statusClass = 'task-node__status-pill--running';
    statusText = 'Running';
  } else if (isFailed) {
    statusClass = 'task-node__status-pill--failed';
    statusText = 'Failed';
  } else if (isSucceeded) {
    statusClass = 'task-node__status-pill--success';
    statusText = 'Success';
  }

  // Styles for the node based on variant
  const nodeStyle = {
    '--node-color': variant.color,
    '--node-bg': variant.background
  } as React.CSSProperties;

  return (
    <div
      className={`task-node ${isStopTarget ? 'task-node--stop' : ''} ${isSelected ? 'task-node--selected' : ''}`}
      style={nodeStyle}
    >
      <Handle type="target" position={Position.Left} className="task-handle task-handle--target" />

      {/* Colored Left Stripe */}
      <div className="task-node__stripe" style={{ backgroundColor: variant.color }} />

      <div className="task-node__content-wrapper">
        <div className="task-node__header">
          <div className="task-node__icon-container" style={{ color: variant.color, backgroundColor: variant.background }}>
            {variant.icon}
          </div>          <div className="task-node__title-container">
            <div className="task-node__title" title={title}>{title}</div>
            <div className="task-node__id" title={task.id}>{task.id}</div>
          </div>
        </div>

        <div className="task-node__meta">
          <div className="task-node__meta-row">
            <span className="task-node__type-pill" style={{ color: variant.color, backgroundColor: variant.background, borderColor: variant.background }}>
              {variant.label}
            </span>
            {task.operation && (
              <span className="task-node__op-pill" title={task.operation}>
                {task.operation}
              </span>
            )}
          </div>

          {(isRunning || isFailed || isSucceeded || normalizedStatus) && (
            <div className="task-node__meta-row">
              <span className={`task-node__status-pill ${statusClass}`}>
                {statusText}
              </span>
            </div>
          )}
        </div>

        {nestedChildCount ? (
          <div className="task-node__nested-info">
            {nestedChildCount} {nestedChildLabel ?? 'child'} {nestedChildCount === 1 ? 'task' : 'tasks'}
          </div>
        ) : null}
      </div>

      <Handle type="source" position={Position.Right} className="task-handle task-handle--source" />
    </div>
  );
}

export default memo(TaskNode);
