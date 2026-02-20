package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowk/internal/app"
	actionhelp "flowk/internal/cli/actionhelp"
	"flowk/internal/config"
	uiserver "flowk/internal/server/ui"
)

const (
	cliColorReset = "\033[0m"
	cliColorRed   = "\033[31m"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const flowkInfoLogo = `______ _               _   __
|  ___| |             | | / /
| |_  | | _____      _| |/ / 
|  _| | |/ _ \ \ /\ / /    \ 
| |   | | (_) \ V  V /| |\  \
\_|   |_|\___/ \_/\_/ \_| \_/
`

type runArguments struct {
	flowPath      string
	beginFromTask string
	runTaskID     string
	runFlowID     string
	runSubtaskID  string
	validateOnly  bool
	serveUI       bool
	uiAddress     string
	uiDir         string
}

func main() {
	log.SetFlags(0)

	if err := execute(os.Args[0], os.Args[1:]); err != nil {
		var usageErr *usageError
		switch {
		case errors.Is(err, flag.ErrHelp):
			return
		case errors.As(err, &usageErr):
			fmt.Fprintf(os.Stderr, "%s%v%s\n", cliColorRed, usageErr.err, cliColorReset)
			fmt.Fprintln(os.Stderr, usageErr.helpMessage)
			os.Exit(1)
		default:
			log.Fatalf("%sError: %v%s", cliColorRed, err, cliColorReset)
		}
	}
}

func execute(program string, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stdout, generalHelpMessage(program))
		return nil
	}

	switch args[0] {
	case "run":
		if len(args) > 1 && isHelpFlag(args[1]) {
			fmt.Fprintln(os.Stdout, runHelpMessage(program))
			return nil
		}

		runArgs, err := parseRunArgs(args[1:])
		if err != nil {
			return &usageError{err: err, helpMessage: runHelpMessage(program)}
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		startTime := time.Now()
		err = runFlowWithOptions(ctx, runArgs)
		elapsed := time.Since(startTime)
		if runArgs.validateOnly {
			if err == nil {
				log.Printf("Validation successful: %s", runArgs.flowPath)
			}
		} else {
			log.Printf("Flow execution time: %s", formatFlowDuration(elapsed))
		}

		return err

	case "version":
		fmt.Fprintf(os.Stdout, "flowk %s (commit %s, date %s)\n", version, commit, date)
		return nil
	case "info":
		if len(args) > 1 && isHelpFlag(args[1]) {
			fmt.Fprintln(os.Stdout, generalHelpMessage(program))
			return nil
		}
		if len(args) > 1 {
			return &usageError{err: fmt.Errorf("unexpected arguments: %s", strings.Join(args[1:], " ")), helpMessage: generalHelpMessage(program)}
		}
		return printInfo(os.Stdout)

	case "help":
		if len(args) > 1 && strings.EqualFold(args[1], "action") {
			return executeActionHelp(program, args[2:])
		}
		fmt.Fprintln(os.Stdout, generalHelpMessage(program))
		return nil
	case "-help", "--help":
		fmt.Fprintln(os.Stdout, generalHelpMessage(program))
		return nil
	default:
		if isHelpFlag(args[0]) {
			fmt.Fprintln(os.Stdout, generalHelpMessage(program))
			return nil
		}
		return &usageError{err: fmt.Errorf("unknown command %q", args[0]), helpMessage: generalHelpMessage(program)}
	}
}

type usageError struct {
	err         error
	helpMessage string
}

func (e *usageError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func parseRunArgs(args []string) (runArguments, error) {
	var (
		cfg         runArguments
		positionals []string
		configPath  string
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "-serve-ui":
			cfg.serveUI = true
			continue
		case "-validate-only":
			cfg.validateOnly = true
			continue
		}

		if value, consumed, err := parseFlagValue(args, &i, "-config"); err != nil {
			return runArguments{}, err
		} else if consumed {
			configPath = value
			continue
		}

		if value, consumed, err := parseFlagValue(args, &i, "-flow"); err != nil {
			return runArguments{}, err
		} else if consumed {
			cfg.flowPath = value
			continue
		}

		if value, consumed, err := parseFlagValue(args, &i, "-begin-from-task"); err != nil {
			return runArguments{}, err
		} else if consumed {
			cfg.beginFromTask = value
			continue
		}

		if value, consumed, err := parseFlagValue(args, &i, "-run-task"); err != nil {
			return runArguments{}, err
		} else if consumed {
			cfg.runTaskID = value
			continue
		}

		if value, consumed, err := parseFlagValue(args, &i, "-run-flow"); err != nil {
			return runArguments{}, err
		} else if consumed {
			cfg.runFlowID = value
			continue
		}

		if value, consumed, err := parseFlagValue(args, &i, "-run-subtask"); err != nil {
			return runArguments{}, err
		} else if consumed {
			cfg.runSubtaskID = value
			continue
		}

		positionals = append(positionals, arg)
	}

	if cfg.flowPath == "" && len(positionals) > 0 {
		cfg.flowPath = positionals[0]
		positionals = positionals[1:]
	}

	cfg.flowPath = strings.TrimSpace(cfg.flowPath)

	if cfg.validateOnly {
		if cfg.serveUI {
			return runArguments{}, errors.New("flag -validate-only cannot be combined with -serve-ui")
		}
		if strings.TrimSpace(cfg.beginFromTask) != "" || strings.TrimSpace(cfg.runTaskID) != "" || strings.TrimSpace(cfg.runFlowID) != "" || strings.TrimSpace(cfg.runSubtaskID) != "" {
			return runArguments{}, errors.New("flag -validate-only cannot be combined with -begin-from-task, -run-task, -run-subtask, or -run-flow")
		}
	}

	if cfg.flowPath == "" && !cfg.serveUI {
		return runArguments{}, errors.New("missing required -flow flag")
	}

	if cfg.flowPath == "" {
		if strings.TrimSpace(cfg.beginFromTask) != "" || strings.TrimSpace(cfg.runTaskID) != "" || strings.TrimSpace(cfg.runFlowID) != "" || strings.TrimSpace(cfg.runSubtaskID) != "" {
			return runArguments{}, errors.New("flags -begin-from-task, -run-task, -run-subtask, and -run-flow require a flow when -flow is not provided")
		}
	}

	if strings.TrimSpace(cfg.beginFromTask) != "" && strings.TrimSpace(cfg.runTaskID) != "" {
		return runArguments{}, errors.New("flags -begin-from-task and -run-task cannot be used together")
	}

	if strings.TrimSpace(cfg.runSubtaskID) != "" {
		if strings.TrimSpace(cfg.beginFromTask) != "" || strings.TrimSpace(cfg.runTaskID) != "" {
			return runArguments{}, errors.New("flag -run-subtask cannot be combined with -begin-from-task or -run-task")
		}
	}

	if strings.TrimSpace(cfg.runFlowID) != "" {
		if strings.TrimSpace(cfg.beginFromTask) != "" || strings.TrimSpace(cfg.runTaskID) != "" || strings.TrimSpace(cfg.runSubtaskID) != "" {
			return runArguments{}, errors.New("flag -run-flow cannot be combined with -begin-from-task, -run-task, or -run-subtask")
		}
	}

	if len(positionals) > 0 {
		return runArguments{}, fmt.Errorf("unexpected arguments: %s", strings.Join(positionals, " "))
	}

	configResult, err := config.LoadFrom(configPath)
	if err != nil {
		return runArguments{}, err
	}
	cfg.uiAddress = fmt.Sprintf("%s:%d", configResult.Config.UI.Host, configResult.Config.UI.Port)
	cfg.uiDir = configResult.Config.UI.Dir

	return cfg, nil
}

func generalHelpMessage(program string) string {
	return fmt.Sprintf("Usage:\n  %[1]s <command> [options]\n\nAvailable commands:\n  run               Execute a test flow.\n  version           Show build information.\n  info              Show configuration paths and defaults.\n  help              Show this help message.\n\nHelpful references:\n  %[1]s run -help           More information about running flows.\n  %[1]s help action [name]  List actions or display the fields for an action.", program)
}

func runHelpMessage(program string) string {
	return fmt.Sprintf("Usage:\n  %[1]s run [-flow=<action-flow>] [-begin-from-task=<task-id>] [-run-task=<task-id>] [-run-subtask=<task-id>] [-run-flow=<flow-id>] [options]\n\nFlags:\n  -flow              Path to the action flow to execute (required unless -serve-ui is used without an initial run).\n  -begin-from-task   Start executing the flow from the provided task identifier.\n  -run-task          Execute only the specified task identifier.\n  -run-subtask       Execute only the specified subtask identifier (nested in PARALLEL/FOR).\n  -run-flow          Execute the specified nested flow identifier.\n  -validate-only     Validate the flow definition and exit without running tasks.\n  -serve-ui          Start an HTTP server to serve the visual UI and live execution events (UI host/port/dir are read from config.yaml).\n  -config            Path to a config.yaml file that overrides the XDG config location.", program)
}

func formatFlowDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}

	tenths := int((d + 50*time.Millisecond) / (100 * time.Millisecond))
	totalSeconds := tenths / 10
	tenthsRemainder := tenths % 10

	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	secondsLabel := "seconds"
	if seconds == 1 && tenthsRemainder == 0 {
		secondsLabel = "second"
	}

	return fmt.Sprintf("%s, %s, %02d.%d %s", formatDurationUnit(hours, "hour", "hours"), formatDurationUnit(minutes, "minute", "minutes"), seconds, tenthsRemainder, secondsLabel)
}

func formatDurationUnit(value int, singular, plural string) string {
	label := plural
	if value == 1 {
		label = singular
	}
	return fmt.Sprintf("%02d %s", value, label)
}

func runFlowWithOptions(ctx context.Context, args runArguments) (err error) {
	if args.validateOnly {
		return app.ValidateFlow(args.flowPath)
	}
	if !args.serveUI {
		return app.Run(ctx, args.flowPath, log.Default(), args.beginFromTask, args.runTaskID, args.runFlowID, args.runSubtaskID)
	}

	hub := uiserver.NewEventHub()
	observer := uiserver.NewHubObserver(hub)

	uiCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	flowRunner := uiserver.NewFlowRunner(uiCtx, observer, args.flowPath, args.beginFromTask, args.runTaskID, args.runFlowID, args.runSubtaskID, log.Default())
	staticDir, uiFound := resolveUIStaticDir(args.uiDir)
	if !uiFound {
		log.Printf("UI assets not found at %s; static UI will be unavailable", staticDir)
		_ = printInfo(os.Stderr)
		return fmt.Errorf("ui assets not found at %s", staticDir)
	} else {
		log.Printf("Using UI assets from %s", staticDir)
	}

	server, err := uiserver.NewServer(uiserver.Config{
		Address:       args.uiAddress,
		FlowPath:      args.flowPath,
		Hub:           hub,
		StaticDir:     staticDir,
		Runner:        flowRunner,
		FlowUploadDir: "",
	})
	if err != nil {
		return err
	}

	runner := uiserver.NewRunner(args.uiAddress, server.Handle())
	if bindErr := runner.Bind(); bindErr != nil {
		log.Printf("UI server failed to bind %s: %v", args.uiAddress, bindErr)
		return bindErr
	}
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- runner.Start()
	}()

	var (
		serverErr    error
		serverClosed bool
		initialRunCh <-chan error
	)

	defer func() {
		shutdownErr := runner.Shutdown(context.Background())
		hub.Close()

		if !serverClosed {
			serverErr = <-serverErrCh
			serverClosed = true
		}

		if err == nil {
			if shutdownErr != nil {
				err = shutdownErr
			} else if serverErr != nil {
				err = serverErr
			}
		}
	}()

	select {
	case startErr := <-serverErrCh:
		serverErr = startErr
		serverClosed = true
		if startErr != nil {
			return startErr
		}
		return errors.New("ui server stopped unexpectedly immediately after start")
	default:
	}

	if uiFound {
		log.Printf("Flowk UI is available at http://%s", args.uiAddress)
	} else {
		log.Printf("Flowk API is available at http://%s (UI assets missing)", args.uiAddress)
	}

	if flowRunner != nil && strings.TrimSpace(args.flowPath) != "" {
		initialRunCh, err = flowRunner.Start(nil)
		if err != nil {
			return err
		}
	}

	for {
		select {
		case runErr := <-initialRunCh:
			initialRunCh = nil
			if runErr != nil {
				err = runErr
				return err
			}
			log.Printf("Flow completed successfully. The Flowk UI will remain available at http://%s until you terminate the process.", args.uiAddress)
		case serverErr = <-serverErrCh:
			serverClosed = true
			if serverErr != nil {
				err = serverErr
			} else {
				err = errors.New("ui server stopped unexpectedly")
			}
			return err
		case <-ctx.Done():
			err = ctx.Err()
			return err
		}
	}
}

func uiStaticDir(candidate string) string {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return config.DefaultUIDir
	}
	return trimmed
}

func resolveUIStaticDir(candidate string) (string, bool) {
	dir := uiStaticDir(candidate)
	if filepath.IsAbs(dir) {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir, true
		}
		return dir, false
	}

	if wd, err := os.Getwd(); err == nil {
		joined := filepath.Join(wd, dir)
		if info, err := os.Stat(joined); err == nil && info.IsDir() {
			return joined, true
		}
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		joined := filepath.Join(exeDir, dir)
		if info, err := os.Stat(joined); err == nil && info.IsDir() {
			return joined, true
		}
	}

	return dir, false
}

func parseFlagValue(args []string, index *int, name string) (string, bool, error) {
	arg := args[*index]

	if arg == name {
		if *index+1 >= len(args) {
			return "", false, fmt.Errorf("flag %s requires a value", name)
		}
		*index = *index + 1
		return args[*index], true, nil
	}

	prefix := name + "="
	if strings.HasPrefix(arg, prefix) {
		value := strings.TrimPrefix(arg, prefix)
		if value != "" {
			return value, true, nil
		}
		if *index+1 >= len(args) {
			return "", false, fmt.Errorf("flag %s requires a value", name)
		}
		*index = *index + 1
		return args[*index], true, nil
	}

	return "", false, nil
}

func isHelpFlag(arg string) bool {
	switch arg {
	case "-h", "--help", "help":
		return true
	}
	return false
}

func executeActionHelp(program string, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stdout, actionhelp.Index(program))
		return nil
	}

	if len(args) > 1 {
		return &usageError{err: fmt.Errorf("unexpected arguments: %s", strings.Join(args[1:], " ")), helpMessage: actionhelp.Usage(program)}
	}

	actionName := strings.TrimSpace(args[0])
	if actionName == "" {
		fmt.Fprintln(os.Stdout, actionhelp.Index(program))
		return nil
	}

	message, err := actionhelp.Build(actionName)
	if err != nil {
		var lookupErr actionhelp.LookupError
		if errors.As(err, &lookupErr) {
			return &usageError{err: err, helpMessage: actionhelp.Usage(program)}
		}
		return err
	}

	fmt.Fprintln(os.Stdout, message)
	return nil
}

func printInfo(out io.Writer) error {
	configResult, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Fprint(out, flowkInfoLogo)
	fmt.Fprintf(out, "Version: %s\n", version)
	fmt.Fprintf(out, "Config file: %s\n", configResult.Path)
	if configResult.Loaded {
		fmt.Fprintln(out, "Config loaded: yes")
	} else {
		fmt.Fprintln(out, "Config loaded: no (using defaults)")
	}
	fmt.Fprintf(out, "UI host: %s\n", configResult.Config.UI.Host)
	fmt.Fprintf(out, "UI port: %d\n", configResult.Config.UI.Port)
	fmt.Fprintf(out, "UI dir: %s\n", configResult.Config.UI.Dir)
	return nil
}
