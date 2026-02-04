# GoFlow - A Flexible Workflow Engine

GoFlow is a flexible workflow engine inspired by PocketFlow and LangGraph concepts. It allows you to define complex workflows with conditional branching, checkpointing, and more.

## Features

- **Modular Architecture**: Clean separation of concerns with nodes, flows, checkpointer, and utilities
- **Chainable API**: Fluent interface for building workflows with method chaining
- **Flexible Nodes**: Support for different types of nodes (LLM, Conditional, Router, Loop, etc.)
- **User-Friendly Node Definition**: Easy creation of custom nodes with function-based approach
- **Checkpointing**: Both in-memory and persistent checkpointing capabilities
- **KV Storage**: Built-in key-value storage for persistence
- **Context Control**: Proper context handling with timeout and cancellation support
- **Thread Safety**: Concurrent-safe implementations using mutexes
- **Conditional Branching**: Advanced conditional routing with sub-flow execution
- **Parallel Execution**: Support for running multiple flows in parallel

## Project Structure

```
goflow/
├── core.go                 # Core interfaces and constants
├── go.mod                  # Go module definition
├── cmd/
│   └── goflow/
│       └── main.go         # Example application
├── flows/
│   └── flow.go             # Flow implementation
├── nodes/
│   └── basic_nodes.go      # Various node implementations
├── checkpointer/
│   └── checkpointer.go     # Checkpointing functionality
├── kv/
│   └── kv.go               # Key-value storage implementations
└── utils/
    └── utils.go            # Utility functions
```

## Usage

### Basic Flow

```go
flow := flows.NewFlow(nodes.NewLLMNode("translate to english")).
    Then(nodes.NewLLMNode("polish text")).
    Then(nodes.NewLLMNode("summarize in one sentence"))

err := flow.Run(context.Background(), shared)
```

### Conditional Routing with Sub-Flows

```go
// Define condition function
conditionFunc := func(shared map[string]any) string {
    score, _ := shared["score"].(float64)
    if score > 80 {
        return "high"
    }
    return "low"
}

// Create sub-flows for different branches
highScoreFlow := flows.NewFlowBuilder(
    nodes.NewFunctionNode("high_score_handler", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
        shared["category"] = "high"
        return goflow.ResultWithAction("done"), nil
    }),
).Build()

lowScoreFlow := flows.NewFlowBuilder(
    nodes.NewFunctionNode("low_score_handler", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
        shared["category"] = "low"
        return goflow.ResultWithAction("done"), nil
    }),
).Build()

// Create conditional node with branches
conditionalNode := nodes.NewConditionalNode("score_router", conditionFunc)
conditionalNode.Branch("high", highScoreFlow)
conditionalNode.Branch("low", lowScoreFlow)

// Build main flow
mainFlow := flows.NewFlowBuilder(startNode)
    .Then(conditionalNode)
    .Build()
```

### Parallel Execution

```go
// Create sub-flows for parallel execution
parallelFlow1 := flows.NewFlowBuilder(
    nodes.NewFunctionNode("parallel_task_1", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
        shared["task1_result"] = "完成"
        return goflow.ResultWithAction("next"), nil
    }),
).Build()

parallelFlow2 := flows.NewFlowBuilder(
    nodes.NewFunctionNode("parallel_task_2", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
        shared["task2_result"] = "完成"
        return goflow.ResultWithAction("next"), nil
    }),
).Build()

// Create parallel node
parallelNode := nodes.NewParallelNode("parallel_processing", parallelFlow1, parallelFlow2)

// Build main flow
mainFlow := flows.NewFlowBuilder(inputNode)
    .Then(parallelNode)
    .Build()
```

### Custom Node Definition

```go
// Define custom node using function
customNode := nodes.NewFunctionNode("my_custom_node", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
    input, _ := shared["input"].(string)
    shared["output"] = "Processed: " + input
    return goflow.ResultWithAction("next"), nil
})

// Or create a struct that implements Node interface
type MyNode struct {
    ID string
    ProcessFunc func(context.Context, map[string]any) (goflow.NodeResult, error)
}

func (n *MyNode) Name() string {
    return n.ID
}

func (n *MyNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
    return n.ProcessFunc(ctx, shared)
}

// Create instance
myNode := &MyNode{
    ID: "my_struct_node",
    ProcessFunc: func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
        shared["status"] = "processed_by_struct"
        return goflow.ResultWithAction("success"), nil
    },
}
```

### Checkpointing

```go
// Memory-based checkpointing
memCp := checkpointer.NewMemoryCheckpointer()
err := checkpointer.RunWithCheckpoint(context.Background(), flow, threadID, initialShared, memCp)

// KV store-based checkpointing
kvStore := kv.NewFileBasedKVStore("./checkpoints.json")
kvCp := checkpointer.NewKVCheckpointer(kvStore)
err := checkpointer.RunWithCheckpoint(context.Background(), flow, threadID, initialShared, kvCp)
```

## Node Types

- `LLMNode`: Simulates LLM processing
- `ConditionalRouter`: Routes based on conditions in shared state
- `LLMRouter`: Uses an LLM to decide the next action
- `LoopNode`: Implements loop control
- `ParallelNode`: Executes multiple nodes in parallel
- `TimeoutNode`: Wraps other nodes with timeout functionality
- `NodeResult`: Nodes now return `goflow.NodeResult` maps that can include `action`, `signal`/`signals`, and other metadata. `goflow.ActionFromResult` and `goflow.SignalsFromResult` extract those values inside the flow runner.

### Node Retry Wrapper Example

Flows inspect `nodes.AttributeAwareNode`, so you can decorate any node with attributes (retries, delays, etc.) without modifying the implementation. Wrap the node before wiring it into the flow:

```go
retryable := nodes.WrapNodeWithAttributes(
    nodes.NewLLMNode(client, nodes.LLMNodeConfig{Name: "translate", InputKey: "prompt"}),
    nodes.NodeAttributes{RetryAttempts: 2, RetryDelay: time.Second},
)
flow := flows.NewFlowBuilder(retryable).
    Then(nodes.NewLoggerNode("log_result")).
    Build()
err := flow.Run(ctx, shared)
```

`Flow.Run` will rerun the wrapped node on errors up to `RetryAttempts` and respect the optional `RetryDelay`.

### Signal Listener Example

Nodes can emit one or more signal names in their `NodeResult`; flows register asynchronous listeners with `FlowBuilder.Listen`. Listeners run in separate goroutines and observe the shared state when the signal fires, so they do not block the main flow.

```go
signalHandler := nodes.NewFunctionNode("signal_logger", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
    fmt.Println("Got llm2 completion:", shared["llm2_output"])
    return goflow.ResultWithAction(goflow.ActionNext), nil
})

llm2 := nodes.NewFunctionNode("llm2", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
    // pretend we ran a second LLM and stored the result
    shared["llm2_output"] = "analysis done"
    return goflow.NodeResult{
        "action": goflow.ActionNext,
        "signal": "llm2_done",
    }, nil
})

flow := flows.NewFlowBuilder(llm2).
    Listen("llm2_done", signalHandler).
    Build()
err := flow.Run(ctx, shared)
```

`FlowBuilder.Listen` wires `signalHandler` to run whenever `"llm2_done"` appears in a node’s result, and the flow continues without waiting for the listener to finish.

## Default Nodes & Utilities

### LLMNode backed by go-openai

`nodes.NewLLMNode` now accepts a `*openai.Client` from `github.com/sashabaranov/go-openai` and a `nodes.LLMNodeConfig`. The node combines a system instruction with the shared `input` value, runs `client.CreateChatCompletion`, and stores the reply in `shared["llm_output"]`. A nil client triggers a lightweight mock response so examples stay runnable without an API key; exports still work when you set `OPENAI_API_KEY` and pass `openai.NewClient(os.Getenv("OPENAI_API_KEY"))`.

### LLMRouter

`nodes.NewLLMRouter` routes dynamically by prompting the same client for which action to take from an `Actions` list. It defaults to the first action when the model’s text does not contain one of the known names.

### Utility nodes

- `LoggerNode`: prints shared fields plus a fixed message.
- `DelayNode`: pauses for a configured duration while respecting the flow context.
- `KVReadNode` / `KVWriteNode`: integrate with `kv.KVStore` (in-memory or file-based) so you can persist pieces of `shared` mid-flow.
These helpers are available out of the box and meant to reduce boilerplate when wiring flows in `cmd/goflow` or your own integrations.

## License

MIT
