完全可以在 `Flow` 里面塞进**非常多的事情**，包括条件分支、循环、路由、错误处理、重试、超时、并行、动态决定下一个节点等等。PocketFlow 的设计理念本身就鼓励这种“极简但表达力强”的方式。

### 1. 最简单：条件分支（if-else 风格）

```go
// 在 Flow 上加一个条件连接方法
func (f *Flow) If(from Node, condition func(shared map[string]any) string, branches map[string]Node) *Flow {
    fromName := from.Name()
    if _, ok := f.transitions[fromName]; !ok {
        f.transitions[fromName] = make(map[string]string)
    }

    // condition 是一个函数，运行时根据 shared 状态决定走哪个 action
    // 但为了链式，我们这里先记录 condition，后续在 Run 时动态计算
    // （更彻底的做法是把 condition 也做成一种特殊的 Node）

    // 简化版：假设 condition 返回 action 字符串
    // 你可以把 condition 存到一个单独的 map[string]func(...)string
    f.conditions = append(f.conditions, conditionEntry{from: fromName, cond: condition, branches: branches})

    // 注册所有可能的 to 节点
    for action, to := range branches {
        f.Add(to)
        f.transitions[fromName][action] = to.Name()
    }
    return f
}

// 在 Run 里处理
// （伪码，实际要改动 current 选择逻辑）
if conds, ok := f.getConditionsFor(current.Name()); ok {
    for _, entry := range conds {
        action := entry.cond(shared)
        if nextName, has := f.transitions[current.Name()][action]; has {
            current = f.nodes[nextName]
            goto nextIteration
        }
    }
}
```

更推荐的做法：把**条件判断本身做成一个 Node**（ConditionalRouter），这样链式更统一：

```go
type ConditionalRouter struct {
    name      string
    condition func(shared map[string]any) string // 返回 "yes", "no", "error" 等
}

func (r *ConditionalRouter) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
    return goflow.NodeResult{"action": r.condition(shared)}, nil
}
```

然后链式这样写：

```go
flow := NewFlow(startLLM).
    Then(analyzeNode).
    Then(&ConditionalRouter{
        condition: func(s map[string]any) string {
            if score, ok := s["relevance_score"].(float64); ok && score > 0.7 {
                return "high"
            }
            return "low"
        },
    }).
    Connect(conditional, "high", summarizeNode).
    Connect(conditional, "low",  searchMoreNode)
```

### 2. 动态路由（LLM 决定下一个节点）

这几乎是 PocketFlow / LangGraph 风格的核心：

```go
type LLMRouter struct {
    llm    LLMClient
    prompt string
}

func (r *LLMRouter) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
    // 用 LLM 决定 action
    response := r.llm.Call(ctx, r.prompt + shared["last_output"].(string))
    action := strings.TrimSpace(response) // 假设返回 "search", "summarize", "end" 等
    return goflow.NodeResult{"action": action}, nil
}
```

链式：

```go
flow.NewFlow(entryNode).
    Then(llmThinkNode).
    Then(&LLMRouter{llm: myLLM, prompt: `根据以下内容决定下一步: {input}\n选项: search / summarize / critique / end`}).
    Connect(router, "search",    searchToolNode).
    Connect(router, "summarize", finalNode).
    Connect(router, "critique",  critiqueNode).
    Connect(router, "end",       nil) // 或直接 break
```

### 3. 循环 / 迭代 / MaxSteps 控制

```go
type LoopNode struct {
    maxIterations int
    counter       int
}

func (l *LoopNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
    l.counter++
    if l.counter >= l.maxIterations {
        return goflow.NodeResult{"action": "max_reached"}, nil
    }
    return goflow.NodeResult{"action": "continue"}, nil
}
```

或者更简单：在 `Flow.Run()` 里加一个全局计数器：

```go
func (f *Flow) WithMaxSteps(max int) *Flow {
    f.maxSteps = max
    return f
}

// Run 里
steps := 0
for current != nil {
    if f.maxSteps > 0 && steps >= f.maxSteps {
        return fmt.Errorf("max steps exceeded")
    }
    steps++
    // ...
}
```

### 4. 并行（Fan-out / Fan-in）

```go
type ParallelNode struct {
    subNodes []Node
}

func (p *ParallelNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
    var wg sync.WaitGroup
    results := make([]string, len(p.subNodes))
    errs := make(chan error, len(p.subNodes))

    for i, node := range p.subNodes {
        wg.Add(1)
        go func(idx int, n Node) {
            defer wg.Done()
            res, err := n.Run(ctx, shared) // 注意：shared 要加锁或用 copy
            if err != nil {
                errs <- err
                return
            }
            results[idx] = goflow.ActionFromResult(res)
        }(i, node)
    }
    wg.Wait()
    // 合并结果到 shared
    return goflow.NodeResult{"action": "parallel_done"}, nil
}
```

链式：

```go
flow.Then(&ParallelNode{subNodes: []Node{toolA, toolB, toolC}}).
    Then(mergeResultsNode)
```

### 5. 其他很常见的扩展（都可以链式加进去）

- `.WithRetry(max int, backoff time.Duration)`
- `.WithTimeout(d time.Duration)`（用 context.WithTimeout 包装每个 Run）
- `.WithLogger(logger)`
- `.WithSharedInitializer(func() map[string]any)`
- `.WithErrorHandler(func(err error, shared) string)` → 决定是 retry / skip / end
- `.Branch(name string)` → 命名分支，便于复杂图
- `.Merge(branches ...string)` → 扇入

### 建议的进阶设计原则

- **把一切都做成 Node**（包括 Router、Loop、Retry、Timeout 等），这样链式最统一
- **transitions 用 map[string]map[string]string** 表达图（from → action → to）
- **shared 用 Mutex 保护**，或者用 immutable copy + merge
- **Run 支持 streaming**（如果 LLM 支持），可以 yield 中间结果
- **支持子 Flow**（把一个 Flow 当成一个超级Node)


# 节点控制

要实现**控制**（如暂停/中断/人工干预/Human-in-the-Loop）和**状态恢复**（checkpoint、持久化、可重启），在 Go 实现的类似 PocketFlow / LangGraph 风格的流程系统中，主要有以下几个核心思路。以下从简单到生产级逐步展开。

### 1. 最简单版：内存中的“可中断 + 手动恢复”

核心：把当前执行位置（current node）和 shared state 一起保存下来。

```go
type Checkpoint struct {
    ThreadID     string                 // 会话/任务唯一标识
    CurrentNode  string                 // 当前节点名
    Shared       map[string]any         // 完整状态（或序列化后存）
    StepCount    int
    Timestamp    time.Time
    // 可选：History []string  // 记录走过的路径，用于调试
}

type Flow struct {
    // ... 之前有的字段
    checkpointer Checkpointer // 接口，下面定义
}

type Checkpointer interface {
    Save(ctx context.Context, cp *Checkpoint) error
    Load(ctx context.Context, threadID string) (*Checkpoint, error)
    ListThreads() ([]string, error) // 用于管理多个会话
}

// 内存实现（开发/测试用）
type MemoryCheckpointer struct {
    mu    sync.RWMutex
    store map[string]*Checkpoint
}

func (m *MemoryCheckpointer) Save(_ context.Context, cp *Checkpoint) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.store[cp.ThreadID] = cp
    return nil
}

func (m *MemoryCheckpointer) Load(_ context.Context, threadID string) (*Checkpoint, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    cp, ok := m.store[threadID]
    if !ok {
        return nil, fmt.Errorf("checkpoint not found")
    }
    return cp, nil
}
```

在 `Run` 里每步后保存（或在特定节点保存）：

```go
func (f *Flow) Run(ctx context.Context, threadID string, initialShared map[string]any) error {
    var shared map[string]any
    var current Node

    // 尝试恢复
    if cp, err := f.checkpointer.Load(ctx, threadID); err == nil && cp != nil {
        shared = cp.Shared
        current = f.nodes[cp.CurrentNode]
        fmt.Printf("从 checkpoint 恢复: node=%s, steps=%d\n", current.Name(), cp.StepCount)
    } else {
        shared = initialShared
        current = f.start
    }

    steps := 0
    for current != nil {
        // 可中断点：检查 context 是否被取消（Ctrl+C、超时、外部信号）
        if ctx.Err() != nil {
            // 保存当前状态
            cp := &Checkpoint{
                ThreadID:    threadID,
                CurrentNode: current.Name(),
                Shared:      shared,
                StepCount:   steps,
                Timestamp:   time.Now(),
            }
            f.checkpointer.Save(ctx, cp)
            return ctx.Err() // 返回错误，外部可重试
        }

        action, err := current.Run(ctx, shared)
        if err != nil {
            // 可选：保存错误状态
            return err
        }

        // 保存 checkpoint（可以每步都存，或只在关键节点存）
        cp := &Checkpoint{
            ThreadID:    threadID,
            CurrentNode: current.Name(), // 下一步开始前存
            Shared:      shared,         // 或 deep copy
            StepCount:   steps + 1,
        }
        f.checkpointer.Save(ctx, cp)

        // 查找下一个
        nextName, ok := f.transitions[current.Name()][action]
        if !ok {
            break
        }
        current = f.nodes[nextName]
        steps++
    }

    // 成功完成，可选：删除 checkpoint 或标记 done
    return nil
}
```

使用方式：

```go
// 第一次运行
flow.Run(ctx, "task-uuid-123", map[string]any{"question": "..."}) 

// 中断后重启（同一个 threadID）
flow.Run(newCtx, "task-uuid-123", nil)  // initialShared 可以传 nil
```

### 2. 加入显式“暂停 / Human-in-the-Loop”控制

让节点自己决定是否暂停：

```go
type InterruptibleNode interface {
    Run(ctx context.Context, shared map[string]any) (action string, shouldPause bool, err error)
}

// 或直接用特殊 action
const (
    ActionContinue = "continue"
    ActionPause    = "pause_for_human"
    ActionEnd      = "end"
)
```

在 Run 循环中：

```go
action, err := node.Run(...)
if action == ActionPause {
    // 保存 checkpoint 并返回特殊错误
    cp := ... 
    f.checkpointer.Save(ctx, cp)
    return ErrPausedForHuman{ThreadID: threadID}
}
```

外部代码：

```go
err := flow.Run(ctx, threadID, nil)
if errors.Is(err, ErrPausedForHuman) {
    // 通知人工，展示 shared 内容
    // 人工修改 shared 后，再次调用 Run（同一个 threadID）
}
```

### 3. 生产级持久化（推荐做法对比）

| 方式                  | 实现难度 | 持久化后端          | 适合场景                          | 备注                              |
|-----------------------|----------|----------------------|-----------------------------------|-----------------------------------|
| MemoryCheckpointer    | ★☆☆☆☆    | 内存                 | 开发、测试、短期任务              | 最快，但进程重启丢失              |
| JSON + 文件/Redis     | ★★☆☆☆    | 文件 / Redis / KV    | 小型服务、中型任务                | 简单，但并发要加锁                |
| gob / msgpack + DB    | ★★★☆☆    | PostgreSQL / MongoDB | 中大型、需要事务                  | 推荐用 json + postgres            |
| 类似 LangGraph 的 Checkpointer | ★★★★☆ | Postgres / Redis / DynamoDB | 生产级、多租户、时间旅行调试     | 需要实现线程概念（thread_id）     |

推荐最实用的 Postgres 版（用 sqlc 或 bun/gorm）：

- 表结构建议：

  ```sql
  CREATE TABLE checkpoints (
      thread_id     TEXT PRIMARY KEY,
      checkpoint    JSONB NOT NULL,   -- 整个 Checkpoint struct json
      created_at    TIMESTAMPTZ,
      updated_at    TIMESTAMPTZ
  );
  ```

- shared 里避免放超大内容（如完整 embedding），可以只存 reference（document_id、s3 key 等）。

### 4. 更进一步的控制能力（常见需求）

- **时间旅行调试**：保存多个历史 checkpoint（加 version/step_id），支持从任意点回放
- **分支 / 版本**：thread_id + branch_id（类似 git branch）
- **自动重试 + 幂等**：节点实现幂等（看到 shared 已有结果就 skip）
- **超时控制**：每个节点用子 context.WithTimeout
- **事件/日志**：每次 checkpoint 触发事件，可用于监控、审计

一句话总结当前最实用起点：

1. 先用 `MemoryCheckpointer` 实现 Run 时每步（或关键节点）保存 current + shared
2. 让 Run 支持从 checkpoint 恢复（threadID 作为 key）
3. 加 context 取消检测 → 中断时自动保存
4. 再逐步替换成文件/DB 持久化


# 内置一些数据库 比如 kv数据库就好了
