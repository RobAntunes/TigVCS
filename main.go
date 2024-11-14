package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"tig/internal/api"
	"tig/internal/config"
	content "tig/internal/content"
	"tig/internal/intent/storage"
	"tig/internal/logging"
	"tig/internal/middleware"
	streamStorage "tig/internal/stream/storage"
	ws "tig/internal/workspace"

	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatal("failed to load config:", err)
	}

	// Initialize logger
	logger, err := logging.NewLogger(cfg.LogLevel)
	if err != nil {
		log.Fatal("failed to initialize logger:", err)
	}
	defer logger.Sync()

	// Initialize BadgerDB
	db, err := badger.Open(badger.DefaultOptions(cfg.Database.Path))
	if err != nil {
		logger.Fatal("failed to open database", zap.Error(err))
	}
	defer db.Close()

	// Initialize content store
	contentStore, err := content.NewFileStore(filepath.Join(cfg.Database.Path, "objects"))
	if err != nil {
		logger.Fatal("failed to initialize content store", zap.Error(err))
	}

	// Initialize workspace
	ws, err := ws.NewLocalWorkspace(cfg.Database.Path, db, contentStore.Safe)
	if err != nil {
		logger.Fatal("failed to initialize workspace", zap.Error(err))
	}

	// Initialize repositories
	intentStore := storage.NewStore(db, ws)
	streamStore := streamStorage.NewStore(db, intentStore)

	// Initialize handlers
	intentHandler := api.NewIntentHandler(intentStore)
	streamHandler := api.NewStreamHandler(streamStore)
	// Set up router
	mux := http.NewServeMux()

	// Health checks
	mux.HandleFunc("/health", healthCheck)

	// Intent endpoints
	mux.HandleFunc("/api/intents", intentHandler.Create)
	mux.HandleFunc("/api/intents/{id}", intentHandler.Get)
	mux.HandleFunc("/api/intents/{id}", intentHandler.Update)

	// Stream endpoints
	mux.HandleFunc("/api/streams", streamHandler.Create)
	mux.HandleFunc("/api/streams/{id}", streamHandler.Delete)
	mux.HandleFunc("/api/streams/{id}/intents", streamHandler.AddIntent)
	mux.HandleFunc("/api/streams/{id}/feature-flags", streamHandler.SetFeatureFlag)
	mux.HandleFunc("/api/streams/{id}/feature-flags", streamHandler.GetFeatureFlags)

	// Apply middleware
	handler := middleware.Chain(
		mux,
		middleware.RequestID,
		middleware.Logger(logger),
		middleware.Recover(logger),
	)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Info("starting server", zap.String("address", addr))

	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Fatal("server failed", zap.Error(err))
	}
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"healthy"}`))
}
