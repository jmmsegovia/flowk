package helm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	helmAction "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/helmpath"
	"helm.sh/helm/v3/pkg/lint"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/strvals"
)

const ActionName = "HELM"

const (
	OperationRepoAdd        = "REPO_ADD"
	OperationRepoUpdate     = "REPO_UPDATE"
	OperationRepoList       = "REPO_LIST"
	OperationSearchRepo     = "SEARCH_REPO"
	OperationInstall        = "INSTALL"
	OperationUpgrade        = "UPGRADE"
	OperationUpgradeInstall = "UPGRADE_INSTALL"
	OperationList           = "LIST"
	OperationStatus         = "STATUS"
	OperationGetValues      = "GET_VALUES"
	OperationGetAll         = "GET_ALL"
	OperationHistory        = "HISTORY"
	OperationRollback       = "ROLLBACK"
	OperationUninstall      = "UNINSTALL"
	OperationShowValues     = "SHOW_VALUES"
	OperationShowChart      = "SHOW_CHART"
	OperationTemplate       = "TEMPLATE"
	OperationLint           = "LINT"
	OperationCreate         = "CREATE"
)

type Payload struct { /* unchanged */
	Operation       string   `json:"operation"`
	RepositoryName  string   `json:"repository_name,omitempty"`
	RepositoryURL   string   `json:"repository_url,omitempty"`
	Query           string   `json:"query,omitempty"`
	ReleaseName     string   `json:"release_name,omitempty"`
	Chart           string   `json:"chart,omitempty"`
	Namespace       string   `json:"namespace,omitempty"`
	ValuesFiles     []string `json:"values_files,omitempty"`
	SetValues       []string `json:"set_values,omitempty"`
	Version         string   `json:"version,omitempty"`
	KubeContext     string   `json:"kube_context,omitempty"`
	Destination     string   `json:"destination,omitempty"`
	Revision        *int     `json:"revision,omitempty"`
	CreateNamespace bool     `json:"create_namespace,omitempty"`
	Wait            bool     `json:"wait,omitempty"`
	TimeoutSeconds  float64  `json:"timeout_seconds,omitempty"`
	AllNamespaces   bool     `json:"all_namespaces,omitempty"`
	IncludeAll      bool     `json:"include_all,omitempty"`
	DryRun          bool     `json:"dry_run,omitempty"`
}

type ExecutionResult struct {
	Operation       string   `json:"operation"`
	Command         []string `json:"command"`
	ExitCode        int      `json:"exitCode"`
	Stdout          string   `json:"stdout"`
	Stderr          string   `json:"stderr"`
	DurationSeconds float64  `json:"durationSeconds"`
}

type commandOutput struct {
	stdout   string
	stderr   string
	exitCode int
}

type runnerFunc func(ctx context.Context, payload Payload, args []string) (commandOutput, error)

var commandRunner runnerFunc = runCommand

func (p *Payload) Validate() error {
	p.Operation = strings.ToUpper(strings.TrimSpace(p.Operation))
	p.RepositoryName = strings.TrimSpace(p.RepositoryName)
	p.RepositoryURL = strings.TrimSpace(p.RepositoryURL)
	p.Query = strings.TrimSpace(p.Query)
	p.ReleaseName = strings.TrimSpace(p.ReleaseName)
	p.Chart = strings.TrimSpace(p.Chart)
	p.Namespace = strings.TrimSpace(p.Namespace)
	p.Version = strings.TrimSpace(p.Version)
	p.KubeContext = strings.TrimSpace(p.KubeContext)
	p.Destination = strings.TrimSpace(p.Destination)
	p.ValuesFiles = normalizeStringSlice(p.ValuesFiles)
	for i := range p.SetValues {
		p.SetValues[i] = strings.TrimSpace(p.SetValues[i])
		if p.SetValues[i] == "" {
			return fmt.Errorf("helm task: set_values[%d] is required", i)
		}
		if !strings.Contains(p.SetValues[i], "=") {
			return fmt.Errorf("helm task: set_values[%d] must be in KEY=VALUE format", i)
		}
	}
	switch p.Operation {
	case OperationRepoUpdate, OperationRepoList, OperationList:
		return nil
	case OperationRepoAdd:
		if p.RepositoryName == "" {
			return fmt.Errorf("helm task: repository_name is required for %s", p.Operation)
		}
		if p.RepositoryURL == "" {
			return fmt.Errorf("helm task: repository_url is required for %s", p.Operation)
		}
	case OperationSearchRepo:
		if p.Query == "" {
			return fmt.Errorf("helm task: query is required for %s", p.Operation)
		}
	case OperationInstall, OperationUpgrade, OperationUpgradeInstall, OperationTemplate:
		if p.ReleaseName == "" {
			return fmt.Errorf("helm task: release_name is required for %s", p.Operation)
		}
		if p.Chart == "" {
			return fmt.Errorf("helm task: chart is required for %s", p.Operation)
		}
	case OperationStatus, OperationGetValues, OperationGetAll, OperationHistory, OperationUninstall:
		if p.ReleaseName == "" {
			return fmt.Errorf("helm task: release_name is required for %s", p.Operation)
		}
	case OperationRollback:
		if p.ReleaseName == "" {
			return fmt.Errorf("helm task: release_name is required for %s", p.Operation)
		}
		if p.Revision == nil {
			return fmt.Errorf("helm task: revision is required for %s", p.Operation)
		}
		if *p.Revision < 1 {
			return fmt.Errorf("helm task: revision must be greater than or equal to 1")
		}
	case OperationShowValues, OperationShowChart, OperationLint, OperationCreate:
		if p.Chart == "" {
			return fmt.Errorf("helm task: chart is required for %s", p.Operation)
		}
	default:
		if p.Operation == "" {
			return fmt.Errorf("helm task: operation is required")
		}
		return fmt.Errorf("helm task: unsupported operation %q", p.Operation)
	}
	if p.TimeoutSeconds < 0 {
		return fmt.Errorf("helm task: timeout_seconds must be greater than or equal to zero")
	}
	return nil
}

func Execute(ctx context.Context, payload Payload) (ExecutionResult, error) {
	args, err := buildArgs(payload)
	if err != nil {
		return ExecutionResult{}, err
	}
	start := time.Now()
	out, runErr := commandRunner(ctx, payload, args)
	duration := time.Since(start)
	result := ExecutionResult{Operation: payload.Operation, Command: sanitizeCommand(args), ExitCode: out.exitCode, Stdout: out.stdout, Stderr: out.stderr, DurationSeconds: duration.Seconds()}
	if runErr != nil {
		return result, fmt.Errorf("helm task: command failed for %s (exit code %d): %w", payload.Operation, out.exitCode, runErr)
	}
	return result, nil
}

func runCommand(ctx context.Context, payload Payload, _ []string) (commandOutput, error) {
	out, err := executeWithSDK(ctx, payload)
	if err != nil {
		if out.exitCode == 0 {
			out.exitCode = 1
		}
		if out.stderr == "" {
			out.stderr = err.Error()
		}
		return out, err
	}
	out.exitCode = 0
	return out, nil
}

func executeWithSDK(ctx context.Context, payload Payload) (commandOutput, error) {
	settings := cli.New()
	if payload.KubeContext != "" {
		settings.KubeContext = payload.KubeContext
	}
	if payload.Namespace != "" {
		settings.SetNamespace(payload.Namespace)
	}

	switch payload.Operation {
	case OperationRepoAdd:
		return executeRepoAdd(settings, payload)
	case OperationRepoUpdate:
		return executeRepoUpdate(settings)
	case OperationRepoList:
		return executeRepoList(settings)
	case OperationSearchRepo:
		return executeSearchRepo(settings, payload.Query)
	case OperationShowValues, OperationShowChart, OperationLint, OperationCreate:
		return executeLocalOps(settings, payload)
	default:
		return executeReleaseOps(ctx, settings, payload)
	}
}

func executeReleaseOps(ctx context.Context, settings *cli.EnvSettings, payload Payload) (commandOutput, error) {
	cfg := new(helmAction.Configuration)
	if err := cfg.Init(settings.RESTClientGetter(), settings.Namespace(), os.Getenv("HELM_DRIVER"), func(string, ...interface{}) {}); err != nil {
		return commandOutput{}, err
	}
	vals := map[string]interface{}{}
	for _, setExpr := range payload.SetValues {
		if err := strvals.ParseInto(setExpr, vals); err != nil {
			return commandOutput{}, err
		}
	}
	for _, file := range payload.ValuesFiles {
		b, err := os.ReadFile(file)
		if err != nil {
			return commandOutput{}, err
		}
		next := map[string]interface{}{}
		if err := json.Unmarshal(b, &next); err == nil {
			for k, v := range next {
				vals[k] = v
			}
		}
	}
	timeout := time.Duration(payload.TimeoutSeconds * float64(time.Second))
	switch payload.Operation {
	case OperationInstall, OperationTemplate:
		inst := helmAction.NewInstall(cfg)
		inst.ReleaseName = payload.ReleaseName
		inst.Namespace = settings.Namespace()
		inst.Version = payload.Version
		inst.CreateNamespace = payload.CreateNamespace
		inst.Wait = payload.Wait
		inst.Timeout = timeout
		inst.DryRun = payload.DryRun || payload.Operation == OperationTemplate
		inst.ClientOnly = payload.Operation == OperationTemplate
		cp, err := inst.ChartPathOptions.LocateChart(payload.Chart, settings)
		if err != nil {
			return commandOutput{}, err
		}
		ch, err := loader.Load(cp)
		if err != nil {
			return commandOutput{}, err
		}
		rel, err := inst.RunWithContext(ctx, ch, vals)
		if err != nil {
			return commandOutput{}, err
		}
		return marshalStdout(rel)
	case OperationUpgrade, OperationUpgradeInstall:
		up := helmAction.NewUpgrade(cfg)
		up.Namespace = settings.Namespace()
		up.Version = payload.Version
		up.Install = payload.Operation == OperationUpgradeInstall
		up.Wait = payload.Wait
		up.Timeout = timeout
		up.DryRun = payload.DryRun
		cp, err := up.ChartPathOptions.LocateChart(payload.Chart, settings)
		if err != nil {
			return commandOutput{}, err
		}
		ch, err := loader.Load(cp)
		if err != nil {
			return commandOutput{}, err
		}
		rel, err := up.RunWithContext(ctx, payload.ReleaseName, ch, vals)
		if err != nil {
			return commandOutput{}, err
		}
		return marshalStdout(rel)
	case OperationList:
		ls := helmAction.NewList(cfg)
		ls.AllNamespaces = payload.AllNamespaces
		rels, err := ls.Run()
		if err != nil {
			return commandOutput{}, err
		}
		return marshalStdout(rels)
	case OperationStatus:
		st := helmAction.NewStatus(cfg)
		rel, err := st.Run(payload.ReleaseName)
		if err != nil {
			return commandOutput{}, err
		}
		return marshalStdout(rel)
	case OperationGetValues:
		gv := helmAction.NewGetValues(cfg)
		gv.AllValues = payload.IncludeAll
		v, err := gv.Run(payload.ReleaseName)
		if err != nil {
			return commandOutput{}, err
		}
		return marshalStdout(v)
	case OperationGetAll:
		g := helmAction.NewGet(cfg)
		res, err := g.Run(payload.ReleaseName)
		if err != nil {
			return commandOutput{}, err
		}
		return marshalStdout(res)
	case OperationHistory:
		h := helmAction.NewHistory(cfg)
		rels, err := h.Run(payload.ReleaseName)
		if err != nil {
			return commandOutput{}, err
		}
		return marshalStdout(rels)
	case OperationRollback:
		r := helmAction.NewRollback(cfg)
		r.Version = *payload.Revision
		r.Wait = payload.Wait
		r.Timeout = timeout
		if err := r.Run(payload.ReleaseName); err != nil {
			return commandOutput{}, err
		}
		return commandOutput{stdout: "rollback completed"}, nil
	case OperationUninstall:
		u := helmAction.NewUninstall(cfg)
		u.Wait = payload.Wait
		u.Timeout = timeout
		res, err := u.Run(payload.ReleaseName)
		if err != nil {
			return commandOutput{}, err
		}
		return marshalStdout(res)
	default:
		return commandOutput{}, fmt.Errorf("helm task: unsupported operation %q", payload.Operation)
	}
}

func executeLocalOps(settings *cli.EnvSettings, payload Payload) (commandOutput, error) {
	switch payload.Operation {
	case OperationShowValues:
		s := helmAction.NewShow(helmAction.ShowValues)
		out, err := s.Run(payload.Chart)
		return commandOutput{stdout: out}, err
	case OperationShowChart:
		s := helmAction.NewShow(helmAction.ShowChart)
		out, err := s.Run(payload.Chart)
		return commandOutput{stdout: out}, err
	case OperationLint:
		l := helmAction.NewLint()
		res := l.Run([]string{payload.Chart}, valsFromSet(payload.SetValues))
		if len(res.Errors) > 0 {
			errs := make([]string, 0, len(res.Errors))
			for _, lintErr := range res.Errors {
				errs = append(errs, lintErr.Error())
			}
			return commandOutput{stdout: "lint failed", stderr: strings.Join(errs, "; "), exitCode: 1}, fmt.Errorf("lint failed")
		}
		return marshalStdout(res.Messages)
	case OperationCreate:
		dest := payload.Destination
		if dest == "" {
			dest = "."
		}
		createdPath, err := chartutil.Create(payload.Chart, dest)
		if err != nil {
			return commandOutput{}, err
		}
		return commandOutput{stdout: createdPath}, nil
	default:
		return commandOutput{}, fmt.Errorf("helm task: unsupported operation %q", payload.Operation)
	}
}

func executeRepoAdd(settings *cli.EnvSettings, payload Payload) (commandOutput, error) {
	rf, _ := repo.LoadFile(settings.RepositoryConfig)
	if rf.Has(payload.RepositoryName) {
		return commandOutput{stdout: "repository already exists"}, nil
	}
	entry := &repo.Entry{Name: payload.RepositoryName, URL: payload.RepositoryURL}
	cr, err := repo.NewChartRepository(entry, getter.All(settings))
	if err != nil {
		return commandOutput{}, err
	}
	cr.CachePath = settings.RepositoryCache
	if _, err := cr.DownloadIndexFile(); err != nil {
		return commandOutput{}, err
	}
	rf.Update(entry)
	if err := rf.WriteFile(settings.RepositoryConfig, 0o644); err != nil {
		return commandOutput{}, err
	}
	return commandOutput{stdout: "repository added"}, nil
}

func executeRepoUpdate(settings *cli.EnvSettings) (commandOutput, error) {
	rf, err := repo.LoadFile(settings.RepositoryConfig)
	if err != nil {
		return commandOutput{}, err
	}
	for _, cfg := range rf.Repositories {
		cr, err := repo.NewChartRepository(cfg, getter.All(settings))
		if err != nil {
			return commandOutput{}, err
		}
		cr.CachePath = settings.RepositoryCache
		if _, err := cr.DownloadIndexFile(); err != nil {
			return commandOutput{}, err
		}
	}
	return commandOutput{stdout: "repositories updated"}, nil
}

func executeRepoList(settings *cli.EnvSettings) (commandOutput, error) {
	rf, err := repo.LoadFile(settings.RepositoryConfig)
	if err != nil {
		return commandOutput{}, err
	}
	return marshalStdout(rf.Repositories)
}

func executeSearchRepo(settings *cli.EnvSettings, query string) (commandOutput, error) {
	rf, err := repo.LoadFile(settings.RepositoryConfig)
	if err != nil {
		return commandOutput{}, err
	}
	results := []map[string]string{}
	for _, r := range rf.Repositories {
		idxPath := filepath.Join(settings.RepositoryCache, helmpath.CacheIndexFile(r.Name))
		idx, err := repo.LoadIndexFile(idxPath)
		if err != nil {
			continue
		}
		for name, versions := range idx.Entries {
			if !strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
				continue
			}
			if len(versions) == 0 {
				continue
			}
			results = append(results, map[string]string{"name": r.Name + "/" + name, "version": versions[0].Version, "description": versions[0].Description})
		}
	}
	return marshalStdout(results)
}

func valsFromSet(setValues []string) map[string]interface{} {
	vals := map[string]interface{}{}
	for _, e := range setValues {
		_ = strvals.ParseInto(e, vals)
	}
	return vals
}

func marshalStdout(v any) (commandOutput, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return commandOutput{}, err
	}
	return commandOutput{stdout: string(b)}, nil
}

func buildArgs(payload Payload) ([]string, error) {
	args := make([]string, 0, 24)
	switch payload.Operation {
	case OperationRepoAdd:
		args = append(args, "repo", "add", payload.RepositoryName, payload.RepositoryURL)
	case OperationRepoUpdate:
		args = append(args, "repo", "update")
	case OperationRepoList:
		args = append(args, "repo", "list", "-o", "json")
	case OperationSearchRepo:
		args = append(args, "search", "repo", payload.Query)
	case OperationInstall:
		args = append(args, "install", payload.ReleaseName, payload.Chart)
	case OperationUpgrade:
		args = append(args, "upgrade", payload.ReleaseName, payload.Chart)
	case OperationUpgradeInstall:
		args = append(args, "upgrade", "--install", payload.ReleaseName, payload.Chart)
	case OperationList:
		args = append(args, "list", "-o", "json")
	case OperationStatus:
		args = append(args, "status", payload.ReleaseName, "-o", "json")
	case OperationGetValues:
		args = append(args, "get", "values", payload.ReleaseName, "-o", "json")
		if payload.IncludeAll {
			args = append(args, "--all")
		}
	case OperationGetAll:
		args = append(args, "get", "all", payload.ReleaseName)
	case OperationHistory:
		args = append(args, "history", payload.ReleaseName, "-o", "json")
	case OperationRollback:
		args = append(args, "rollback", payload.ReleaseName, fmt.Sprintf("%d", *payload.Revision))
	case OperationUninstall:
		args = append(args, "uninstall", payload.ReleaseName)
	case OperationShowValues:
		args = append(args, "show", "values", payload.Chart)
	case OperationShowChart:
		args = append(args, "show", "chart", payload.Chart)
	case OperationTemplate:
		args = append(args, "template", payload.ReleaseName, payload.Chart)
	case OperationLint:
		args = append(args, "lint", payload.Chart)
	case OperationCreate:
		args = append(args, "create", payload.Chart)
	default:
		return nil, fmt.Errorf("helm task: unsupported operation %q", payload.Operation)
	}
	args = appendHelmContextArgs(args, payload)
	args = appendHelmReleaseArgs(args, payload)
	return args, nil
}

func appendHelmContextArgs(args []string, payload Payload) []string {
	if payload.KubeContext != "" {
		args = append(args, "--kube-context", payload.KubeContext)
	}
	if payload.Namespace != "" {
		args = append(args, "--namespace", payload.Namespace)
	}
	if payload.AllNamespaces && payload.Operation == OperationList {
		args = append(args, "--all-namespaces")
	}
	return args
}
func appendHelmReleaseArgs(args []string, payload Payload) []string {
	for _, file := range payload.ValuesFiles {
		args = append(args, "--values", file)
	}
	for _, entry := range payload.SetValues {
		args = append(args, "--set", entry)
	}
	if payload.Version != "" {
		args = append(args, "--version", payload.Version)
	}
	if payload.CreateNamespace {
		args = append(args, "--create-namespace")
	}
	if payload.Wait {
		args = append(args, "--wait")
	}
	if payload.TimeoutSeconds > 0 {
		args = append(args, "--timeout", fmt.Sprintf("%.0fs", payload.TimeoutSeconds))
	}
	if payload.DryRun {
		args = append(args, "--dry-run")
	}
	if payload.Destination != "" && payload.Operation == OperationCreate {
		args = append(args, payload.Destination)
	}
	return args
}
func sanitizeCommand(args []string) []string {
	sanitized := make([]string, len(args)+1)
	sanitized[0] = "helm"
	for i, arg := range args {
		if i > 0 && args[i-1] == "--set" {
			sanitized[i+1] = maskSetValue(arg)
			continue
		}
		sanitized[i+1] = arg
	}
	return sanitized
}
func maskSetValue(value string) string {
	if value == "" {
		return value
	}
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return "****"
	}
	return parts[0] + "=****"
}
func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for idx := range values {
		trimmed := strings.TrimSpace(values[idx])
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

var _ = lint.All
var _ = bytes.Buffer{}
var _ = release.Release{}
