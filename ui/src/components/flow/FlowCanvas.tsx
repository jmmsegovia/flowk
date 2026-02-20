import { useCallback, useEffect, useMemo, useRef } from 'react';
import ReactFlow, {
  Background,
  Controls,
  Edge,
  MarkerType,
  Node,
  NodeMouseHandler,
  Position,
  useNodesState,
  ReactFlowInstance
} from 'reactflow';
import 'reactflow/dist/style.css';
import { FlowDefinition, TaskDefinition } from '../../types/flow';
import TaskNode from './TaskNode';
import SubflowGroupNode from './SubflowGroupNode';
import ForLoopGroupNode from './ForLoopGroupNode';
import ParallelGroupNode from './ParallelGroupNode';
import ParallelChildNode, { ParallelChildNodeData } from './ParallelChildNode';
import {
  EvaluateBranchName,
  extractEvaluateBranchInfo,
  extractForChildren,
  extractParallelChildren,
  getStringField,
  isEvaluateTask,
  isForTask,
  isParallelTask,
  isRecord
} from '../../utils/flowUtils';

const nodeTypes = {
  taskNode: TaskNode,
  subflowGroup: SubflowGroupNode,
  forLoopGroup: ForLoopGroupNode,
  parallelGroup: ParallelGroupNode,
  parallelChild: ParallelChildNode
};

interface FlowCanvasProps {
  flow: FlowDefinition;
  selectedTaskId?: string;
  stopAtTaskId?: string;
  onTaskSelect: (taskId: string) => void;
  focusTaskId?: string;
  onTaskFocusHandled?: (taskId: string) => void;
}

interface TaskNodeData {
  task: TaskDefinition;
  isSelected: boolean;
  isStopTarget?: boolean;
  parallelChildCount?: number;
  nestedChildCount?: number;
  nestedChildLabel?: string;
}

interface SubflowGroupData {
  label: string;
}

interface ForLoopGroupData {
  label: string;
  taskCount: number;
}

interface ParallelGroupData {
  label: string;
  taskCount: number;
}

type FlowNodeData = TaskNodeData | SubflowGroupData | ForLoopGroupData | ParallelGroupData | ParallelChildNodeData;



const estimateNodeWidth = (task: TaskDefinition): number => {
  const descriptionLength =
    typeof task.description === 'string' ? task.description.length : 0;
  const actionLength = typeof task.action === 'string' ? task.action.length : 0;
  const base = 280;
  const estimate = base + (descriptionLength + actionLength + task.id.length) * 2.5;
  return Math.min(Math.max(estimate, 320), 680);
};

const estimateNodeHeight = (task: TaskDefinition): number => {
  const base = 170;
  const descriptionLength = typeof task.description === 'string' ? task.description.length : 0;
  const extraLines = Math.max(0, Math.ceil(descriptionLength / 70) - 1);
  const hasOperation = typeof task.operation === 'string' && task.operation.trim().length > 0;
  return base + extraLines * 18 + (hasOperation ? 10 : 0);
};



function FlowCanvas({
  flow,
  onTaskSelect,
  selectedTaskId,
  stopAtTaskId,
  focusTaskId,
  onTaskFocusHandled
}: FlowCanvasProps) {
  const rfInstanceRef = useRef<ReactFlowInstance | null>(null);
  const lastCenteredTaskIdRef = useRef<string | undefined>(undefined);
  const taskLookup = useMemo(() => {
    const map = new Map<string, TaskDefinition>();
    const visit = (tasks: TaskDefinition[]) => {
      tasks.forEach((task) => {
        map.set(task.id, task);
        if (task.children?.length) {
          visit(task.children);
        }
      });
    };
    visit(flow.tasks);
    return map;
  }, [flow.tasks]);

  const buildNodes = useCallback(
    (
      tasks: TaskDefinition[],
      lane: number,
      baseX = 0,
      baseY = 0
    ): { nodes: Node<FlowNodeData>[]; inlineEdges: Edge[] } => {
      const nodes: Node<FlowNodeData>[] = [];
      const inlineEdges: Edge[] = [];
      const horizontalGap = 100;
      const verticalSpacing = 200;
      let currentX = baseX;

      tasks.forEach((task) => {
        const nodeId = task.id;
        const width = estimateNodeWidth(task);
        const baseHeight = estimateNodeHeight(task);

        // No inline layout - both FOR and PARALLEL will use visual grouping
        const height = baseHeight;

        const node: Node<TaskNodeData> = {
          id: nodeId,
          type: 'taskNode',
          data: {
            task,
            isSelected: false,
            isStopTarget: false
          },
          position: { x: currentX, y: baseY },
          sourcePosition: Position.Right,
          targetPosition: Position.Left,
          style: { width, height }
        };
        nodes.push(node);

        // Render FOR or PARALLEL children as separate nodes
        let maxChildX = currentX; // Track the rightmost position of children

        if (isForTask(task)) {
          const forChildren = extractForChildren(task);
          if (forChildren.length > 0) {
            const childResult = buildNodes(forChildren, lane + 1, currentX, baseY + verticalSpacing);
            nodes.push(...childResult.nodes);
            inlineEdges.push(...childResult.inlineEdges);

            // Calculate the maximum X position occupied by children
            childResult.nodes.forEach((childNode) => {
              if (childNode.type === 'taskNode') {
                const childWidth = typeof childNode.style?.width === 'number'
                  ? childNode.style.width
                  : estimateNodeWidth((childNode.data as TaskNodeData).task);
                const childRightEdge = childNode.position.x + childWidth;
                maxChildX = Math.max(maxChildX, childRightEdge);
              }
            });

            // Add edges from FOR parent to each child
            forChildren.forEach((child) => {
              inlineEdges.push({
                id: `for-edge-${task.id}-${child.id}`,
                source: task.id,
                target: child.id,
                animated: true,
                style: { stroke: '#f59e0b', strokeWidth: 2 },
                markerEnd: { type: MarkerType.ArrowClosed, color: '#f59e0b' }
              });
            });
          }
        } else if (isParallelTask(task)) {
          const parallelChildren = extractParallelChildren(task);
          if (parallelChildren.length > 0) {
            const childResult = buildNodes(parallelChildren, lane + 1, currentX, baseY + verticalSpacing);
            nodes.push(...childResult.nodes);
            inlineEdges.push(...childResult.inlineEdges);

            // Calculate the maximum X position occupied by children
            childResult.nodes.forEach((childNode) => {
              if (childNode.type === 'taskNode') {
                const childWidth = typeof childNode.style?.width === 'number'
                  ? childNode.style.width
                  : estimateNodeWidth((childNode.data as TaskNodeData).task);
                const childRightEdge = childNode.position.x + childWidth;
                maxChildX = Math.max(maxChildX, childRightEdge);
              }
            });

            // Add edges from PARALLEL parent to each child
            parallelChildren.forEach((child) => {
              inlineEdges.push({
                id: `parallel-edge-${task.id}-${child.id}`,
                source: task.id,
                target: child.id,
                animated: true,
                style: { stroke: '#06b6d4', strokeWidth: 2 },
                markerEnd: { type: MarkerType.ArrowClosed, color: '#06b6d4' }
              });
            });
          }
        }

        if (task.children?.length) {
          const childResult = buildNodes(task.children, lane + 1, currentX, baseY + verticalSpacing);
          nodes.push(...childResult.nodes);
          inlineEdges.push(...childResult.inlineEdges);
        }

        // Use the maximum of parent width or children extent for spacing
        const effectiveWidth = Math.max(width, maxChildX - currentX);
        currentX += effectiveWidth + horizontalGap;
      });

      return { nodes, inlineEdges };
    },
    []
  );

  const { nodes: initialTaskNodes, inlineEdges: parallelChildEdges } = useMemo(
    () => buildNodes(flow.tasks, 0),
    [flow.tasks, buildNodes]
  );

  const nodesWithGroups = useMemo(() => {
    const rootFlowId = flow.id;
    const paddingX = 32;
    const paddingY = 48;
    const subflowGroups = new Map<
      string,
      { minX: number; minY: number; maxX: number; maxY: number }
    >();
    const forLoopGroups = new Map<
      string,
      { minX: number; minY: number; maxX: number; maxY: number; parentTask: TaskDefinition; childCount: number }
    >();
    const parallelGroups = new Map<
      string,
      { minX: number; minY: number; maxX: number; maxY: number; parentTask: TaskDefinition; childCount: number }
    >();
    const subflowAssignments = new Map<string, string>();
    const forLoopAssignments = new Map<string, string>();
    const parallelAssignments = new Map<string, string>();

    // First pass: identify FOR loop and PARALLEL parent tasks and their children
    const forLoopParents = new Map<string, TaskDefinition>();
    const parallelParents = new Map<string, TaskDefinition>();

    initialTaskNodes.forEach((node) => {
      if (node.type !== 'taskNode') return;
      const taskData = node.data as TaskNodeData;
      if (!taskData?.task) return;

      if (isForTask(taskData.task)) {
        const forChildren = extractForChildren(taskData.task);
        if (forChildren.length > 0) {
          forLoopParents.set(taskData.task.id, taskData.task);
        }
      } else if (isParallelTask(taskData.task)) {
        const parallelChildren = extractParallelChildren(taskData.task);
        if (parallelChildren.length > 0) {
          parallelParents.set(taskData.task.id, taskData.task);
        }
      }
    });

    // Second pass: group nodes
    initialTaskNodes.forEach((node) => {
      if (node.type !== 'taskNode') {
        return;
      }
      const taskData = node.data as TaskNodeData;
      if (!taskData?.task) {
        return;
      }

      const flowId = taskData.task.flowId;
      if (flowId && flowId !== rootFlowId) {
        const width = typeof node.style?.width === 'number' ? node.style.width : estimateNodeWidth(taskData.task);
        const height = typeof node.style?.height === 'number' ? node.style.height : estimateNodeHeight(taskData.task);
        const left = node.position.x;
        const top = node.position.y;
        const right = left + width;
        const bottom = top + height;
        const bounds = subflowGroups.get(flowId);
        if (!bounds) {
          subflowGroups.set(flowId, { minX: left, minY: top, maxX: right, maxY: bottom });
        } else {
          bounds.minX = Math.min(bounds.minX, left);
          bounds.minY = Math.min(bounds.minY, top);
          bounds.maxX = Math.max(bounds.maxX, right);
          bounds.maxY = Math.max(bounds.maxY, bottom);
        }
      }

      // Check if this node IS a FOR or PARALLEL parent
      const isForParent = forLoopParents.has(taskData.task.id);
      const isParallelParent = parallelParents.has(taskData.task.id);

      // Check if this node is a child of a FOR loop or PARALLEL
      let isForLoopChild = false;
      let isParallelChild = false;
      let parentForId: string | undefined;
      let parentParallelId: string | undefined;

      for (const [forParentId, forParentTask] of forLoopParents.entries()) {
        const forChildren = extractForChildren(forParentTask);
        if (forChildren.some(child => child.id === taskData.task.id)) {
          isForLoopChild = true;
          parentForId = forParentId;
          break;
        }
      }

      if (!isForLoopChild) {
        for (const [parallelParentId, parallelParentTask] of parallelParents.entries()) {
          const parallelChildren = extractParallelChildren(parallelParentTask);
          if (parallelChildren.some(child => child.id === taskData.task.id)) {
            isParallelChild = true;
            parentParallelId = parallelParentId;
            break;
          }
        }
      }

      // If this is a FOR parent or child, include in FOR group
      if (isForParent || isForLoopChild) {
        const groupId = isForParent ? taskData.task.id : parentForId!;
        const forParentTask = forLoopParents.get(groupId)!;
        const forChildren = extractForChildren(forParentTask);

        forLoopAssignments.set(node.id, groupId);

        const width = typeof node.style?.width === 'number' ? node.style.width : estimateNodeWidth(taskData.task);
        const height = typeof node.style?.height === 'number' ? node.style.height : estimateNodeHeight(taskData.task);
        const left = node.position.x;
        const top = node.position.y;
        const right = left + width;
        const bottom = top + height;

        const bounds = forLoopGroups.get(groupId);
        if (!bounds) {
          forLoopGroups.set(groupId, {
            minX: left,
            minY: top,
            maxX: right,
            maxY: bottom,
            parentTask: forParentTask,
            childCount: forChildren.length
          });
        } else {
          bounds.minX = Math.min(bounds.minX, left);
          bounds.minY = Math.min(bounds.minY, top);
          bounds.maxX = Math.max(bounds.maxX, right);
          bounds.maxY = Math.max(bounds.maxY, bottom);
        }
      }
      // If this is a PARALLEL parent or child, include in PARALLEL group
      else if (isParallelParent || isParallelChild) {
        const groupId = isParallelParent ? taskData.task.id : parentParallelId!;
        const parallelParentTask = parallelParents.get(groupId)!;
        const parallelChildren = extractParallelChildren(parallelParentTask);

        parallelAssignments.set(node.id, groupId);

        const width = typeof node.style?.width === 'number' ? node.style.width : estimateNodeWidth(taskData.task);
        const height = typeof node.style?.height === 'number' ? node.style.height : estimateNodeHeight(taskData.task);
        const left = node.position.x;
        const top = node.position.y;
        const right = left + width;
        const bottom = top + height;

        const bounds = parallelGroups.get(groupId);
        if (!bounds) {
          parallelGroups.set(groupId, {
            minX: left,
            minY: top,
            maxX: right,
            maxY: bottom,
            parentTask: parallelParentTask,
            childCount: parallelChildren.length
          });
        } else {
          bounds.minX = Math.min(bounds.minX, left);
          bounds.minY = Math.min(bounds.minY, top);
          bounds.maxX = Math.max(bounds.maxX, right);
          bounds.maxY = Math.max(bounds.maxY, bottom);
        }
      }
      // If not a FOR/PARALLEL parent or child, assign to subflow group
      else if (flowId && flowId !== rootFlowId) {
        subflowAssignments.set(node.id, flowId);
      }
    });

    // Expand subflow bounds to include FOR/PARALLEL group containers
    forLoopGroups.forEach(({ minX, minY, maxX, maxY, parentTask }) => {
      const flowId = parentTask.flowId;
      if (!flowId || flowId === rootFlowId) {
        return;
      }
      const bounds = subflowGroups.get(flowId);
      if (!bounds) {
        return;
      }
      const left = minX - paddingX;
      const top = minY - paddingY;
      const right = maxX + paddingX;
      const bottom = maxY + paddingY;
      bounds.minX = Math.min(bounds.minX, left);
      bounds.minY = Math.min(bounds.minY, top);
      bounds.maxX = Math.max(bounds.maxX, right);
      bounds.maxY = Math.max(bounds.maxY, bottom);
    });

    parallelGroups.forEach(({ minX, minY, maxX, maxY, parentTask }) => {
      const flowId = parentTask.flowId;
      if (!flowId || flowId === rootFlowId) {
        return;
      }
      const bounds = subflowGroups.get(flowId);
      if (!bounds) {
        return;
      }
      const left = minX - paddingX;
      const top = minY - paddingY;
      const right = maxX + paddingX;
      const bottom = maxY + paddingY;
      bounds.minX = Math.min(bounds.minX, left);
      bounds.minY = Math.min(bounds.minY, top);
      bounds.maxX = Math.max(bounds.maxX, right);
      bounds.maxY = Math.max(bounds.maxY, bottom);
    });

    if (subflowGroups.size === 0 && forLoopGroups.size === 0 && parallelGroups.size === 0) {
      return initialTaskNodes as Node<FlowNodeData>[];
    }

    const updatedTaskNodes = initialTaskNodes.map((node) => {
      if (node.type !== 'taskNode') {
        return node as Node<FlowNodeData>;
      }

      // Check FOR loop assignment first
      const forLoopId = forLoopAssignments.get(node.id);
      if (forLoopId) {
        const bounds = forLoopGroups.get(forLoopId);
        if (bounds) {
          const offsetX = bounds.minX - paddingX;
          const offsetY = bounds.minY - paddingY;
          return {
            ...node,
            parentNode: `for-loop-group-${forLoopId}`,
            extent: 'parent',
            position: {
              x: node.position.x - offsetX,
              y: node.position.y - offsetY
            }
          } as Node<FlowNodeData>;
        }
      }

      // Check PARALLEL assignment
      const parallelId = parallelAssignments.get(node.id);
      if (parallelId) {
        const bounds = parallelGroups.get(parallelId);
        if (bounds) {
          const offsetX = bounds.minX - paddingX;
          const offsetY = bounds.minY - paddingY;
          return {
            ...node,
            parentNode: `parallel-group-${parallelId}`,
            extent: 'parent',
            position: {
              x: node.position.x - offsetX,
              y: node.position.y - offsetY
            }
          } as Node<FlowNodeData>;
        }
      }

      // Check subflow assignment
      const flowId = subflowAssignments.get(node.id);
      if (flowId) {
        const bounds = subflowGroups.get(flowId);
        if (bounds) {
          const offsetX = bounds.minX - paddingX;
          const offsetY = bounds.minY - paddingY;
          return {
            ...node,
            parentNode: `subflow-group-${flowId}`,
            extent: 'parent',
            position: {
              x: node.position.x - offsetX,
              y: node.position.y - offsetY
            }
          } as Node<FlowNodeData>;
        }
      }

      return node as Node<FlowNodeData>;
    });

    const formatLabel = (flowId: string) => {
      const parts = flowId.split('.');
      return parts[parts.length - 1] || flowId;
    };

    const subflowGroupNodes: Node<FlowNodeData>[] = Array.from(subflowGroups.entries()).map(
      ([flowId, bounds]) => {
        const width = bounds.maxX - bounds.minX + paddingX * 2;
        const height = bounds.maxY - bounds.minY + paddingY * 2;
        return {
          id: `subflow-group-${flowId}`,
          type: 'subflowGroup',
          position: { x: bounds.minX - paddingX, y: bounds.minY - paddingY },
          data: { label: formatLabel(flowId) },
          style: { width, height },
          draggable: false,
          selectable: false,
          focusable: false,
          zIndex: 0
        };
      }
    );

    const forLoopGroupNodes: Node<FlowNodeData>[] = Array.from(forLoopGroups.entries()).map(
      ([forParentId, { minX, minY, maxX, maxY, parentTask, childCount }]) => {
        const width = maxX - minX + paddingX * 2;
        const height = maxY - minY + paddingY * 2;
        const label = typeof parentTask.description === 'string' && parentTask.description.trim()
          ? parentTask.description
          : parentTask.id;
        const flowId = parentTask.flowId;
        const subflowBounds = flowId && flowId !== rootFlowId ? subflowGroups.get(flowId) : undefined;
        const absolutePosition = { x: minX - paddingX, y: minY - paddingY };
        const parentOrigin = subflowBounds
          ? { x: subflowBounds.minX - paddingX, y: subflowBounds.minY - paddingY }
          : undefined;
        return {
          id: `for-loop-group-${forParentId}`,
          type: 'forLoopGroup',
          position: parentOrigin
            ? { x: absolutePosition.x - parentOrigin.x, y: absolutePosition.y - parentOrigin.y }
            : absolutePosition,
          data: { label, taskCount: childCount },
          style: { width, height },
          draggable: false,
          selectable: false,
          focusable: false,
          parentNode: parentOrigin ? `subflow-group-${flowId}` : undefined,
          extent: parentOrigin ? 'parent' : undefined,
          zIndex: 0
        };
      }
    );

    const parallelGroupNodes: Node<FlowNodeData>[] = Array.from(parallelGroups.entries()).map(
      ([parallelParentId, { minX, minY, maxX, maxY, parentTask, childCount }]) => {
        const width = maxX - minX + paddingX * 2;
        const height = maxY - minY + paddingY * 2;
        const label = typeof parentTask.description === 'string' && parentTask.description.trim()
          ? parentTask.description
          : parentTask.id;
        const flowId = parentTask.flowId;
        const subflowBounds = flowId && flowId !== rootFlowId ? subflowGroups.get(flowId) : undefined;
        const absolutePosition = { x: minX - paddingX, y: minY - paddingY };
        const parentOrigin = subflowBounds
          ? { x: subflowBounds.minX - paddingX, y: subflowBounds.minY - paddingY }
          : undefined;
        return {
          id: `parallel-group-${parallelParentId}`,
          type: 'parallelGroup',
          position: parentOrigin
            ? { x: absolutePosition.x - parentOrigin.x, y: absolutePosition.y - parentOrigin.y }
            : absolutePosition,
          data: { label, taskCount: childCount },
          style: { width, height },
          draggable: false,
          selectable: false,
          focusable: false,
          parentNode: parentOrigin ? `subflow-group-${flowId}` : undefined,
          extent: parentOrigin ? 'parent' : undefined,
          zIndex: 0
        };
      }
    );

    return [...subflowGroupNodes, ...forLoopGroupNodes, ...parallelGroupNodes, ...updatedTaskNodes];
  }, [initialTaskNodes, flow.id]);

  const [nodes, setNodes, onNodesChange] = useNodesState<FlowNodeData>(nodesWithGroups);
  const viewportRef = useRef<{ x: number; y: number; zoom: number }>({ x: 0, y: 0, zoom: 1 });
  const lastAutoCenteredRef = useRef<{ failed?: string; inProgress?: string }>({});

  const inProgressTaskId = useMemo(() => {
    const findInProgress = (tasks: TaskDefinition[]): string | undefined => {
      for (const t of tasks) {
        const s = typeof t.status === 'string' ? t.status.toLowerCase() : '';
        if (s === 'in progress' || s === 'running') {
          return t.id;
        }
        if (t.children?.length) {
          const nested = findInProgress(t.children);
          if (nested) return nested;
        }
      }
      return undefined;
    };
    return findInProgress(flow.tasks);
  }, [flow.tasks]);

  const failedTaskId = useMemo(() => {
    const findFailed = (tasks: TaskDefinition[]): string | undefined => {
      for (const t of tasks) {
        const s = typeof t.status === 'string' ? t.status.toLowerCase() : '';
        const hasFailed = t.success === false || s.includes('fail') || s.includes('error');
        if (hasFailed) {
          return t.id;
        }
        if (t.children?.length) {
          const nested = findFailed(t.children);
          if (nested) return nested;
        }
      }
      return undefined;
    };
    return findFailed(flow.tasks);
  }, [flow.tasks]);

  useEffect(() => {
    setNodes(
      nodesWithGroups.map((node) => {
        if (node.type !== 'taskNode') {
          return node;
        }
        const data = node.data as TaskNodeData;
        return {
          ...node,
          data: {
            ...data,
            isSelected: node.id === selectedTaskId,
            isStopTarget: node.id === stopAtTaskId
          }
        };
      })
    );
  }, [nodesWithGroups, selectedTaskId, stopAtTaskId, setNodes]);

  useEffect(() => {
    const instance = rfInstanceRef.current;
    if (!instance) return;
    const anyInstance = instance as unknown as { updateNodeInternals?: (id: string) => void };
    nodes.forEach((node) => anyInstance.updateNodeInternals?.(node.id));
  }, [nodes]);

  const centerNode = useCallback(
    (targetId: string, duration = 600) => {
      const instance = rfInstanceRef.current;
      if (!instance) {
        return false;
      }
      const anyInstance: any = instance as any;
      const zoom = viewportRef.current.zoom || 1;
      const rfNode = anyInstance?.getNode?.(targetId);
      const fallbackNode = nodes.find((n) => n.id === targetId);
      if (!rfNode && !fallbackNode) {
        return false;
      }

      const fallbackPosition = fallbackNode?.position;
      const fallbackWidth =
        fallbackNode?.style && typeof fallbackNode.style.width === 'number'
          ? fallbackNode.style.width
          : undefined;
      const width =
        rfNode?.measured?.width ??
        rfNode?.width ??
        (fallbackNode?.type === 'taskNode'
          ? fallbackWidth ?? estimateNodeWidth((fallbackNode.data as TaskNodeData).task)
          : 0);
      const height =
        rfNode?.measured?.height ??
        rfNode?.height ??
        (fallbackNode?.type === 'taskNode'
          ? estimateNodeHeight((fallbackNode.data as TaskNodeData).task)
          : 0);

      const nx = rfNode?.positionAbsolute?.x ?? rfNode?.position?.x ?? fallbackPosition?.x ?? 0;
      const ny = rfNode?.positionAbsolute?.y ?? rfNode?.position?.y ?? fallbackPosition?.y ?? 0;
      const cx = nx + (width ? width / 2 : 0);
      const cy = ny + (height ? height / 2 : 0);

      if (anyInstance?.setCenter) {
        anyInstance.setCenter(cx || nx, cy || ny, { zoom, duration });
        return true;
      }

      instance.fitView({
        nodes: [{ id: targetId }],
        padding: 0.2,
        duration,
        // @ts-ignore allow options that may exist at runtime
        minZoom: zoom,
        // @ts-ignore
        maxZoom: zoom
      });
      return true;
    },
    [nodes]
  );

  useEffect(() => {
    const prevFailed = lastAutoCenteredRef.current.failed;
    const prevInProgress = lastAutoCenteredRef.current.inProgress;

    // Priority 1: Failed tasks (only react when the failed task changes)
    if (failedTaskId) {
      if (failedTaskId !== prevFailed) {
        const timeoutId = setTimeout(() => {
          if (centerNode(failedTaskId, 800)) {
            lastCenteredTaskIdRef.current = failedTaskId;
            lastAutoCenteredRef.current.failed = failedTaskId;
          }
        }, 200);
        return () => clearTimeout(timeoutId);
      }
    } else if (prevFailed) {
      lastAutoCenteredRef.current.failed = undefined;
    }

    // Priority 2: In-progress tasks (only react when the task changes)
    if (inProgressTaskId && !failedTaskId) {
      if (inProgressTaskId !== prevInProgress) {
        const timeoutId = setTimeout(() => {
          if (centerNode(inProgressTaskId, 600)) {
            lastCenteredTaskIdRef.current = inProgressTaskId;
            lastAutoCenteredRef.current.inProgress = inProgressTaskId;
          }
        }, 150);
        return () => clearTimeout(timeoutId);
      }
    } else if (prevInProgress) {
      lastAutoCenteredRef.current.inProgress = undefined;
    }

    if (!failedTaskId && !inProgressTaskId) {
      lastCenteredTaskIdRef.current = undefined;
    }
  }, [failedTaskId, inProgressTaskId, centerNode]);

  useEffect(() => {
    if (!focusTaskId) {
      return;
    }

    const schedule = () => {
      if (centerNode(focusTaskId, 450)) {
        lastCenteredTaskIdRef.current = focusTaskId;
        onTaskFocusHandled?.(focusTaskId);
      }
    };

    if (typeof window !== 'undefined' && 'requestAnimationFrame' in window) {
      const handle = requestAnimationFrame(schedule);
      return () => cancelAnimationFrame(handle);
    }

    schedule();
  }, [focusTaskId, centerNode, onTaskFocusHandled]);

  const edges = useMemo(() => {
    const collectEdges = (tasks: TaskDefinition[], acc: Edge[] = []): Edge[] => {
      tasks.forEach((task, index) => {
        const nextTask = tasks[index + 1];

        if (isEvaluateTask(task)) {
          const branches: EvaluateBranchName[] = ['then', 'else'];
          branches.forEach((branchName) => {
            const branch = extractEvaluateBranchInfo(task, branchName);
            let targetId: string | undefined;
            let usesGoto = false;

            if (branch.goto && taskLookup.has(branch.goto)) {
              targetId = branch.goto;
              usesGoto = true;
            } else if (branch.continues && nextTask) {
              targetId = nextTask.id;
            }

            if (!targetId) {
              return;
            }

            const edgeId = `${task.id}-${branchName}-${targetId}`;
            acc.push({
              id: edgeId,
              source: task.id,
              target: targetId,
              animated: usesGoto,
              label: branchName === 'then' ? 'then' : 'else',
              markerEnd: { type: MarkerType.ArrowClosed },
              style: usesGoto ? { stroke: '#16a34a' } : undefined,
              labelBgPadding: [4, 2],
              labelBgBorderRadius: 4,
              labelBgStyle: { fill: '#ffffff', color: '#0f172a' }
            });
          });
        } else if (nextTask) {
          acc.push({
            id: `${task.id}-${nextTask.id}`,
            source: task.id,
            target: nextTask.id,
            markerEnd: { type: MarkerType.ArrowClosed }
          });
        }

        if (task.children?.length) {
          task.children.forEach((child) => {
            acc.push({
              id: `${task.id}-${child.id}`,
              source: task.id,
              target: child.id,
              animated: true,
              markerEnd: { type: MarkerType.ArrowClosed }
            });
          });
          collectEdges(task.children, acc);
        }
      });
      return acc;
    };

    const baseEdges = collectEdges(flow.tasks);
    return [...baseEdges, ...parallelChildEdges];
  }, [flow.tasks, taskLookup, parallelChildEdges]);

  const handleNodeClick: NodeMouseHandler = (_, node) => {
    if (node.type !== 'taskNode') {
      return;
    }
    onTaskSelect(node.id);
  };

  return (
    <div className="flow-canvas">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onNodeClick={handleNodeClick}
        nodeTypes={nodeTypes}
        onMoveEnd={(_, viewport) => {
          // viewport: { x, y, zoom }
          viewportRef.current = viewport as { x: number; y: number; zoom: number };
        }}
        onInit={(instance) => {
          rfInstanceRef.current = instance;
          instance.fitView({ padding: 0.2 });
        }}
        style={{ width: '100%', height: '100%' }}
      >
        <Controls />
        <Background gap={16} />
      </ReactFlow>
    </div>
  );
}

export type { TaskNodeData };
export default FlowCanvas;
