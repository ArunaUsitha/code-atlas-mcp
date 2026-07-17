package resolver

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
)

type CallSite struct {
	Name      string
	Line      int
	StartByte int
}

func ExtractCalls(filePath string, langType string) ([]CallSite, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var lang *sitter.Language
	var queryStr string

	switch strings.ToLower(langType) {
	case "python":
		lang = python.GetLanguage()
		queryStr = `
			(call
				function: [
					(identifier) @call.name
					(attribute attribute: (identifier) @call.name)
				]) @call.expr
		`
	case "go":
		lang = golang.GetLanguage()
		queryStr = `
			(call_expression
				function: [
					(identifier) @call.name
					(selector_expression field: (field_identifier) @call.name)
				]) @call.expr
		`
	default:
		return nil, nil // No call extraction for other languages
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

	var calls []CallSite
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}

		for _, capture := range m.Captures {
			captureName := query.CaptureNameForId(capture.Index)
			if captureName == "call.name" {
				node := capture.Node
				calls = append(calls, CallSite{
					Name:      node.Content(content),
					Line:      int(node.StartPoint().Row) + 1,
					StartByte: int(node.StartByte()),
				})
			}
		}
	}

	return calls, nil
}

func ResolveProjectCalls(db *sql.DB, project string, repoRoot string) error {
	// Query all function nodes in the project
	rows, err := db.Query("SELECT id, name, qualified_name, file_path, kind FROM nodes WHERE project = ?", project)
	if err != nil {
		return fmt.Errorf("failed to query project nodes: %w", err)
	}
	defer rows.Close()

	type NodeInfo struct {
		ID            string
		Name          string
		QualifiedName string
		FilePath      string
		Kind          string
	}

	var allNodes []NodeInfo
	nodeByName := make(map[string][]NodeInfo)

	for rows.Next() {
		var n NodeInfo
		if err := rows.Scan(&n.ID, &n.Name, &n.QualifiedName, &n.FilePath, &n.Kind); err != nil {
			return err
		}
		allNodes = append(allNodes, n)
		nodeByName[n.Name] = append(nodeByName[n.Name], n)
	}

	// For each function node, scan the file it is defined in to find which other functions it calls
	// Wait, we can scan files directly and associate the call site with the function enclosing it!
	for _, node := range allNodes {
		if node.Kind != "function" {
			continue
		}

		fullPath := repoRoot + "/" + node.FilePath
		lang := ""
		if strings.HasSuffix(node.FilePath, ".go") {
			lang = "go"
		} else if strings.HasSuffix(node.FilePath, ".py") {
			lang = "python"
		}

		if lang == "" {
			continue
		}

		calls, err := ExtractCalls(fullPath, lang)
		if err != nil {
			continue // Skip files we can't parse
		}

		// Find calls that happen inside the start_line and end_line of the function
		// Let's get start_line and end_line of the enclosing function
		var startLine, endLine int
		err = db.QueryRow("SELECT start_line, end_line FROM nodes WHERE id = ?", node.ID).Scan(&startLine, &endLine)
		if err != nil {
			continue
		}

		for _, call := range calls {
			// Enclosed within the function?
			if call.Line >= startLine && call.Line <= endLine {
				// Try to resolve target nodes
				if targets, exists := nodeByName[call.Name]; exists {
					for _, target := range targets {
						// Don't link self
						if target.ID == node.ID {
							continue
						}

						// Insert edge 'CALLS'
						_, _ = db.Exec(`
							INSERT OR IGNORE INTO edges (source_id, target_id, type, project)
							VALUES (?, ?, 'CALLS', ?)`,
							node.ID, target.ID, project,
						)
					}
				}
			}
		}
	}

	return nil
}
