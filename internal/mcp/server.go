// Package mcp implements an MCP (Model Context Protocol) server for agentbox.
// It uses JSON-RPC 2.0 over stdio to expose agentbox capabilities to any MCP client.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

// JSONRPCRequest represents an incoming JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents an outgoing JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ToolDefinition describes a tool available via MCP.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolCallResult is the MCP result format for tools/call.
type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a single content item in a tool result.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Server is the MCP JSON-RPC server.
type Server struct {
	reader  *bufio.Reader
	writer  io.Writer
	logger  *slog.Logger
	mu      sync.Mutex
	handler *ToolHandler
}

// NewServer creates a new MCP server reading from r and writing to w.
func NewServer(r io.Reader, w io.Writer) *Server {
	return &Server{
		reader:  bufio.NewReader(r),
		writer:  w,
		logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		handler: NewToolHandler(),
	}
}

// Run starts the server message loop, processing requests until EOF or error.
func (s *Server) Run() error {
	s.logger.Info("agentbox MCP server starting")
	for {
		err := s.processOne()
		if err == io.EOF {
			s.logger.Info("client disconnected")
			return nil
		}
		if err != nil {
			s.logger.Error("processing request", "error", err)
			return err
		}
	}
}

// processOne reads and handles a single JSON-RPC message.
func (s *Server) processOne() error {
	line, err := s.reader.ReadBytes('\n')
	if err != nil {
		if err == io.EOF && len(line) == 0 {
			return io.EOF
		}
		if err == io.EOF && len(line) > 0 {
			// Process the last line without newline
		} else {
			return err
		}
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(line, &req); err != nil {
		// If we can't parse the request, send a parse error
		s.sendError(nil, -32700, "Parse error")
		return nil
	}

	// If ID is nil/missing, this is a notification — no response needed.
	if len(req.ID) == 0 || string(req.ID) == "null" {
		s.logger.Debug("received notification", "method", req.Method)
		return nil
	}

	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	case "ping":
		s.sendResult(req.ID, map[string]interface{}{})
	default:
		s.sendError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}

	return nil
}

// handleInitialize responds to the initialize handshake.
func (s *Server) handleInitialize(req JSONRPCRequest) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "agentbox",
			"version": "0.1.0",
		},
	}
	s.sendResult(req.ID, result)
}

// handleToolsList returns all available tool definitions.
func (s *Server) handleToolsList(req JSONRPCRequest) {
	tools := AllTools()
	result := map[string]interface{}{
		"tools": tools,
	}
	s.sendResult(req.ID, result)
}

// handleToolsCall dispatches a tool call to the appropriate handler.
func (s *Server) handleToolsCall(req JSONRPCRequest) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params")
		return
	}

	result := s.handler.Call(params.Name, params.Arguments)
	s.sendResult(req.ID, result)
}

// sendResult sends a successful JSON-RPC response.
func (s *Server) sendResult(id json.RawMessage, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeResponse(resp)
}

// sendError sends a JSON-RPC error response.
func (s *Server) sendError(id json.RawMessage, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	}
	s.writeResponse(resp)
}

func (s *Server) writeResponse(resp JSONRPCResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("failed to marshal response", "error", err)
		return
	}
	data = append(data, '\n')
	if _, err := s.writer.Write(data); err != nil {
		s.logger.Error("failed to write response", "error", err)
	}
}
