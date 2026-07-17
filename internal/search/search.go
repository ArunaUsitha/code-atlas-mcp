package search

import (
	"database/sql"
	"fmt"
)

type SearchResult struct {
	NodeID        string  `json:"node_id"`
	Name          string  `json:"name"`
	QualifiedName string  `json:"qualified_name"`
	Signature     string  `json:"signature"`
	Rank          float64 `json:"rank"`
}

func SearchBM25(db *sql.DB, queryStr string, project string) ([]SearchResult, error) {
	// Query FTS5 matching records and filter by project using JOIN on nodes
	sqlQuery := `
		SELECT fts.node_id, fts.name, fts.qualified_name, fts.signature, fts.rank
		FROM nodes_fts fts
		JOIN nodes n ON fts.node_id = n.id
		WHERE nodes_fts MATCH ? AND n.project = ?
		ORDER BY rank LIMIT 50;
	`
	rows, err := db.Query(sqlQuery, queryStr, project)
	if err != nil {
		return nil, fmt.Errorf("FTS5 query failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.NodeID, &r.Name, &r.QualifiedName, &r.Signature, &r.Rank); err == nil {
			results = append(results, r)
		}
	}
	return results, nil
}
