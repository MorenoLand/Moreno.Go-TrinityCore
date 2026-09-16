package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/auth"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/world"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

type Kind string

const (
	Auth  Kind = "authserver"
	World Kind = "worldserver"
)

type Service struct {
	Kind    Kind
	Address string
	Store   *database.Store
	Handler func(context.Context, net.Conn)
	Stop    func()
}

func RunCombined(ctx context.Context, c config.Config, logger *slog.Logger) error {
	stores, err := database.OpenSet(ctx, c)
	if err != nil {
		return err
	}
	defer stores.Close()
	authServer := auth.NewServer(stores.Auth, logger, c.RealmID, c)
	worldServer := world.NewServer(stores, logger, c.RealmID, c)
	traceRecorder, err := configureProtocolTrace(c.ProtocolTracePath)
	if err != nil {
		return err
	}
	worldServer.TraceRecorder = traceRecorder
	authServer.TraceRecorder = traceRecorder
	stores.Auth.TraceRecorder = traceRecorder
	stores.Characters.TraceRecorder = traceRecorder
	stores.World.TraceRecorder = traceRecorder
	defer persistProtocolTrace(c.ProtocolTracePath, traceRecorder)
	if err := worldServer.Initialize(ctx); err != nil {
		return err
	}
	authService := &Service{Kind: Auth, Address: fmt.Sprintf(":%d", c.RealmServerPort), Store: stores.Auth, Handler: authServer.Handle}
	worldService := &Service{Kind: World, Address: fmt.Sprintf(":%d", c.WorldServerPort), Store: stores.World, Handler: worldServer.Handle, Stop: worldServer.Stop}
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	errs := make(chan error, 2)
	go func() { errs <- authService.Run(ctx, logger) }()
	go func() { errs <- worldService.Run(ctx, logger) }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errs:
		if errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
}

func RunSingle(ctx context.Context, c config.Config, kind Kind, logger *slog.Logger) error {
	stores, err := database.OpenSet(ctx, c)
	if err != nil {
		return err
	}
	defer stores.Close()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if kind == Auth {
		server := auth.NewServer(stores.Auth, logger, c.RealmID, c)
		traceRecorder, err := configureProtocolTrace(c.ProtocolTracePath)
		if err != nil {
			return err
		}
		server.TraceRecorder = traceRecorder
		stores.Auth.TraceRecorder = traceRecorder
		stores.Characters.TraceRecorder = traceRecorder
		stores.World.TraceRecorder = traceRecorder
		defer persistProtocolTrace(c.ProtocolTracePath, traceRecorder)
		return (&Service{Kind: kind, Address: fmt.Sprintf(":%d", c.RealmServerPort), Store: stores.Auth, Handler: server.Handle}).Run(ctx, logger)
	}
	server := world.NewServer(stores, logger, c.RealmID, c)
	traceRecorder, err := configureProtocolTrace(c.ProtocolTracePath)
	if err != nil {
		return err
	}
	server.TraceRecorder = traceRecorder
	defer persistProtocolTrace(c.ProtocolTracePath, traceRecorder)
	if err := server.Initialize(ctx); err != nil {
		return err
	}
	return (&Service{Kind: kind, Address: fmt.Sprintf(":%d", c.WorldServerPort), Store: stores.World, Handler: server.Handle, Stop: server.Stop}).Run(ctx, logger)
}

func configureProtocolTrace(path string) (*protocoltrace.Recorder, error) {
	if path == "" {
		return nil, nil
	}
	return protocoltrace.NewRecorder("worldserver"), nil
}

func persistProtocolTrace(path string, recorder *protocoltrace.Recorder) {
	if path == "" || recorder == nil {
		return
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return
	}
	defer file.Close()
	_ = recorder.Snapshot().Write(file)
}

func authHandler(store *database.Store, logger *slog.Logger, realmID uint32) func(context.Context, net.Conn) {
	return auth.NewServer(store, logger, realmID).Handle
}

func (s *Service) Run(ctx context.Context, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", s.Address)
	if err != nil {
		return fmt.Errorf("%s listen %s: %w", s.Kind, s.Address, err)
	}
	defer func() {
		if s.Stop != nil {
			s.Stop()
		}
	}()
	storeName := ""
	storeBackend := ""
	if s.Store != nil {
		storeName = s.Store.Name
		storeBackend = string(s.Store.Backend)
	}
	logger.Info("service listening", "service", s.Kind, "address", listener.Addr().String(), "database", storeName, "backend", storeBackend)
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		if s.Handler != nil {
			go s.Handler(ctx, conn)
		} else {
			go func() { defer conn.Close(); _, _ = io.Copy(io.Discard, conn) }()
		}
	}
}
