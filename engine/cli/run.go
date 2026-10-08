package cli

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/service"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/version"
)

func Run(kind *service.Kind) int {
	fs := flag.NewFlagSet(string(*kind), flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "configuration file")
	workDir := fs.String("work", "", "working directory for configs, database, data, and lua scripts")
	workDirAlias := fs.String("work-dir", "", "alias for -work")
	backend := fs.String("backend", "", "database backend: sqlite, mysql, or mariadb")
	dataDir := fs.String("data-dir", "", "runtime data directory")
	tracePath := fs.String("trace", "", "write a world protocol trace JSONL file on shutdown")
	showVersion := fs.Bool("version", false, "show version")
	debug := fs.Bool("debug", false, "enable basic authentication and runtime debug logs")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Printf("%s %s (%s)\n", version.Product, version.String(), version.Revision())
		return 0
	}
	effectiveWorkDir := *workDir
	if effectiveWorkDir == "" {
		effectiveWorkDir = *workDirAlias
	}
	if effectiveWorkDir == "" {
		effectiveWorkDir = os.Getenv("MORENOCORE_WORK_DIR")
	}
	if effectiveWorkDir == "" {
		effectiveWorkDir = os.Getenv("MORENOCORE_WORK")
	}
	selectedConfig := discoverConfig(*configPath, *kind, effectiveWorkDir)
	c, err := config.Load(selectedConfig)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	c.ApplyEnv()
	if effectiveWorkDir != "" {
		c.ApplyWorkDir(effectiveWorkDir)
	}
	if *backend != "" {
		c.Backend = *backend
	}
	if *dataDir != "" {
		c.DataDir = *dataDir
	}
	if *tracePath != "" {
		c.ProtocolTracePath = *tracePath
	}
	c.ResolvePaths()
	if err := c.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	logger := newLogger(*debug)
	if selectedConfig != "" {
		logger.Info("using configuration file", "path", selectedConfig)
	} else {
		logger.Warn("no configuration file found, using built-in defaults")
	}
	if len(c.UnrecognizedKeys) > 0 {
		logger.Warn("configuration contains unrecognized or unmapped keys", "unmapped_count", len(c.UnrecognizedKeys))
	}
	if err := writePidFile(c.PidFile, logger); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := service.RunSingle(context.Background(), c, *kind, logger); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func RunCombined() int {
	fs := flag.NewFlagSet("morenocore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "configuration file")
	workDir := fs.String("work", "", "working directory for configs, database, data, and lua scripts")
	workDirAlias := fs.String("work-dir", "", "alias for -work")
	backend := fs.String("backend", "", "database backend: sqlite, mysql, or mariadb")
	dataDir := fs.String("data-dir", "", "runtime data directory")
	tracePath := fs.String("trace", "", "write a world protocol trace JSONL file on shutdown")
	showVersion := fs.Bool("version", false, "show version")
	debug := fs.Bool("debug", false, "enable basic authentication and runtime debug logs")
	authOnly := fs.Bool("auth", false, "run only the authentication service")
	worldOnly := fs.Bool("world", false, "run only the world service")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Printf("%s %s (%s)\n", version.Product, version.String(), version.Revision())
		return 0
	}
	effectiveWorkDir := *workDir
	if effectiveWorkDir == "" {
		effectiveWorkDir = *workDirAlias
	}
	if effectiveWorkDir == "" {
		effectiveWorkDir = os.Getenv("MORENOCORE_WORK_DIR")
	}
	if effectiveWorkDir == "" {
		effectiveWorkDir = os.Getenv("MORENOCORE_WORK")
	}
	configKind := service.World
	if *authOnly {
		configKind = service.Auth
	}
	if *worldOnly {
		configKind = service.World
	}
	selectedConfig := discoverConfig(*configPath, configKind, effectiveWorkDir)
	c, err := config.Load(selectedConfig)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	c.ApplyEnv()
	if effectiveWorkDir != "" {
		c.ApplyWorkDir(effectiveWorkDir)
	}
	if *backend != "" {
		c.Backend = *backend
	}
	if *dataDir != "" {
		c.DataDir = *dataDir
	}
	if *tracePath != "" {
		c.ProtocolTracePath = *tracePath
	}
	c.ResolvePaths()
	if err := c.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	logger := newLogger(*debug)
	if selectedConfig != "" {
		logger.Info("using configuration file", "path", selectedConfig)
	} else {
		logger.Warn("no configuration file found, using built-in defaults")
	}
	if len(c.UnrecognizedKeys) > 0 {
		logger.Warn("configuration contains unrecognized or unmapped keys", "unmapped_count", len(c.UnrecognizedKeys))
	}
	if err := writePidFile(c.PidFile, logger); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *authOnly && *worldOnly {
		fmt.Fprintln(os.Stderr, "--auth and --world cannot be used together")
		return 2
	}
	if *authOnly {
		kind := service.Auth
		if err := service.RunSingle(context.Background(), c, kind, logger); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if *worldOnly {
		kind := service.World
		if err := service.RunSingle(context.Background(), c, kind, logger); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if err := service.RunCombined(context.Background(), c, logger); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func newLogger(debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

// writePidFile mirrors the authserver/worldserver Main.cpp PidFile arm:
// when a PID file path is configured it is created at startup holding the
// process ID; a creation failure aborts startup like the C++ return 1.
func writePidFile(path string, logger *slog.Logger) error {
	if path == "" {
		return nil
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		return fmt.Errorf("cannot create PID file %s: %w", path, err)
	}
	logger.Info("daemon PID file created", "path", path, "pid", os.Getpid())
	return nil
}

func discoverConfig(explicit string, kind service.Kind, workDir string) string {
	if explicit != "" {
		return explicit
	}
	candidates := make([]string, 0, 16)
	if workDir != "" {
		clean := filepath.Clean(workDir)
		if kind == service.Auth {
			candidates = append(candidates,
				filepath.Join(clean, "authserver.conf"),
				filepath.Join(clean, "bin", "authserver.conf"),
			)
		} else {
			candidates = append(candidates,
				filepath.Join(clean, "worldserver.conf"),
				filepath.Join(clean, "bin", "worldserver.conf"),
			)
		}
	}
	cwd, err := os.Getwd()
	if err == nil {
		cleanCwd := filepath.Clean(cwd)
		if kind == service.Auth {
			candidates = append(candidates,
				filepath.Join(cleanCwd, "bin", "authserver.conf"),
				filepath.Join(cleanCwd, "authserver.conf"),
				filepath.Join(cleanCwd, "..", "bin", "authserver.conf"),
				filepath.Join(cleanCwd, "..", "..", "bin", "authserver.conf"),
				filepath.Join(cleanCwd, "configs", "authserver.conf.dist"),
			)
		} else {
			candidates = append(candidates,
				filepath.Join(cleanCwd, "bin", "worldserver.conf"),
				filepath.Join(cleanCwd, "worldserver.conf"),
				filepath.Join(cleanCwd, "..", "bin", "worldserver.conf"),
				filepath.Join(cleanCwd, "..", "..", "bin", "worldserver.conf"),
				filepath.Join(cleanCwd, "configs", "worldserver.conf.dist"),
			)
		}
	}
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		if kind == service.Auth {
			candidates = append(candidates,
				filepath.Join(execDir, "authserver.conf"),
				filepath.Join(execDir, "bin", "authserver.conf"),
				filepath.Join(execDir, "..", "bin", "authserver.conf"),
				filepath.Join(execDir, "..", "..", "bin", "authserver.conf"),
			)
		} else {
			candidates = append(candidates,
				filepath.Join(execDir, "worldserver.conf"),
				filepath.Join(execDir, "bin", "worldserver.conf"),
				filepath.Join(execDir, "..", "bin", "worldserver.conf"),
				filepath.Join(execDir, "..", "..", "bin", "worldserver.conf"),
			)
		}
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
