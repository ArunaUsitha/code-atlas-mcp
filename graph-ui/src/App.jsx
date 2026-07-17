import React, { useEffect, useState } from 'react';
import ForceGraph3D from 'react-force-graph-3d';

export default function App() {
  const [graphData, setGraphData] = useState({ nodes: [], links: [] });
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    // Fetch nodes and edges from Go embedded endpoint
    fetch('/api/graph')
      .then((res) => res.json())
      .then((data) => {
        setGraphData(data);
        setLoading(false);
      })
      .catch((err) => {
        console.error("Failed to load graph data", err);
        setLoading(false);
      });
  }, []);

  // Node coloration based on kind/group
  const getNodeColor = (node) => {
    switch (node.group) {
      case 'file': return '#4299e1';       // Blue
      case 'class': return '#ecc94b';      // Yellow
      case 'function': return '#48bb78';   // Green
      case 'http_route': return '#f56565'; // Red
      default: return '#a0aec0';           // Grey
    }
  };

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-900 text-white font-sans">
        <div className="text-xl animate-pulse">Loading Codebase Architecture Graph...</div>
      </div>
    );
  }

  return (
    <div className="relative h-screen w-screen bg-slate-950 overflow-hidden">
      {/* 3D Force Graph Render Canvas */}
      <ForceGraph3D
        graphData={graphData}
        nodeLabel={(node) => `${node.name} (${node.kind})`}
        nodeColor={getNodeColor}
        nodeVal={(node) => (node.kind === 'file' ? 6 : 3)} // Files rendered larger
        linkDirectionalArrowLength={3.5}
        linkDirectionalArrowRelPos={1}
        linkColor={() => 'rgba(255,255,255,0.15)'}
        linkWidth={0.7}
      />

      {/* Floating Control Legend */}
      <div className="absolute top-4 left-4 p-4 rounded-xl bg-slate-900/80 backdrop-blur-md border border-slate-800 text-white font-sans text-xs flex flex-col gap-2">
        <h3 className="font-bold text-sm mb-1 text-sky-400">Legend</h3>
        <div className="flex items-center gap-2">
          <span className="h-3 w-3 rounded-full bg-blue-500" /> Files
        </div>
        <div className="flex items-center gap-2">
          <span className="h-3 w-3 rounded-full bg-yellow-500" /> Classes
        </div>
        <div className="flex items-center gap-2">
          <span className="h-3 w-3 rounded-full bg-green-500" /> Functions
        </div>
        <div className="flex items-center gap-2">
          <span className="h-3 w-3 rounded-full bg-red-500" /> HTTP/gRPC Routes
        </div>
      </div>
    </div>
  );
}
