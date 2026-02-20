package gcloudstorage

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    fp "path/filepath"
    pth "path"
    "strings"
    "time"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

type Operation string

const (
	OperationCopy     Operation = "CP"
	OperationMove     Operation = "MV"
	OperationRemove   Operation = "RM"
	OperationList     Operation = "LS"
	OperationAuthInfo Operation = "AUTH_INFO"
)

type Payload struct {
	Operation Operation      `json:"operation"`
	Copy      *CopyPayload   `json:"copy,omitempty"`
	Move      *MovePayload   `json:"move,omitempty"`
	Remove    *RemovePayload `json:"remove,omitempty"`
	List      *ListPayload   `json:"list,omitempty"`
}

type CopyPayload struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Recursive   bool   `json:"recursive,omitempty"`
}

type MovePayload struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Recursive   bool   `json:"recursive,omitempty"`
}

type RemovePayload struct {
	Targets   []string `json:"targets"`
	Recursive bool     `json:"recursive,omitempty"`
}

type ListPayload struct {
	Target    string `json:"target"`
	Recursive bool   `json:"recursive,omitempty"`
}

type CopyResult struct {
	Entries []CopyEntry `json:"entries"`
}

type CopyEntry struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Copied      bool   `json:"copied"`
	Skipped     string `json:"skipped,omitempty"`
}

type MoveResult struct {
	Entries []MoveEntry `json:"entries"`
}

type MoveEntry struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Moved       bool   `json:"moved"`
	Skipped     string `json:"skipped,omitempty"`
}

type RemoveResult struct {
	Entries []RemoveEntry `json:"entries"`
}

type RemoveEntry struct {
	Target  string `json:"target"`
	Deleted bool   `json:"deleted"`
	Message string `json:"message,omitempty"`
}

type ListResult struct {
	Target   string         `json:"target"`
	Exists   bool           `json:"exists"`
	Objects  []ListedObject `json:"objects,omitempty"`
	Prefixes []string       `json:"prefixes,omitempty"`
	Message  string         `json:"message,omitempty"`
}

type ListedObject struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	Updated      time.Time `json:"updated"`
	ContentType  string    `json:"contentType,omitempty"`
	StorageClass string    `json:"storageClass,omitempty"`
}

type AuthInfo struct {
	ProjectID string   `json:"projectId,omitempty"`
	Account   string   `json:"account,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	Source    string   `json:"source,omitempty"`
}

type ListResponse struct {
	List ListResult `json:"list"`
}

type AuthInfoResult struct {
	Info AuthInfo `json:"info"`
}

type serviceFactory func(ctx context.Context) (Service, error)

type Service interface {
	Close() error
	ObjectExists(ctx context.Context, path StoragePath) (bool, error)
	CopyObject(ctx context.Context, src, dst StoragePath) error
	DeleteObject(ctx context.Context, path StoragePath) error
	List(ctx context.Context, path StoragePath, recursive bool) (ServiceListResult, error)
	FetchAuthInfo(ctx context.Context) (AuthInfo, error)
}

type ServiceListResult struct {
	Exists   bool
	Objects  []ObjectAttrs
	Prefixes []string
}

type ObjectAttrs struct {
	Name         string
	Size         int64
	Updated      time.Time
	ContentType  string
	StorageClass string
}

type StoragePath struct {
	Bucket string
	Object string
}

func (p *Payload) Validate() error {
	switch strings.ToUpper(string(p.Operation)) {
	case string(OperationCopy):
		if p.Copy == nil {
			return errors.New("copy payload is required for CP operation")
		}
		return p.Copy.Validate()
	case string(OperationMove):
		if p.Move == nil {
			return errors.New("move payload is required for MV operation")
		}
		return p.Move.Validate()
	case string(OperationRemove):
		if p.Remove == nil {
			return errors.New("remove payload is required for RM operation")
		}
		return p.Remove.Validate()
	case string(OperationList):
		if p.List == nil {
			return errors.New("list payload is required for LS operation")
		}
		return p.List.Validate()
	case string(OperationAuthInfo):
		return nil
	default:
		if strings.TrimSpace(string(p.Operation)) == "" {
			return errors.New("operation is required")
		}
		return fmt.Errorf("unsupported operation %q", p.Operation)
	}
}

func (c *CopyPayload) Validate() error {
	if strings.TrimSpace(c.Source) == "" {
		return errors.New("source is required")
	}
	if strings.TrimSpace(c.Destination) == "" {
		return errors.New("destination is required")
	}
	return nil
}

func (m *MovePayload) Validate() error {
	if strings.TrimSpace(m.Source) == "" {
		return errors.New("source is required")
	}
	if strings.TrimSpace(m.Destination) == "" {
		return errors.New("destination is required")
	}
	return nil
}

func (r *RemovePayload) Validate() error {
	if len(r.Targets) == 0 {
		return errors.New("at least one target is required")
	}
	for i := range r.Targets {
		if strings.TrimSpace(r.Targets[i]) == "" {
			return fmt.Errorf("targets[%d] cannot be empty", i)
		}
	}
	return nil
}

func (l *ListPayload) Validate() error {
	if strings.TrimSpace(l.Target) == "" {
		return errors.New("target is required")
	}
	return nil
}

type action struct {
	factory serviceFactory
}

func init() {
	registry.Register(NewAction())
}

func NewAction() registry.Action {
	return action{factory: newGCSService}
}

func (a action) Name() string {
	return "GCLOUD_STORAGE"
}

func (a action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	var cfg Payload
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return registry.Result{}, fmt.Errorf("decoding GCLOUD_STORAGE payload: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return registry.Result{}, err
	}

	service, err := a.factory(ctx)
	if err != nil {
		return registry.Result{}, fmt.Errorf("initializing storage client: %w", err)
	}
	defer service.Close()

	op := Operation(strings.ToUpper(string(cfg.Operation)))

	switch op {
	case OperationCopy:
		result, err := executeCopy(ctx, service, cfg.Copy, execCtx)
		if err != nil {
			return registry.Result{}, err
		}
		return registry.Result{Value: result, Type: flow.ResultTypeJSON}, nil
	case OperationMove:
		result, err := executeMove(ctx, service, cfg.Move, execCtx)
		if err != nil {
			return registry.Result{}, err
		}
		return registry.Result{Value: result, Type: flow.ResultTypeJSON}, nil
	case OperationRemove:
		result, err := executeRemove(ctx, service, cfg.Remove, execCtx)
		if err != nil {
			return registry.Result{}, err
		}
		return registry.Result{Value: result, Type: flow.ResultTypeJSON}, nil
	case OperationList:
		result, err := executeList(ctx, service, cfg.List, execCtx)
		if err != nil {
			return registry.Result{}, err
		}
		return registry.Result{Value: result, Type: flow.ResultTypeJSON}, nil
	case OperationAuthInfo:
		result, err := executeAuthInfo(ctx, service, execCtx)
		if err != nil {
			return registry.Result{}, err
		}
		return registry.Result{Value: result, Type: flow.ResultTypeJSON}, nil
	default:
		return registry.Result{}, fmt.Errorf("unsupported operation %q", cfg.Operation)
	}
}

func executeCopy(ctx context.Context, service Service, cfg *CopyPayload, execCtx *registry.ExecutionContext) (CopyResult, error) {
    // Determine if destination is GCS or local path
    dstIsGCS := strings.HasPrefix(strings.TrimSpace(cfg.Destination), "gs://")

    // Parse source GCS path
    srcPath, err := parseGCSPath(cfg.Source)
    if err != nil {
        return CopyResult{}, err
    }

    // Detect glob in source and derive base prefix
    hasGlob := strings.ContainsAny(srcPath.Object, "*?[")
    basePrefix := srcPath.Object
    pattern := ""
    listPath := srcPath
    recursive := cfg.Recursive
    if hasGlob {
        wc := strings.IndexAny(srcPath.Object, "*?[")
        slash := strings.LastIndex(srcPath.Object[:wc], "/")
        if slash >= 0 {
            basePrefix = srcPath.Object[:slash+1]
            pattern = srcPath.Object[slash+1:]
        } else {
            basePrefix = ""
            pattern = srcPath.Object
        }
        listPath.Object = basePrefix
        recursive = true
    }

    // If destination is GCS, parse it
    var dstPath StoragePath
    if dstIsGCS {
        dstPath, err = parseGCSPath(cfg.Destination)
        if err != nil {
            return CopyResult{}, err
        }
    }

    entries := make([]CopyEntry, 0)

    // Helper to append entry with logging
    addEntry := func(entry CopyEntry) {
        if execCtx != nil && execCtx.Logger != nil && entry.Copied {
            execCtx.Logger.Printf("Copied %s to %s", entry.Source, entry.Destination)
        }
        entries = append(entries, entry)
    }

    // Recursive or prefix/glob copy path
    if recursive || strings.HasSuffix(srcPath.Object, "/") || srcPath.Object == "" || hasGlob {
        list, err := service.List(ctx, listPath, true)
        if err != nil {
            return CopyResult{}, err
        }
        if !list.Exists {
            return CopyResult{}, fmt.Errorf("source %s does not exist", cfg.Source)
        }
        prefix := ensureTrailingSlash(basePrefix)
        destPrefix := ""
        if dstIsGCS {
            destPrefix = ensureTrailingSlash(dstPath.Object)
        }
        for _, obj := range list.Objects {
            // Filter by glob if needed
            if hasGlob {
                rel := strings.TrimPrefix(obj.Name, prefix)
                match, _ := pth.Match(pattern, rel)
                if !match {
                    continue
                }
            }
            relative := strings.TrimPrefix(obj.Name, prefix)
            srcObj := StoragePath{Bucket: srcPath.Bucket, Object: obj.Name}

            if dstIsGCS {
                destination := dstPath
                destination.Object = destPrefix + relative
                entry := CopyEntry{Source: buildGCSURI(srcPath.Bucket, obj.Name), Destination: buildGCSURI(destination.Bucket, destination.Object)}
                if err := service.CopyObject(ctx, srcObj, destination); err != nil {
                    entry.Copied = false
                    entry.Skipped = err.Error()
                } else {
                    entry.Copied = true
                }
                addEntry(entry)
            } else {
                // Local destination download
                destPathLocal := cfg.Destination
                // Treat destination as directory when empty, ".", or ends with separator
                if destPathLocal == "" || destPathLocal == "." || strings.HasSuffix(destPathLocal, string(fp.Separator)) {
                    destPathLocal = fp.Join(destPathLocal, fp.FromSlash(relative))
                } else {
                    // If destination is a directory, join; else write as provided
                    info, err := os.Stat(destPathLocal)
                    if err == nil && info.IsDir() {
                        destPathLocal = fp.Join(destPathLocal, fp.FromSlash(relative))
                    }
                }
                if err := downloadObjectToFile(ctx, service, srcObj, destPathLocal); err != nil {
                    addEntry(CopyEntry{Source: buildGCSURI(srcObj.Bucket, srcObj.Object), Destination: destPathLocal, Copied: false, Skipped: err.Error()})
                } else {
                    addEntry(CopyEntry{Source: buildGCSURI(srcObj.Bucket, srcObj.Object), Destination: destPathLocal, Copied: true})
                }
            }
        }
        if len(entries) == 0 {
            return CopyResult{}, fmt.Errorf("no objects found under %s", cfg.Source)
        }
        return CopyResult{Entries: entries}, nil
    }

    // Single object copy
    exists, err := service.ObjectExists(ctx, srcPath)
    if err != nil {
        return CopyResult{}, err
    }
    entry := CopyEntry{Source: cfg.Source, Destination: cfg.Destination}
    if !exists {
        entry.Copied = false
        entry.Skipped = "source not found"
        addEntry(entry)
        return CopyResult{Entries: entries}, nil
    }
    if dstIsGCS {
        if err := service.CopyObject(ctx, srcPath, dstPath); err != nil {
            entry.Copied = false
            entry.Skipped = err.Error()
        } else {
            entry.Copied = true
        }
        addEntry(entry)
        return CopyResult{Entries: entries}, nil
    }
    // Single object download to local path
    destLocal := cfg.Destination
    // If destination is a directory, place with same base name
    if destLocal == "" || destLocal == "." || strings.HasSuffix(destLocal, string(fp.Separator)) {
        destLocal = fp.Join(destLocal, fp.Base(srcPath.Object))
    } else {
        if info, err := os.Stat(destLocal); err == nil && info.IsDir() {
            destLocal = fp.Join(destLocal, fp.Base(srcPath.Object))
        }
    }
    if err := downloadObjectToFile(ctx, service, srcPath, destLocal); err != nil {
        entry.Copied = false
        entry.Skipped = err.Error()
        entry.Destination = destLocal
    } else {
        entry.Copied = true
        entry.Destination = destLocal
    }
    addEntry(entry)
    return CopyResult{Entries: entries}, nil
}

func downloadObjectToFile(ctx context.Context, svc Service, src StoragePath, destPath string) error {
    // Access underlying client
    gs, ok := svc.(*gcsService)
    if !ok || gs.client == nil {
        return fmt.Errorf("unsupported service for local download")
    }
    // Ensure directory exists
    if err := os.MkdirAll(fp.Dir(destPath), 0o755); err != nil {
        return fmt.Errorf("create dir: %w", err)
    }
    rc, err := gs.client.Bucket(src.Bucket).Object(src.Object).NewReader(ctx)
    if err != nil {
        return err
    }
    defer rc.Close()
    f, err := os.Create(destPath)
    if err != nil {
        return err
    }
    defer func() { _ = f.Close() }()
    if _, err := io.Copy(f, rc); err != nil {
        return err
    }
    return nil
}

func executeMove(ctx context.Context, service Service, cfg *MovePayload, execCtx *registry.ExecutionContext) (MoveResult, error) {
    srcPath, err := parseGCSPath(cfg.Source)
    if err != nil {
        return MoveResult{}, err
    }
    dstPath, err := parseGCSPath(cfg.Destination)
    if err != nil {
        return MoveResult{}, err
    }

    entries := make([]MoveEntry, 0)

    // Support glob in source
    hasGlob := strings.ContainsAny(srcPath.Object, "*?[")
    basePrefix := srcPath.Object
    listPath := srcPath
    if hasGlob {
        wc := strings.IndexAny(srcPath.Object, "*?[")
        slash := strings.LastIndex(srcPath.Object[:wc], "/")
        if slash >= 0 {
            basePrefix = srcPath.Object[:slash+1]
        } else {
            basePrefix = ""
        }
        listPath.Object = basePrefix
    }

    if cfg.Recursive || strings.HasSuffix(srcPath.Object, "/") || srcPath.Object == "" || hasGlob {
        list, err := service.List(ctx, listPath, true)
        if err != nil {
            return MoveResult{}, err
        }
        if !list.Exists {
            return MoveResult{}, fmt.Errorf("source %s does not exist", cfg.Source)
        }
        var pattern string
        if hasGlob {
            pattern = strings.TrimPrefix(srcPath.Object, basePrefix)
        }
        prefix := ensureTrailingSlash(basePrefix)
        destPrefix := ensureTrailingSlash(dstPath.Object)
        for _, obj := range list.Objects {
            rel := strings.TrimPrefix(obj.Name, prefix)
            if hasGlob {
                match, _ := pth.Match(pattern, rel)
                if !match {
                    continue
                }
            }
            destination := dstPath
            destination.Object = destPrefix + rel
            entry := MoveEntry{Source: buildGCSURI(srcPath.Bucket, obj.Name), Destination: buildGCSURI(destination.Bucket, destination.Object)}
            if err := service.CopyObject(ctx, StoragePath{Bucket: srcPath.Bucket, Object: obj.Name}, destination); err != nil {
                entry.Moved = false
                entry.Skipped = err.Error()
            } else if err := service.DeleteObject(ctx, StoragePath{Bucket: srcPath.Bucket, Object: obj.Name}); err != nil {
                entry.Moved = false
                entry.Skipped = fmt.Sprintf("copied but failed to delete source: %v", err)
            } else {
                entry.Moved = true
                if execCtx != nil && execCtx.Logger != nil {
                    execCtx.Logger.Printf("Moved %s to %s", entry.Source, entry.Destination)
                }
            }
            entries = append(entries, entry)
        }
        if len(entries) == 0 {
            return MoveResult{}, fmt.Errorf("no objects found under %s", cfg.Source)
        }
    } else {
        exists, err := service.ObjectExists(ctx, srcPath)
        if err != nil {
            return MoveResult{}, err
        }
        entry := MoveEntry{Source: cfg.Source, Destination: cfg.Destination}
        if !exists {
            entry.Moved = false
            entry.Skipped = "source not found"
        } else {
            if err := service.CopyObject(ctx, srcPath, dstPath); err != nil {
                entry.Moved = false
                entry.Skipped = err.Error()
            } else if err := service.DeleteObject(ctx, srcPath); err != nil {
                entry.Moved = false
                entry.Skipped = fmt.Sprintf("copied but failed to delete source: %v", err)
            } else {
                entry.Moved = true
                if execCtx != nil && execCtx.Logger != nil {
                    execCtx.Logger.Printf("Moved %s to %s", cfg.Source, cfg.Destination)
                }
            }
        }
        entries = append(entries, entry)
    }

    return MoveResult{Entries: entries}, nil
}

func executeRemove(ctx context.Context, service Service, cfg *RemovePayload, execCtx *registry.ExecutionContext) (RemoveResult, error) {
    entries := make([]RemoveEntry, 0, len(cfg.Targets))

    for _, target := range cfg.Targets {
        path, err := parseGCSPath(target)
        if err != nil {
            return RemoveResult{}, err
        }

        // Glob support per target
        hasGlob := strings.ContainsAny(path.Object, "*?[")
        basePrefix := path.Object
        listPath := path
        if hasGlob {
            wc := strings.IndexAny(path.Object, "*?[")
            slash := strings.LastIndex(path.Object[:wc], "/")
            if slash >= 0 {
                basePrefix = path.Object[:slash+1]
            } else {
                basePrefix = ""
            }
            listPath.Object = basePrefix
        }

        if cfg.Recursive || strings.HasSuffix(path.Object, "/") || path.Object == "" || hasGlob {
            list, err := service.List(ctx, listPath, true)
            if err != nil {
                return RemoveResult{}, err
            }
            if !list.Exists {
                entries = append(entries, RemoveEntry{Target: target, Deleted: false, Message: "not found"})
                continue
            }
            var pattern string
            if hasGlob {
                pattern = strings.TrimPrefix(path.Object, basePrefix)
            }
            deletedAny := false
            prefix := ensureTrailingSlash(basePrefix)
            for _, obj := range list.Objects {
                rel := strings.TrimPrefix(obj.Name, prefix)
                if hasGlob {
                    match, _ := pth.Match(pattern, rel)
                    if !match {
                        continue
                    }
                }
                if err := service.DeleteObject(ctx, StoragePath{Bucket: path.Bucket, Object: obj.Name}); err != nil {
                    entries = append(entries, RemoveEntry{Target: buildGCSURI(path.Bucket, obj.Name), Deleted: false, Message: err.Error()})
                    continue
                }
                deletedAny = true
                if execCtx != nil && execCtx.Logger != nil {
                    execCtx.Logger.Printf("Deleted %s", buildGCSURI(path.Bucket, obj.Name))
                }
                entries = append(entries, RemoveEntry{Target: buildGCSURI(path.Bucket, obj.Name), Deleted: true})
            }
            if !deletedAny {
                entries = append(entries, RemoveEntry{Target: target, Deleted: false, Message: "no objects matched"})
            }
            continue
        }

        exists, err := service.ObjectExists(ctx, path)
        if err != nil {
            return RemoveResult{}, err
        }
        entry := RemoveEntry{Target: target}
        if !exists {
            entry.Deleted = false
            entry.Message = "not found"
        } else if err := service.DeleteObject(ctx, path); err != nil {
            entry.Deleted = false
            entry.Message = err.Error()
        } else {
            entry.Deleted = true
            if execCtx != nil && execCtx.Logger != nil {
                execCtx.Logger.Printf("Deleted %s", target)
            }
        }
        entries = append(entries, entry)
    }

    return RemoveResult{Entries: entries}, nil
}

func executeList(ctx context.Context, service Service, cfg *ListPayload, execCtx *registry.ExecutionContext) (ListResponse, error) {
    // Accept targets without scheme by prefixing gs:// automatically
    normalizedTarget := ensureGCSURI(cfg.Target)
    path, err := parseGCSPath(normalizedTarget)
    if err != nil {
        return ListResponse{}, err
    }

    // Detect glob and derive listing prefix + pattern
    hasGlob := strings.ContainsAny(path.Object, "*?[")
    var pattern string
    var basePrefix string
    listPath := path
    recursive := cfg.Recursive
    if hasGlob {
        wc := strings.IndexAny(path.Object, "*?[")
        slash := strings.LastIndex(path.Object[:wc], "/")
        if slash >= 0 {
            basePrefix = path.Object[:slash+1]
            pattern = path.Object[slash+1:]
        } else {
            basePrefix = ""
            pattern = path.Object
        }
        listPath.Object = basePrefix
        recursive = true
    }

    if execCtx != nil && execCtx.Logger != nil {
        if hasGlob {
            execCtx.Logger.Printf("Listing GCS: bucket=%s prefix=%s recursive=%v (glob: basePrefix=%s pattern=%s)", listPath.Bucket, listPath.Object, recursive, basePrefix, pattern)
        } else {
            execCtx.Logger.Printf("Listing GCS: bucket=%s prefix=%s recursive=%v", listPath.Bucket, listPath.Object, recursive)
        }
    }

    list, err := service.List(ctx, listPath, recursive)
    if err != nil {
        return ListResponse{}, fmt.Errorf("list gs://%s/%s: %w", listPath.Bucket, listPath.Object, err)
    }

    result := ListResult{Target: normalizedTarget, Exists: list.Exists}
    if list.Exists {
        for _, obj := range list.Objects {
            // If using a glob, filter objects by pattern relative to basePrefix.
            if hasGlob {
                rel := strings.TrimPrefix(obj.Name, basePrefix)
                match, _ := pth.Match(pattern, rel)
                if !match {
                    continue
                }
            }
            result.Objects = append(result.Objects, ListedObject{
                Name:         buildGCSURI(path.Bucket, obj.Name),
                Size:         obj.Size,
                Updated:      obj.Updated,
                ContentType:  obj.ContentType,
                StorageClass: obj.StorageClass,
            })
        }
        if !hasGlob {
            for _, p := range list.Prefixes {
                result.Prefixes = append(result.Prefixes, buildGCSURI(path.Bucket, p))
            }
        }
        if execCtx != nil && execCtx.Logger != nil {
            if hasGlob {
                execCtx.Logger.Printf("Listed %d object(s) for %s (filtered by glob)", len(result.Objects), normalizedTarget)
            } else {
                execCtx.Logger.Printf("Listed %d object(s) for %s", len(result.Objects), normalizedTarget)
            }
        }
    } else {
        result.Message = "no objects found"
        if execCtx != nil && execCtx.Logger != nil {
            execCtx.Logger.Printf("No objects found for %s", normalizedTarget)
        }
    }

    return ListResponse{List: result}, nil
}

// ensureGCSURI ensures the provided path starts with gs://. If empty, returns empty.
func ensureGCSURI(uri string) string {
    trimmed := strings.TrimSpace(uri)
    if trimmed == "" {
        return trimmed
    }
    if strings.HasPrefix(trimmed, "gs://") {
        return trimmed
    }
    return "gs://" + trimmed
}

func executeAuthInfo(ctx context.Context, service Service, execCtx *registry.ExecutionContext) (AuthInfoResult, error) {
	info, err := service.FetchAuthInfo(ctx)
	if err != nil {
		return AuthInfoResult{}, err
	}
	if execCtx != nil && execCtx.Logger != nil {
		execCtx.Logger.Printf("Active account: %s", info.Account)
		execCtx.Logger.Printf("Project ID: %s", info.ProjectID)
	}
	return AuthInfoResult{Info: info}, nil
}

func ensureTrailingSlash(value string) string {
	if value == "" {
		return ""
	}
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}

func buildGCSURI(bucket, object string) string {
	if object == "" {
		return fmt.Sprintf("gs://%s", bucket)
	}
	return fmt.Sprintf("gs://%s/%s", bucket, strings.TrimPrefix(object, "/"))
}

func parseGCSPath(uri string) (StoragePath, error) {
	trimmed := strings.TrimSpace(uri)
	if trimmed == "" {
		return StoragePath{}, errors.New("gcs path cannot be empty")
	}
	if !strings.HasPrefix(trimmed, "gs://") {
		return StoragePath{}, fmt.Errorf("unsupported path %q: only gs:// URIs are allowed", uri)
	}
	withoutScheme := strings.TrimPrefix(trimmed, "gs://")
	parts := strings.SplitN(withoutScheme, "/", 2)
	bucket := strings.TrimSpace(parts[0])
	if bucket == "" {
		return StoragePath{}, fmt.Errorf("invalid GCS URI %q: bucket is required", uri)
	}
	object := ""
	if len(parts) == 2 {
		object = strings.TrimPrefix(parts[1], "/")
	}
	return StoragePath{Bucket: bucket, Object: object}, nil
}
