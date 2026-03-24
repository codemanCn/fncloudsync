package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xiaoxuesen/fn-cloudsync/internal/api"
	"github.com/xiaoxuesen/fn-cloudsync/internal/app"
	"github.com/xiaoxuesen/fn-cloudsync/internal/config"
	"github.com/xiaoxuesen/fn-cloudsync/internal/connector/webdav"
	appcrypto "github.com/xiaoxuesen/fn-cloudsync/internal/crypto"
	"github.com/xiaoxuesen/fn-cloudsync/internal/obs"
	"github.com/xiaoxuesen/fn-cloudsync/internal/scheduler"
	sqlitestore "github.com/xiaoxuesen/fn-cloudsync/internal/store/sqlite"
	appsync "github.com/xiaoxuesen/fn-cloudsync/internal/sync"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	logger := obs.NewLogger()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, err := sqlitestore.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := sqlitestore.Migrate(context.Background(), db); err != nil {
		return err
	}

	secrets, err := appcrypto.NewSecretManager(cfg.SecretKey)
	if err != nil {
		return err
	}

	connectionService, err := app.NewConnectionService(sqlitestore.NewConnectionRepository(db), secrets)
	if err != nil {
		return err
	}
	taskService := app.NewTaskService(sqlitestore.NewTaskRepository(db))
	taskService.SetConnectionRepository(sqlitestore.NewConnectionRepository(db))
	taskService.SetSecrets(secrets)
	taskService.SetBaselineRunner(appsync.NewBaselineRunner(webdav.NewClient()))
	taskService.SetRuntimeRepository(sqlitestore.NewTaskRuntimeRepository(db))
	taskService.SetFailureRepository(sqlitestore.NewFailureRecordRepository(db))
	taskService.SetOperationQueueRepository(sqlitestore.NewOperationQueueRepository(db))

	bgScheduler := scheduler.New(taskService, sqlitestore.NewTaskRuntimeRepository(db), time.Second)

	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: api.NewRouter(connectionService, taskService),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		bgScheduler.Run(ctx)
	}()

	go func() {
		logger.Printf("listening on %s", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger.Printf("shutting down")
	return server.Shutdown(shutdownCtx)
}
