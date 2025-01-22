package main

import (
	"log/slog"
	"os"
)

// Global logger instance
var logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

func InitializeLogger() {
	logger.Info("Logger initialized")
}
