package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const traceIDKey = "TraceID"

func startServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", taskHandler)
	mux.HandleFunc("/tasks/", singleTaskHandler) // For operations that require a task ID
	mux.HandleFunc("/users", addUserHandler)
	mux.HandleFunc("/users/list", listUsersHandler)

	loggedMux := TraceMiddleware(mux)

	fmt.Printf("Starting REST API server on http://localhost:8080\n> ")
	if err := http.ListenAndServe(":8080", loggedMux); err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
}

func listUsersHandler(w http.ResponseWriter, _ *http.Request) {
	users := userStore.ListUsers()
	writeJSONResponse(w, http.StatusOK, users)
}

func addUserHandler(w http.ResponseWriter, r *http.Request) {
	var user User
	if !parseJSONRequest(w, r, &user) {
		return
	}

	if err := userStore.AddUser(user.Username, user.Password); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	writeJSONResponse(w, http.StatusCreated, user)
}

func taskHandler(w http.ResponseWriter, r *http.Request) {
	traceID := r.Context().Value(traceIDKey).(string)

	// Use the stored logged-in username
	userName := loggedInUsername

	if userName == "" {
		http.Error(w, "Username is required", http.StatusBadRequest)
		return
	}

	// Check if the username exists
	if !usernameExists(userName) {
		http.Error(w, "Username does not exist", http.StatusNotFound)
		logger.Error("Username does not exist", "traceID", traceID, "userName", userName)
		return
	}

	switch r.Method {
	case http.MethodGet:
		logger.Info("Listing tasks", "traceID", traceID, "userName", userName)
		tasks := taskStore.ListTasks(userName)
		writeJSONResponse(w, http.StatusOK, tasks)

	case http.MethodPost:
		logger.Info("Creating task", "traceID", traceID, "userName", userName)
		var task Task
		if !parseJSONRequest(w, r, &task) {
			return
		}
		newTask := taskStore.AddTask(userName, task.Title, task.Description)
		logger.Info("Added task", "traceID", traceID, "taskID", newTask.ID, "userName", userName)
		writeJSONResponse(w, http.StatusCreated, newTask)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		logger.Error("Unsupported method", "method", r.Method, "traceID", traceID)
	}
}

func singleTaskHandler(w http.ResponseWriter, r *http.Request) {
	traceID := r.Context().Value(traceIDKey).(string)
	userName := r.URL.Query().Get("username") // Get username from query parameters
	idStr := strings.TrimPrefix(r.URL.Path, "/tasks/")

	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		logger.Error("Invalid task id", "id", id, "traceID", traceID)
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet: // Fetch a single task
		logger.Info("Fetching task", "taskID", id, "traceID", traceID, "userName", userName)
		task, err := taskStore.GetTask(userName, id)
		if err != nil {
			logger.Error("Task not found", "taskID", id, "traceID", traceID, "userName", userName)
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
		writeJSONResponse(w, http.StatusOK, task)

	case http.MethodPut: // Update a task (mark as complete)
		logger.Info("Completing task", "taskID", id, "traceID", traceID, "userName", userName)
		if err := taskStore.CompleteTask(userName, id); err != nil {
			logger.Error("Failed to complete task", "taskID", id, "traceID", traceID, "userName", userName, "error", err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)

	case http.MethodDelete: // Delete a task
		logger.Info("Deleting task", "taskID", id, "traceID", traceID, "userName", userName)
		if err := taskStore.RemoveTask(userName, id); err != nil {
			logger.Error("Failed to delete task", "taskID", id, "traceID", traceID, "userName", userName, "error", err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		logger.Error("Unsupported method", "method", r.Method, "traceID", traceID)
	}
}
