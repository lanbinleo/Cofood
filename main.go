package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cofood/internal/api"
	"cofood/internal/config"
	"cofood/internal/database"
	"cofood/internal/embedding"
	"cofood/internal/importer"
	"cofood/internal/search"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	store, err := database.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	embedClient := embedding.NewClient(cfg)
	importSvc := importer.NewService(store, embedClient, cfg)

	if err := importSvc.Bootstrap(context.Background()); err != nil {
		log.Fatalf("bootstrap data: %v", err)
	}

	searchSvc, err := search.NewService(store, embedClient, cfg)
	if err != nil {
		log.Fatalf("create search service: %v", err)
	}

	router := api.NewRouter(cfg, searchSvc)

	server := &http.Server{
		Addr:              cfg.HTTPAddr(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("server listening on %s", cfg.HTTPAddr())
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown server: %v", err)
	}
}
