package mcp

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"codebase-memory-mcp-go/internal/db"
	"codebase-memory-mcp-go/internal/parser"
	"codebase-memory-mcp-go/internal/resolver"
	"codebase-memory-mcp-go/internal/search"
	"codebase-memory-mcp-go/internal/snapshot"
)

type ToolResponse struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func handleToolCall(req *JSONRPCRequest) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendError(req.ID, -32602, "Invalid parameters", nil)
		return
	}

	proj := serverCtx.Project
	dbConn := serverCtx.DB

	// Extract project argument if present in JSON arguments
	var argProj struct {
		Project string `json:"project"`
	}
	_ = json.Unmarshal(params.Arguments, &argProj)
	if argProj.Project != "" {
		proj = argProj.Project
	}

	var resp ToolResponse
	var err error

	switch params.Name {
	case "get_architecture":
		resp, err = handleGetArchitecture(dbConn, proj)
	case "search_graph":
		resp, err = handleSearchGraph(dbConn, proj, params.Arguments)
	case "search_code":
		resp, err = handleSearchCode(dbConn, proj, params.Arguments)
	case "semantic_query":
		resp, err = handleSemanticQuery(dbConn, proj, params.Arguments)
	case "trace_calls":
		resp, err = handleTraceCalls(dbConn, proj, params.Arguments)
	case "detect_changes":
		resp, err = handleDetectChanges(dbConn, proj, params.Arguments)
	case "find_dead_code":
		resp, err = handleFindDeadCode(dbConn, proj)
	case "query_cypher":
		resp, err = handleQueryCypher(dbConn, params.Arguments)
	case "manage_adr":
		resp, err = handleManageADR(dbConn, params.Arguments)
	case "index_repository":
		resp, err = handleIndexRepository(params.Arguments)
	case "detect_cross_links":
		resp, err = handleDetectCrossLinks(dbConn, proj)
	case "get_file_symbols":
		resp, err = handleGetFileSymbols(dbConn, proj, params.Arguments)
	case "get_impact_analysis":
		resp, err = handleGetImpactAnalysis(dbConn, proj, params.Arguments)
	case "clear_project_index":
		resp, err = handleClearProjectIndex(dbConn, proj)
	default:
		sendError(req.ID, -32601, fmt.Sprintf("Tool %s not found", params.Name), nil)
		return
	}

	if err != nil {
		sendResponse(req.ID, ToolResponse{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		})
	} else {
		sendResponse(req.ID, resp)
	}
}

func handleGetArchitecture(dbConn *sql.DB, project string) (ToolResponse, error) {
	// Count nodes by kind
	rows, err := dbConn.Query("SELECT kind, COUNT(*) FROM nodes WHERE project = ? GROUP BY kind", project)
	if err != nil {
		return ToolResponse{}, err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Architecture Overview for Project [%s]:\n\n", project))
	sb.WriteString("Symbol Node Distributions:\n")

	hasNodes := false
	for rows.Next() {
		hasNodes = true
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err == nil {
			sb.WriteString(fmt.Sprintf("  - %s: %d\n", kind, count))
		}
	}
	if !hasNodes {
		sb.WriteString("  No symbols indexed yet. Use index_repository first.\n")
	}

	// Count cross-service routes
	var routeCount int
	_ = dbConn.QueryRow("SELECT COUNT(*) FROM nodes WHERE project = ? AND kind = 'http_route'", project).Scan(&routeCount)
	sb.WriteString(fmt.Sprintf("\nAPI endpoints: %d route(s) registered.\n", routeCount))

	// Get major packages/directories (top levels)
	dirRows, err := dbConn.Query("SELECT DISTINCT file_path FROM nodes WHERE project = ? AND kind = 'file' LIMIT 10", project)
	if err == nil {
		defer dirRows.Close()
		sb.WriteString("\nTop Indexed Files:\n")
		for dirRows.Next() {
			var path string
			if err := dirRows.Scan(&path); err == nil {
				sb.WriteString(fmt.Sprintf("  - %s\n", path))
			}
		}
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleSearchGraph(dbConn *sql.DB, project string, args json.RawMessage) (ToolResponse, error) {
	var params struct {
		NamePattern string `json:"name_pattern"`
		Kind        string `json:"kind"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ToolResponse{}, err
	}

	sqlQuery := "SELECT name, qualified_name, kind, file_path, start_line, end_line FROM nodes WHERE project = ? AND (name LIKE ? OR qualified_name LIKE ?)"
	var queryArgs []interface{}
	queryArgs = append(queryArgs, project, "%"+params.NamePattern+"%", "%"+params.NamePattern+"%")

	if params.Kind != "" {
		sqlQuery += " AND kind = ?"
		queryArgs = append(queryArgs, params.Kind)
	}
	sqlQuery += " LIMIT 50"

	rows, err := dbConn.Query(sqlQuery, queryArgs...)
	if err != nil {
		return ToolResponse{}, err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search Graph Results for pattern '%s':\n\n", params.NamePattern))

	count := 0
	for rows.Next() {
		count++
		var name, qualified, kind, path string
		var start, end int
		if err := rows.Scan(&name, &qualified, &kind, &path, &start, &end); err == nil {
			sb.WriteString(fmt.Sprintf("[%s] %s (%s)\n  Path: %s:%d-%d\n\n", kind, qualified, name, path, start, end))
		}
	}

	if count == 0 {
		sb.WriteString("No matching nodes found.")
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleSearchCode(dbConn *sql.DB, project string, args json.RawMessage) (ToolResponse, error) {
	var params struct {
		Pattern     string `json:"pattern"`
		FilePattern string `json:"file_pattern"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ToolResponse{}, err
	}

	re, err := regexp.Compile(params.Pattern)
	if err != nil {
		return ToolResponse{}, fmt.Errorf("invalid regex pattern: %w", err)
	}

	var fileRe *regexp.Regexp
	if params.FilePattern != "" {
		fileRe, err = regexp.Compile(params.FilePattern)
		if err != nil {
			return ToolResponse{}, fmt.Errorf("invalid file regex pattern: %w", err)
		}
	}

	// Fetch all files
	rows, err := dbConn.Query("SELECT file_path FROM nodes WHERE project = ? AND kind = 'file'", project)
	if err != nil {
		return ToolResponse{}, err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Grep Search Results for '%s':\n\n", params.Pattern))

	matchCount := 0
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			continue
		}

		if fileRe != nil && !fileRe.MatchString(path) {
			continue
		}

		fullPath := filepath.Join(serverCtx.RepoRoot, path)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for idx, line := range lines {
			if re.MatchString(line) {
				matchCount++
				sb.WriteString(fmt.Sprintf("%s:%d: %s\n", path, idx+1, strings.TrimSpace(line)))
				if matchCount >= 100 {
					sb.WriteString("\nMaximum limit of 100 matches reached.")
					return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
				}
			}
		}
	}

	if matchCount == 0 {
		sb.WriteString("No matching code lines found.")
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleSemanticQuery(dbConn *sql.DB, project string, args json.RawMessage) (ToolResponse, error) {
	var params struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ToolResponse{}, err
	}
	if params.Limit <= 0 {
		params.Limit = 5
	}

	queryVec, err := serverCtx.Embedder.GenerateEmbeddings(params.Query)
	if err != nil {
		return ToolResponse{}, err
	}

	// Fetch all node vectors
	rows, err := dbConn.Query(`
		SELECT nv.node_id, n.name, n.qualified_name, n.kind, n.file_path, nv.vector
		FROM node_vectors nv
		JOIN nodes n ON nv.node_id = n.id
		WHERE n.project = ?`, project)
	if err != nil {
		return ToolResponse{}, err
	}
	defer rows.Close()

	type SimMatch struct {
		Name          string
		QualifiedName string
		Kind          string
		FilePath      string
		Similarity    float64
	}

	var matches []SimMatch

	for rows.Next() {
		var nodeID, name, qualified, kind, path string
		var vecBytes []byte
		if err := rows.Scan(&nodeID, &name, &qualified, &kind, &path, &vecBytes); err == nil {
			// Convert bytes to float32 slice
			nodeVec := make([]float32, len(vecBytes)/4)
			for i := 0; i < len(nodeVec); i++ {
				u := binaryToUint32(vecBytes[i*4 : i*4+4])
				nodeVec[i] = mathFloat32FromUint32(u)
			}

			sim := search.CosineSimilarity(queryVec, nodeVec)
			matches = append(matches, SimMatch{
				Name:          name,
				QualifiedName: qualified,
				Kind:          kind,
				FilePath:      path,
				Similarity:    sim,
			})
		}
	}

	// Simple sort matches
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].Similarity > matches[i].Similarity {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Semantic Matches for Query: '%s'\n\n", params.Query))

	displayCount := params.Limit
	if len(matches) < displayCount {
		displayCount = len(matches)
	}

	for i := 0; i < displayCount; i++ {
		m := matches[i]
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n   Path: %s\n   Similarity Score: %.4f\n\n", i+1, m.Kind, m.QualifiedName, m.FilePath, m.Similarity))
	}

	if displayCount == 0 {
		sb.WriteString("No semantic vectors indexed. Try re-indexing the repository.")
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleTraceCalls(dbConn *sql.DB, project string, args json.RawMessage) (ToolResponse, error) {
	var params struct {
		SymbolName string `json:"symbol_name"`
		Direction  string `json:"direction"`
		Depth      int    `json:"depth"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ToolResponse{}, err
	}
	if params.Depth <= 0 {
		params.Depth = 3
	}
	if params.Direction == "" {
		params.Direction = "inbound"
	}

	// BFS traversal of paths
	var startID string
	err := dbConn.QueryRow("SELECT id FROM nodes WHERE project = ? AND (name = ? OR qualified_name = ?)", project, params.SymbolName, params.SymbolName).Scan(&startID)
	if err != nil {
		return ToolResponse{}, fmt.Errorf("starting symbol '%s' not found: %w", params.SymbolName, err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("BFS Call Trace for '%s' (depth: %d, direction: %s):\n\n", params.SymbolName, params.Depth, params.Direction))

	visited := make(map[string]bool)
	type TraceNode struct {
		ID    string
		Path  string
		Depth int
	}
	queue := []TraceNode{{ID: startID, Path: params.SymbolName, Depth: 0}}
	visited[startID] = true

	count := 0
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr.Depth > params.Depth {
			continue
		}

		if curr.ID != startID {
			count++
			sb.WriteString(fmt.Sprintf("  %s\n", curr.Path))
		}

		var queryStr string
		if params.Direction == "inbound" || params.Direction == "both" {
			// Find who CALLS the current node (target is current)
			queryStr = "SELECT source_id, n.qualified_name FROM edges e JOIN nodes n ON e.source_id = n.id WHERE e.target_id = ? AND e.type = 'CALLS'"
			rows, err := dbConn.Query(queryStr, curr.ID)
			if err == nil {
				for rows.Next() {
					var srcID, srcQual string
					if rows.Scan(&srcID, &srcQual) == nil && !visited[srcID] {
						visited[srcID] = true
						queue = append(queue, TraceNode{
							ID:    srcID,
							Path:  fmt.Sprintf("%s <- %s", curr.Path, srcQual),
							Depth: curr.Depth + 1,
						})
					}
				}
				rows.Close()
			}
		}

		if params.Direction == "outbound" || params.Direction == "both" {
			// Find who is CALLED by the current node (source is current)
			queryStr = "SELECT target_id, n.qualified_name FROM edges e JOIN nodes n ON e.target_id = n.id WHERE e.source_id = ? AND e.type = 'CALLS'"
			rows, err := dbConn.Query(queryStr, curr.ID)
			if err == nil {
				for rows.Next() {
					var tgtID, tgtQual string
					if rows.Scan(&tgtID, &tgtQual) == nil && !visited[tgtID] {
						visited[tgtID] = true
						queue = append(queue, TraceNode{
							ID:    tgtID,
							Path:  fmt.Sprintf("%s -> %s", curr.Path, tgtQual),
							Depth: curr.Depth + 1,
						})
					}
				}
				rows.Close()
			}
		}
	}

	if count == 0 {
		sb.WriteString("No call paths detected.")
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleDetectChanges(dbConn *sql.DB, project string, args json.RawMessage) (ToolResponse, error) {
	var params struct {
		BaseBranch string `json:"base_branch"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ToolResponse{}, err
	}
	if params.BaseBranch == "" {
		params.BaseBranch = "main"
	}

	// Run git diff command
	cmd := exec.Command("git", "diff", "--name-only", params.BaseBranch)
	cmd.Dir = serverCtx.RepoRoot
	output, err := cmd.Output()
	if err != nil {
		return ToolResponse{}, fmt.Errorf("git command failed: %w", err)
	}

	changedFiles := strings.Split(strings.TrimSpace(string(output)), "\n")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Git Changed Files relative to '%s' & Risk Profile:\n\n", params.BaseBranch))

	hasChanges := false
	for _, rawFile := range changedFiles {
		file := strings.TrimSpace(rawFile)
		if file == "" {
			continue
		}
		hasChanges = true
		sb.WriteString(fmt.Sprintf("File: %s\n", file))

		// Check if file is indexed and lookup associated functions/classes
		rows, err := dbConn.Query("SELECT qualified_name, kind, id FROM nodes WHERE project = ? AND file_path = ?", project, file)
		if err == nil {
			hasSymbols := false
			for rows.Next() {
				hasSymbols = true
				var qname, kind, id string
				if rows.Scan(&qname, &kind, &id) == nil {
					// Count inbound CALLS (how many places call this modified code)
					var incomingCount int
					_ = dbConn.QueryRow("SELECT COUNT(*) FROM edges WHERE target_id = ? AND type = 'CALLS'", id).Scan(&incomingCount)

					risk := "LOW"
					if incomingCount > 5 {
						risk = "HIGH"
					} else if incomingCount > 1 {
						risk = "MEDIUM"
					}

					sb.WriteString(fmt.Sprintf("  - Symbol: %s (%s) | Dependent Callers: %d | Risk: %s\n", qname, kind, incomingCount, risk))
				}
			}
			rows.Close()
			if !hasSymbols {
				sb.WriteString("  - No parsed symbols (potentially new file or configuration/documentation)\n")
			}
		}
		sb.WriteString("\n")
	}

	if !hasChanges {
		sb.WriteString("No changes detected relative to " + params.BaseBranch)
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleFindDeadCode(dbConn *sql.DB, project string) (ToolResponse, error) {
	// Query functions with 0 inbound CALLS edges, excluding entry points
	sqlQuery := `
		SELECT name, qualified_name, file_path, start_line
		FROM nodes n
		WHERE n.project = ? AND n.kind = 'function'
		  AND NOT EXISTS (
		      SELECT 1 FROM edges e
		      WHERE e.target_id = n.id AND e.type = 'CALLS'
		  )
		  AND LOWER(n.name) NOT LIKE 'main%'
		  AND LOWER(n.name) NOT LIKE 'init%'
		  AND LOWER(n.name) NOT LIKE 'test%'
	`
	rows, err := dbConn.Query(sqlQuery, project)
	if err != nil {
		return ToolResponse{}, err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Dead/Unused Function Candidates (Zero incoming call connections, ignoring main/init/tests):\n\n")

	count := 0
	for rows.Next() {
		count++
		var name, qualified, path string
		var line int
		if err := rows.Scan(&name, &qualified, &path, &line); err == nil {
			sb.WriteString(fmt.Sprintf("  - %s (%s)\n    File: %s:%d\n\n", qualified, name, path, line))
		}
	}

	if count == 0 {
		sb.WriteString("No dead function nodes detected.")
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleQueryCypher(dbConn *sql.DB, args json.RawMessage) (ToolResponse, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ToolResponse{}, err
	}

	engine := search.NewCypherEngine(dbConn)
	results, err := engine.ExecuteQuery(params.Query)
	if err != nil {
		return ToolResponse{}, err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Cypher Results for Query: %s\n\n", params.Query))

	for i, r := range results {
		if r.Kind != "" {
			sb.WriteString(fmt.Sprintf("%d. [%s] %s (%s)\n", i+1, r.Kind, r.QualifiedName, r.Name))
		} else {
			sb.WriteString(fmt.Sprintf("%d. %s (%s)\n", i+1, r.QualifiedName, r.Name))
		}
	}

	if len(results) == 0 {
		sb.WriteString("No matching paths found.")
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleManageADR(dbConn *sql.DB, args json.RawMessage) (ToolResponse, error) {
	var params struct {
		Action    string `json:"action"`
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		Decisions string `json:"decisions"`
		Context   string `json:"context"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ToolResponse{}, err
	}

	var sb strings.Builder

	switch params.Action {
	case "list":
		rows, err := dbConn.Query("SELECT id, title, status, date FROM adrs")
		if err != nil {
			return ToolResponse{}, err
		}
		defer rows.Close()

		sb.WriteString("Architecture Decision Records (ADRs):\n\n")
		count := 0
		for rows.Next() {
			count++
			var id, title, status, date string
			if rows.Scan(&id, &title, &status, &date) == nil {
				sb.WriteString(fmt.Sprintf("[%s] %s - %s (Status: %s)\n", date, id, title, status))
			}
		}
		if count == 0 {
			sb.WriteString("No ADRs recorded yet.")
		}

	case "get":
		if params.ID == "" {
			return ToolResponse{}, fmt.Errorf("ADR ID is required for get action")
		}
		var title, status, date, decisions, contextStr string
		err := dbConn.QueryRow("SELECT title, status, date, decisions, context FROM adrs WHERE id = ?", params.ID).Scan(&title, &status, &date, &decisions, &contextStr)
		if err != nil {
			return ToolResponse{}, fmt.Errorf("ADR '%s' not found: %w", params.ID, err)
		}

		sb.WriteString(fmt.Sprintf("# %s: %s\n\n", params.ID, title))
		sb.WriteString(fmt.Sprintf("**Date**: %s\n", date))
		sb.WriteString(fmt.Sprintf("**Status**: %s\n\n", status))
		sb.WriteString("## Context\n")
		sb.WriteString(contextStr + "\n\n")
		sb.WriteString("## Decision & Consequences\n")
		sb.WriteString(decisions + "\n")

	case "create":
		if params.ID == "" || params.Title == "" {
			return ToolResponse{}, fmt.Errorf("ID and Title are required to create ADR")
		}
		dateStr := time.Now().Format("2006-01-02")
		if params.Status == "" {
			params.Status = "proposed"
		}
		_, err := dbConn.Exec(`
			INSERT INTO adrs (id, title, status, date, decisions, context)
			VALUES (?, ?, ?, ?, ?, ?)`,
			params.ID, params.Title, params.Status, dateStr, params.Decisions, params.Context,
		)
		if err != nil {
			return ToolResponse{}, fmt.Errorf("failed to create ADR: %w", err)
		}
		sb.WriteString(fmt.Sprintf("Successfully created ADR: %s", params.ID))

	case "update":
		if params.ID == "" {
			return ToolResponse{}, fmt.Errorf("ADR ID is required to update ADR")
		}
		_, err := dbConn.Exec(`
			UPDATE adrs
			SET status = COALESCE(NULLIF(?, ''), status),
			    decisions = COALESCE(NULLIF(?, ''), decisions),
			    context = COALESCE(NULLIF(?, ''), context)
			WHERE id = ?`,
			params.Status, params.Decisions, params.Context, params.ID,
		)
		if err != nil {
			return ToolResponse{}, fmt.Errorf("failed to update ADR: %w", err)
		}
		sb.WriteString(fmt.Sprintf("Successfully updated ADR: %s", params.ID))

	default:
		return ToolResponse{}, fmt.Errorf("unsupported ADR action: %s", params.Action)
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleIndexRepository(args json.RawMessage) (ToolResponse, error) {
	var params struct {
		RepoPath    string `json:"repo_path"`
		ProjectName string `json:"project_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ToolResponse{}, err
	}

	if params.ProjectName == "" {
		params.ProjectName = serverCtx.Project
	}

	// Create memory DB compiler
	compiler, err := db.NewRAMCompiler()
	if err != nil {
		return ToolResponse{}, err
	}
	defer compiler.Close()

	memDB := compiler.DB()

	// Walk folder to parse files
	var files []string
	err = filepath.WalkDir(params.RepoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip standard git, dependency, or build folders
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".go" || name == ".gcc" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".go" || ext == ".py" || ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" || d.Name() == "Dockerfile" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return ToolResponse{}, fmt.Errorf("failed to walk workspace: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Indexing Repository: %s\n", params.RepoPath))
	sb.WriteString(fmt.Sprintf("Found %d file(s) to process.\n\n", len(files)))

	parsedCount := 0
	nodeCount := 0

	// Start database transaction
	tx, err := memDB.Begin()
	if err != nil {
		return ToolResponse{}, err
	}

	for _, file := range files {
		relPath, _ := filepath.Rel(params.RepoPath, file)
		relPath = filepath.ToSlash(relPath)

		// 1. Create file node
		fileID := hashSHA256(params.ProjectName + ":" + relPath)
		_, err = tx.Exec(`
			INSERT OR REPLACE INTO nodes (id, project, name, qualified_name, kind, file_path, start_line, end_line, signature, content_hash)
			VALUES (?, ?, ?, ?, 'file', ?, 1, 1, '', '')`,
			fileID, params.ProjectName, filepath.Base(file), relPath, relPath,
		)

		lang := parser.DetectLanguage(file)
		if lang == "unknown" {
			continue
		}

		symbols, err := parser.ParseFile(file, lang)
		if err != nil {
			log.Printf("Parser warning on file %s: %v", relPath, err)
			continue
		}

		parsedCount++

		// Insert nodes
		for _, sym := range symbols {
			nodeCount++
			symID := hashSHA256(params.ProjectName + ":" + relPath + ":" + sym.Name)
			_, _ = tx.Exec(`
				INSERT OR REPLACE INTO nodes (id, project, name, qualified_name, kind, file_path, start_line, end_line, signature, content_hash)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
				symID, params.ProjectName, sym.Name, relPath+"::"+sym.Name, sym.Kind, relPath, sym.StartLine, sym.EndLine, sym.Signature,
			)

			// Generate and store embedding vectors
			textToEmbed := fmt.Sprintf("%s %s %s", sym.Name, sym.Kind, sym.Signature)
			vec, err := serverCtx.Embedder.GenerateEmbeddings(textToEmbed)
			if err == nil {
				// Convert float32 slice to bytes
				vecBytes := make([]byte, len(vec)*4)
				for i, f := range vec {
					bits := mathFloat32ToUint32(f)
					binaryUint32ToBytes(vecBytes[i*4:i*4+4], bits)
				}
				_, _ = tx.Exec(`
					INSERT OR REPLACE INTO node_vectors (node_id, vector)
					VALUES (?, ?)`,
					symID, vecBytes,
				)
			}

			// Insert parent-child DEFINE edge
			_, _ = tx.Exec(`
				INSERT OR IGNORE INTO edges (source_id, target_id, type, project)
				VALUES (?, ?, 'DEFINES', ?)`,
				fileID, symID, params.ProjectName,
			)
		}

		// Read content for API routing/call and IaC parsing
		contentBytes, err := os.ReadFile(file)
		if err == nil {
			contentStr := string(contentBytes)

			// FTS5 populate
			_, _ = tx.Exec(`
				INSERT INTO nodes_fts (node_id, name, qualified_name, signature, content)
				VALUES (?, ?, ?, '', ?)`,
				fileID, filepath.Base(file), relPath, contentStr,
			)

			// Extract routes
			routes := parser.ExtractRoutes(contentStr, relPath)
			for _, r := range routes {
				routeID := hashSHA256(params.ProjectName + ":route:" + r.Method + ":" + r.Path)
				_, _ = tx.Exec(`
					INSERT OR REPLACE INTO nodes (id, project, name, qualified_name, kind, file_path, start_line, end_line, signature, content_hash)
					VALUES (?, ?, ?, ?, 'http_route', ?, ?, ?, ?, '')`,
					routeID, params.ProjectName, r.Path, r.Method+":"+r.Path, relPath, r.Line, r.Line, r.Method,
				)
				_, _ = tx.Exec(`
					INSERT OR IGNORE INTO edges (source_id, target_id, type, project)
					VALUES (?, ?, 'DEFINES', ?)`,
					fileID, routeID, params.ProjectName,
				)
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return ToolResponse{}, err
	}

	// 2. Resolve cross-references and function calls
	_ = resolver.ResolveProjectCalls(memDB, params.ProjectName, params.RepoPath)

	// Close disk DB to release the lock on Windows before overwriting it
	if serverCtx.DB != nil {
		serverCtx.DB.Close()
		serverCtx.DB = nil
	}

	// Flush RAM DB compiler to local disk storage
	err = compiler.FlushToDisk(serverCtx.DBPath)
	if err != nil {
		return ToolResponse{}, fmt.Errorf("failed to save SQLite file to disk: %w", err)
	}

	// 3. Compress snapshot using Zstd compactor
	zstdPath := filepath.Join(filepath.Dir(serverCtx.DBPath), "snapshot.db.zst")
	_ = snapshot.CompressDatabase(serverCtx.DBPath, zstdPath)

	sb.WriteString(fmt.Sprintf("Index Successful:\n"))
	sb.WriteString(fmt.Sprintf("  - Processed files: %d\n", parsedCount))
	sb.WriteString(fmt.Sprintf("  - Extracted symbol nodes: %d\n", nodeCount))
	sb.WriteString(fmt.Sprintf("  - Backup snapshot saved: %s\n", zstdPath))

	// Reopen DB connections to keep system operational
	// (Flushing compiler closed its sqlite connection, so we recreate the connection in our main context)
	mainDB, err := db.InitDB(serverCtx.DBPath)
	if err == nil {
		serverCtx.DB = mainDB
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleDetectCrossLinks(dbConn *sql.DB, project string) (ToolResponse, error) {
	// Query route nodes
	routeRows, err := dbConn.Query("SELECT name, qualified_name, file_path, start_line FROM nodes WHERE project = ? AND kind = 'http_route'", project)
	if err != nil {
		return ToolResponse{}, err
	}
	defer routeRows.Close()

	var routes []parser.APIRoute
	for routeRows.Next() {
		var name, qname, file string
		var line int
		if routeRows.Scan(&name, &qname, &file, &line) == nil {
			parts := strings.Split(qname, ":")
			method := "GET"
			if len(parts) > 0 {
				method = parts[0]
			}
			routes = append(routes, parser.APIRoute{
				Method: method,
				Path:   name,
				File:   file,
				Line:   line,
			})
		}
	}

	// Fetch files to check callsites (regex-based in routing.go)
	fileRows, err := dbConn.Query("SELECT file_path FROM nodes WHERE project = ? AND kind = 'file'", project)
	if err != nil {
		return ToolResponse{}, err
	}
	defer fileRows.Close()

	var allCalls []parser.APICallSite
	for fileRows.Next() {
		var path string
		if fileRows.Scan(&path) == nil {
			fullPath := filepath.Join(serverCtx.RepoRoot, path)
			content, err := os.ReadFile(fullPath)
			if err == nil {
				calls := parser.ExtractCallSites(string(content), path)
				allCalls = append(allCalls, calls...)
			}
		}
	}

	matches := parser.MatchEndpoints(routes, allCalls)

	var sb strings.Builder
	sb.WriteString("Cross-Service Endpoint Links Detected:\n\n")

	for i, m := range matches {
		sb.WriteString(fmt.Sprintf("%d. Service API endpoint [%s %s] at %s:%d\n", i+1, m.Route.Method, m.Route.Path, m.Route.File, m.Route.Line))
		sb.WriteString(fmt.Sprintf("   called by client at %s:%d\n\n", m.Call.File, m.Call.Line))
	}

	if len(matches) == 0 {
		sb.WriteString("No cross-service API link connections discovered.")
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleGetFileSymbols(dbConn *sql.DB, project string, args json.RawMessage) (ToolResponse, error) {
	var params struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ToolResponse{}, err
	}

	rows, err := dbConn.Query("SELECT name, qualified_name, kind, start_line, end_line, signature FROM nodes WHERE project = ? AND file_path = ? AND kind != 'file'", project, params.FilePath)
	if err != nil {
		return ToolResponse{}, err
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Flat AST Symbol List for file '%s':\n\n", params.FilePath))

	count := 0
	for rows.Next() {
		count++
		var name, qualified, kind, signature string
		var start, end int
		if err := rows.Scan(&name, &qualified, &kind, &start, &end, &signature); err == nil {
			sb.WriteString(fmt.Sprintf("  - [%s] %s\n    Lines: %d-%d | Sig: %s\n\n", kind, qualified, start, end, signature))
		}
	}

	if count == 0 {
		sb.WriteString("No symbols parsed in this file.")
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleGetImpactAnalysis(dbConn *sql.DB, project string, args json.RawMessage) (ToolResponse, error) {
	var params struct {
		SymbolName string `json:"symbol_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ToolResponse{}, err
	}

	// BFS traversal downstream to trace impact of changing a symbol interface
	var startID string
	err := dbConn.QueryRow("SELECT id FROM nodes WHERE project = ? AND (name = ? OR qualified_name = ?)", project, params.SymbolName, params.SymbolName).Scan(&startID)
	if err != nil {
		return ToolResponse{}, fmt.Errorf("starting symbol '%s' not found: %w", params.SymbolName, err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Downstream Impact Analysis (nodes affected if '%s' changes):\n\n", params.SymbolName))

	visited := make(map[string]bool)
	type ImpactNode struct {
		ID   string
		Path string
	}
	queue := []ImpactNode{{ID: startID, Path: params.SymbolName}}
	visited[startID] = true

	count := 0
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		// Find who calls this node (source depends on target. Here target is curr.ID, source is the caller that will be impacted!)
		rows, err := dbConn.Query("SELECT source_id, n.qualified_name, n.kind FROM edges e JOIN nodes n ON e.source_id = n.id WHERE e.target_id = ? AND e.type = 'CALLS'", curr.ID)
		if err == nil {
			for rows.Next() {
				var srcID, srcQual, srcKind string
				if rows.Scan(&srcID, &srcQual, &srcKind) == nil && !visited[srcID] {
					visited[srcID] = true
					count++
					path := fmt.Sprintf("%s -> %s (%s)", curr.Path, srcQual, srcKind)
					sb.WriteString(fmt.Sprintf("  - Impacted caller: %s\n", path))
					queue = append(queue, ImpactNode{ID: srcID, Path: path})
				}
			}
			rows.Close()
		}
	}

	if count == 0 {
		sb.WriteString("No downstream impacts detected. The symbol can be refactored safely.")
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: sb.String()}}}, nil
}

func handleClearProjectIndex(dbConn *sql.DB, project string) (ToolResponse, error) {
	tx, err := dbConn.Begin()
	if err != nil {
		return ToolResponse{}, err
	}

	_, _ = tx.Exec("DELETE FROM edges WHERE project = ?", project)
	_, _ = tx.Exec("DELETE FROM node_vectors WHERE node_id IN (SELECT id FROM nodes WHERE project = ?)", project)
	_, _ = tx.Exec("DELETE FROM nodes WHERE project = ?", project)
	_, _ = tx.Exec("DELETE FROM nodes_fts WHERE node_id NOT IN (SELECT id FROM nodes)")

	err = tx.Commit()
	if err != nil {
		return ToolResponse{}, err
	}

	return ToolResponse{Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Cleared database and snapshots successfully for project [%s]", project)}}}, nil
}

// Helpers
func hashSHA256(text string) string {
	h := sha256.New()
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}

func mathFloat32ToUint32(f float32) uint32 {
	return math.Float32bits(f)
}

func mathFloat32FromUint32(u uint32) float32 {
	return math.Float32frombits(u)
}

func binaryUint32ToBytes(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func binaryToUint32(b []byte) uint32 {
	_ = b[3] // bounds check hint
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// ExecuteToolDirectly runs a tool function directly bypassing JSON-RPC stdio loop.
func ExecuteToolDirectly(ctx *ServerContext, name string, argsJSON string) (ToolResponse, error) {
	serverCtx = ctx
	dbConn := ctx.DB
	proj := ctx.Project

	// Extract project argument if present in JSON arguments
	var argProj struct {
		Project string `json:"project"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &argProj)
	if argProj.Project != "" {
		proj = argProj.Project
	}

	var resp ToolResponse
	var err error

	switch name {
	case "get_architecture":
		resp, err = handleGetArchitecture(dbConn, proj)
	case "search_graph":
		resp, err = handleSearchGraph(dbConn, proj, json.RawMessage(argsJSON))
	case "search_code":
		resp, err = handleSearchCode(dbConn, proj, json.RawMessage(argsJSON))
	case "semantic_query":
		resp, err = handleSemanticQuery(dbConn, proj, json.RawMessage(argsJSON))
	case "trace_calls":
		resp, err = handleTraceCalls(dbConn, proj, json.RawMessage(argsJSON))
	case "detect_changes":
		resp, err = handleDetectChanges(dbConn, proj, json.RawMessage(argsJSON))
	case "find_dead_code":
		resp, err = handleFindDeadCode(dbConn, proj)
	case "query_cypher":
		resp, err = handleQueryCypher(dbConn, json.RawMessage(argsJSON))
	case "manage_adr":
		resp, err = handleManageADR(dbConn, json.RawMessage(argsJSON))
	case "index_repository":
		resp, err = handleIndexRepository(json.RawMessage(argsJSON))
	case "detect_cross_links":
		resp, err = handleDetectCrossLinks(dbConn, proj)
	case "get_file_symbols":
		resp, err = handleGetFileSymbols(dbConn, proj, json.RawMessage(argsJSON))
	case "get_impact_analysis":
		resp, err = handleGetImpactAnalysis(dbConn, proj, json.RawMessage(argsJSON))
	case "clear_project_index":
		resp, err = handleClearProjectIndex(dbConn, proj)
	default:
		return ToolResponse{}, fmt.Errorf("tool %s not found", name)
	}

	return resp, err
}
