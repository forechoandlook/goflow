package dsl

import (
	"context"
	"testing"
)

func TestBuildFlowFromScript(t *testing.T) {
	script := `
		# DSL script that drives a simple flow
		set greeting Hello
		set message "Message is {{greeting}}"
		log "{{message}} has been composed"
		delay 1ms
		signal done
	`

	flow, err := BuildFlowFromScript(script)
	if err != nil {
		t.Fatalf("unexpected error building flow: %v", err)
	}

	if flow == nil {
		t.Fatal("expected flow, got nil")
	}

	shared := make(map[string]any)
	if err := flow.Run(context.Background(), shared); err != nil {
		t.Fatalf("flow run failed: %v", err)
	}

	if got, ok := shared["message"].(string); !ok || got != "Message is Hello" {
		t.Fatalf("render template failed, got %q", shared["message"])
	}
}

func TestBuildFlowFromScriptEmpty(t *testing.T) {
	if _, err := BuildFlowFromScript("   \n# nothing here\n"); err == nil {
		t.Fatal("expected error when script has no commands")
	}
}
