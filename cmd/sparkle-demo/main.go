package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/local/sparkle-demo/internal/demo"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := loadConfig()

	temporalClient, err := client.Dial(client.Options{
		HostPort:  cfg.TemporalAddress,
		Namespace: cfg.Namespace,
		Logger:    temporalLogger{logger: logger},
	})
	if err != nil {
		logger.Error("connect to Temporal", "address", cfg.TemporalAddress, "error", err)
		os.Exit(1)
	}
	defer temporalClient.Close()

	store, err := demo.NewStore(cfg.DataPath)
	if err != nil {
		logger.Error("open read model", "path", cfg.DataPath, "error", err)
		os.Exit(1)
	}

	activities := &demo.Activities{Store: store}
	temporalWorker := worker.New(temporalClient, cfg.TaskQueue, worker.Options{})
	temporalWorker.RegisterWorkflowWithOptions(demo.ImageProcessingWorkflow, workflow.RegisterOptions{
		Name: demo.ImageProcessingWorkflowName,
	})
	temporalWorker.RegisterActivity(activities)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := temporalWorker.Start(); err != nil {
		logger.Error("start Temporal worker", "taskQueue", cfg.TaskQueue, "error", err)
		os.Exit(1)
	}
	defer temporalWorker.Stop()

	broker := demo.NewBroker()
	bridge := &demo.Bridge{
		Broker:    broker,
		Store:     store,
		Client:    temporalClient,
		TaskQueue: cfg.TaskQueue,
		Logger:    logger,
	}
	go func() {
		if err := bridge.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("bridge stopped", "error", err)
			stop()
		}
	}()

	system := demo.SystemStatus{
		Environment: "Local",
		KafkaSim:    "In-process",
		TemporalDev: cfg.TemporalAddress,
		Worker:      cfg.TaskQueue,
		API:         cfg.APIAddress,
	}
	api := demo.NewAPIServer(store, broker, system, logger)
	server := &http.Server{
		Addr:              cfg.APIAddress,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("Sparkle demo API started", "address", cfg.APIAddress, "taskQueue", cfg.TaskQueue)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api server stopped", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown api server", "error", err)
	}
}

func loadConfig() demo.RuntimeConfig {
	return demo.RuntimeConfig{
		TemporalAddress: env("TEMPORAL_ADDRESS", demo.DefaultTemporalAddress),
		Namespace:       env("TEMPORAL_NAMESPACE", demo.DefaultNamespace),
		TaskQueue:       env("SPARKLE_TASK_QUEUE", demo.DefaultTaskQueue),
		APIAddress:      env("SPARKLE_API_ADDRESS", demo.DefaultAPIAddress),
		DataPath:        env("SPARKLE_DATA_PATH", demo.DefaultDataPath),
	}
}

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type temporalLogger struct {
	logger *slog.Logger
}

func (logger temporalLogger) Debug(msg string, keyvals ...interface{}) {
	logger.logger.Debug(msg, keyvals...)
}

func (logger temporalLogger) Info(msg string, keyvals ...interface{}) {
	logger.logger.Info(msg, keyvals...)
}

func (logger temporalLogger) Warn(msg string, keyvals ...interface{}) {
	logger.logger.Warn(msg, keyvals...)
}

func (logger temporalLogger) Error(msg string, keyvals ...interface{}) {
	logger.logger.Error(msg, keyvals...)
}
