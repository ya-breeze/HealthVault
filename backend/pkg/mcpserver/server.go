package mcpserver

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ya-breeze/healthvault/pkg/database"
)

// Handler returns an http.Handler that serves the MCP streamable HTTP endpoint.
// The endpoint is unauthenticated; authentication is handled at the MCP protocol
// level by the client (Claude Desktop, Claude Code, etc.).
func Handler(storage database.Storage) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "healthvault",
		Version: "1.0.0",
	}, &mcp.ServerOptions{Instructions: instructions})

	registerTools(server, storage)

	return mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
		return server
	}, nil)
}
