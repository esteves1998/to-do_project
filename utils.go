package main

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"net/http"
	"os"
)

func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate and attach a unique trace ID
		traceID := uuid.NewString()
		ctx := context.WithValue(r.Context(), traceIDKey, traceID)
		r = r.WithContext(ctx)

		// Override HTTP method if `_method` is provided
		if r.Method == http.MethodPost {
			overrideMethod := r.FormValue("_method")
			if overrideMethod == http.MethodPut || overrideMethod == http.MethodDelete {
				r.Method = overrideMethod
			}
		}

		logger.Info("Request received", "method", r.Method, "url", r.URL.String(), "traceID", traceID)
		next.ServeHTTP(w, r)
	})
}

func createEmptyJSONFile(filePath string) error {
	// Create the file if it doesn't exist
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			logger.Error("Error closing file:", "error", err)
		}
	}(file)

	// Write an empty JSON object to the file
	_, err = file.WriteString("{}")
	return err
}

func safeClose(c io.Closer) {
	if err := c.Close(); err != nil {
		logger.Error("Error closing connection:", "error", err)
	}
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Error("Failed to encode JSON response", "error", err)
		http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError)
	}
}

func parseJSONRequest(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if r.Body == nil {
		http.Error(w, "Request body is empty", http.StatusBadRequest)
		return false
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			logger.Error("Failed to close request body", "error", err)
		}
	}(r.Body)

	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		logger.Error("Failed to decode JSON request", "error", err)
		http.Error(w, "Invalid JSON request", http.StatusBadRequest)
		return false
	}
	return true
}
