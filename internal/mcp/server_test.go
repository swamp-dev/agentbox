package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestParseJSONRPCRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    JSONRPCRequest
		wantErr bool
	}{
		{
			name:  "valid initialize request",
			input: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
			want: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`1`),
				Method:  "initialize",
				Params:  json.RawMessage(`{"protocolVersion":"2024-11-05"}`),
			},
		},
		{
			name:  "valid tools/list request",
			input: `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
			want: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`2`),
				Method:  "tools/list",
			},
		},
		{
			name:  "valid tools/call request with string id",
			input: `{"jsonrpc":"2.0","id":"abc","method":"tools/call","params":{"name":"agentbox_status","arguments":{}}}`,
			want: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`"abc"`),
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name":"agentbox_status","arguments":{}}`),
			},
		},
		{
			name:    "invalid json",
			input:   `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got JSONRPCRequest
			err := json.Unmarshal([]byte(tt.input), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.JSONRPC != tt.want.JSONRPC {
				t.Errorf("JSONRPC = %q, want %q", got.JSONRPC, tt.want.JSONRPC)
			}
			if got.Method != tt.want.Method {
				t.Errorf("Method = %q, want %q", got.Method, tt.want.Method)
			}
			if string(got.ID) != string(tt.want.ID) {
				t.Errorf("ID = %s, want %s", string(got.ID), string(tt.want.ID))
			}
		})
	}
}

func TestJSONRPCResponseSerialization(t *testing.T) {
	t.Run("success response", func(t *testing.T) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`1`),
			Result:  map[string]string{"status": "ok"},
		}
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if !strings.Contains(string(data), `"result"`) {
			t.Errorf("expected result field in %s", string(data))
		}
		if strings.Contains(string(data), `"error"`) {
			t.Errorf("expected no error field in %s", string(data))
		}
	})

	t.Run("error response", func(t *testing.T) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`2`),
			Error:   &JSONRPCError{Code: -32601, Message: "Method not found"},
		}
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if !strings.Contains(string(data), `"error"`) {
			t.Errorf("expected error field in %s", string(data))
		}
	})
}

func TestServerInitialize(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	// Process one message
	err := srv.processOne()
	if err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(&stdout).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", resp.JSONRPC, "2.0")
	}
	if string(resp.ID) != "1" {
		t.Errorf("ID = %s, want 1", string(resp.ID))
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	// Check result contains serverInfo
	resultJSON, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	if !strings.Contains(string(resultJSON), "agentbox") {
		t.Errorf("result should contain server name 'agentbox', got %s", string(resultJSON))
	}
	if !strings.Contains(string(resultJSON), "tools") {
		t.Errorf("result should mention tools capability, got %s", string(resultJSON))
	}
}

func TestServerToolsList(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	if err := srv.processOne(); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(&stdout).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	resultJSON, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	var listResult struct {
		Tools []ToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(resultJSON, &listResult); err != nil {
		t.Fatalf("failed to unmarshal tools list: %v", err)
	}

	expectedTools := []string{
		"agentbox_run",
		"agentbox_ralph_start",
		"agentbox_sprint_start",
		"agentbox_status",
		"agentbox_journal",
		"agentbox_task_list",
		"agentbox_sprint_status",
	}

	if len(listResult.Tools) != len(expectedTools) {
		t.Fatalf("expected %d tools, got %d", len(expectedTools), len(listResult.Tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range listResult.Tools {
		toolNames[tool.Name] = true
		// Every tool must have a description
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		// Every tool must have an input schema
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil input schema", tool.Name)
		}
	}

	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("expected tool %q not found", name)
		}
	}
}

func TestServerToolsCallUnknown(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nonexistent","arguments":{}}}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	if err := srv.processOne(); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(&stdout).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// MCP returns tool errors in the result content, not as JSON-RPC errors
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(resultJSON), "unknown tool") {
		t.Errorf("expected 'unknown tool' in result, got %s", string(resultJSON))
	}
}

func TestServerUnknownMethod(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"unknown/method"}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	if err := srv.processOne(); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(&stdout).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", resp.Error.Code)
	}
}

func TestServerHandlesEOF(t *testing.T) {
	stdin := strings.NewReader("") // empty = EOF
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	err := srv.processOne()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestServerNotification(t *testing.T) {
	// notifications/initialized has no id — server should not respond
	input := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
	stdin := strings.NewReader(input)
	var stdout bytes.Buffer

	srv := NewServer(stdin, &stdout, nil)
	if err := srv.processOne(); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}

	if stdout.Len() != 0 {
		t.Errorf("expected no response for notification, got %s", stdout.String())
	}
}
