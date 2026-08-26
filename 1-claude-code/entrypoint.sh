#!/bin/sh
# Registers the chrome-devtools MCP server with Claude Code before handing
# off to claude. --autoConnect attaches to a Chrome (144+) already running
# in the environment instead of launching its own.
set -eu

if ! claude mcp get chrome-devtools >/dev/null 2>&1; then
    claude mcp add --scope user chrome-devtools -- chrome-devtools-mcp --autoConnect
fi

exec claude "$@"
