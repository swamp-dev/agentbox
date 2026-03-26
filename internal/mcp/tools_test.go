package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestToolDefinitions(t *testing.T) {
	tools := AllTools()

	t.Run("all tools have required fields", func(t *testing.T) {
		for _, tool := range tools {
			if tool.Name == "" {
				t.Error("tool has empty name")
			}
			if tool.Description == "" {
				t.Errorf("tool %q has empty description", tool.Name)
			}
			if tool.InputSchema == nil {
				t.Errorf("tool %q has nil input schema", tool.Name)
			}
			// InputSchema must have type=object
			schemaJSON, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Errorf("tool %q: failed to marshal schema: %v", tool.Name, err)
				continue
			}
			if !strings.Contains(string(schemaJSON), `"type":"object"`) {
				t.Errorf("tool %q: input schema type should be 'object', got %s", tool.Name, string(schemaJSON))
			}
		}
	})

	t.Run("agentbox_run has required properties", func(t *testing.T) {
		tool := findTool(tools, "agentbox_run")
		if tool == nil {
			t.Fatal("agentbox_run tool not found")
		}
		schema := tool.InputSchema
		props, ok := schema["properties"].(map[string]interface{})
		if !ok {
			t.Fatal("expected properties map")
		}
		for _, required := range []string{"project_dir", "agent", "prompt"} {
			if _, exists := props[required]; !exists {
				t.Errorf("expected property %q in agentbox_run schema", required)
			}
		}
		// Check required list
		requiredList, ok := schema["required"].([]interface{})
		if !ok {
			// try []string
			requiredStrList, ok := schema["required"].([]string)
			if !ok {
				t.Fatal("expected required list")
			}
			checkRequired(t, requiredStrList, []string{"project_dir", "agent", "prompt"})
		} else {
			strs := make([]string, len(requiredList))
			for i, v := range requiredList {
				strs[i] = v.(string)
			}
			checkRequired(t, strs, []string{"project_dir", "agent", "prompt"})
		}
	})

	t.Run("agentbox_ralph_start has project_dir required", func(t *testing.T) {
		tool := findTool(tools, "agentbox_ralph_start")
		if tool == nil {
			t.Fatal("agentbox_ralph_start tool not found")
		}
		schema := tool.InputSchema
		props, ok := schema["properties"].(map[string]interface{})
		if !ok {
			t.Fatal("expected properties map")
		}
		if _, exists := props["project_dir"]; !exists {
			t.Error("expected property 'project_dir' in agentbox_ralph_start schema")
		}
	})

	t.Run("agentbox_journal has session_id required", func(t *testing.T) {
		tool := findTool(tools, "agentbox_journal")
		if tool == nil {
			t.Fatal("agentbox_journal tool not found")
		}
		schema := tool.InputSchema
		props, ok := schema["properties"].(map[string]interface{})
		if !ok {
			t.Fatal("expected properties map")
		}
		if _, exists := props["session_id"]; !exists {
			t.Error("expected property 'session_id'")
		}
	})
}

func TestToolCallAgentboxStatus(t *testing.T) {
	// Call agentbox_status with no args — should return graceful message
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agentbox_status","arguments":{}}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	if err := srv.processOne(); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(&stdout).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	// Result should be a tool call result with content
	resultJSON, _ := json.Marshal(resp.Result)
	var callResult ToolCallResult
	if err := json.Unmarshal(resultJSON, &callResult); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(callResult.Content) == 0 {
		t.Fatal("expected at least one content block")
	}
	if callResult.Content[0].Type != "text" {
		t.Errorf("expected text content type, got %q", callResult.Content[0].Type)
	}
}

func TestToolCallAgentboxTaskList(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agentbox_task_list","arguments":{}}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	if err := srv.processOne(); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(&stdout).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestToolCallAgentboxJournal(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agentbox_journal","arguments":{"session_id":"1"}}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	if err := srv.processOne(); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(&stdout).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestToolCallAgentboxRun(t *testing.T) {
	// agentbox_run without Docker will fail gracefully
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agentbox_run","arguments":{"project_dir":"/tmp","agent":"claude","prompt":"test"}}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	if err := srv.processOne(); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(&stdout).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}

	// Should return a result (possibly with isError=true since no Docker)
	resultJSON, _ := json.Marshal(resp.Result)
	var callResult ToolCallResult
	if err := json.Unmarshal(resultJSON, &callResult); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(callResult.Content) == 0 {
		t.Fatal("expected at least one content block")
	}
}

func TestToolCallAgentboxRalphStart(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agentbox_ralph_start","arguments":{"project_dir":"/tmp/nonexistent"}}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	if err := srv.processOne(); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(&stdout).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolCallAgentboxSprintStart(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agentbox_sprint_start","arguments":{"project_dir":"/tmp/nonexistent"}}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	if err := srv.processOne(); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(&stdout).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolCallAgentboxSprintStatus(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agentbox_sprint_status","arguments":{"session_id":"nonexistent"}}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	if err := srv.processOne(); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(&stdout).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

// --- helpers ---

func findTool(tools []ToolDefinition, name string) *ToolDefinition {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}

func checkRequired(t *testing.T, got, want []string) {
	t.Helper()
	m := make(map[string]bool)
	for _, s := range got {
		m[s] = true
	}
	for _, s := range want {
		if !m[s] {
			t.Errorf("expected %q in required list", s)
		}
	}
}
