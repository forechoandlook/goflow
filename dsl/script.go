package dsl

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
	"unicode"

	goflow "goflow"
	"goflow/flows"
	"goflow/nodes"
)

// BuildFlowFromScript transforms a lightweight script into a Flow.
func BuildFlowFromScript(script string) (*flows.Flow, error) {
	var steps []nodes.Node
	for idx, raw := range strings.Split(script, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		name, rest := takeToken(line)
		if name == "" {
			continue
		}

		step, err := buildStep(idx+1, name, strings.TrimSpace(rest))
		if err != nil {
			return nil, err
		}

		steps = append(steps, step)
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf("script contains no executable steps")
	}

	builder := flows.NewFlowBuilder(steps[0])
	for _, step := range steps[1:] {
		builder.Then(step)
	}

	return builder.Build(), nil
}

func buildStep(line int, name, args string) (nodes.Node, error) {
	switch strings.ToLower(name) {
	case "set":
		key, rest := takeToken(args)
		if key == "" {
			return nil, fmt.Errorf("dsl line %d: missing key for set", line)
		}
		value, err := parseStringArgument(rest)
		if err != nil {
			return nil, fmt.Errorf("dsl line %d: invalid value for %s: %w", line, key, err)
		}
		preset := value
		return nodes.NewFunctionNode(fmt.Sprintf("dsl-set-%d", line), func(_ context.Context, shared map[string]any) (goflow.NodeResult, error) {
			shared[key] = renderTemplate(preset, shared)
			return goflow.ResultWithAction(goflow.ActionNext), nil
		}), nil
	case "log":
		message, err := parseStringArgument(args)
		if err != nil {
			return nil, fmt.Errorf("dsl line %d: log message %w", line, err)
		}
		preset := message
		return nodes.NewFunctionNode(fmt.Sprintf("dsl-log-%d", line), func(_ context.Context, shared map[string]any) (goflow.NodeResult, error) {
			log.Println(renderTemplate(preset, shared))
			return goflow.ResultWithAction(goflow.ActionNext), nil
		}), nil
	case "delay":
		arg := strings.TrimSpace(args)
		if arg == "" {
			return nil, fmt.Errorf("dsl line %d: delay duration required", line)
		}
		dur, err := time.ParseDuration(arg)
		if err != nil {
			return nil, fmt.Errorf("dsl line %d: invalid duration %q: %w", line, arg, err)
		}
		return nodes.NewDelayNode(fmt.Sprintf("dsl-delay-%d", line), dur), nil
	case "signal":
		target, err := parseStringArgument(args)
		if err != nil {
			return nil, fmt.Errorf("dsl line %d: signal target %w", line, err)
		}
		preset := target
		return nodes.NewFunctionNode(fmt.Sprintf("dsl-signal-%d", line), func(_ context.Context, shared map[string]any) (goflow.NodeResult, error) {
			return goflow.NodeResult{
				"action": goflow.ActionNext,
				"signal": preset,
			}, nil
		}), nil
	default:
		return nil, fmt.Errorf("dsl line %d: unknown command %q", line, name)
	}
}

func takeToken(line string) (string, string) {
	line = strings.TrimLeftFunc(line, unicode.IsSpace)
	if line == "" {
		return "", ""
	}

	if line[0] == '"' {
		var builder strings.Builder
		escaping := false
		for i := 1; i < len(line); i++ {
			ch := line[i]
			if escaping {
				builder.WriteByte(ch)
				escaping = false
				continue
			}

			if ch == '\\' {
				escaping = true
				continue
			}

			if ch == '"' {
				return builder.String(), line[i+1:]
			}

			builder.WriteByte(ch)
		}
		return builder.String(), ""
	}

	i := 0
	for i < len(line) && !unicode.IsSpace(rune(line[i])) {
		i++
	}

	return line[:i], line[i:]
}

func parseStringArgument(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("expected a value")
	}

	if value[0] == '"' {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", err
		}
		return unquoted, nil
	}

	return value, nil
}

func renderTemplate(template string, shared map[string]any) string {
	if shared == nil || !strings.Contains(template, "{{") {
		return template
	}

	var builder strings.Builder
	for i := 0; i < len(template); {
		if i+1 < len(template) && template[i] == '{' && template[i+1] == '{' {
			if close := strings.Index(template[i+2:], "}}"); close >= 0 {
				key := strings.TrimSpace(template[i+2 : i+2+close])
				if val, ok := shared[key]; ok {
					builder.WriteString(fmt.Sprint(val))
				} else {
					builder.WriteString("{{")
					builder.WriteString(key)
					builder.WriteString("}}")
				}
				i += close + 4
				continue
			}
		}
		builder.WriteByte(template[i])
		i++
	}

	return builder.String()
}
