package flows

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	goflow "goflow"
	"goflow/nodes"
	"unicode"
)

type dslTransition struct {
	from   string
	action string
	to     string
}

type dslParser struct {
	nodes       map[string]Node
	start       string
	firstNode   string
	transitions []dslTransition
}

// ParseFlowDSL builds a flow from a simple line-oriented DSL.
func ParseFlowDSL(script string) (*Flow, error) {
	parser := &dslParser{
		nodes: make(map[string]Node),
	}
	if err := parser.parse(script); err != nil {
		return nil, err
	}
	return parser.build()
}

func (p *dslParser) parse(script string) error {
	scanner := bufio.NewScanner(strings.NewReader(script))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		tokens, err := tokenizeLine(raw)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNum, err)
		}
		if len(tokens) == 0 {
			continue
		}
		switch tokens[0] {
		case "node":
			if err := p.parseNode(tokens); err != nil {
				return fmt.Errorf("line %d: %w", lineNum, err)
			}
		case "start":
			if err := p.parseStart(tokens); err != nil {
				return fmt.Errorf("line %d: %w", lineNum, err)
			}
		case "connect":
			if err := p.parseConnect(tokens); err != nil {
				return fmt.Errorf("line %d: %w", lineNum, err)
			}
		default:
			return fmt.Errorf("line %d: unsupported directive %q", lineNum, tokens[0])
		}
	}
	return scanner.Err()
}

func (p *dslParser) parseNode(tokens []string) error {
	if len(tokens) < 4 || tokens[2] != "=" {
		return fmt.Errorf("invalid node definition, expected `node <id> = <type> ...`")
	}
	id := tokens[1]
	if _, exists := p.nodes[id]; exists {
		return fmt.Errorf("node %q already defined", id)
	}
	nodeType := tokens[3]
	args := tokens[4:]
	node, err := buildDSLNode(id, nodeType, args)
	if err != nil {
		return err
	}
	p.nodes[id] = node
	if p.firstNode == "" {
		p.firstNode = id
	}
	return nil
}

func (p *dslParser) parseStart(tokens []string) error {
	if len(tokens) != 2 {
		return fmt.Errorf("start directive expects a single node name")
	}
	p.start = tokens[1]
	return nil
}

func (p *dslParser) parseConnect(tokens []string) error {
	if len(tokens) < 3 {
		return fmt.Errorf("connect directive requires at least source and target node")
	}
	from := tokens[1]
	action := ""
	to := ""

	switch len(tokens) {
	case 3:
		to = tokens[2]
	case 4:
		if tokens[2] == "->" {
			to = tokens[3]
		} else {
			action = tokens[2]
			to = tokens[3]
		}
	case 5:
		if tokens[2] != "->" {
			return fmt.Errorf("unexpected connect syntax")
		}
		to = tokens[3]
		action = tokens[4]
	default:
		return fmt.Errorf("unexpected connect syntax")
	}

	if action == "" {
		action = goflow.ActionNext
	}

	p.transitions = append(p.transitions, dslTransition{from: from, action: action, to: to})
	return nil
}

func (p *dslParser) build() (*Flow, error) {
	if len(p.nodes) == 0 {
		return nil, fmt.Errorf("no nodes defined in DSL")
	}

	start := p.start
	if start == "" {
		start = p.firstNode
	}
	if start == "" {
		return nil, fmt.Errorf("a start node must be declared")
	}

	startNode, ok := p.nodes[start]
	if !ok {
		return nil, fmt.Errorf("start node %q is not defined", start)
	}

	builder := NewFlowBuilder(startNode)
	for name, node := range p.nodes {
		if name == start {
			continue
		}
		builder.Add(node)
	}

	for _, tr := range p.transitions {
		fromNode, ok := p.nodes[tr.from]
		if !ok {
			return nil, fmt.Errorf("transition from undefined node %q", tr.from)
		}
		toNode, ok := p.nodes[tr.to]
		if !ok {
			return nil, fmt.Errorf("transition to undefined node %q", tr.to)
		}
		if tr.action == "" {
			tr.action = goflow.ActionNext
		}
		builder.Connect(fromNode, tr.action, toNode)
	}

	return builder.Build(), nil
}

func buildDSLNode(id, nodeType string, args []string) (Node, error) {
	positional, named := splitArgs(args)
	switch nodeType {
	case "llm":
		return buildDSLNodeLLM(id, positional, named)
	case "logger":
		return buildDSLNodeLogger(id, positional)
	case "delay":
		return buildDSLNodeDelay(id, positional)
	case "set":
		return buildDSLNodeSet(id, positional, named)
	case "http":
		return buildDSLNodeHTTP(id, positional, named)
	case "shell":
		return buildDSLNodeShell(id, positional, named)
	case "lua":
		return buildDSLNodeLua(id, positional, named)
	default:
		return nil, fmt.Errorf("unsupported node type %q", nodeType)
	}
}

func buildDSLNodeHTTP(id string, positional []string, named map[string]string) (Node, error) {
	cfg := nodes.DefaultHTTPNodeConfig(id)
	if len(positional) > 0 && cfg.URL == "" {
		cfg.URL = positional[0]
	}
	if u, ok := named["url"]; ok && u != "" {
		cfg.URL = u
	}
	if cfg.URL == "" {
		return nil, fmt.Errorf("http node requires url")
	}
	if method, ok := named["method"]; ok && method != "" {
		cfg.Method = method
	}
	if body, ok := named["body"]; ok && body != "" {
		cfg.BodyTemplate = body
	}
	if timeout, ok := named["timeout"]; ok && timeout != "" {
		dur, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout %q: %w", timeout, err)
		}
		cfg.Timeout = dur
	}
	if responseKey, ok := named["response"]; ok && responseKey != "" {
		cfg.ResponseKey = responseKey
	}
	if statusKey, ok := named["status"]; ok && statusKey != "" {
		cfg.StatusKey = statusKey
	}
	if parseJSON, ok := named["json"]; ok && parseJSON != "" {
		cfg.ResponseAsJSON = parseBool(parseJSON)
	}
	return nodes.NewHTTPNode(cfg)
}

func buildDSLNodeShell(id string, positional []string, named map[string]string) (Node, error) {
	if len(positional) == 0 {
		return nil, fmt.Errorf("shell node requires command")
	}
	cfg := nodes.DefaultShellNodeConfig(id)
	cfg.Command = positional[0]
	if len(positional) > 1 {
		cfg.Args = positional[1:]
	}
	if dir, ok := named["dir"]; ok {
		cfg.Dir = dir
	}
	if inputKey, ok := named["input"]; ok {
		cfg.InputKey = inputKey
	}
	if outputKey, ok := named["output"]; ok {
		cfg.OutputKey = outputKey
	}
	if timeout, ok := named["timeout"]; ok && timeout != "" {
		dur, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout %q: %w", timeout, err)
		}
		cfg.Timeout = dur
	}
	if parseJSON, ok := named["json"]; ok && parseJSON != "" {
		cfg.ParseJSON = parseBool(parseJSON)
	}
	return nodes.NewShellNode(cfg)
}

func buildDSLNodeLua(id string, positional []string, named map[string]string) (Node, error) {
	cfg := nodes.DefaultLuaNodeConfig(id)
	if script, ok := named["script"]; ok && script != "" {
		cfg.Script = script
	}
	if path, ok := named["path"]; ok && path != "" {
		cfg.ScriptPath = path
	}
	if cfg.ScriptPath == "" && len(positional) > 0 {
		cfg.ScriptPath = positional[0]
	}
	if working, ok := named["dir"]; ok && working != "" {
		cfg.WorkingDir = working
	}
	if args, ok := named["args"]; ok && args != "" {
		cfg.Args = strings.Split(args, ",")
	}
	if timeout, ok := named["timeout"]; ok && timeout != "" {
		dur, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout %q: %w", timeout, err)
		}
		cfg.Timeout = dur
	}
	if parseOutput, ok := named["parse_output"]; ok && parseOutput != "" {
		cfg.ParseOutput = parseBool(parseOutput)
	}
	return nodes.NewLuaNode(cfg)
}

func parseBool(raw string) bool {
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func buildDSLNodeLLM(id string, positional []string, named map[string]string) (Node, error) {
	cfg := nodes.DefaultLLMNodeConfig("")
	cfg.Name = id

	if model, ok := named["model"]; ok && model != "" {
		cfg.Model = model
	}
	if system, ok := named["system"]; ok && system != "" {
		cfg.SystemPrompt = system
	}
	if system, ok := named["system_prompt"]; ok && system != "" {
		cfg.SystemPrompt = system
	}
	if prompt, ok := named["prompt"]; ok && prompt != "" && cfg.SystemPrompt == "" {
		cfg.SystemPrompt = prompt
	}
	if temp, ok := named["temperature"]; ok {
		parsed, err := strconv.ParseFloat(temp, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid temperature %q: %w", temp, err)
		}
		cfg.Temperature = float32(parsed)
	}
	if input, ok := named["input"]; ok && input != "" {
		cfg.InputKey = input
	}
	if output, ok := named["output"]; ok && output != "" {
		cfg.OutputKey = output
	}

	if len(positional) > 0 && cfg.SystemPrompt == "" {
		cfg.SystemPrompt = positional[0]
	}

	return nodes.NewLLMNode(nil, cfg), nil
}

func buildDSLNodeLogger(id string, args []string) (Node, error) {
	message := id
	var keys []string
	if len(args) > 0 {
		message = args[0]
		if len(args) > 1 {
			keys = args[1:]
		}
	}
	return nodes.NewLoggerNode(id, message, keys...), nil
}

func buildDSLNodeDelay(id string, args []string) (Node, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("delay node requires a duration argument")
	}
	dur, err := time.ParseDuration(args[0])
	if err != nil {
		return nil, err
	}
	return nodes.NewDelayNode(id, dur), nil
}

func buildDSLNodeSet(id string, positional []string, named map[string]string) (Node, error) {
	var key, value string
	if len(positional) >= 2 {
		key = positional[0]
		value = positional[1]
	} else {
		if k, ok := named["key"]; ok {
			key = k
		}
		if v, ok := named["value"]; ok {
			value = v
		}
	}

	if key == "" || value == "" {
		return nil, fmt.Errorf("set node requires key and value")
	}

	return nodes.NewFunctionNode(id, func(_ context.Context, shared map[string]any) (goflow.NodeResult, error) {
		shared[key] = value
		return goflow.ResultWithAction(goflow.ActionNext), nil
	}), nil
}

func splitArgs(args []string) (positional []string, named map[string]string) {
	named = make(map[string]string)
	for _, arg := range args {
		if idx := strings.Index(arg, "="); idx >= 0 {
			named[arg[:idx]] = arg[idx+1:]
			continue
		}
		positional = append(positional, arg)
	}
	return positional, named
}

func tokenizeLine(line string) ([]string, error) {
	var tokens []string
	var buf strings.Builder
	inQuote := false
	escaping := false

	for _, r := range line {
		switch {
		case escaping:
			buf.WriteRune(r)
			escaping = false
		case r == '\\':
			escaping = true
		case r == '"':
			if inQuote {
				tokens = append(tokens, buf.String())
				buf.Reset()
			}
			inQuote = !inQuote
		case unicode.IsSpace(r) && !inQuote:
			if buf.Len() > 0 {
				tokens = append(tokens, buf.String())
				buf.Reset()
			}
		default:
			buf.WriteRune(r)
		}
	}

	if escaping {
		return nil, fmt.Errorf("unfinished escape sequence")
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated quoted string")
	}

	if buf.Len() > 0 {
		tokens = append(tokens, buf.String())
	}

	return tokens, nil
}
