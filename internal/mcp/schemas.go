package mcp

type Property struct {
	Type        string    `json:"type"`
	Description string    `json:"description,omitempty"`
	Enum        []string  `json:"enum,omitempty"`
	Default     interface{} `json:"default,omitempty"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

func getExposedTools() []Tool {
	return []Tool{
		{
			Name:        "get_architecture",
			Description: "Returns high-level structural overview of codebase packages, entry points, REST/gRPC routes, directories, and architectural hotspots.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"project": {Type: "string", Description: "Scope search to a specific project name"},
				},
			},
		},
		{
			Name:        "search_graph",
			Description: "Executes structural lookup on codebase nodes using regex patterns, kinds, and line limits.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"name_pattern": {Type: "string", Description: "Regex matching symbol or file name"},
					"kind":         {Type: "string", Enum: []string{"file", "directory", "class", "function", "http_route", "iac_resource"}, Description: "Filter by node classification"},
					"project":      {Type: "string"},
				},
				Required: []string{"name_pattern"},
			},
		},
		{
			Name:        "search_code",
			Description: "Performs grep-style search over indexed file contents with AST parsing scoping.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"pattern":      {Type: "string", Description: "Regex expression to find inside file bodies"},
					"file_pattern": {Type: "string", Description: "Regex pattern scoping paths to search"},
					"project":      {Type: "string"},
				},
				Required: []string{"pattern"},
			},
		},
		{
			Name:        "semantic_query",
			Description: "Queries vector embeddings of codebase elements to retrieve context using conceptual meaning rather than string matching.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"query":   {Type: "string", Description: "Natural language query (e.g. 'how are billing signatures generated')"},
					"limit":   {Type: "integer", Default: 5},
					"project": {Type: "string"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "trace_calls",
			Description: "Computes BFS path trees tracing calls leading into or out of a targeted function symbol.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"symbol_name": {Type: "string", Description: "Qualified name of starting function/method"},
					"direction":   {Type: "string", Enum: []string{"inbound", "outbound", "both"}, Default: "inbound"},
					"depth":       {Type: "integer", Default: 3},
					"project":     {Type: "string"},
				},
				Required: []string{"symbol_name"},
			},
		},
		{
			Name:        "detect_changes",
			Description: "Queries modified files relative to standard Git HEAD, mapping modifications to affected classes/routes and calculating risk profiles.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"base_branch": {Type: "string", Default: "main", Description: "Compare changes against this base branch"},
					"project":     {Type: "string"},
				},
			},
		},
		{
			Name:        "find_dead_code",
			Description: "Detects function nodes that have zero inbound 'CALLS' or 'HTTP_CALLS' relationships, excluding known entry points.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"project": {Type: "string"},
				},
			},
		},
		{
			Name:        "query_cypher",
			Description: "Executes structural Graph queries using a lightweight Cypher-like language (e.g. MATCH (f:Function)-[:CALLS]->(g) WHERE f.name='main' RETURN g).",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"query": {Type: "string", Description: "Cypher query block"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "manage_adr",
			Description: "Reads, creates, or updates Architecture Decision Records (ADRs) locally.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"action":    {Type: "string", Enum: []string{"list", "get", "create", "update"}, Description: "Operation type"},
					"id":        {Type: "string", Description: "ADR ID identifier (e.g. 'ADR-001')"},
					"title":     {Type: "string"},
					"status":    {Type: "string", Enum: []string{"proposed", "accepted", "rejected", "superseded"}},
					"decisions": {Type: "string", Description: "Decisions content markdown"},
					"context":   {Type: "string"},
				},
				Required: []string{"action"},
			},
		},
		{
			Name:        "index_repository",
			Description: "Explicitly triggers full repository re-scanning, writing indexes to local DB and exporting Zstd snapshot.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"repo_path":    {Type: "string", Description: "Absolute path to repository root"},
					"project_name": {Type: "string"},
				},
				Required: []string{"repo_path"},
			},
		},
		{
			Name:        "detect_cross_links",
			Description: "Identifies cross-service connections by matching HTTP route endpoints in one microservice with HTTP call sites in another.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"project": {Type: "string"},
				},
			},
		},
		{
			Name:        "get_file_symbols",
			Description: "Retrieves flat list of AST nodes parsed inside a specific file path.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"file_path": {Type: "string", Description: "Relative path to target file"},
					"project":   {Type: "string"},
				},
				Required: []string{"file_path"},
			},
		},
		{
			Name:        "get_impact_analysis",
			Description: "Traces what downstream nodes (classes, APIs, files) will be affected if a particular class/method interface is modified.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"symbol_name": {Type: "string", Description: "Qualified name of node being modified"},
					"project":     {Type: "string"},
				},
				Required: []string{"symbol_name"},
			},
		},
		{
			Name:        "clear_project_index",
			Description: "Deletes the index database contents and snapshots for a specified project.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"project": {Type: "string"},
				},
				Required: []string{"project"},
			},
		},
	}
}
