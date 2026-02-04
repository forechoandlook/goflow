# Repository Guidelines

## Project Structure & Module Organization
- `core.go` keeps the central `Node` interface and action constants; `flows/flow.go` wires the builder API, checkpointing hooks, and transition tables.
- `nodes/` now splits implementations into purpose-built files (`function_node.go`, `llm_node.go`, `llm_router.go`, `conditional_node.go`, `loop_node.go`, `parallel_node.go`, `logger_node.go`, `delay_node.go`, `kv_nodes.go`, `registry.go`, `types.go`). Each file focuses on a family of nodes and self-registers metadata for discovery.
- `cmd/goflow/main.go` remains the canonical example runner, while `doc.md` captures architectural intent; keep supporting helpers (`checkpointer/`, `kv/`, `utils/`) adjacent to the workflows they support.

## Build, Test, and Development Commands
- `go build ./...` ensures all packages compile.
- `GOCACHE=/tmp/goflow_gocache go test ./...` (or provide an accessible `GOCACHE` path) runs every test without hitting protected cache directories.
- `go test ./... -run TestName` narrows scope when debugging a single package.
- `go run ./cmd/goflow` exercises the sample flows and exercises nodes in one execution.

## Coding Style & Naming Conventions
- Follow Go idioms: tabs for indentation, descriptive identifiers, exported names matching their packages.
- Run `gofmt` (and `goimports` when editing imports) before committing.
- Keep node IDs expressive, use prefixes such as `llm:`/`router:` when naming prompts, and keep configuration structs (e.g., `LLMNodeConfig`, `LLMRouterConfig`) explicit about their keys.

## Testing Guidelines
- Place tests beside the code they cover (`nodes/logger_node_test.go`, `flows/flow_test.go`, etc.) and use `testing` plus table-driven helpers when behaviors share setup.
- Validate shared-state mutations explicitly (copy inputs, assert keys) so flows remain deterministic.
- Prefer lightweight mocks: nil OpenAI clients already return predictable output, which makes flow tests fast.

## Node Registry & Default Nodes
- `nodes/registry.go` exposes `NodeDefinition`, `RegisterNode`, and `RegisteredNodes()` so you can enumerate the built-in catalog.
- Each built-in node adds metadata in its `init` block, describing its purpose and giving sample constructors (Function, LLM, Router, Conditional, Loop, Parallel, Logger, Delay, KV Read/Write). Register new nodes there to keep tooling/reflection in sync.
- Flow execution now honors `nodes.AttributeAwareNode`, so you can wrap any node via `nodes.WrapNodeWithAttributes` to add retry counts or delays without changing the implementation.

## NodeResult & Signal Listeners
- Nodes return `goflow.NodeResult`, so build payloads with `goflow.ResultWithAction` and include optional `signal`/`signals` keys to notify other listeners.
- Use `flows.FlowBuilder.Listen("signal_name", listenerNode)` (or `Flow.Listen`) to register asynchronous nodes that run in their own goroutines whenever `goflow.SignalsFromResult` sees a matching name.
- Signal listeners reuse the same `shared` map, so to avoid races either take copies or keep your handler idempotent; the main flow continues without waiting for them.

## Commit & Pull Request Guidelines
- Use Conventional Commit prefixes (`feat:`, `fix:`, `docs:`) plus a brief clause on what changed.
- PR descriptions should summarize the rationale, link related issues (if any), and note the commands you ran (e.g., `GOCACHE=/tmp/goflow_gocache go test ./...`).
- Make sure `go test ./...` succeeds locally before merging and mention the outcome in the PR.
