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

	// Initialize slices to empty arrays instead of null in JSON response
	graph.Nodes = make([]NodeJSON, 0)
	graph.Links = make([]LinkJSON, 0)

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

	_ = json.NewEncoder(w).Encode(graph)
}
