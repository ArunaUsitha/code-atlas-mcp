# Complete Production Specification: Next-Gen Codebase Memory MCP Server (Go-based)

This document provides the complete, production-ready specification required to build and deploy a codebase indexing and context-retrieval MCP (Model Context Protocol) server from scratch using Go. It contains all architectural details, schemas, code templates, and design guides for the core engine, advanced features, and the 3D Graph Visualization UI.

---

## 1. Directory Structure

```text
codebase-memory-mcp-go/
├── cmd/
│   └── cbm-server/
│       └── main.go          # Entry point, CLI args, MCP stdio loop initialization
├── internal/
│   ├── config/
│   │   └── config.go        # Config loading (.cbm-config, environment variables)
│   ├── db/
│   │   ├── sqlite.go        # SQLite connection, schema migration, transaction wrapper
│   │   └── ram_pipeline.go  # RAM-first compiler (in-memory compiler -> disk flusher)
│   ├── parser/
│   │   ├── parser.go        # Tree-sitter engine initialization and orchestration
│   │   ├── languages.go     # Language-specific query definitions (Python, TS, Go)
│   │   ├── routing.go       # HTTP/gRPC route & client extraction parser
│   │   └── iac.go           # Infrastructure-as-code parser (Dockerfile, K8s, YAML)
│   ├── resolver/
│   │   └── lsp.go           # Symbol mapping, import resolution, cross-file references
│   ├── search/
│   │   ├── search.go        # BM25 full-text queries
│   │   ├── cypher.go        # Lightweight Cypher-like AST graph query engine
│   │   └── embedder.go      # ONNX Runtime adapter for local embeddings
│   ├── watcher/
│   │   └── watch.go         # fsnotify wrapper for incremental directory indexing
│   ├── snapshot/
│   │   └── zstd.go          # Zstd zlib compaction & import/export snapshot runner
│   ├── ui/
│   │   └── server.go        # Go HTTP Server, API endpoints, embedded assets handler
│   └── mcp/
│       ├── protocol.go      # JSON-RPC protocol parser and writer
│       ├── handlers.go      # MCP Tool controllers (search, trace, architecture)
│       └── schemas.go       # JSON schemas for MCP tool definitions
├── graph-ui/                # React visualizer source code
│   ├── src/
│   │   ├── App.jsx          # Main 3D Graph container component
│   │   └── main.jsx
│   ├── index.html
│   ├── package.json
│   └── vite.config.js
├── go.mod
├── go.sum
└── README.md
```

---

## 2. Core Database Schema (SQLite)

Execute the following SQL commands to initialize the schema when the application boots for the first time. The DB file is persisted to `~/.cache/cbm-go/graph.db`.

```sql
PRAGMA foreign_keys = OFF;

-- Nodes: Represents files, folders, classes, functions, variables, HTTP/gRPC endpoints, and IaC resources.
CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,               -- Unique UUID or SHA-256 of qualified name
    project TEXT NOT NULL,             -- Project workspace identifier
    name TEXT NOT NULL,                -- Base name (e.g. "ProcessPayment" or "/payment/checkout")
    qualified_name TEXT NOT NULL,      -- Full path signature (e.g. "billing.payment.ProcessPayment")
    kind TEXT NOT NULL,                -- 'file', 'directory', 'class', 'function', 'http_route', 'grpc_method', 'iac_resource'
    file_path TEXT NOT NULL,           -- Relative file path from project root
    start_line INTEGER NOT NULL,       -- 1-based start line
    end_line INTEGER NOT NULL,         -- 1-based end line
    signature TEXT,                    -- Parameter/return type signature, or HTTP method/route path
    content_hash TEXT NOT NULL         -- SHA-256 of symbol content (for incremental diffs)
);

CREATE INDEX IF NOT EXISTS idx_nodes_kind ON nodes(kind);
CREATE INDEX IF NOT EXISTS idx_nodes_project ON nodes(project);
CREATE INDEX IF NOT EXISTS idx_nodes_file ON nodes(file_path);

-- Edges: Represents directed relationships between nodes
CREATE TABLE IF NOT EXISTS edges (
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    type TEXT NOT NULL,                -- 'DEFINES', 'CALLS', 'IMPORTS', 'INHERITS', 'HTTP_CALLS', 'DATA_FLOW', 'EMITS', 'LISTENS_ON', 'DEPENDS_ON'
    project TEXT NOT NULL,
    PRIMARY KEY (source_id, target_id, type),
    FOREIGN KEY (source_id) REFERENCES nodes(id) ON DELETE CASCADE,
    FOREIGN KEY (target_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);

-- FTS5 Full-Text Search Virtual Table for fast symbol searching
CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
    node_id UNINDEXED,
    name,
    qualified_name,
    signature,
    content,
    tokenize="unicode61"
);

-- Vectors table for semantic similarity searches
CREATE TABLE IF NOT EXISTS node_vectors (
    node_id TEXT PRIMARY KEY,
    vector BLOB NOT NULL,              -- Float32 array representing embeddings
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- ADRs Table: Architecture Decision Records persistence
CREATE TABLE IF NOT EXISTS adrs (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    status TEXT NOT NULL,              -- 'proposed', 'accepted', 'rejected', 'superseded'
    date TEXT NOT NULL,
    decisions TEXT NOT NULL,           -- Markdown decisions text
    context TEXT NOT NULL              -- Context & consequences
);

PRAGMA foreign_keys = ON;
```

---

## 3. The 14 MCP Tools Schema Specifications

Below is the complete set of tool definitions (name, description, input schema) that the server must announce in the `tools/list` response.

```json
[
  {
    "name": "get_architecture",
    "description": "Returns high-level structural overview of codebase packages, entry points, REST/gRPC routes, directories, and architectural hotspots.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "project": { "type": "string", "description": "Scope search to a specific project name" }
      }
    }
  },
  {
    "name": "search_graph",
    "description": "Executes structural lookup on codebase nodes using regex patterns, kinds, and line limits.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "name_pattern": { "type": "string", "description": "Regex matching symbol or file name" },
        "kind": { "type": "string", "enum": ["file", "directory", "class", "function", "http_route", "iac_resource"], "description": "Filter by node classification" },
        "project": { "type": "string" }
      },
      "required": ["name_pattern"]
    }
  },
  {
    "name": "search_code",
    "description": "Performs grep-style search over indexed file contents with AST parsing scoping.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "pattern": { "type": "string", "description": "Regex expression to find inside file bodies" },
        "file_pattern": { "type": "string", "description": "Regex pattern scoping paths to search" },
        "project": { "type": "string" }
      },
      "required": ["pattern"]
    }
  },
  {
    "name": "semantic_query",
    "description": "Queries vector embeddings of codebase elements to retrieve context using conceptual meaning rather than string matching.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "query": { "type": "string", "description": "Natural language query (e.g. 'how are billing signatures generated')" },
        "limit": { "type": "integer", "default": 5 },
        "project": { "type": "string" }
      },
      "required": ["query"]
    }
  },
  {
    "name": "trace_calls",
    "description": "Computes BFS path trees tracing calls leading into or out of a targeted function symbol.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "symbol_name": { "type": "string", "description": "Qualified name of starting function/method" },
        "direction": { "type": "string", "enum": ["inbound", "outbound", "both"], "default": "inbound" },
        "depth": { "type": "integer", "default": 3 },
        "project": { "type": "string" }
      },
      "required": ["symbol_name"]
    }
  },
  {
    "name": "detect_changes",
    "description": "Queries modified files relative to standard Git HEAD, mapping modifications to affected classes/routes and calculating risk profiles.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "base_branch": { "type": "string", "default": "main", "description": "Compare changes against this base branch" },
        "project": { "type": "string" }
      }
    }
  },
  {
    "name": "find_dead_code",
    "description": "Detects function nodes that have zero inbound 'CALLS' or 'HTTP_CALLS' relationships, excluding known entry points.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "project": { "type": "string" }
      }
    }
  },
  {
    "name": "query_cypher",
    "description": "Executes structural Graph queries using a lightweight Cypher-like language (e.g. MATCH (f:Function)-[:CALLS]->(g) WHERE f.name='main' RETURN g).",
    "inputSchema": {
      "type": "object",
      "properties": {
        "query": { "type": "string", "description": "Cypher query block" }
      },
      "required": ["query"]
    }
  },
  {
    "name": "manage_adr",
    "description": "Reads, creates, or updates Architecture Decision Records (ADRs) locally.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "action": { "type": "string", "enum": ["list", "get", "create", "update"], "description": "Operation type" },
        "id": { "type": "string", "description": "ADR ID identifier (e.g. 'ADR-001')" },
        "title": { "type": "string" },
        "status": { "type": "string", "enum": ["proposed", "accepted", "rejected", "superseded"] },
        "decisions": { "type": "string", "description": "Decisions content markdown" },
        "context": { "type": "string" }
      },
      "required": ["action"]
    }
  },
  {
    "name": "index_repository",
    "description": "Explicitly triggers full repository re-scanning, writing indexes to local DB and exporting Zstd snapshot.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "repo_path": { "type": "string", "description": "Absolute path to repository root" },
        "project_name": { "type": "string" }
      },
      "required": ["repo_path"]
    }
  },
  {
    "name": "detect_cross_links",
    "description": "Identifies cross-service connections by matching HTTP route endpoints in one microservice with HTTP call sites in another.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "project": { "type": "string" }
      }
    }
  },
  {
    "name": "get_file_symbols",
    "description": "Retrieves flat list of AST nodes parsed inside a specific file path.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "file_path": { "type": "string", "description": "Relative path to target file" },
        "project": { "type": "string" }
      },
      "required": ["file_path"]
    }
  },
  {
    "name": "get_impact_analysis",
    "description": "Traces what downstream nodes (classes, APIs, files) will be affected if a particular class/method interface is modified.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "symbol_name": { "type": "string", "description": "Qualified name of node being modified" },
        "project": { "type": "string" }
      },
      "required": ["symbol_name"]
    }
  },
  {
    "name": "clear_project_index",
    "description": "Deletes the index database contents and snapshots for a specified project.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "project": { "type": "string" }
      },
      "required": ["project"]
    }
  }
]
```

---

## 4. AST Parser Engine (Tree-sitter)

Use `github.com/smacker/go-tree-sitter` for parsing. This template shows how to load parsers and query the AST.

```go
package parser

import (
	"context"
	"fmt"
	"os"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
)

type Symbol struct {
	Name      string
	Kind      string
	StartLine int
	EndLine   int
	Signature string
}

// ParseFile extracts symbols from a file using tree-sitter AST queries
func ParseFile(filePath string, langType string) ([]Symbol, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var lang *sitter.Language
	var queryStr string

	switch langType {
	case "python":
		lang = python.GetLanguage()
		queryStr = `
			(function_definition
				name: (identifier) @func.name
				parameters: (parameters) @func.params) @func.def
			(class_definition
				name: (identifier) @class.name) @class.def
		`
	case "go":
		lang = golang.GetLanguage()
		queryStr = `
			(function_declaration
				name: (identifier) @func.name
				parameter_list: (parameter_list) @func.params) @func.def
			(method_declaration
				name: (field_identifier) @func.name
				parameter_list: (parameter_list) @func.params) @func.def
			(type_spec
				name: (type_identifier) @class.name
				type: (struct_type)) @class.def
		`
	default:
		return nil, fmt.Errorf("unsupported language: %s", langType)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}

	rootNode := tree.RootNode()
	query, err := sitter.NewQuery([]byte(queryStr), lang)
	if err != nil {
		return nil, err
	}

	cursor := sitter.NewQueryCursor()
	cursor.Exec(query, rootNode)

	var symbols []Symbol
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}

		for _, capture := range m.Captures {
			captureName := query.CaptureNameForId(capture.Index)
			node := capture.Node

			if captureName == "func.def" {
				symbols = append(symbols, Symbol{
					Name:      node.ChildByFieldName("name").Content(content),
					Kind:      "function",
					StartLine: int(node.StartPoint().Row) + 1,
					EndLine:   int(node.EndPoint().Row) + 1,
				})
			} else if captureName == "class.def" {
				symbols = append(symbols, Symbol{
					Name:      node.ChildByFieldName("name").Content(content),
					Kind:      "class",
					StartLine: int(node.StartPoint().Row) + 1,
					EndLine:   int(node.EndPoint().Row) + 1,
				})
			}
		}
	}

	return symbols, nil
}
```

---

## 5. RAM-First SQLite Compiler Pipeline

To achieve sub-second indexing on massive repos, follow a **RAM-first** compilation design:
1. Initialize a temporary SQLite database in-memory: `sqlite3_open(":memory:")`.
2. Disable synchronous writes, checkpoints, and journaling to maximize I/O speed.
3. Write all AST parser threads outputs concurrently into the in-memory database using a transactional connection.
4. Execute `VACUUM INTO '<disk_destination_path>'` at the very end to serialize the processed structure to disk in one single-pass write.

```go
package db

import (
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
)

type RAMCompiler struct {
	memDB *sql.DB
}

func NewRAMCompiler() (*RAMCompiler, error) {
	// Connect to in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}

	// Apply highly-performant parsing configurations
	pragmas := []string{
		"PRAGMA journal_mode = OFF;",
		"PRAGMA synchronous = OFF;",
		"PRAGMA locking_mode = EXCLUSIVE;",
		"PRAGMA cache_size = -2000000;", // ~2GB memory cache
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return nil, err
		}
	}

	return &RAMCompiler{memDB: db}, nil
}

// FlushToDisk serializes the memory tables directly to target file
func (rc *RAMCompiler) FlushToDisk(diskPath string) error {
	// SQLite supports standard VACUUM INTO to clone database structures
	_, err := rc.memDB.Exec(fmt.Sprintf("VACUUM INTO '%s';", diskPath))
	return err
}
```

---

## 6. HTTP & gRPC Cross-Service Linker

Extract API routes and client call sites dynamically from source code, and match endpoints across services.

```go
package parser

import (
	"regexp"
)

type APIRoute struct {
	Method string // 'GET', 'POST'
	Path   string // '/api/payment/checkout'
	File   string
	Line   int
}

type APICallSite struct {
	URLPattern string // 'https://checkout-service/api/payment/checkout' or '/api/payment/checkout'
	File       string
	Line       int
}

// ExtractRoutes parses files for REST routes using AST or regex pattern fallbacks
func ExtractRoutes(fileContent string, file string) []APIRoute {
	var routes []APIRoute
	// Match Python FastAPI route decorators e.g. @app.get("/items")
	re := regexp.MustCompile(`@(?:app|router)\.(get|post|put|delete)\(['"]([^'"]+)['"]`)
	matches := re.FindAllStringSubmatch(fileContent, -1)

	for _, match := range matches {
		routes = append(routes, APIRoute{
			Method: match[1],
			Path:   match[2],
			File:   file,
		})
	}
	return routes
}

// MatchEndpoints compares API callsites in caller services to API routes in callee services
func MatchEndpoints(routes []APIRoute, calls []APICallSite) []struct{ Route APIRoute; Call APICallSite } {
	var matches []struct {
		Route APIRoute
		Call  APICallSite
	}

	// Compile match validation: route '/api/payment/checkout' matches call url '/api/payment/checkout'
	for _, route := range routes {
		for _, call := range calls {
			// Strip protocol + domain for matching cross-service calls
			cleanCallURL := cleanURL(call.URLPattern)
			if cleanCallURL == route.Path {
				matches = append(matches, struct {
					Route APIRoute
					Call  APICallSite
				}{Route: route, Call: call})
			}
		}
	}
	return matches
}

func cleanURL(rawURL string) string {
	// Remove 'https://host:port' using regex
	re := regexp.MustCompile(`^(?:https?://[^/]+)?(.*)$`)
	res := re.FindStringSubmatch(rawURL)
	if len(res) > 1 {
		return res[1]
	}
	return rawURL
}
```

---

## 7. Infrastructure-as-Code (IaC) Indexing

Scan configuration files to resolve environmental configuration variables and map dependencies between services.

```go
package parser

import (
	"bufio"
	"strings"
)

type IaCResource struct {
	Type      string // 'docker_image', 'k8s_service'
	Name      string
	DependsOn []string
}

// ParseDockerfile parses ENV and ARG parameters in Dockerfile config
func ParseDockerfile(content string) []string {
	var envKeys []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "ENV ") || strings.HasPrefix(line, "ARG ") {
			parts := strings.Fields(line[4:])
			if len(parts) > 0 {
				eqIdx := strings.Index(parts[0], "=")
				if eqIdx != -1 {
					envKeys = append(envKeys, parts[0][:eqIdx])
				} else {
					envKeys = append(envKeys, parts[0])
				}
			}
		}
	}
	return envKeys
}
```

---

## 8. Cypher-like Query Parser

A simple parser that matches simple AST patterns like `MATCH (f:Function)-[:CALLS]->(g) WHERE f.name = 'main' RETURN g.name` and translates them into SQLite recursive CTE commands.

```go
package search

import (
	"database/sql"
	"regexp"
)

type CypherEngine struct {
	db *sql.DB
}

func NewCypherEngine(db *sql.DB) *CypherEngine {
	return &CypherEngine{db: db}
}

// ExecuteQuery converts basic cypher constructs to SQL queries
func (ce *CypherEngine) ExecuteQuery(cypher string) (*sql.Rows, error) {
	// Parse e.g. MATCH (f:Function)-[:CALLS]->(g) WHERE f.name = 'X' RETURN g.name
	re := regexp.MustCompile(`MATCH\s+\(.*?:(Function|Class)\)-\[:(CALLS|IMPORTS)\]->\((.*?)\)\s+WHERE\s+.*?\s*=\s*'([^']+)'\s+RETURN\s+(.*)`)
	matches := re.FindStringSubmatch(cypher)

	if len(matches) == 0 {
		// Fallback to standard SQL selection on nodes
		return ce.db.Query("SELECT name, qualified_name, kind FROM nodes LIMIT 50;")
	}

	sourceKind := matches[1]
	edgeType := matches[2]
	filterVal := matches[4]

	sqlQuery := `
		SELECT n2.name, n2.qualified_name
		FROM nodes n1
		JOIN edges e ON n1.id = e.source_id
		JOIN nodes n2 ON e.target_id = n2.id
		WHERE n1.kind = ? AND e.type = ? AND n1.name = ?
	`
	return ce.db.Query(sqlQuery, sourceKind, edgeType, filterVal)
}
```

---

## 9. Zstd Compactor (Team-Shared Snapshots)

Implement the `.codebase-memory/graph.db.zst` snapshot compression using `github.com/klauspost/compress/zstd`.

```go
package snapshot

import (
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
)

// CompressDatabase creates a compacted zstd archive of the sqlite database
func CompressDatabase(sqlitePath string, zstdDestPath string) error {
	inputFile, err := os.Open(sqlitePath)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	outputFile, err := os.Create(zstdDestPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	writer, err := zstd.NewWriter(outputFile, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return err
	}
	defer writer.Close()

	_, err = io.Copy(writer, inputFile)
	return err
}

// DecompressDatabase extracts the zstd archive to sqlite destination
func DecompressDatabase(zstdPath string, sqliteDestPath string) error {
	inputFile, err := os.Open(zstdPath)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	reader, err := zstd.NewReader(inputFile)
	if err != nil {
		return err
	}
	defer reader.Close()

	outputFile, err := os.Create(sqliteDestPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, reader)
	return err
}
```

---

## 10. Search & Local ONNX Embedder

For local vector search, use ONNX Runtime bindings to execute model inferences locally without third-party APIs.

```go
package search

import (
	"fmt"
	"math"
	"unsafe"

	ort "github.com/yalue/onnxruntime_go"
)

type LocalEmbedder struct {
	session *ort.AdvancedSession
}

func NewLocalEmbedder(modelPath string) (*LocalEmbedder, error) {
	// Initialize ONNX shared library
	ort.SetSharedLibraryPath("libonnxruntime.so") // Adjust for Windows/macOS (.dll / .dylib)
	err := ort.InitializeEnvironment()
	if err != nil {
		return nil, err
	}

	// Load model
	session, err := ort.NewAdvancedSession(modelPath,
		[]string{"input_ids", "attention_mask"},
		[]string{"output"},
		nil)
	if err != nil {
		return nil, err
	}

	return &LocalEmbedder{session: session}, nil
}

// GenerateEmbeddings runs local inference to convert text into float32 array
func (le *LocalEmbedder) GenerateEmbeddings(tokens []int64, attentionMask []int64) ([]float32, error) {
	inputShape := ort.NewShape(1, int64(len(tokens)))

	inputTensor, err := ort.NewTensor(inputShape, tokens)
	if err != nil {
		return nil, err
	}
	defer inputTensor.Destroy()

	maskTensor, err := ort.NewTensor(inputShape, attentionMask)
	if err != nil {
		return nil, err
	}
	defer maskTensor.Destroy()

	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 768)) // Output dimension matches Nomic embed
	if err != nil {
		return nil, err
	}
	defer outputTensor.Destroy()

	err = le.session.Run(
		[]ort.ArbitraryTensor{inputTensor, maskTensor},
		[]ort.ArbitraryTensor{outputTensor},
	)
	if err != nil {
		return nil, err
	}

	return outputTensor.GetData(), nil
}

// CosineSimilarity calculates the similarity metric between two float32 slices
func CosineSimilarity(v1, v2 []float32) float64 {
	var dotProduct, normA, normB float64
	for i := 0; i < len(v1); i++ {
		dotProduct += float64(v1[i] * v2[i])
		normA += float64(v1[i] * v1[i])
		normB += float64(v2[i] * v2[i])
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
```

---

## 11. 3D Graph Visualization UI Architecture

To build the optional 3D graph visualizer, set up a React web application inside the binary. The UI is built using React + Vite, packaged to static files, embedded into Go using `go:embed`, and served locally at `localhost:9749`.

### A. Go HTTP API Server (`internal/ui/server.go`)
This serves the visualizer assets and exposes JSON APIs to retrieve the database graph representation.

```go
package ui

import (
	"database/sql"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
)

//go:embed dist/*
var frontendAssets embed.FS

type UIServer struct {
	db *sql.DB
}

func StartUIServer(port string, db *sql.DB) error {
	srv := &UIServer{db: db}

	mux := http.NewServeMux()

	// 1. API Route: Return nodes and edges for rendering
	mux.HandleFunc("/api/graph", srv.handleGetGraph)

	// 2. Static Assets Route: Serve embedded React distribution build
	distFS, err := fs.Sub(frontendAssets, "dist")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(distFS)))

	// Bind ONLY loopback for secure local execution
	return http.ListenAndServe("127.0.0.1:"+port, mux)
}

type GraphJSON struct {
	Nodes []NodeJSON `json:"nodes"`
	Links []LinkJSON `json:"links"`
}

type NodeJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Group string `json:"group"`
}

type LinkJSON struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

func (srv *UIServer) handleGetGraph(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:9749")

	var graph GraphJSON

	// Fetch Nodes
	rows, err := srv.db.Query("SELECT id, name, kind FROM nodes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var n NodeJSON
		if err := rows.Scan(&n.ID, &n.Name, &n.Kind); err == nil {
			n.Group = n.Kind // Color group by kind
			graph.Nodes = append(graph.Nodes, n)
		}
	}

	// Fetch Links/Edges
	edgeRows, err := srv.db.Query("SELECT source_id, target_id, type FROM edges")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer edgeRows.Close()

	for edgeRows.Next() {
		var l LinkJSON
		if err := edgeRows.Scan(&l.Source, &l.Target, &l.Type); err == nil {
			graph.Links = append(graph.Links, l)
		}
	}

	json.NewEncoder(w).Encode(graph)
}
```

### B. React 3D Graph Component (`graph-ui/src/App.jsx`)
The frontend uses `react-force-graph-3d` (wrapping three.js) to display nodes as spheres and edges as links in a 3D canvas with orbital controls.

```jsx
import React, { useEffect, useState } from 'react';
import ForceGraph3D from 'react-force-graph-3d';

export default function App() {
  const [graphData, setGraphData] = useState({ nodes: [], links: [] });
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    // Fetch nodes and edges from Go embedded endpoint
    fetch('/api/graph')
      .then((res) => res.json())
      .then((data) => {
        setGraphData(data);
        setLoading(false);
      })
      .catch((err) => console.error("Failed to load graph data", err));
  }, []);

  // Node coloration based on kind/group
  const getNodeColor = (node) => {
    switch (node.group) {
      case 'file': return '#4299e1';       // Blue
      case 'class': return '#ecc94b';      // Yellow
      case 'function': return '#48bb78';   // Green
      case 'http_route': return '#f56565'; // Red
      default: return '#a0aec0';           // Grey
    }
  };

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-900 text-white font-sans">
        <div className="text-xl animate-pulse">Loading Codebase Architecture Graph...</div>
      </div>
    );
  }

  return (
    <div className="relative h-screen w-screen bg-slate-950 overflow-hidden">
      {/* 3D Force Graph Render Canvas */}
      <ForceGraph3D
        graphData={graphData}
        nodeLabel={(node) => `${node.name} (${node.kind})`}
        nodeColor={getNodeColor}
        nodeVal={(node) => (node.kind === 'file' ? 6 : 3)} // Files rendered larger
        linkDirectionalArrowLength={3.5}
        linkDirectionalArrowRelPos={1}
        linkColor={() => 'rgba(255,255,255,0.15)'}
        linkWidth={0.7}
      />

      {/* Floating Control Legend */}
      <div className="absolute top-4 left-4 p-4 rounded-xl bg-slate-900/80 backdrop-blur-md border border-slate-800 text-white font-sans text-xs flex flex-col gap-2">
        <h3 className="font-bold text-sm mb-1 text-sky-400">Legend</h3>
        <div className="flex items-center gap-2">
          <span className="h-3 w-3 rounded-full bg-blue-500" /> Files
        </div>
        <div className="flex items-center gap-2">
          <span className="h-3 w-3 rounded-full bg-yellow-500" /> Classes
        </div>
        <div className="flex items-center gap-2">
          <span className="h-3 w-3 rounded-full bg-green-500" /> Functions
        </div>
        <div className="flex items-center gap-2">
          <span className="h-3 w-3 rounded-full bg-red-500" /> HTTP/gRPC Routes
        </div>
      </div>
    </div>
  );
}
```

---

## 12. MCP Protocol Implementation & Tool Dispatcher

The core server loops standard inputs translating JSON-RPC requests, resolving SQL parameters, and formatting output payloads.

```go
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

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

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type ToolListResult struct {
	Tools []Tool `json:"tools"`
}

func StartServer() {
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
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
			"capabilities":    map[string]interface{}{},
			"serverInfo": map[string]string{
				"name":    "cbm-go-server",
				"version": "1.0.0",
			},
		})
	case "tools/list":
		// Tools JSON Schema is read from schemas.go (Section 3 list)
		sendResponse(req.ID, getExposedTools())
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
	bytes, _ := json.Marshal(resp)
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
	bytes, _ := json.Marshal(resp)
	fmt.Printf("%s\n", bytes)
}
```

```go
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func handleToolCall(req *JSONRPCRequest) {
	var params ToolCallParams
	json.Unmarshal(req.Params, &params)

	switch params.Name {
	case "get_architecture":
		// Query SQLite to assemble modules, routes and packages
		sendResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "Module architecture layout: main, billing, auth",
				},
			},
		})
	case "query_cypher":
		// Call CypherEngine
		sendResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "Cypher execution output results",
				},
			},
		})
	default:
		sendError(req.ID, -32602, "Invalid params or tool not fully configured in stub", nil)
	}
}
```

---

## 13. Incremental File System Watcher

This package integrates `fsnotify` to track local workspace edits and trigger incremental recompilation of the AST without scanning unmodified files.

```go
package watcher

import (
	"log"

	"github.com/fsnotify/fsnotify"
)

type CodeWatcher struct {
	watcher *fsnotify.Watcher
}

func NewCodeWatcher() (*CodeWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &CodeWatcher{watcher: w}, nil
}

// WatchWorkspace registers files in the workspace directories
func (cw *CodeWatcher) WatchWorkspace(rootPath string, onFileChange func(filePath string)) {
	go func() {
		for {
			select {
			case event, ok := <-cw.watcher.Events:
				if !ok {
					return
				}
				// Filter modifications
				if event.Has(fsnotify.Write) {
					onFileChange(event.Name) // Trigger AST parser callback
				}
			case err, ok := <-cw.watcher.Errors:
				if !ok {
					return
				}
				log.Println("watcher error:", err)
			}
		}
	}()

	err := cw.watcher.Add(rootPath)
	if err != nil {
		log.Fatal(err)
	}
}

func (cw *CodeWatcher) Close() {
	cw.watcher.Close()
}
```

---

## 14. Step-by-Step Implementation & Assembly Guide

### Step 1: Initialize Workspace & Go Dependencies
Create a clean directory and initialize Go packages:
```bash
mkdir -p codebase-memory-mcp-go
cd codebase-memory-mcp-go
go mod init codebase-memory-mcp-go

# Fetch dependencies
go get github.com/smacker/go-tree-sitter
go get github.com/smacker/go-tree-sitter/golang
go get github.com/smacker/go-tree-sitter/python
go get github.com/yalue/onnxruntime_go
go get github.com/fsnotify/fsnotify
go get github.com/klauspost/compress/zstd
# SQLite dependency
go get modernc.org/sqlite
```

### Step 2: Establish the Database & Migrations
Create `internal/db/sqlite.go`. Paste the database connection setup and standard execution of SQL commands defined in Section 2. Ensure foreign key constraints are explicitly enabled in `sqlite.go` (`PRAGMA foreign_keys = ON;`).

### Step 3: Implement AST Extractor
Create `internal/parser/parser.go` and input the tree-sitter node extractors. You can configure additional queries for JavaScript, TypeScript, or Rust by downloading the respective Go-tree-sitter language extensions.

### Step 4: Assemble LSP Relationship Resolver
In `internal/resolver/lsp.go`, implement cross-symbol resolution logic:
1. When a function node is parsed, register its global namespace (`package::struct::method`).
2. Search AST call nodes (`(call_expression)`).
3. If a call is made (e.g. `billing.ProcessPayment()`), query the SQLite nodes table to check if a function with this name or prefix is defined in the database.
4. If found, insert a row into the `edges` table with `type = 'CALLS'`.

### Step 5: Implement Embeddings & ONNX Runner
* Fetch a pre-trained embedding model (e.g., download a quantized ONNX `nomic-embed-text-v1.5.onnx` model from Hugging Face).
* Place it in local directory: `~/.cache/cbm-go/models/nomic-embed-text.onnx`.
* Write the ONNX Runtime integration code defined in Section 10. Ensure you specify correct library linking paths depending on the host OS (`.so`, `.dylib`, or `.dll`).

### Step 6: Build the Graph UI React App
1. Set up a React + Vite + Tailwind project inside the `graph-ui/` directory.
2. Install 3D Force Graph library dependencies:
   ```bash
   cd graph-ui
   npm install react-force-graph-3d
   ```
3. Input the React component code shown in Section 11B into `graph-ui/src/App.jsx`.
4. Compile the production bundle:
   ```bash
   npm run build
   ```
   This will output the static assets to the `graph-ui/dist/` directory. Copy the `dist` folder to `internal/ui/dist/`.

### Step 7: Define MCP JSON-RPC handlers
* Implement the stdio loop (`StartServer`) in `internal/mcp/protocol.go`.
* Implement tool responses in `internal/mcp/handlers.go` querying SQLite FTS5 for symbol searches and performing cosine similarity calculations on raw embedding bytes.

### Step 8: Build & Packaging
Compile the binary statically:
```bash
go build -ldflags="-extldflags=-static" -o build/cbm-server cmd/cbm-server/main.go
```
Configure your coding assistant's MCP configurations (e.g., `~/.claude/.mcp.json` or equivalent user settings) to reference the compiled binary paths:
```json
{
  "mcpServers": {
    "cbm-go": {
      "command": "/absolute/path/to/codebase-memory-mcp-go/build/cbm-server",
      "args": []
    }
  }
}
```
