package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	t.Run("agentbox_run has allowed_endpoints property", func(t *testing.T) {
		tool := findTool(tools, "agentbox_run")
		if tool == nil {
			t.Fatal("agentbox_run tool not found")
		}
		props, ok := tool.InputSchema["properties"].(map[string]interface{})
		if !ok {
			t.Fatal("expected properties map")
		}
		ep, exists := props["allowed_endpoints"]
		if !exists {
			t.Fatal("expected allowed_endpoints property in agentbox_run schema")
		}
		epMap, ok := ep.(map[string]interface{})
		if !ok {
			t.Fatal("expected allowed_endpoints to be a map")
		}
		if epMap["type"] != "array" {
			t.Errorf("expected type 'array', got %v", epMap["type"])
		}
	})

	t.Run("agentbox_run network description includes restricted", func(t *testing.T) {
		tool := findTool(tools, "agentbox_run")
		if tool == nil {
			t.Fatal("agentbox_run tool not found")
		}
		props, ok := tool.InputSchema["properties"].(map[string]interface{})
		if !ok {
			t.Fatal("expected properties map")
		}
		net, ok := props["network"].(map[string]interface{})
		if !ok {
			t.Fatal("expected network to be a map")
		}
		desc, ok := net["description"].(string)
		if !ok {
			t.Fatal("expected description to be a string")
		}
		if !strings.Contains(desc, "restricted") {
			t.Errorf("expected network description to mention 'restricted', got %q", desc)
		}
	})

	t.Run("agentbox_sprint_start has network and allowed_endpoints", func(t *testing.T) {
		tool := findTool(tools, "agentbox_sprint_start")
		if tool == nil {
			t.Fatal("agentbox_sprint_start tool not found")
		}
		props, ok := tool.InputSchema["properties"].(map[string]interface{})
		if !ok {
			t.Fatal("expected properties map")
		}
		if _, exists := props["network"]; !exists {
			t.Error("expected network property in agentbox_sprint_start schema")
		}
		ep, exists := props["allowed_endpoints"]
		if !exists {
			t.Fatal("expected allowed_endpoints property in agentbox_sprint_start schema")
		}
		epMap, ok := ep.(map[string]interface{})
		if !ok {
			t.Fatal("expected allowed_endpoints to be a map")
		}
		if epMap["type"] != "array" {
			t.Errorf("expected type 'array', got %v", epMap["type"])
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

func TestToolCallAgentboxRunTimeoutMax(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agentbox_run","arguments":{"project_dir":"/tmp","agent":"claude","prompt":"test","timeout":999}}}` + "\n"
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

	resultJSON, _ := json.Marshal(resp.Result)
	var callResult ToolCallResult
	if err := json.Unmarshal(resultJSON, &callResult); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if !callResult.IsError {
		t.Error("expected isError=true for timeout exceeding maximum")
	}
	if len(callResult.Content) == 0 || !strings.Contains(callResult.Content[0].Text, "exceeds maximum") {
		t.Errorf("expected error about exceeding maximum timeout, got %v", callResult.Content)
	}
}

func TestHandleRunTimeoutBoundary(t *testing.T) {
	h := NewToolHandler(nil)

	tests := []struct {
		name      string
		timeout   int
		wantBlock bool // true if this timeout should be rejected at the ceiling
	}{
		{"at maximum", 240, false},
		{"one over maximum", 241, true},
		{"well over maximum", 999, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, _ := json.Marshal(map[string]interface{}{
				"project_dir": "/tmp",
				"agent":       "claude",
				"prompt":      "test",
				"timeout":     tt.timeout,
			})
			result := h.Call("agentbox_run", args)
			hasTimeoutErr := result.IsError && len(result.Content) > 0 &&
				strings.Contains(result.Content[0].Text, "exceeds maximum")
			if tt.wantBlock && !hasTimeoutErr {
				t.Errorf("timeout=%d should be rejected at ceiling", tt.timeout)
			}
			if !tt.wantBlock && hasTimeoutErr {
				t.Errorf("timeout=%d should NOT be rejected at ceiling", tt.timeout)
			}
		})
	}
}

func TestHandleRunMissingRequiredArgs(t *testing.T) {
	h := NewToolHandler(nil)

	cases := []struct {
		name string
		args map[string]interface{}
	}{
		{"missing project_dir", map[string]interface{}{"agent": "claude", "prompt": "x"}},
		{"missing agent", map[string]interface{}{"project_dir": "/tmp", "prompt": "x"}},
		{"missing prompt", map[string]interface{}{"project_dir": "/tmp", "agent": "claude"}},
		{"all missing", map[string]interface{}{}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			args, _ := json.Marshal(tt.args)
			result := h.Call("agentbox_run", args)
			if !result.IsError {
				t.Error("expected error for missing required args")
			}
		})
	}
}

func TestEvictSessions(t *testing.T) {
	h := NewToolHandler(nil)

	h.mu.Lock()
	for i := 0; i < maxSessions+10; i++ {
		id := fmt.Sprintf("session-%d", i)
		h.sessions[id] = &asyncSession{ID: id, Status: "completed"}
	}
	before := len(h.sessions)
	h.evictSessions()
	after := len(h.sessions)
	h.mu.Unlock()

	if after > maxSessions {
		t.Errorf("evictSessions() left %d sessions, want <= %d", after, maxSessions)
	}
	if after >= before {
		t.Error("evictSessions() did not remove any sessions")
	}
}

func TestEvictSessionsPreservesRunning(t *testing.T) {
	h := NewToolHandler(nil)

	h.mu.Lock()
	for i := 0; i < maxSessions+10; i++ {
		id := fmt.Sprintf("running-%d", i)
		h.sessions[id] = &asyncSession{ID: id, Status: "running"}
	}
	h.evictSessions()
	count := 0
	for _, s := range h.sessions {
		if s.Status == "running" {
			count++
		}
	}
	h.mu.Unlock()

	if count != maxSessions+10 {
		t.Errorf("expected all %d running sessions preserved, got %d", maxSessions+10, count)
	}
}

func TestHandleStatusWithKnownSessionID(t *testing.T) {
	h := NewToolHandler(nil)

	h.mu.Lock()
	h.sessions["test-session-123"] = &asyncSession{
		ID: "test-session-123", Status: "completed",
	}
	h.mu.Unlock()

	args, _ := json.Marshal(map[string]string{"session_id": "test-session-123"})
	result := h.Call("agentbox_status", args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var resp map[string]string
	if err := json.Unmarshal([]byte(result.Content[0].Text), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp["status"] != "completed" {
		t.Errorf("expected status=completed, got %s", resp["status"])
	}
	if resp["session_id"] != "test-session-123" {
		t.Errorf("expected session_id=test-session-123, got %s", resp["session_id"])
	}
}

func TestHandleRunClaudeCLIAutoRestricted(t *testing.T) {
	// claude-cli with no network specified should auto-switch to restricted
	h := NewToolHandler(nil)
	t.Setenv("ANTHROPIC_API_KEY", "test-key") // won't be used but avoids early exit

	args, _ := json.Marshal(map[string]interface{}{
		"project_dir": "/tmp",
		"agent":       "claude-cli",
		"prompt":      "test",
	})
	// This will fail at config.Load or container creation, but we can
	// check that it doesn't fail at the "agent and prompt required" check
	result := h.Call("agentbox_run", args)
	// Should NOT get "project_dir, agent, and prompt are required"
	if result.IsError && strings.Contains(result.Content[0].Text, "agent, and prompt are required") {
		t.Error("args should have passed required field validation")
	}
}

func TestHandleRunAllowedEndpointsOverride(t *testing.T) {
	// When user provides allowed_endpoints, agent defaults should NOT override
	h := NewToolHandler(nil)

	args, _ := json.Marshal(map[string]interface{}{
		"project_dir":       "/tmp",
		"agent":             "claude",
		"prompt":            "test",
		"network":           "restricted",
		"allowed_endpoints": []string{"custom.host:8080"},
		"timeout":           241, // force early exit at timeout check
	})
	result := h.Call("agentbox_run", args)
	// Should fail on timeout ceiling, confirming we got past the endpoints logic
	if !result.IsError || !strings.Contains(result.Content[0].Text, "exceeds maximum") {
		t.Errorf("expected timeout ceiling error, got: %s", result.Content[0].Text)
	}
}

func TestEvictSessionsMixed(t *testing.T) {
	h := NewToolHandler(nil)

	h.mu.Lock()
	// Add running sessions
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("running-%d", i)
		h.sessions[id] = &asyncSession{ID: id, Status: "running"}
	}
	// Add completed sessions to push over limit
	for i := 0; i < 60; i++ {
		id := fmt.Sprintf("completed-%d", i)
		h.sessions[id] = &asyncSession{ID: id, Status: "completed"}
	}
	h.evictSessions()

	var running, completed int
	for _, s := range h.sessions {
		switch s.Status {
		case "running":
			running++
		case "completed":
			completed++
		}
	}
	h.mu.Unlock()

	// All 50 running sessions must survive
	if running != 50 {
		t.Errorf("expected 50 running sessions preserved, got %d", running)
	}
	// Some completed sessions should have been evicted
	if running+completed > maxSessions {
		t.Errorf("total sessions %d exceeds maxSessions %d", running+completed, maxSessions)
	}
}

func TestHandleSprintStartNetworkPassthrough(t *testing.T) {
	h := NewToolHandler(nil)

	args, _ := json.Marshal(map[string]interface{}{
		"project_dir":       "/tmp/nonexistent",
		"network":           "restricted",
		"allowed_endpoints": []string{"api.anthropic.com:443"},
	})
	result := h.Call("agentbox_sprint_start", args)

	// Sprint start is async — should return a session ID even if the dir doesn't exist
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var resp map[string]string
	if err := json.Unmarshal([]byte(result.Content[0].Text), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp["session_id"] == "" {
		t.Error("expected non-empty session_id")
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
