package mcp

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"codebase-memory-mcp-go/internal/search"
)

type ServerContext struct {
	DB       *sql.DB
	Embedder *search.LocalEmbedder
	RepoRoot string
	Project  string
	DBPath   string
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

var serverCtx *ServerContext

func StartServer(ctx *ServerContext) {
	serverCtx = ctx
	// Redirect standard log output to stderr to prevent corrupting stdio MCP communication channel
	log.SetOutput(os.Stderr)

	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading line: %v", err)
			}
			break
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(req.ID, -32700, "Parse error", nil)
			continue
		}

		handleRequest(&req)
	}
}

func handleRequest(req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		sendResponse(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]string{
				"name":    "cbm-go-server",
				"version": "1.0.0",
			},
		})
	case "tools/list":
		sendResponse(req.ID, map[string]interface{}{
			"tools": getExposedTools(),
		})
	case "tools/call":
		handleToolCall(req)
	default:
		sendError(req.ID, -32601, "Method not found", nil)
	}
}

func sendResponse(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return
	}
	fmt.Printf("%s\n", bytes)
}

func sendError(id interface{}, code int, message string, data interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: map[string]interface{}{
			"code":    code,
			"message": message,
			"data":    data,
		},
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal error response: %v", err)
		return
	}
	fmt.Printf("%s\n", bytes)
}
