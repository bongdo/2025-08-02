package main

import (
	"2025-08-02/config"
	_ "2025-08-02/docs"
	"2025-08-02/handlers"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"
)

// @title           File Archiver API
// @version         1.0
// @description     This is a server for archiving files from URLs.
// @host      localhost:8080
// @BasePath  /

//go:generate swag init
func main() {
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	taskManager := handlers.NewTaskManager(cfg)

	go cleanupOldArchives(10 * time.Minute)

	r := mux.NewRouter()
	r.HandleFunc("/tasks", taskManager.CreateTaskHandler).Methods("POST")
	r.HandleFunc("/tasks/{id}/files", taskManager.AddFileHandler).Methods("POST")
	r.HandleFunc("/tasks/{id}", taskManager.GetTaskStatusHandler).Methods("GET")
	r.HandleFunc("/archives/{filename}", taskManager.ServeArchiveHandler).Methods("GET")
	r.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("Server starting on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

func cleanupOldArchives(maxAge time.Duration) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() && filepath.Ext(path) == ".zip" {
				if time.Since(info.ModTime()) > maxAge {
					log.Printf("Deleting old archive: %s", path)
					os.Remove(path)
				}
			}
			return nil
		})
	}
}
