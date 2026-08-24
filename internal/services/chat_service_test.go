package services

import (
	"strings"
	"testing"
)

func TestNormalizeMsg(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello, World!", "hello world"},
		{"What is your pricing?", "what is your pricing"},
		{"  How are you?  ", "how are you"},
		{"Fast-delivery_guarantee!", "fastdeliveryguarantee"},
	}

	for _, tt := range tests {
		got := NormalizeMsg(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeMsg(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCompileSystemPersona(t *testing.T) {
	// 1. Plain text persona
	plain := "You are an AI assistant for {{client_name}} at {{domain}}."
	compiled := CompileSystemPersona(plain, "Alvex Inc", "alvex.io")
	if !strings.Contains(compiled, "Alvex Inc") || !strings.Contains(compiled, "alvex.io") {
		t.Errorf("expected placeholders replaced in plain persona, got: %s", compiled)
	}

	// 2. JSON modular persona
	jsonPersona := `{
		"mode": "simple",
		"role": "technical support representative",
		"tone": "Professional & Courteous",
		"length": "Concise",
		"rules": "Never discuss pricing.",
		"fallback": "Transfer to human."
	}`

	compiledJSON := CompileSystemPersona(jsonPersona, "MegaCorp", "megacorp.com")
	if !strings.Contains(compiledJSON, "technical support representative") {
		t.Errorf("expected role in compiled JSON persona, got: %s", compiledJSON)
	}
	if !strings.Contains(compiledJSON, "MegaCorp") {
		t.Errorf("expected client name in compiled JSON persona, got: %s", compiledJSON)
	}
	if !strings.Contains(compiledJSON, "Never discuss pricing.") {
		t.Errorf("expected rules in compiled JSON persona, got: %s", compiledJSON)
	}
}

func TestDetectHandoffTrigger(t *testing.T) {
	tests := []struct {
		msg      string
		expected bool
	}{
		{"I want to talk to human please", true},
		{"Can I speak to agent?", true},
		{"Connect me with a real person", true},
		{"mujhy kisi insan sy baat krni hai", true},
		{"What are your opening hours?", false},
		{"How much does the basic plan cost?", false},
	}

	for _, tt := range tests {
		got, _ := detectHandoffTrigger(tt.msg)
		if got != tt.expected {
			t.Errorf("detectHandoffTrigger(%q) = %v; want %v", tt.msg, got, tt.expected)
		}
	}
}
