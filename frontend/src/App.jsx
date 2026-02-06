import React from 'react';
import useStore from './store';
import FlowGraph from './components/FlowGraph';

function App() {
  const { count, increment, decrement, reset } = useStore();

  const runFlow = async () => {
      try {
          await fetch('/api/run', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ input: "test execution", score: 85 })
          });
      } catch (e) {
          console.error("Failed to run flow", e);
      }
  };

  return (
    <div className="flex flex-col min-h-screen bg-gray-100 p-4">
      <header className="mb-8 text-center">
          <h1 className="text-3xl font-bold underline mb-4 text-blue-600">
            GoFlow Dashboard
          </h1>
      </header>

      <main className="flex-1 flex flex-col gap-8 max-w-6xl mx-auto w-full">

          <div className="bg-white p-6 rounded-lg shadow-md">
              <div className="flex justify-between items-center mb-4">
                  <h2 className="text-xl font-semibold">Flow Visualization</h2>
                  <button
                    onClick={runFlow}
                    className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 font-bold"
                  >
                      Run Workflow
                  </button>
              </div>
              <FlowGraph />
          </div>

          <div className="bg-white p-6 rounded-lg shadow-md max-w-md mx-auto w-full">
            <h2 className="text-xl font-semibold mb-4 text-center">
              Simple Counter (Zustand)
            </h2>
            <div className="flex flex-col items-center gap-4">
                <span className="text-2xl font-mono">{count}</span>
                <div className="flex gap-2">
                <button
                    onClick={increment}
                    className="px-4 py-2 bg-green-500 text-white rounded hover:bg-green-600"
                >
                    Increment
                </button>
                <button
                    onClick={decrement}
                    className="px-4 py-2 bg-red-500 text-white rounded hover:bg-red-600"
                >
                    Decrement
                </button>
                <button
                    onClick={reset}
                    className="px-4 py-2 bg-gray-500 text-white rounded hover:bg-gray-600"
                >
                    Reset
                </button>
                </div>
            </div>
          </div>
      </main>
    </div>
  );
}

export default App;
