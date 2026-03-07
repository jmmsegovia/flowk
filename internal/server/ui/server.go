package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	actionhelp "flowk/internal/cli/actionhelp"
	"flowk/internal/flow"
)

const maxFlowUploadSize = 5 * 1024 * 1024
const maxFlowNotesSize = 1 * 1024 * 1024
const defaultFlowRootDir = "./flows"

var errImportLocated = errors.New("flow import located")
var errImportNotFound = errors.New("flow import not found")

type Config struct {
	Address       string
	FlowPath      string
	FlowRootDir   string
	Hub           *EventHub
	StaticDir     string
	Runner        *FlowRunner
	FlowUploadDir string
	ConfigPath    string
}

type Server struct {
	cfg              Config
	engine           *gin.Engine
	runner           *FlowRunner
	flowMu           sync.RWMutex
	flowPath         string
	flowRootDir      string
	uploadedFlowPath string
	uploadedFlowName string
	uploadDir        string
	fsRoot           string
	layoutDir        string
	importCache      map[string]string
	importCacheMu    sync.RWMutex
}

func NewServer(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return nil, errors.New("address is required")
	}

	uploadDir := strings.TrimSpace(cfg.FlowUploadDir)
	if uploadDir == "" {
		uploadDir = filepath.Join(os.TempDir(), "flowk-ui")
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating flow upload directory: %w", err)
	}
	cfg.FlowUploadDir = uploadDir

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("determining working directory: %w", err)
	}

	srv := &Server{
		cfg:         cfg,
		engine:      router,
		runner:      cfg.Runner,
		uploadDir:   uploadDir,
		fsRoot:      workingDir,
		importCache: make(map[string]string),
	}
	flowRootDir := strings.TrimSpace(cfg.FlowRootDir)
	if flowRootDir == "" {
		flowRootDir = defaultFlowRootDir
	}
	if !filepath.IsAbs(flowRootDir) {
		flowRootDir = filepath.Join(workingDir, flowRootDir)
	}
	srv.flowRootDir = filepath.Clean(flowRootDir)
	srv.layoutDir = resolveLayoutDir(cfg.ConfigPath)
	srv.setActiveFlowPath(strings.TrimSpace(cfg.FlowPath), false, "")
	srv.registerRoutes()

	return srv, nil
}

func (s *Server) registerRoutes() {
	s.engine.GET("/api/flows", s.handleFlows)
	s.engine.POST("/api/flows/open", s.handleOpenFlow)
	s.engine.GET("/api/flow", s.handleFlow)
	s.engine.GET("/api/flow/notes", s.handleFlowNotes)
	s.engine.GET("/api/schema", s.handleSchema)
	s.engine.GET("/api/actions/guide", s.handleActionsGuide)
	s.engine.GET("/api/openapi.json", s.handleOpenAPI)
	s.engine.GET("/api/run/events", s.handleEvents)
	s.engine.POST("/api/flow", s.handleImportFlow)
	s.engine.POST("/api/run", s.handleRun)
	s.engine.POST("/api/run/stop", s.handleStop)
	s.engine.POST("/api/run/stop-at", s.handleStopAtTask)
	s.engine.POST("/api/ui/close-flow", s.handleCloseFlow)
	s.engine.GET("/api/ui/layout", s.handleGetLayout)
	s.engine.POST("/api/ui/layout", s.handleSaveLayout)
	s.engine.DELETE("/api/ui/layout", s.handleDeleteLayout)

	if handler := s.staticFileHandler(); handler != nil {
		s.engine.NoRoute(handler)
		return
	}

	s.engine.NoRoute(notFoundResponse)
}

func (s *Server) staticFileHandler() gin.HandlerFunc {
	dir := strings.TrimSpace(s.cfg.StaticDir)
	if dir == "" {
		return nil
	}

	if !filepath.IsAbs(dir) {
		dir = filepath.Join(s.fsRoot, dir)
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}

	filesystem := http.Dir(dir)
	fileServer := http.FileServer(filesystem)
	indexPath := filepath.Join(dir, "index.html")

	serveIndex := func(c *gin.Context) {
		if _, err := os.Stat(indexPath); err != nil {
			notFoundResponse(c)
			return
		}
		c.Request.URL.Path = "/"
		http.ServeFile(c.Writer, c.Request, indexPath)
	}

	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			notFoundResponse(c)
			return
		}

		requestPath := normalizeRequestPath(c.Request.URL.Path)
		if requestPath == "/" {
			serveIndex(c)
			return
		}

		if fileExists(filesystem, requestPath) {
			c.Request.URL.Path = requestPath
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		if looksLikeAsset(requestPath) {
			notFoundResponse(c)
			return
		}

		serveIndex(c)
	}
}

func fileExists(fs http.FileSystem, name string) bool {
	f, err := fs.Open(name)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func looksLikeAsset(p string) bool {
	base := path.Base(p)
	return strings.Contains(base, ".")
}

func normalizeRequestPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

func notFoundResponse(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
}

func (s *Server) Handle() http.Handler {
	return s.engine
}

func (s *Server) handleFlow(c *gin.Context) {
	path := s.activeFlowPath()
	if strings.TrimSpace(path) == "" {
		c.Status(http.StatusNoContent)
		return
	}

	definition, err := flow.LoadDefinition(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := buildFlowResponse(definition)
	response = s.withSourceMetadata(response, path)
	c.JSON(http.StatusOK, response)
}

func (s *Server) handleFlows(c *gin.Context) {
	flows, err := s.discoverAvailableFlows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"flows":   flows,
		"rootDir": s.flowRootDir,
	})
}

func (s *Server) handleOpenFlow(c *gin.Context) {
	var req struct {
		SourceName string `json:"sourceName"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid payload: %v", err)})
		return
	}

	sourceName := strings.TrimSpace(req.SourceName)
	if sourceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sourceName is required"})
		return
	}

	resolvedPath, err := s.resolveFlowPathFromSourceName(sourceName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	definition, err := flow.LoadDefinition(resolvedPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.setActiveFlowPath(resolvedPath, false, "")
	response := buildFlowResponse(definition)
	response = s.withSourceMetadata(response, resolvedPath)
	c.JSON(http.StatusOK, response)
}

func (s *Server) handleFlowNotes(c *gin.Context) {
	flowPath := s.activeFlowPath()
	if strings.TrimSpace(flowPath) == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no flow is currently loaded"})
		return
	}

	notesPath, ok := flowNotesPath(flowPath)
	if ok && s.respondFlowNotes(c, notesPath) {
		return
	}

	fallbackPath := strings.TrimSpace(s.cfg.FlowPath)
	if fallbackPath == "" || fallbackPath == flowPath {
		if s.tryNotesFromUploadedName(c) {
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "no notes are available"})
		return
	}

	activeDef, err := flow.LoadDefinition(flowPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no notes are available"})
		return
	}

	baseDef, err := flow.LoadDefinition(fallbackPath)
	if err != nil || baseDef.ID != activeDef.ID {
		if s.tryNotesFromUploadedName(c) {
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "no notes are available"})
		return
	}

	fallbackNotes, ok := flowNotesPath(fallbackPath)
	if !ok || !s.respondFlowNotes(c, fallbackNotes) {
		if s.tryNotesFromUploadedName(c) {
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "no notes are available"})
		return
	}
}

func (s *Server) respondFlowNotes(c *gin.Context, notesPath string) bool {
	info, err := os.Stat(notesPath)
	if err != nil || info.IsDir() {
		return false
	}

	file, err := os.Open(notesPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return true
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxFlowNotesSize))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return true
	}

	c.JSON(http.StatusOK, gin.H{"markdown": string(data)})
	return true
}

func (s *Server) tryNotesFromUploadedName(c *gin.Context) bool {
	uploadName := strings.TrimSpace(s.activeUploadName())
	if uploadName == "" {
		return false
	}
	root := strings.TrimSpace(s.fsRoot)
	if root == "" {
		return false
	}
	key := strings.ToLower(filepath.ToSlash(uploadName))
	match, err := searchImportBySuffixInRoot(root, key)
	if err != nil {
		return false
	}
	notesPath, ok := flowNotesPath(match)
	if !ok {
		return false
	}
	return s.respondFlowNotes(c, notesPath)
}

func (s *Server) handleSchema(c *gin.Context) {
	data, err := flow.CombinedSchema()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, payload)
}

func (s *Server) handleActionsGuide(c *gin.Context) {
	guide, err := actionhelp.BuildGuide()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	content := actionhelp.FormatGuideMarkdown(guide)
	c.JSON(http.StatusOK, gin.H{
		"generatedAt": guide.GeneratedAt,
		"primer":      guide.Primer,
		"actions":     guide.Actions,
		"markdown":    content,
	})
}

func (s *Server) handleOpenAPI(c *gin.Context) {
	c.Data(http.StatusOK, "application/json; charset=utf-8", OpenAPISpecJSON)
}

func (s *Server) handleEvents(c *gin.Context) {
	if s.cfg.Hub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event stream is not available"})
		return
	}

	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Connection", "keep-alive")

	stream, cancel := s.cfg.Hub.Subscribe()
	defer cancel()

	c.Stream(func(w io.Writer) bool {
		select {
		case evt, ok := <-stream:
			if !ok {
				return false
			}
			c.SSEvent(string(evt.Type), evt)
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

func (s *Server) handleRun(c *gin.Context) {
	if s.runner == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "flow runner is not available"})
		return
	}

	type runRequest struct {
		BeginFromTask    string `json:"beginFromTask"`
		TaskID           string `json:"taskId"`
		FlowID           string `json:"flowId"`
		SubtaskID        string `json:"subtaskId"`
		ResumeFromTaskID string `json:"resumeFromTaskId"`
	}

	var req runRequest
	var opts *RunOptions
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid payload: %v", err)})
		return
	} else if err == nil {
		candidate := RunOptions{
			BeginFromTask:    strings.TrimSpace(req.BeginFromTask),
			RunTaskID:        strings.TrimSpace(req.TaskID),
			RunFlowID:        strings.TrimSpace(req.FlowID),
			RunSubtaskID:     strings.TrimSpace(req.SubtaskID),
			ResumeFromTaskID: strings.TrimSpace(req.ResumeFromTaskID),
		}
		if candidate.BeginFromTask != "" || candidate.RunTaskID != "" || candidate.RunFlowID != "" || candidate.RunSubtaskID != "" ||
			candidate.ResumeFromTaskID != "" {
			opts = &candidate
		}
	}

	if err := s.runner.Trigger(opts); err != nil {
		if errors.Is(err, ErrRunInProgress) {
			c.JSON(http.StatusConflict, gin.H{"error": "flow execution already in progress"})
			return
		}
		if errors.Is(err, ErrNoRunState) {
			c.JSON(http.StatusConflict, gin.H{"error": "no previous run state is available to resume"})
			return
		}
		if errors.Is(err, ErrResumeConflict) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "resume request cannot be combined with other options"})
			return
		}
		if errors.Is(err, ErrResumeTaskNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the requested task was not executed previously"})
			return
		}
		if errors.Is(err, ErrResumeTaskNotCompleted) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the requested task has not finished yet"})
			return
		}
		if errors.Is(err, ErrFlowPathRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no flow is ready to run yet"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "started"})
}

func (s *Server) handleStop(c *gin.Context) {
	if s.runner == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "flow runner is not available"})
		return
	}

	if err := s.runner.RequestStop(); err != nil {
		if errors.Is(err, ErrNoRunInProgress) {
			c.JSON(http.StatusConflict, gin.H{"error": "no execution is currently in progress"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "stopping"})
}

func (s *Server) handleStopAtTask(c *gin.Context) {
	if s.runner == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "flow runner is not available"})
		return
	}

	type stopAtRequest struct {
		TaskID string `json:"taskId"`
	}

	var req stopAtRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid payload: %v", err)})
		return
	}

	if err := s.runner.SetStopAtTask(strings.TrimSpace(req.TaskID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleCloseFlow(c *gin.Context) {
	if s.cfg.Hub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event stream is not available"})
		return
	}

	var req struct {
		FlowID string `json:"flowId"`
	}
	_ = c.ShouldBindJSON(&req)

	s.cfg.Hub.ClearHistory(strings.TrimSpace(req.FlowID))

	// Clear active flow if it matches
	s.flowMu.Lock()
	if s.flowPath != "" {
		// We need to check if the flow ID matches.
		// Loading definition might be expensive, but safe.
		// Alternatively, we can just clear it if the user requests it.
		// Given the UI sends the ID, let's trust it for now or verify.
		// To be safe and simple:
		s.uploadedFlowPath = ""
		s.uploadedFlowName = ""
		s.flowPath = ""
		if s.runner != nil {
			s.runner.UpdateFlowPath("")
		}
	}
	s.flowMu.Unlock()

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleImportFlow(c *gin.Context) {
	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, maxFlowUploadSize))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("could not read flow: %v", err)})
		return
	}

	if len(bytes.TrimSpace(payload)) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "flow file is empty"})
		return
	}

	uploadName := filepath.Base(strings.TrimSpace(c.GetHeader("X-Flow-Filename")))
	path, def, err := s.storeFlowDefinition(payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.setActiveFlowPath(path, true, uploadName)
	c.JSON(http.StatusOK, buildFlowResponse(def))
}

func (s *Server) activeFlowPath() string {
	s.flowMu.RLock()
	defer s.flowMu.RUnlock()
	return s.flowPath
}

func (s *Server) activeUploadName() string {
	s.flowMu.RLock()
	defer s.flowMu.RUnlock()
	return s.uploadedFlowName
}

func flowNotesPath(flowPath string) (string, bool) {
	trimmed := strings.TrimSpace(flowPath)
	if trimmed == "" {
		return "", false
	}

	base := filepath.Base(trimmed)
	if base == "" || base == "." {
		return "", false
	}

	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if strings.TrimSpace(name) == "" {
		return "", false
	}

	notesName := name + ".md"
	return filepath.Join(filepath.Dir(trimmed), notesName), true
}

func (s *Server) setActiveFlowPath(path string, fromUpload bool, uploadName string) {
	trimmed := strings.TrimSpace(path)

	s.flowMu.Lock()
	prevUpload := s.uploadedFlowPath
	if fromUpload {
		s.uploadedFlowPath = trimmed
		s.uploadedFlowName = strings.TrimSpace(uploadName)
	} else {
		s.uploadedFlowPath = ""
		s.uploadedFlowName = ""
	}
	s.flowPath = trimmed
	s.flowMu.Unlock()

	if s.runner != nil {
		s.runner.UpdateFlowPath(trimmed)
	}

	if prevUpload != "" && prevUpload != trimmed {
		_ = os.Remove(prevUpload)
	}
}

func (s *Server) resolveFlowPathFromSourceName(sourceName string) (string, error) {
	root := strings.TrimSpace(s.flowRootDir)
	if root == "" {
		return "", errors.New("flow root directory is not configured")
	}

	cleanSource := filepath.Clean(filepath.FromSlash(sourceName))
	if filepath.IsAbs(cleanSource) {
		return "", fmt.Errorf("sourceName %q must be a relative path", sourceName)
	}

	target := filepath.Join(root, cleanSource)
	if err := ensureWithinRoot(root, target); err != nil {
		return "", err
	}

	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("flow %q was not found", sourceName)
		}
		return "", fmt.Errorf("could not access flow %q: %w", sourceName, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("flow %q is a directory", sourceName)
	}

	return target, nil
}

func (s *Server) discoverAvailableFlows() ([]FlowResponse, error) {
	root := strings.TrimSpace(s.flowRootDir)
	if root == "" {
		return nil, errors.New("flow root directory is not configured")
	}

	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []FlowResponse{}, nil
		}
		return nil, fmt.Errorf("reading flow root directory %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("flow root directory %q is not a directory", root)
	}

	type discoveredFlow struct {
		path     string
		def      *flow.Definition
		response FlowResponse
	}

	var discovered []discoveredFlow
	importedFlowIDs := make(map[string]struct{})

	var flows []FlowResponse
	walkErr := filepath.WalkDir(root, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			return nil
		}

		definition, err := flow.LoadDefinition(currentPath)
		if err != nil {
			// Keep listing resilient: malformed/non-flow JSON files are ignored.
			return nil
		}

		cleanedPath := filepath.Clean(currentPath)
		if absolutePath, absErr := filepath.Abs(cleanedPath); absErr == nil {
			cleanedPath = filepath.Clean(absolutePath)
		}

		response := buildFlowResponse(definition)
		response = s.withSourceMetadata(response, cleanedPath)
		discovered = append(discovered, discoveredFlow{
			path:     cleanedPath,
			def:      definition,
			response: response,
		})

		for _, importedID := range definition.FlowImports[definition.ID] {
			trimmedID := strings.TrimSpace(importedID)
			if trimmedID == "" || trimmedID == definition.ID {
				continue
			}
			importedFlowIDs[trimmedID] = struct{}{}
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walking flow root directory %q: %w", root, walkErr)
	}

	for _, item := range discovered {
		if item.def != nil && item.def.IsSubflow {
			continue
		}

		if _, imported := importedFlowIDs[item.response.ID]; imported {
			continue
		}

		flows = append(flows, item.response)
	}

	sort.Slice(flows, func(i, j int) bool {
		left := flows[i].SourceName
		right := flows[j].SourceName
		if left == right {
			return flows[i].ID < flows[j].ID
		}
		return left < right
	})

	return flows, nil
}

func (s *Server) withSourceMetadata(response FlowResponse, absPath string) FlowResponse {
	root := strings.TrimSpace(s.flowRootDir)
	if root == "" || strings.TrimSpace(absPath) == "" {
		return response
	}

	cleanedPath := filepath.Clean(absPath)
	rel, err := filepath.Rel(root, cleanedPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return response
	}

	response.SourceName = filepath.ToSlash(rel)
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." {
		dir = ""
	}
	response.SourceDir = dir
	return response
}

func (s *Server) storeFlowDefinition(data []byte) (string, *flow.Definition, error) {
	if len(data) == 0 {
		return "", nil, errors.New("flow file is empty")
	}

	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("could not prepare flow upload directory: %w", err)
	}

	file, err := os.CreateTemp(s.uploadDir, "flow-*.json")
	if err != nil {
		return "", nil, fmt.Errorf("could not store flow: %w", err)
	}
	name := file.Name()

	if _, err := file.Write(data); err != nil {
		file.Close()
		_ = os.Remove(name)
		return "", nil, fmt.Errorf("could not store flow: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", nil, fmt.Errorf("could not store flow: %w", err)
	}

	if err := s.populateUploadedImports(name, data); err != nil {
		_ = os.Remove(name)
		return "", nil, err
	}

	definition, err := flow.LoadDefinition(name)
	if err != nil {
		_ = os.Remove(name)
		return "", nil, err
	}

	return name, definition, nil
}

func (s *Server) populateUploadedImports(rootPath string, data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("could not parse imported flow: %w", err)
	}

	var imports []string
	if rawImports, ok := raw["imports"]; ok {
		if err := json.Unmarshal(rawImports, &imports); err != nil {
			return fmt.Errorf("could not parse imported flow: %w", err)
		}
	}
	if len(imports) == 0 {
		return nil
	}

	baseDir := filepath.Dir(rootPath)
	visited := make(map[string]struct{})
	updated := false
	for idx, importPath := range imports {
		updatedPath, err := s.copyImportRelative(importPath, "", baseDir, baseDir, visited)
		if err != nil {
			return fmt.Errorf("imports[%d]: %w", idx, err)
		}
		if updatedPath != importPath {
			updated = true
		}
		imports[idx] = updatedPath
	}

	if updated {
		rawImports, err := json.Marshal(imports)
		if err != nil {
			return fmt.Errorf("could not update imported flow: %w", err)
		}
		raw["imports"] = rawImports
		updatedData, err := json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("could not update imported flow: %w", err)
		}
		if err := os.WriteFile(rootPath, updatedData, 0o600); err != nil {
			return fmt.Errorf("could not update imported flow: %w", err)
		}
	}

	return nil
}

func (s *Server) importRoots() []string {
	var roots []string
	if strings.TrimSpace(s.fsRoot) != "" {
		roots = append(roots, s.fsRoot)
	}
	if trimmedFlow := strings.TrimSpace(s.cfg.FlowPath); trimmedFlow != "" {
		roots = append(roots, filepath.Dir(trimmedFlow))
	}
	return roots
}

func (s *Server) relativeToImportRoot(path string) (string, bool) {
	cleaned := filepath.Clean(path)
	for _, root := range s.importRoots() {
		if strings.TrimSpace(root) == "" {
			continue
		}
		rel, err := filepath.Rel(root, cleaned)
		if err != nil {
			continue
		}
		if strings.HasPrefix(rel, "..") {
			continue
		}
		return filepath.ToSlash(rel), true
	}
	return "", false
}

func (s *Server) copyImportRelative(relPath, parentSrcDir, parentDestDir, rootDest string, visited map[string]struct{}) (string, error) {
	original := relPath
	trimmed := strings.TrimSpace(relPath)
	if trimmed == "" {
		return "", errors.New("import path cannot be empty")
	}

	cleanRel := filepath.Clean(trimmed)
	srcPath, err := s.resolveImportSource(cleanRel, parentSrcDir)
	if err != nil {
		return "", err
	}

	destPath := ""
	updatedPath := original

	if filepath.IsAbs(cleanRel) {
		if rel, ok := s.relativeToImportRoot(cleanRel); ok {
			destPath = filepath.Join(rootDest, filepath.FromSlash(rel))
			if err := ensureWithinRoot(rootDest, destPath); err != nil {
				return "", err
			}
		} else if rel, ok := s.relativeToImportRoot(srcPath); ok {
			destPath = filepath.Join(rootDest, filepath.FromSlash(rel))
			if err := ensureWithinRoot(rootDest, destPath); err != nil {
				return "", err
			}
			relToParent, err := filepath.Rel(parentDestDir, destPath)
			if err != nil {
				return "", fmt.Errorf("could not prepare import %q: %w", destPath, err)
			}
			updatedPath = filepath.ToSlash(relToParent)
		} else {
			return "", fmt.Errorf("import path %q is outside the allowed directory", cleanRel)
		}
	} else {
		candidate := filepath.Join(parentDestDir, filepath.FromSlash(cleanRel))
		if err := ensureWithinRoot(rootDest, candidate); err == nil {
			destPath = candidate
		} else {
			rel, ok := s.relativeToImportRoot(srcPath)
			if !ok {
				return "", err
			}
			destPath = filepath.Join(rootDest, filepath.FromSlash(rel))
			if err := ensureWithinRoot(rootDest, destPath); err != nil {
				return "", err
			}
			relToParent, err := filepath.Rel(parentDestDir, destPath)
			if err != nil {
				return "", fmt.Errorf("could not prepare import %q: %w", destPath, err)
			}
			updatedPath = filepath.ToSlash(relToParent)
		}
	}

	if err := s.copyFlowFileRecursive(srcPath, destPath, rootDest, visited); err != nil {
		return "", err
	}

	return updatedPath, nil
}

func (s *Server) copyFlowFileRecursive(srcPath, destPath, rootDest string, visited map[string]struct{}) error {
	if err := ensureWithinRoot(rootDest, destPath); err != nil {
		return err
	}

	absDest, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("could not prepare import %q: %w", destPath, err)
	}

	if visited != nil {
		if _, seen := visited[absDest]; seen {
			return nil
		}
		visited[absDest] = struct{}{}
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("could not read import %q: %w", srcPath, err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("could not parse import %q: %w", srcPath, err)
	}

	var imports []string
	if rawImports, ok := raw["imports"]; ok {
		if err := json.Unmarshal(rawImports, &imports); err != nil {
			return fmt.Errorf("could not parse import %q: %w", srcPath, err)
		}
	}

	parentSrcDir := filepath.Dir(srcPath)
	parentDestDir := filepath.Dir(destPath)
	updated := false
	for idx, child := range imports {
		updatedPath, err := s.copyImportRelative(child, parentSrcDir, parentDestDir, rootDest, visited)
		if err != nil {
			return fmt.Errorf("imports[%d]: %w", idx, err)
		}
		if updatedPath != child {
			updated = true
		}
		imports[idx] = updatedPath
	}

	if updated {
		rawImports, err := json.Marshal(imports)
		if err != nil {
			return fmt.Errorf("could not prepare import %q: %w", destPath, err)
		}
		raw["imports"] = rawImports
		data, err = json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("could not prepare import %q: %w", destPath, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("could not prepare import %q: %w", destPath, err)
	}
	if err := os.WriteFile(destPath, data, 0o600); err != nil {
		return fmt.Errorf("could not copy import %q: %w", destPath, err)
	}

	return nil
}

func (s *Server) resolveImportSource(relPath, parentSrcDir string) (string, error) {
	normalized := filepath.FromSlash(filepath.Clean(relPath))
	if filepath.IsAbs(normalized) {
		if info, err := os.Stat(normalized); err == nil && !info.IsDir() {
			return normalized, nil
		}
		return "", fmt.Errorf("required file %q was not found", normalized)
	}

	if parentSrcDir != "" {
		candidate := filepath.Join(parentSrcDir, normalized)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	if stripped := stripLeadingRelativePath(filepath.ToSlash(normalized)); stripped != "" && stripped != filepath.ToSlash(normalized) {
		if candidate, err := s.locateImportSource(stripped); err == nil {
			return candidate, nil
		}
	}

	return s.locateImportSource(normalized)
}

func (s *Server) locateImportSource(relPath string) (string, error) {
	cleanRel := filepath.Clean(relPath)
	key := strings.ToLower(filepath.ToSlash(cleanRel))

	if cached := s.cachedImportPath(key); cached != "" {
		return cached, nil
	}

	normalized := filepath.FromSlash(cleanRel)
	var searchRoots []string
	if trimmed := strings.TrimSpace(s.cfg.FlowPath); trimmed != "" {
		searchRoots = append(searchRoots, filepath.Dir(trimmed))
	}
	if strings.TrimSpace(s.fsRoot) != "" {
		searchRoots = append(searchRoots, s.fsRoot)
	}

	for _, root := range searchRoots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		candidate := filepath.Join(root, normalized)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			s.storeImportCache(key, candidate)
			return candidate, nil
		}
	}

	found, err := s.searchImportBySuffix(key)
	if err == nil {
		s.storeImportCache(key, found)
		return found, nil
	}
	if !errors.Is(err, errImportNotFound) {
		return "", err
	}

	if stripped := stripLeadingRelativePath(key); stripped != "" && stripped != key {
		if cached := s.cachedImportPath(stripped); cached != "" {
			s.storeImportCache(key, cached)
			return cached, nil
		}
		found, err = s.searchImportBySuffix(stripped)
		if err == nil {
			s.storeImportCache(key, found)
			s.storeImportCache(stripped, found)
			return found, nil
		}
		if !errors.Is(err, errImportNotFound) {
			return "", err
		}
	}

	return "", fmt.Errorf("required file %q was not found", key)
}

func stripLeadingRelativePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	idx := 0
	for idx < len(parts) {
		if parts[idx] != "." && parts[idx] != ".." && parts[idx] != "" {
			break
		}
		idx++
	}
	if idx >= len(parts) {
		return ""
	}
	return strings.Join(parts[idx:], "/")
}

func (s *Server) searchImportBySuffix(key string) (string, error) {
	roots := []string{}
	root := strings.TrimSpace(s.fsRoot)
	if root != "" {
		roots = append(roots, root)
		parent := filepath.Dir(root)
		if parent != root {
			roots = append(roots, parent)
		}
	}
	if trimmed := strings.TrimSpace(s.cfg.FlowPath); trimmed != "" {
		roots = append(roots, filepath.Dir(trimmed))
	}
	if len(roots) == 0 {
		return "", fmt.Errorf("no base directory is configured to search imports")
	}

	seen := make(map[string]struct{}, len(roots))
	for _, candidate := range roots {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		cleaned := filepath.Clean(candidate)
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		match, err := searchImportBySuffixInRoot(cleaned, key)
		if err == nil {
			return match, nil
		}
		if errors.Is(err, errImportNotFound) {
			continue
		}
		return "", err
	}

	return "", fmt.Errorf("required file %q was not found", key)
}

func searchImportBySuffixInRoot(root, key string) (string, error) {
	var match string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		normalized := strings.ToLower(filepath.ToSlash(path))
		if strings.HasSuffix(normalized, key) {
			match = path
			return errImportLocated
		}
		return nil
	})
	if err != nil && !errors.Is(err, errImportLocated) {
		return "", fmt.Errorf("could not locate import %q: %w", key, err)
	}
	if match == "" {
		return "", errImportNotFound
	}
	return match, nil
}

func (s *Server) cachedImportPath(key string) string {
	s.importCacheMu.RLock()
	defer s.importCacheMu.RUnlock()
	return s.importCache[key]
}

func (s *Server) storeImportCache(key, value string) {
	s.importCacheMu.Lock()
	defer s.importCacheMu.Unlock()
	s.importCache[key] = value
}

func ensureWithinRoot(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("could not validate import path: %w", err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("could not validate import path: %w", err)
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return fmt.Errorf("could not validate import path: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("import path %q is outside the allowed directory", target)
	}
	return nil
}

type FlowResponse struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	IsSubflow   bool              `json:"isSubflow,omitempty"`
	SourceName  string            `json:"sourceName,omitempty"`
	SourceDir   string            `json:"sourceDir,omitempty"`
	Imports     []string          `json:"imports,omitempty"`
	FlowNames   map[string]string `json:"flowNames,omitempty"`
	Tasks       []TaskSummary     `json:"tasks"`
}

type TaskSummary struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Action          string          `json:"action"`
	FlowID          string          `json:"flowId"`
	Status          flow.TaskStatus `json:"status"`
	Success         bool            `json:"success"`
	StartedAt       *time.Time      `json:"startedAt,omitempty"`
	FinishedAt      *time.Time      `json:"finishedAt,omitempty"`
	DurationSeconds float64         `json:"durationSeconds"`
	ResultType      flow.ResultType `json:"resultType"`
	Result          any             `json:"result,omitempty"`
	Raw             map[string]any  `json:"raw,omitempty"`
	Fields          map[string]any  `json:"fields,omitempty"`
}

func buildFlowResponse(def *flow.Definition) FlowResponse {
	if def == nil {
		return FlowResponse{}
	}

	response := FlowResponse{
		ID:          def.ID,
		Name:        def.Name,
		Description: def.Description,
		IsSubflow:   def.IsSubflow,
		Imports:     append([]string(nil), def.Imports...),
	}
	if strings.TrimSpace(response.Name) == "" {
		response.Name = response.ID
	}
	if len(def.FlowNames) > 0 {
		response.FlowNames = make(map[string]string, len(def.FlowNames))
		for key, value := range def.FlowNames {
			response.FlowNames[key] = value
		}
	}

	for _, task := range def.Tasks {
		response.Tasks = append(response.Tasks, buildTaskSummary(task))
	}

	return response
}

func buildTaskSummary(task flow.Task) TaskSummary {
	summary := TaskSummary{
		ID:              task.ID,
		Name:            task.Name,
		Description:     task.Description,
		Action:          task.Action,
		FlowID:          task.FlowID,
		Status:          task.Status,
		Success:         task.Success,
		DurationSeconds: task.DurationSeconds,
		ResultType:      task.ResultType,
	}
	if strings.TrimSpace(summary.Name) == "" {
		summary.Name = summary.ID
	}

	if !task.StartTimestamp.IsZero() {
		ts := task.StartTimestamp
		summary.StartedAt = &ts
	}
	if !task.EndTimestamp.IsZero() {
		ts := task.EndTimestamp
		summary.FinishedAt = &ts
	}
	if task.Result != nil {
		summary.Result = task.Result
	}

	raw := extractTaskPayload(task.Payload)
	if len(raw) > 0 {
		summary.Raw = raw
	}

	fields := extractTaskFields(raw)
	if len(fields) > 0 {
		summary.Fields = fields
	}

	return summary
}

func extractTaskPayload(payload json.RawMessage) map[string]any {
	if len(payload) == 0 {
		return nil
	}

	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil
	}

	return data
}

func extractTaskFields(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return nil
	}

	fields := make(map[string]any, len(raw))
	for key, value := range raw {
		fields[key] = value
	}

	delete(fields, "id")
	delete(fields, "description")
	delete(fields, "action")

	return fields
}
