import { memo } from 'react';
import { NodeProps } from 'reactflow';

interface ParallelGroupData {
    label: string;
    taskCount: number;
}

function ParallelGroupNode({ data }: NodeProps<ParallelGroupData>) {
    return (
        <div className="parallel-group">
            <div className="parallel-group__header">
                <div className="parallel-group__label">{data.label}</div>
                <div className="parallel-group__count">
                    {data.taskCount} {data.taskCount === 1 ? 'task' : 'tasks'}
                </div>
            </div>
        </div>
    );
}

export default memo(ParallelGroupNode);
