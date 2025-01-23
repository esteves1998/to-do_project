package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const traceIDKey = "TraceID"

func startServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", taskHandler)           // Task list and creation
	mux.HandleFunc("/tasks/", singleTaskHandler)    // Single task operations by ID
	mux.HandleFunc("/users", addUserHandler)        // User creation
	mux.HandleFunc("/users/list", listUsersHandler) // List users
	mux.HandleFunc("/login", loginHandler)          // Login page
	mux.HandleFunc("/register", registerHandler)    // Registration page
	mux.HandleFunc("/tasks/view", tasksHandler)     // View tasks (templated UI)

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

	var userName string
	if r.URL.Query().Get("username") != "" { // API case
		userName = r.URL.Query().Get("username")
	} else { // CLI case or session
		userName = loggedInUsername
	}

	if userName == "" {
		http.Error(w, "Username is required", http.StatusBadRequest)
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

	case http.MethodPut: // Mark task as complete
		logger.Info("Marking task as complete", "taskID", id, "traceID", traceID, "userName", userName)
		if err := taskStore.CompleteTask(userName, id); err != nil {
			logger.Error("Failed to complete task", "taskID", id, "traceID", traceID, "userName", userName, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

func loginHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, _ := template.ParseFiles("templates/login.html")

	if r.Method == http.MethodGet {
		err := tmpl.Execute(w, nil)
		if err != nil {
			return
		}
		return
	}

	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		if err := userStore.CheckPassword(username, password); err != nil {
			err := tmpl.Execute(w, map[string]string{"Error": "Invalid credentials"})
			if err != nil {
				return
			}
			return
		}

		//set flag and username so that CLI works even if we log in through the web app
		isLoggedIn = true
		loggedInUsername = username

		http.Redirect(w, r, "/tasks/view?username="+username, http.StatusSeeOther)
	}
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, _ := template.ParseFiles("templates/register.html")

	if r.Method == http.MethodGet {
		err := tmpl.Execute(w, nil)
		if err != nil {
			return
		}
		return
	}

	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		if err := userStore.AddUser(username, password); err != nil {
			err := tmpl.Execute(w, map[string]string{"Error": "User already exists"})
			if err != nil {
				return
			}
			return
		}

		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func tasksHandler(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "User not specified", http.StatusBadRequest)
		return
	}

	tasks := taskStore.ListTasks(username)
	tmpl, err := template.ParseFiles("templates/tasks.html")
	if err != nil {
		http.Error(w, "Unable to load template", http.StatusInternalServerError)
		return
	}

	// Render the template with the task list and username
	err = tmpl.Execute(w, struct {
		Username string
		Tasks    []Task
	}{
		Username: username,
		Tasks:    tasks,
	})
	if err != nil {
		http.Error(w, "Unable to render template", http.StatusInternalServerError)
	}
}
