package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"sync"

	enginecontext "lumin-engine/internal/context"
	"lumin-engine/internal/config"
	"lumin-engine/internal/hardware"
	"lumin-engine/internal/inference"
	"lumin-engine/internal/ipc"
	"lumin-engine/internal/ipc/stream"
	"lumin-engine/internal/observability"
	"lumin-engine/internal/orchestrator"
	"lumin-engine/internal/permissions"
	"lumin-engine/internal/recovery"
	"lumin-engine/internal/tools"
	"lumin-engine/internal/agent"
	"github.com/coreos/go-systemd/v22/activation"
)

type engine struct {
	model      *inference.Model
	policy     *permissions.Policy
	executor   *tools.Executor
	agent      *agent.Agent
	agentMu    sync.Mutex
	metrics    *observability.Metrics
	recovery   *recovery.Manager
	config     config.Config
	probe      hardware.ProbeResult
	ctxManager *enginecontext.Manager
}

func (e *engine) Generate(prompt string, maxTokens int) (string, error) {
	if err := e.model.EnsureLoaded(); err != nil {
		return "", err
	}
	return e.model.Generate(prompt, maxTokens)
}

func (e *engine) Health() map[string]any {
	return map[string]any{
		"status":      "ok",
		"model":       e.model.Status(),
		"hardware":    e.probe,
		"max_context":  e.ctxManager.MaxTokens,
		"permissions":  e.policy.Summary(),
		"socket_path":  e.config.SocketPath,
		"audit_log":    e.config.AuditLogPath,
		"metrics":      e.metrics.Snapshot(),
		"recovery_path": e.config.RecoveryPath,
	}
}

func (e *engine) LoadModel(path string) error {
	return e.model.Load(path)
}

func (e *engine) UnloadModel() error {
	return e.model.Unload()
}

func (e *engine) Tool(name string, args []byte) (any, error) {
	return e.executor.Execute(name, args)
}

func (e *engine) RunAgent(ctx context.Context, message string, out chan<- agent.Event) {
	e.agentMu.Lock()
	defer e.agentMu.Unlock()
	e.agent.Run(ctx, message, out)
}

func main() {
	configPath := flag.String("config", "", "path to config file")
	socketPath := flag.String("socket", "", "unix socket path")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *socketPath != "" {
		cfg.SocketPath = *socketPath
	}
	if cfg.IPCMode == "" {
		cfg.IPCMode = "jsonrpc"
	}

	socketPathValue := cfg.SocketPath
	if strings.EqualFold(cfg.IPCMode, "stream") && cfg.StreamSocketPath != "" {
		socketPathValue = cfg.StreamSocketPath
	}

	if err := os.MkdirAll(filepath.Dir(socketPathValue), 0o755); err != nil {
		log.Fatalf("prepare socket directory: %v", err)
	}

	policy, err := permissions.Load(cfg.PermissionsPath)
	if err != nil {
		log.Fatalf("load permissions: %v", err)
	}

	probe := hardware.ProbeSystem()
	model := inference.NewModel(cfg.ModelPath, cfg.MaxContextTokens)
	if cfg.ModelPath != "" {
		if err := model.Load(""); err != nil {
			log.Fatalf("load model: %v", err)
		}
	}
	ctxManager := enginecontext.NewManager(cfg.MaxContextTokens, nil)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	streamer := orchestrator.NewModelStreamer(model, cfg.MaxGenerateTokens)
	promptBuilder := orchestrator.NewPromptBuilder(model, cfg.SystemPrompt, cfg.PromptTemplate, cfg.MaxContextTokens)
	executor := tools.NewExecutor(policy, cfg.AuditLogPath)
	ollieAgent := agent.New(streamer, promptBuilder, executor, policy, logger)
	metrics := observability.NewMetrics()
	recoveryMgr := recovery.NewManager(cfg.RecoveryPath)
	backend := &engine{
		model:      model,
		policy:     policy,
		executor:   executor,
		agent:      ollieAgent,
		metrics:    metrics,
		recovery:   recoveryMgr,
		config:     cfg,
		probe:      probe,
		ctxManager: ctxManager,
	}

	handler := ipc.NewHandler(backend)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Prefer systemd socket activation; fall back to manual unix socket.
	var listener net.Listener
	listeners, err := activation.Listeners()
	if err != nil {
		log.Fatalf("check systemd activation: %v", err)
	}

	if len(listeners) > 0 {
		// Use systemd-provided listener.
		listener = listeners[0]
		log.Printf("lumin-engine listening on systemd-activated socket")
	} else {
		// Manual unix socket setup.
		if err := os.MkdirAll(filepath.Dir(socketPathValue), 0o755); err != nil {
			log.Fatalf("prepare socket directory: %v", err)
		}
		if err := os.Remove(socketPathValue); err != nil && !os.IsNotExist(err) {
			log.Fatalf("remove stale socket: %v", err)
		}

		unixListener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPathValue, Net: "unix"})
		if err != nil {
			log.Fatalf("listen on socket: %v", err)
		}
		if err := os.Chmod(socketPathValue, 0o666); err != nil {
			log.Fatalf("chmod socket: %v", err)
		}
		listener = unixListener
		log.Printf("lumin-engine listening on %s", socketPathValue)
	}
	defer listener.Close()

		if strings.EqualFold(cfg.IPCMode, "stream") {
			streamHandler := stream.NewHandler(backend).WithMaxFrameSize(cfg.StreamMaxFrameSize).WithMetrics(metrics)
			streamServer := stream.NewServer(listener, streamHandler)
			if err := streamServer.Serve(ctx); err != nil && err != context.Canceled {
				log.Fatalf("serve stream: %v", err)
			}
		} else {
			server := ipc.NewServer(listener, handler)
			if err := server.Serve(ctx); err != nil && err != context.Canceled {
				log.Fatalf("serve: %v", err)
			}
		}
	fmt.Println("lumin-engine stopped")
}
