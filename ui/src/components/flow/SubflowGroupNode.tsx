import { memo } from 'react';
import { NodeProps } from 'reactflow';

interface SubflowGroupData {
  label: string;
}

function SubflowGroupNode({ data }: NodeProps<SubflowGroupData>) {
  return (
    <div className="subflow-group">
      <div className="subflow-group__label">{data.label}</div>
    </div>
  );
}

export default memo(SubflowGroupNode);
