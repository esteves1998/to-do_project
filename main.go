package main

import (
	"bufio"
	"bytes"
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
	AddTask(title, description string) Task
	RemoveTask(id int) error
	ListTasks() ([]Task, error)
	GetTask(id int) (*Task, error)
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
		taskStore = localMemoryStore()
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
		handleListTasks(w, traceID)
	case http.MethodPost:
		handleCreateTask(w, r, traceID)
	default:
		logger.Error("Method not allowed", "method", r.Method, "traceID", traceID)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleListTasks(w http.ResponseWriter, traceID string) {
	logger.Info("Listing tasks", "traceID", traceID)
	tasks, err := taskStore.ListTasks()
	if err != nil {
		logger.Error("Failed to list tasks", "error", err, "traceID", traceID)
		http.Error(w, "Failed to retrieve tasks", http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, tasks, http.StatusOK, traceID, "tasks")
}

func handleCreateTask(w http.ResponseWriter, r *http.Request, traceID string) {
	logger.Info("Creating task", "traceID", traceID)
	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		logger.Error("Invalid task data", "error", err, "traceID", traceID)
		http.Error(w, "Invalid task data", http.StatusBadRequest)
		return
	}

	newTask := taskStore.AddTask(task.Title, task.Description)
	logger.Info("Added task", "traceID", traceID, "taskID", newTask.ID)

	writeJSONResponse(w, newTask, http.StatusCreated, traceID, "new task")
}

func writeJSONResponse(w http.ResponseWriter, data interface{}, statusCode int, traceID, dataDescription string) {
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(data); err != nil {
		logger.Error("Failed to encode "+dataDescription, "error", err, "traceID", traceID)
		http.Error(w, "Failed to encode "+dataDescription, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(statusCode)
	if _, err := w.Write(buffer.Bytes()); err != nil {
		logger.Error("Failed to write response", "error", err, "traceID", traceID)
	}
}

func singleTaskHandler(w http.ResponseWriter, r *http.Request) {
	traceID := r.Context().Value(traceIDKey).(string)
	id, err := parseTaskID(r.URL.Path)
	if err != nil {
		logger.Error("Invalid task ID", "traceID", traceID, "error", err)
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		handleTaskRetrieval(w, id, traceID)
	case http.MethodPut:
		handleTaskUpdate(w, id, traceID)
	case http.MethodDelete:
		handleTaskDeletion(w, id, traceID)
	default:
		logger.Error("Method not allowed", "method", r.Method, "traceID", traceID)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func parseTaskID(path string) (int, error) {
	idStr := strings.TrimPrefix(path, "/tasks/")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid task ID")
	}
	return id, nil
}

func handleTaskRetrieval(w http.ResponseWriter, id int, traceID string) {
	task, err := taskStore.GetTask(id)
	if err != nil {
		logger.Error("Task not found", "traceID", traceID, "error", err)
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	writeJSONResponse(w, task, http.StatusOK, traceID, "task")
}

func handleTaskUpdate(w http.ResponseWriter, id int, traceID string) {
	err := taskStore.CompleteTask(id)
	if err != nil {
		logger.Error("Failed to complete task", "traceID", traceID, "error", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	logger.Info("Task completed", "traceID", traceID, "taskID", id)
	w.WriteHeader(http.StatusOK)
}

func handleTaskDeletion(w http.ResponseWriter, id int, traceID string) {
	logger.Info("Deleting task", "traceID", traceID, "taskID", id)
	if err := taskStore.RemoveTask(id); err != nil {
		logger.Error("Failed to delete task", "traceID", traceID, "taskID", id, "error", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	logger.Info("Task deleted successfully", "traceID", traceID, "taskID", id)
	w.WriteHeader(http.StatusOK)
}

func runCLI() {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Task Manager (connected to REST API)")
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
	if len(args) < 2 {
		fmt.Println("Usage: add <title> <description>")
		return
	}
	title := args[0]
	description := strings.Join(args[1:], " ")

	task := Task{
		Title:       title,
		Description: description,
	}
	resp, err := http.Post("http://localhost:8080/tasks", "application/json", toJSON(task))
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		fmt.Println("Task added successfully.")
	} else {
		fmt.Println("Failed to add task.")
	}
}

func handleList() {
	resp, err := http.Get("http://localhost:8080/tasks")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer safeClose(resp.Body)

	var tasks []Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		fmt.Println("Error:", err)
		return
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks available.")
		return
	}

	for _, task := range tasks {
		fmt.Printf("ID: %d, Title: %s, Description: %s, Completed: %v\n",
			task.ID, task.Title, task.Description, task.Completed)
	}
}

func handleGetTaskByID(args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: get <id>")
		return
	}

	id := args[0]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s", id)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusOK {
		var task Task
		if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
			fmt.Println("Error decoding response:", err)
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
		fmt.Println("Usage: complete <id>")
		return
	}

	id := args[0]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s", id)

	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("Task %s marked as completed.\n", id)
	} else {
		fmt.Printf("Failed to complete task %s: %s\n", id, resp.Status)
	}
}

func handleDelete(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: delete <id>")
		return
	}

	id := args[0]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s", id)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
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
		fmt.Println("Error closing resource:", err)
	}
}

func toJSON(task Task) *strings.Reader {
	data, _ := json.Marshal(task)
	return strings.NewReader(string(data))
}

type inMemoryTaskStore struct {
	tasks map[int]Task
	mutex sync.Mutex
	idSeq int
}

func localMemoryStore() *inMemoryTaskStore {
	return &inMemoryTaskStore{
		tasks: make(map[int]Task),
		idSeq: 0,
	}
}

func (store *inMemoryTaskStore) AddTask(title, description string) Task {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.idSeq++
	task := Task{
		ID:          store.idSeq,
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
	return nil
}

func (store *inMemoryTaskStore) ListTasks() ([]Task, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	taskList := make([]Task, 0, len(store.tasks))

	for _, task := range store.tasks {
		taskList = append(taskList, task)
	}

	return taskList, nil
}

func (store *inMemoryTaskStore) GetTask(id int) (*Task, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	task, exists := store.tasks[id]
	if !exists {
		return nil, errors.New("task not found")
	}
	return &task, nil
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
	filePath string
	mutex    sync.Mutex
	tasks    map[int]Task
	idSeq    int
}

func newJSONTaskStore(filePath string) *jsonTaskStore {
	store := &jsonTaskStore{
		filePath: filePath,
		tasks:    make(map[int]Task),
		idSeq:    0,
	}

	if err := store.load(); err != nil {
		fmt.Println("Error loading tasks from file:", err)
	}
	return store
}

func (store *jsonTaskStore) load() error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	file, err := os.Open(store.filePath)
	if os.IsNotExist(err) {
		return nil // No file exists yet, nothing to load
	} else if err != nil {
		return err
	}
	defer file.Close()

	var tasks []Task
	if err := json.NewDecoder(file).Decode(&tasks); err != nil {
		return err
	}

	for _, task := range tasks {
		store.tasks[task.ID] = task
		if task.ID > store.idSeq {
			store.idSeq = task.ID
		}
	}
	return nil
}

func (store *jsonTaskStore) save() error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	file, err := os.Create(store.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	tasks := make([]Task, 0, len(store.tasks))
	for _, task := range store.tasks {
		tasks = append(tasks, task)
	}

	return json.NewEncoder(file).Encode(tasks)
}

func (store *jsonTaskStore) AddTask(title, description string) Task {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.idSeq++
	task := Task{
		ID:          store.idSeq,
		Title:       title,
		Description: description,
		Completed:   false,
	}
	store.tasks[task.ID] = task
	_ = store.save() // Save changes to the file
	return task
}

func (store *jsonTaskStore) RemoveTask(id int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if _, exists := store.tasks[id]; !exists {
		return errors.New("task not found")
	}

	delete(store.tasks, id)
	return store.save()
}

func (store *jsonTaskStore) ListTasks() ([]Task, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	tasks := make([]Task, 0, len(store.tasks))
	for _, task := range store.tasks {
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (store *jsonTaskStore) GetTask(id int) (*Task, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	task, exists := store.tasks[id]
	if !exists {
		return nil, errors.New("task not found")
	}
	return &task, nil
}

func (store *jsonTaskStore) CompleteTask(id int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	task, exists := store.tasks[id]
	if !exists {
		return errors.New("task not found")
	}

	task.Completed = true
	store.tasks[id] = task
	return store.save()
}
