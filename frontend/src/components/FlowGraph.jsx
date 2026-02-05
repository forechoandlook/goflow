import React, { useEffect, useState } from 'react';
import { ReactFlow, useNodesState, useEdgesState, Background, Controls, MiniMap } from '@xyflow/react';
import '@xyflow/react/dist/style.css';

// Simple auto-layout function
const getLayoutedElements = (nodes, edges) => {
  const levels = {};
  const adj = {};
  const inDegree = {};

  nodes.forEach(n => {
    inDegree[n.id] = 0;
    adj[n.id] = [];
  });

  edges.forEach(edge => {
    if (adj[edge.source]) adj[edge.source].push(edge.target);
    inDegree[edge.target] = (inDegree[edge.target] || 0) + 1;
  });

  const queue = nodes.filter(n => inDegree[n.id] === 0).map(n => ({ id: n.id, level: 0 }));

  // Handling cycles or no start node
  if (queue.length === 0 && nodes.length > 0) {
      queue.push({ id: nodes[0].id, level: 0 });
  }

  const visited = new Set();
  const resultNodes = [];

  // BFS for levels
  const levelMap = {};

  // Initialize queue
  queue.forEach(item => {
      levelMap[item.id] = 0;
  });

  // We need to process all nodes.
  // Simple approach: BFS. If we encounter a node again, we might update its level?
  // For DAGs, we want longest path.
  // Let's just do simple BFS.

  const bfsQueue = [...queue];

  while (bfsQueue.length > 0) {
    const { id, level } = bfsQueue.shift();

    // If we've seen this node at a deeper level, skip (or update?)
    // Actually for tree-like, just simple BFS

    if (adj[id]) {
        adj[id].forEach(target => {
            if (levelMap[target] === undefined) {
                 levelMap[target] = level + 1;
                 bfsQueue.push({ id: target, level: level + 1 });
            }
        });
    }
  }

  // Handle disconnected nodes
  nodes.forEach(node => {
      if (levelMap[node.id] === undefined) {
          levelMap[node.id] = 0;
      }
  });

  const levelCounts = {};

  return {
    nodes: nodes.map(node => {
      const level = levelMap[node.id];
      const count = levelCounts[level] || 0;
      levelCounts[level] = count + 1;

      return {
        ...node,
        position: { x: count * 200, y: level * 150 },
      };
    }),
    edges,
  };
};

export default function FlowGraph() {
  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const [loading, setLoading] = useState(true);

  const fetchData = async () => {
      try {
          const res = await fetch('/api/graph');
          const data = await res.json();

          const rawNodes = data.nodes ? data.nodes.map(n => ({
              id: n.id,
              data: { label: n.id },
              position: { x: 0, y: 0 }, // temp
              style: { border: '1px solid #777', padding: '10px', borderRadius: '5px' }
          })) : [];

          const rawEdges = data.edges ? data.edges.map((e, i) => ({
              id: `e${i}`,
              source: e.from,
              target: e.to,
              label: e.action,
              animated: true
          })) : [];

          const layouted = getLayoutedElements(rawNodes, rawEdges);
          setNodes(layouted.nodes);
          setEdges(layouted.edges);
          setLoading(false);
      } catch (err) {
          console.error("Failed to fetch graph", err);
      }
  };

  useEffect(() => {
    fetchData();
  }, []);

  // Poll for events
  useEffect(() => {
    const interval = setInterval(async () => {
        try {
            const res = await fetch('/api/events');
            const events = await res.json();

            setNodes((nds) => nds.map((node) => {
                // Determine node state based on events
                // Find latest event for this node
                const nodeEvents = events.filter(e => e.Node === node.id);
                if (nodeEvents.length === 0) return {
                     ...node,
                     style: { ...node.style, background: '#fff', color: '#000' }
                };

                const lastEvent = nodeEvents[nodeEvents.length - 1];
                let bg = '#fff';
                let color = '#000';

                if (lastEvent.Type === 'node_start') {
                    bg = '#ffeb3b'; // yellow for running
                } else if (lastEvent.Type === 'node_end') {
                    bg = '#4caf50'; // green for success
                    color = '#fff';
                } else if (lastEvent.Type === 'node_error') {
                    bg = '#f44336'; // red for error
                    color = '#fff';
                }

                return {
                    ...node,
                    style: { ...node.style, background: bg, color: color }
                };
            }));
        } catch (e) {
            console.error(e);
        }
    }, 500);
    return () => clearInterval(interval);
  }, [setNodes]);

  if (loading) return <div>Loading graph...</div>;

  return (
    <div style={{ width: '100%', height: '500px', border: '1px solid #ccc' }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        fitView
      >
        <Background />
        <Controls />
        <MiniMap />
      </ReactFlow>
    </div>
  );
}
