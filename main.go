package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type Task struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Completed   bool   `json:"completed"`
}

type TaskStore interface {
	AddTask(title string, description string) Task
	RemoveTask(id int) error
	ListTasks() []Task
	GetTask(id int) (Task, error)
	CompleteTask(id int) error
}

const traceIDKey = "TraceID"

var taskStore TaskStore
var logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

func main() {
	// Command-line argument to choose the task store type.
	storeType := flag.String("store", "memory", "Specify the task store: 'memory' or 'json'")
	flag.Parse()

	// Initialize the task store based on the provided type.
	switch *storeType {
	case "json":
		taskStore = newJSONTaskStore("tasks.json")
	case "memory":
		taskStore = localTaskStore()
	default:
		fmt.Println("Invalid store type. Use 'memory' or 'json'.")
		os.Exit(1)
	}

	// Start the server and CLI concurrently.
	go startServer()
	runCLI()
}

func startServer() {

	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", taskHandler)
	mux.HandleFunc("/tasks/", singleTaskHandler) // For operations that require a task ID

	loggedMux := TraceMiddleware(mux)

	fmt.Printf("Starting REST API server on http://localhost:8080\n> ")
	if err := http.ListenAndServe(":8080", loggedMux); err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
}

func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := uuid.NewString()
		ctx := context.WithValue(r.Context(), traceIDKey, traceID)
		r = r.WithContext(ctx)

		logger.Info("Request received", "method", r.Method, "url", r.URL.String(), "traceID", traceID)
		next.ServeHTTP(w, r)
	})
}

func taskHandler(w http.ResponseWriter, r *http.Request) {
	traceID := r.Context().Value(traceIDKey).(string)

	switch r.Method {
	case http.MethodGet:
		logger.Info("Listing tasks", "traceID", traceID)
		tasks := taskStore.ListTasks()
		writeJSONResponse(w, http.StatusOK, tasks)

	case http.MethodPost:
		logger.Info("Creating task", "traceID", traceID)
		var task Task
		if !parseJSONRequest(w, r, &task) {
			return
		}
		newTask := taskStore.AddTask(task.Title, task.Description)
		logger.Info("Added task", "traceID", traceID, "taskID", newTask.ID)
		writeJSONResponse(w, http.StatusCreated, newTask)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		logger.Error("Unsupported method", "method", r.Method, "traceID", traceID)
	}
}

func singleTaskHandler(w http.ResponseWriter, r *http.Request) {
	traceID := r.Context().Value(traceIDKey).(string)
	idStr := strings.TrimPrefix(r.URL.Path, "/tasks/")

	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		logger.Error("Invalid task id", "id", id, "traceID", traceID)
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet: // Fetch a single task
		logger.Info("Fetching task", "taskID", id, "traceID", traceID)
		task, err := taskStore.GetTask(id)
		if err != nil {
			logger.Error("Task not found", "taskID", id, "traceID", traceID)
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
		writeJSONResponse(w, http.StatusOK, task)

	case http.MethodPut: // Update a task (mark as complete)
		logger.Info("Completing task", "taskID", id, "traceID", traceID)
		if err := taskStore.CompleteTask(id); err != nil {
			logger.Error("Failed to complete task", "taskID", id, "traceID", traceID, "error", err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)

	case http.MethodDelete: // Delete a task
		logger.Info("Deleting task", "taskID", id, "traceID", traceID)
		if err := taskStore.RemoveTask(id); err != nil {
			logger.Error("Failed to delete task", "taskID", id, "traceID", traceID, "error", err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		logger.Error("Unsupported method", "method", r.Method, "traceID", traceID)
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

func runCLI() {
	scanner := bufio.NewScanner(os.Stdin)
	logger.Info("Task Manager started (connected to REST API)")
	printHelp()

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		parts := strings.Fields(input)

		//if user gives a blank command do nothing
		if len(parts) == 0 {
			continue
		}

		cmd := parts[0]
		args := parts[1:]

		switch cmd {
		case "add":
			handleAdd(args)
		case "list":
			handleList()
		case "get":
			handleGetTaskByID(args)
		case "complete":
			handleComplete(args)
		case "delete":
			handleDelete(args)
		case "help":
			printHelp()
		case "exit":
			fmt.Println("Exiting Task Manager.")
			os.Exit(0)
		default:
			fmt.Println("Unknown command. Type 'help' for available commands.")
		}
	}
}

func printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  add <title> <description>    Add a new task")
	fmt.Println("  list                         List all tasks")
	fmt.Println("  complete <id>                Mark a task as completed")
	fmt.Println("  delete <id>                  Delete a task")
	fmt.Println("  help                         Show this help message")
	fmt.Println("  exit                         Exit the program")
}

func handleAdd(args []string) {
	command := strings.Join(args, " ")

	quoteRegex := regexp.MustCompile(`"(.*?)"`)
	matches := quoteRegex.FindAllStringSubmatch(command, -1)

	if len(matches) < 2 {
		logger.Info("Usage: add \"<title>\" \"<description>\"", "args", args)
		return
	}

	title := matches[0][1]
	description := matches[1][1]

	task := Task{
		Title:       title,
		Description: description,
	}
	resp, err := http.Post("http://localhost:8080/tasks", "application/json", toJSON(task))
	if err != nil {
		logger.Error("Failed to add task", "taskID", task.ID, "error", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		logger.Info("Task added successfully", "taskID", task.ID)
	} else {
		logger.Error("Failed to add task", "error", err)
	}
}

func handleList() {
	resp, err := http.Get("http://localhost:8080/tasks")
	if err != nil {
		logger.Error("Failed to list tasks", "error", err)
		return
	}
	defer safeClose(resp.Body)

	var tasks []Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		logger.Error("Failed to list tasks", "error", err)
		return
	}

	if len(tasks) == 0 {
		logger.Info("No tasks found")
		return
	}

	for _, task := range tasks {
		logger.Info("Task added successfully", "taskID", task.ID)
	}
}

func handleGetTaskByID(args []string) {
	if len(args) != 1 {
		logger.Info("Usage: get <id>")
		return
	}

	id := args[0]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s", id)

	resp, err := http.Get(url)
	if err != nil {
		logger.Error("Failed to get task", "id", id, "error", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusOK {
		var task Task
		if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
			logger.Error("Error decoding response:", "error", err)
			return
		}
		fmt.Printf("ID: %d, Title: %s, Description: %s, Completed: %v\n",
			task.ID, task.Title, task.Description, task.Completed)
	} else if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("Task with ID %s not found.\n", id)
	} else {
		fmt.Printf("Unexpected error: %s\n", resp.Status)
	}
}

func handleComplete(args []string) {
	if len(args) < 1 {
		logger.Info("Usage: complete <id>")
		return
	}

	id := args[0]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s", id)

	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		logger.Error("Error creating request:", "error", err)
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to complete task", "id", id, "error", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusOK {
		logger.Info("Task completed successfully", "id", id)
	} else {
		logger.Error("Failed to complete task", "id", id, "error", resp.Status)
	}
}

func handleDelete(args []string) {
	if len(args) < 1 {
		logger.Info("Usage: delete <id>")
		return
	}

	id := args[0]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s", id)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		logger.Error("Error creating request:", "error", err)
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to delete task", "id", id, "error", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("Task %s deleted successfully.\n", id)
	} else {
		fmt.Printf("Failed to delete task %s: %s\n", id, resp.Status)
	}
}

func safeClose(c io.Closer) {
	if err := c.Close(); err != nil {
		logger.Error("Error closing connection:", "error", err)
	}
}

func toJSON(task Task) *strings.Reader {
	data, _ := json.Marshal(task)
	return strings.NewReader(string(data))
}

type inMemoryTaskStore struct {
	tasks       map[int]Task
	mutex       sync.Mutex
	idSeq       int
	reusableIds []int
}

func localTaskStore() *inMemoryTaskStore {
	return &inMemoryTaskStore{
		tasks: make(map[int]Task),
	}
}

func (store *inMemoryTaskStore) AddTask(title string, description string) Task {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	var id int

	if len(store.reusableIds) > 0 {
		id = store.reusableIds[0]
		store.reusableIds = store.reusableIds[1:]
	} else {
		store.idSeq++
		id = store.idSeq
	}

	task := Task{
		ID:          id,
		Title:       title,
		Description: description,
		Completed:   false,
	}

	store.tasks[task.ID] = task
	return task
}

func (store *inMemoryTaskStore) RemoveTask(id int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if _, ok := store.tasks[id]; !ok {
		return errors.New("task not found")
	}

	delete(store.tasks, id)
	store.reusableIds = append(store.reusableIds, id)
	sort.Ints(store.reusableIds)
	return nil
}

func (store *inMemoryTaskStore) ListTasks() []Task {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	taskList := make([]Task, 0, len(store.tasks))

	for _, task := range store.tasks {
		taskList = append(taskList, task)
	}

	return taskList
}

func (store *inMemoryTaskStore) GetTask(id int) (Task, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	task, exists := store.tasks[id]
	if !exists {
		return Task{}, errors.New("task not found")
	}
	return task, nil
}

func (store *inMemoryTaskStore) CompleteTask(id int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	task, exists := store.tasks[id]
	if !exists {
		return errors.New("task not found")
	}

	task.Completed = true
	store.tasks[id] = task
	return nil
}

type jsonTaskStore struct {
	filePath    string
	mutex       sync.Mutex
	tasks       map[int]Task
	idSeq       int
	reusableIds []int
}

func newJSONTaskStore(filePath string) *jsonTaskStore {
	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Create an empty file if it doesn't exist
		if err := createEmptyJSONFile(filePath); err != nil {
			logger.Error("Failed to create empty JSON file", "error", err)
			os.Exit(1)
		}
	}

	// Initialize the task store
	store := &jsonTaskStore{
		filePath:    filePath,
		tasks:       make(map[int]Task),
		reusableIds: []int{},
	}

	// Load tasks from the file during initialization
	if err := store.loadFromFile(); err != nil {
		logger.Error("Failed to load JSON file", "error", err)
		os.Exit(1)
	}

	return store
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

	// Write an empty JSON array to the file
	_, err = file.WriteString("[]")
	return err
}

func (store *jsonTaskStore) loadFromFile() error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	file, err := os.Open(store.filePath)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			logger.Error("Error closing file:", "error", err)
		}
	}(file)

	tasks := make(map[int]Task) // Match the type used in saveToFile
	if err := json.NewDecoder(file).Decode(&tasks); err != nil {
		return err
	}

	store.tasks = tasks

	// Reset reusableIds and track used IDs
	store.reusableIds = []int{}
	usedIds := make(map[int]bool)

	// Determine the highest ID to update the sequence
	highestID := 0

	for id := range tasks {
		usedIds[id] = true // Mark ID as used
		if id > highestID {
			highestID = id // Update the highest ID
		}
	}

	// Populate reusableIds with missing IDs
	for id := 1; id < highestID; id++ {
		if !usedIds[id] {
			store.reusableIds = append(store.reusableIds, id)
		}
	}

	store.idSeq = highestID

	return nil
}

func (store *jsonTaskStore) saveToFile() error {
	file, err := os.Create(store.filePath)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			logger.Error("Error closing file:", "error", err)
		}
	}(file)

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(store.tasks)
}

func (store *jsonTaskStore) AddTask(title string, description string) Task {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	var id int
	if len(store.reusableIds) > 0 {
		id = store.reusableIds[0]
		store.reusableIds = store.reusableIds[1:] // Remove the first element
	} else {
		store.idSeq++
		id = store.idSeq
	}

	task := Task{
		ID:          id,
		Title:       title,
		Description: description,
		Completed:   false,
	}
	store.tasks[task.ID] = task

	if err := store.saveToFile(); err != nil {
		logger.Error("Failed to save JSON file", "error", err)
	}

	return task
}

func (store *jsonTaskStore) RemoveTask(id int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if _, exists := store.tasks[id]; !exists {
		logger.Error("Task not found for deletion", "taskID", id)
		return errors.New("task not found")
	}

	delete(store.tasks, id)
	store.reusableIds = append(store.reusableIds, id) // Add ID to reusable IDs

	if err := store.saveToFile(); err != nil {
		logger.Error("Error saving to file after deletion", "error", err)
		return err
	}

	logger.Info("Task deleted and file updated", "taskID", id)
	return nil
}

func (store *jsonTaskStore) ListTasks() []Task {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	taskList := make([]Task, 0, len(store.tasks))

	for _, task := range store.tasks {
		taskList = append(taskList, task)
	}

	sort.Slice(taskList, func(i, j int) bool {
		return taskList[i].ID < taskList[j].ID
	})

	return taskList
}

func (store *jsonTaskStore) GetTask(id int) (Task, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	task, exists := store.tasks[id]
	if !exists {
		return Task{}, errors.New("task not found")
	}
	return task, nil
}

func (store *jsonTaskStore) CompleteTask(id int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	task, exists := store.tasks[id]
	if !exists {
		logger.Error("Task not found", "taskID", id)
		return errors.New("task not found")
	}

	task.Completed = true
	store.tasks[id] = task

	if err := store.saveToFile(); err != nil {
		logger.Error("Error saving to file", "error", err)
		return err
	}

	logger.Info("Task marked as complete and saved to file", "taskID", id)
	return nil
}
