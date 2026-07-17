# Workspace Rules: CodeAtlas MCP Server

This file outlines the workspace-specific configuration rules for AI agents collaborating on this codebase.

## Database Location
- **Centralized Database**: The SQLite database file (`graph.db`) is stored globally in the user's home cache directory (`~/.cache/codebase-memory-mcp/graph.db`).
- **Default Resolution**: When compiling or launching the server, the database path defaults to `~/.cache/codebase-memory-mcp/graph.db`.
- **Shared Context**: All projects indexed by this server share this centralized database, isolated by their respective project identifiers.
