package search

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

type CypherEngine struct {
	db *sql.DB
}

func NewCypherEngine(db *sql.DB) *CypherEngine {
	return &CypherEngine{db: db}
}

type CypherResult struct {
	Name          string `json:"name"`
	QualifiedName string `json:"qualified_name"`
	Kind          string `json:"kind,omitempty"`
}

// ExecuteQuery converts basic cypher constructs to SQL queries and returns parsed structures
func (ce *CypherEngine) ExecuteQuery(cypher string) ([]CypherResult, error) {
	// Parse e.g. MATCH (f:Function)-[:CALLS]->(g) WHERE f.name = 'main' RETURN g.name
	// Case-insensitive regex match
	re := regexp.MustCompile(`(?i)MATCH\s+\([a-zA-Z0-9_]*?:(function|class|file|route|http_route)\)-\[:(CALLS|IMPORTS|DEFINES|INHERITS)\]->\(([a-zA-Z0-9_]*?)\)\s+WHERE\s+[a-zA-Z0-9_.]+?\s*=\s*'([^']+)'\s+RETURN\s+(.*)`)
	matches := re.FindStringSubmatch(cypher)

	if len(matches) == 0 {
		// Fallback to standard selection on nodes if it doesn't match
		rows, err := ce.db.Query("SELECT name, qualified_name, kind FROM nodes LIMIT 50;")
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var results []CypherResult
		for rows.Next() {
			var r CypherResult
			if err := rows.Scan(&r.Name, &r.QualifiedName, &r.Kind); err == nil {
				results = append(results, r)
			}
		}
		return results, nil
	}

	sourceKind := strings.ToLower(matches[1])
	edgeType := strings.ToUpper(matches[2])
	filterVal := matches[4]

	sqlQuery := `
		SELECT n2.name, n2.qualified_name, n2.kind
		FROM nodes n1
		JOIN edges e ON n1.id = e.source_id
		JOIN nodes n2 ON e.target_id = n2.id
		WHERE LOWER(n1.kind) = ? AND e.type = ? AND n1.name = ?
	`
	rows, err := ce.db.Query(sqlQuery, sourceKind, edgeType, filterVal)
	if err != nil {
		return nil, fmt.Errorf("cypher SQL query failed: %w", err)
	}
	defer rows.Close()

	var results []CypherResult
	for rows.Next() {
		var r CypherResult
		if err := rows.Scan(&r.Name, &r.QualifiedName, &r.Kind); err == nil {
			results = append(results, r)
		}
	}
	return results, nil
}
