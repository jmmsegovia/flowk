package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

func main() {
	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "FlowK UI Server API",
			"description": "Contract between the FlowK UI and flowk run -serve-ui backend.",
			"version":     "1.0.0",
		},
		"paths": map[string]any{
			"/api/flows": map[string]any{
				"get": map[string]any{"summary": "List available flow definitions", "responses": map[string]any{"200": map[string]any{"description": "Available flows"}}},
			},
			"/api/flows/open": map[string]any{
				"post": map[string]any{"summary": "Open flow by source path", "responses": map[string]any{"200": map[string]any{"description": "Opened flow"}}},
			},
			"/api/flow": map[string]any{
				"get":  map[string]any{"summary": "Get active flow definition", "responses": map[string]any{"200": map[string]any{"description": "Flow definition"}, "204": map[string]any{"description": "No flow loaded"}}},
				"post": map[string]any{"summary": "Upload/import flow definition", "responses": map[string]any{"200": map[string]any{"description": "Imported flow"}}},
			},
			"/api/flow/notes": map[string]any{
				"get": map[string]any{"summary": "Get flow notes markdown", "responses": map[string]any{"200": map[string]any{"description": "Notes"}, "404": map[string]any{"description": "No notes available"}}},
			},
			"/api/schema": map[string]any{
				"get": map[string]any{"summary": "Get combined flow schema", "responses": map[string]any{"200": map[string]any{"description": "Schema JSON"}}},
			},
			"/api/actions/guide": map[string]any{
				"get": map[string]any{"summary": "Get actions guide", "responses": map[string]any{"200": map[string]any{"description": "Actions guide"}}},
			},
			"/api/run": map[string]any{
				"post": map[string]any{"summary": "Start flow run", "responses": map[string]any{"202": map[string]any{"description": "Run started"}, "400": map[string]any{"description": "Invalid run request"}, "409": map[string]any{"description": "Run conflict"}}},
			},
			"/api/run/stop": map[string]any{
				"post": map[string]any{"summary": "Stop active run", "responses": map[string]any{"202": map[string]any{"description": "Stop requested"}}},
			},
			"/api/run/stop-at": map[string]any{
				"post": map[string]any{"summary": "Set/clear stop-at task", "responses": map[string]any{"200": map[string]any{"description": "Stop-at updated"}}},
			},
			"/api/run/events": map[string]any{
				"get": map[string]any{"summary": "Subscribe to runtime events (SSE)", "responses": map[string]any{"200": map[string]any{"description": "text/event-stream"}}},
			},
			"/api/ui/layout": map[string]any{
				"get":    map[string]any{"summary": "Get saved layout", "responses": map[string]any{"200": map[string]any{"description": "Layout snapshot"}, "404": map[string]any{"description": "Not found"}}},
				"post":   map[string]any{"summary": "Save layout", "responses": map[string]any{"200": map[string]any{"description": "Saved"}}},
				"delete": map[string]any{"summary": "Delete saved layout", "responses": map[string]any{"204": map[string]any{"description": "Deleted"}, "404": map[string]any{"description": "Not found"}}},
			},
			"/api/ui/close-flow": map[string]any{
				"post": map[string]any{"summary": "Clear active flow from UI session", "responses": map[string]any{"200": map[string]any{"description": "Flow closed"}}},
			},
			"/api/openapi.json": map[string]any{
				"get": map[string]any{"summary": "Get API contract", "responses": map[string]any{"200": map[string]any{"description": "OpenAPI spec"}}},
			},
		},
	}

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		log.Fatalf("marshal openapi: %v", err)
	}
	data = append(data, '\n')

	out := filepath.Join("openapi.json")
	if err := os.WriteFile(out, data, 0o644); err != nil {
		log.Fatalf("write %s: %v", out, err)
	}
}
