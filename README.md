# CodeAtlas MCP (Go)

CodeAtlas MCP is a high-performance codebase indexing, visual graph representation, and context-retrieval MCP (Model Context Protocol) server written in Go. It parses your project structures, tracks connections between functions, classes, dependencies, and APIs, and exposes powerful code intelligence tools to your AI assistants (like Claude Desktop, Cursor, or Aider).

It also embeds an interactive **3D Graph Visualization UI** to help you inspect and walk your codebase's architectural layers in your browser.

---

## Features

- **Multi-Language Parsing:** AST-based code analysis supporting 158 languages (Go, Python, JS, TS, etc.) powered by Tree-sitter.
- **MCP Protocol Integration:** Exposes tools for semantic search, structural pattern querying, inbound/outbound call-graph tracing, and risk-profiling.
- **Built-in Interactive 3D Web UI:** Visualizes nodes (files, classes, routes) and relationships (defines, imports, calls, inherits) as a reactive 3D graph.
- **Graceful Embeddings Fallback:** Incorporates ONNX runtime support for local semantic vector searches, automatically falling back to a deterministic mock embedder if dependencies aren't set up.
- **CGO & Pure-Go mix:** Uses a pure-Go SQLite driver to keep databases robust and portable, using CGO bindings only for grammar AST parsing.
- **Double Command Interfaces:** Operates both as a background stdio JSON-RPC server and an interactive command-line tool.

---

## Directory Structure

```text
├── cmd/
│   └── cbm-server/
│       └── main.go          # Entry point, CLI flags, MCP stdio loop setup
├── internal/
│   ├── config/              # Config settings (.cbm-config, environment variables)
│   ├── db/                  # SQLite DB connections, compiler pipelines (RAM to Disk)
│   ├── parser/              # Tree-sitter grammar parsers, IaC and router mappings
│   ├── resolver/            # Cross-file symbol resolution and dependency mapping
│   ├── search/              # BM25 search, Cypher queries, ONNX semantic embedding engine
│   ├── ui/                  # HTTP Server for visualization UI
│   └── mcp/                 # JSON-RPC protocol implementation and tool handlers
└── graph-ui/                # React visualizer source code (3D Force-Directed Graph)
```

---

## Getting Started

### 1. Build the Binary
To build the server binary locally (ensuring CGO is enabled since tree-sitter bindings require C compilation):

**Windows (PowerShell):**
```powershell
$env:CGO_ENABLED="1"
go build -o cbm-server.exe ./cmd/cbm-server
```
*(If you need a local GCC compiler, you can reference the path of any local MinGW toolchain in your PATH variable).*

**Linux / macOS:**
```bash
CGO_ENABLED=1 go build -o cbm-server ./cmd/cbm-server
```

---

## Usage Modes

### Mode A: Command Line Interface (CLI)
You can call any of the exposed MCP tools directly from your terminal using the `cli` command prefix:

```bash
cbm-server cli <command> '[json_arguments]'
```

#### Available Commands:
- `index_repository`: Scan, parse, and build the codebase graph.
- `get_architecture`: Returns high-level package/directory structure overview.
- `search_graph`: Structural lookup for symbols using regex patterns.
- `search_code`: Grep-style regex search scoped by AST structure.
- `semantic_query`: Conceptual similarity lookup on codebase nodes.
- `trace_calls`: Traces BFS path call chains leading in/out of a function.
- `detect_changes`: Risk-profile analysis comparing workspace changes to base Git branch.
- `find_dead_code`: Detects symbols with zero inbound calls.
- `get_file_symbols`: Lists all definitions in a specific file.
- `clear_project_index`: Deletes all nodes and edges for the specified project.

#### CLI Examples:

**Index a workspace:**
```bash
./cbm-server.exe --db my_project.db cli index_repository '{"repo_path": "C:/projects/my-app", "project_name": "my-app"}'
```

**Get high-level architecture overview:**
```bash
./cbm-server.exe --db my_project.db cli get_architecture '{"project": "my-app"}'
```

**Trace inbound call chains for a function:**
```bash
./cbm-server.exe --db my_project.db cli trace_calls '{"symbol_name": "ProcessPayment", "direction": "inbound"}'
```

---

### Mode B: Model Context Protocol (MCP) Server
To use the server with AI editors (e.g. Claude Desktop or Cursor), register the binary in the client configuration file.

#### Claude Desktop Configuration (`claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "code-atlas": {
      "command": "C:\\path\\to\\cbm-server.exe",
      "args": [
        "--db", "C:\\Users\\Username\\.cache\\cbm-go\\graph.db",
        "--project", "my-active-project",
        "--repo", "C:\\path\\to\\my-active-project"
      ]
    }
  }
}
```

---

### Mode C: 3D Visualization UI
Whenever the server is running (either in MCP mode or manually), a background HTTP server launches on loopback.
- Open your browser to `http://127.0.0.1:8080` (or the custom port set using the `--port` flag).
- You can inspect files, methods, dependencies, and API endpoints as nodes and edges in a interactive 3D graph model.
