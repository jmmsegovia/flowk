import { memo } from 'react';
import { NodeProps } from 'reactflow';
import { TaskDefinition } from '../../types/flow';

interface ParallelChildNodeData {
  task: TaskDefinition;
}

function ParallelChildNode({ data }: NodeProps<ParallelChildNodeData>) {
  const { task } = data;
  const description =
    typeof task.description === 'string' && task.description.trim().length > 0
      ? task.description
      : task.id;
  const operation = typeof task.operation === 'string' ? task.operation : undefined;
  const platform = typeof task.platform === 'string' ? task.platform : undefined;
  return (
    <div className="parallel-child-node">
      <div className="parallel-child-node__header">
        <span className="parallel-child-node__action">{task.action}</span>
        <span className="parallel-child-node__id">{task.id}</span>
      </div>
      <p className="parallel-child-node__description">{description}</p>
      <div className="parallel-child-node__meta">
        {operation ? <span>Op: {operation}</span> : null}
        {platform ? <span>Platform: {platform}</span> : null}
      </div>
    </div>
  );
}

export type { ParallelChildNodeData };
export default memo(ParallelChildNode);
