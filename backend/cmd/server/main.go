package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nash-load-lab/backend/internal/api"
	"nash-load-lab/backend/internal/store"
)

func main() {
	address := flag.String("addr", ":8080", "адрес HTTP-сервера")
	databasePath := flag.String("db", "./data/nash-load-lab.db", "путь к SQLite-базе")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := store.Open(ctx, store.Config{Path: *databasePath, BusyTimeout: 5 * time.Second})
	if err != nil {
		log.Fatalf("не удалось открыть SQLite: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("не удалось закрыть SQLite: %v", err)
		}
	}()

	server := &http.Server{
		Addr:              *address,
		Handler:           api.New(database),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Nash Load Lab API слушает %s", *address)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP-сервер завершился с ошибкой: %v", err)
		}
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			log.Printf("ошибка штатного завершения HTTP-сервера: %v", err)
		}
	}
}
