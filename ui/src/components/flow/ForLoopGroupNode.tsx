import { memo } from 'react';
import { NodeProps } from 'reactflow';

interface ForLoopGroupData {
    label: string;
    taskCount: number;
}

function ForLoopGroupNode({ data }: NodeProps<ForLoopGroupData>) {
    return (
        <div className="for-loop-group">
            <div className="for-loop-group__header">
                <div className="for-loop-group__label">{data.label}</div>
                <div className="for-loop-group__count">
                    {data.taskCount} {data.taskCount === 1 ? 'iteration' : 'iterations'}
                </div>
            </div>
        </div>
    );
}

export default memo(ForLoopGroupNode);
