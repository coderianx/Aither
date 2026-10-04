package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/coderianx/Aither/internal/handlers"
	"github.com/coderianx/Aither/internal/store"
	"github.com/coderianx/Aither/web"
	"github.com/go-chi/chi/v5"
)

func main() {
	port := flag.String("port", ":8000", "Port for server")
	dbPath := flag.String("db", "", "SQLite database path (default: ~/.aither/aither.db)")

	flag.Parse()

	path, err := resolveDBPath(*dbPath)
	if err != nil {
		log.Fatalf("Cannot resolve database path: %v", err)
	}

	if err := store.ConnectDB(path); err != nil {
		log.Fatalf("Database connect error: %v", err)
	}
	defer store.CloseDB()

	log.Printf("Database connected: %s", path)

	router := chi.NewRouter()

	router.Route("/api/v1", func(r chi.Router) {
		r.Get("/providers", handlers.ListProviders)
		r.Post("/providers", handlers.ConnectProvider)
		r.Delete("/providers/{provider_id}", handlers.DeleteProvider)

		r.Get("/conversations", handlers.ListConversations)
		r.Post("/conversations", handlers.CreateConversation)
		r.Get("/conversations/{conversation_id}", handlers.GetConversation)
		r.Patch("/conversations/{conversation_id}", handlers.UpdateConversation)
		r.Delete("/conversations/{conversation_id}", handlers.DeleteConversation)

		r.Get("/conversations/{conversation_id}/messages", handlers.ListMessages)
		r.Post("/conversations/{conversation_id}/messages", handlers.SendMessage)

		r.Get("/providers/{provider_id}/models", handlers.ListModels)
	})

	// Embedded single-page client: serves index.html at the site root.
	router.Handle("/*", staticHandler())

	if err := http.ListenAndServe(*port, router); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// staticHandler serves the embedded web client. Assets are always
// revalidated so a rebuilt binary never serves stale files.
func staticHandler() http.Handler {
	fileServer := http.FileServer(http.FS(web.FS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	})
}

// resolveDBPath returns the database path to use. When no explicit path is
// given it falls back to ~/.aither/aither.db, creating the directory if needed.
func resolveDBPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(home, ".aither")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	return filepath.Join(dir, "aither.db"), nil
}
