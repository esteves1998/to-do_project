package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
)

var isLoggedIn bool
var loggedInUsername string

type Task struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Completed   bool   `json:"completed"`
}

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TaskStore interface {
	AddTask(userName, title string, description string) Task
	RemoveTask(userName string, id int) error
	ListTasks(userName string) []Task
	GetTask(userName string, id int) (Task, error)
	CompleteTask(userName string, id int) error
}

type UserStore struct {
	users map[string]User
	mutex sync.Mutex
}

var userStore UserStore
var logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

const traceIDKey = "TraceID"

var taskStore TaskStore

func main() {
	// Initialize the user store
	initializeUserStore()

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

	initializeUserStore()

	// Start the server and CLI concurrently.
	go startServer()
	runCLI()
}

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

func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := uuid.NewString()
		ctx := context.WithValue(r.Context(), traceIDKey, traceID)
		r = r.WithContext(ctx)

		logger.Info("Request received", "method", r.Method, "url", r.URL.String(), "traceID", traceID)
		next.ServeHTTP(w, r)
	})
}

// Function to initialize the user store by checking and creating users.json
func initializeUserStore() {
	if _, err := os.Stat("users.json"); os.IsNotExist(err) {
		// Create an empty users.json file
		if err := createEmptyJSONFile("users.json"); err != nil {
			logger.Error("Failed to create empty users.json file", "error", err)
			os.Exit(1)
		}
	} else {
		// Load users from the existing users.json file
		if err := loadUsersFromFile(); err != nil {
			logger.Error("Failed to load users from file", "error", err)
			os.Exit(1)
		}
	}
}

// Function to load users from users.json
func loadUsersFromFile() error {
	file, err := os.Open("users.json")
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			logger.Error("Error closing file:", "error", err)
		}
	}(file)

	users := make(map[string]User)
	if err := json.NewDecoder(file).Decode(&users); err != nil {
		return err
	}

	userStore.users = users

	return nil
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

	// Loop until the user is logged in
	for {
		loginOrRegister(scanner) // Prompt for login or registration

		// After successful login, break out of the loop
		if isLoggedIn {
			break
		}
	}

	printHelp()

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		parts := strings.Fields(input)

		// If user gives a blank command do nothing
		if len(parts) == 0 {
			continue
		}

		cmd := parts[0]
		args := parts[1:]

		switch cmd {
		case "listUsers":
			handleListUsers()
		case "add":
			handleAdd(args)
		case "list":
			handleList(args)
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
	fmt.Println("  add \"<title>\" \"<description>\"    Add a new task for the logged-in user")
	fmt.Println("  list                                 List all tasks for the logged-in user")
	fmt.Println("  complete <id>                       Mark a task as completed for the logged-in user")
	fmt.Println("  delete <id>                         Delete a task for the logged-in user")
	fmt.Println("  help                                 Show this help message")
	fmt.Println("  exit                                 Exit the program")
	fmt.Println("  listUsers                            List all users")
}

func handleAdd(args []string) {
	if len(args) < 2 {
		logger.Info("Usage: add \"<title>\" \"<description>\"")
		return
	}

	// Use the stored logged-in username
	userName := loggedInUsername

	if userName == "" {
		logger.Info("You must be logged in to add a task.")
		return
	}

	quoteRegex := regexp.MustCompile(`"(.*?)"`)
	matches := quoteRegex.FindAllStringSubmatch(strings.Join(args, " "), -1)

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
	resp, err := http.Post(fmt.Sprintf("http://localhost:8080/tasks?username=%s", userName), "application/json", toJSON(task))
	if err != nil {
		logger.Error("Failed to add task", "error", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		logger.Info("Task added successfully", "title", title)
	} else {
		logger.Error("Failed to add task", "error", err)
	}
}

func handleList(args []string) {
	// Use the stored logged-in username
	userName := loggedInUsername

	if userName == "" {
		logger.Info("You must be logged in to list tasks.")
		return
	}

	resp, err := http.Get(fmt.Sprintf("http://localhost:8080/tasks?username=%s", userName))
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
		logger.Info("No tasks found for user", "userName", userName)
		return
	}

	for _, task := range tasks {
		logger.Info("Task found", "taskID", task.ID, "userName", userName)
	}
}

func handleGetTaskByID(args []string) {
	if len(args) != 2 {
		logger.Info("Usage: get <username> <id>")
		return
	}

	userName := args[0]
	id := args[1]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s?username=%s", id, userName)

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
		fmt.Printf("Task with ID %s not found for user %s.\n", id, userName)
	} else {
		fmt.Printf("Unexpected error: %s\n", resp.Status)
	}
}

func handleComplete(args []string) {
	if len(args) < 1 {
		logger.Info("Usage: complete <id>")
		return
	}

	// Use the stored logged-in username
	userName := loggedInUsername

	if userName == "" {
		logger.Info("You must be logged in to complete a task.")
		return
	}

	id := args[0]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s?username=%s", id, userName)

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
		logger.Info("Task completed successfully", "id", id, "userName", userName)
	} else {
		logger.Error("Failed to complete task", "id", id, "error", resp.Status)
	}
}

func handleDelete(args []string) {
	if len(args) < 1 {
		logger.Info("Usage: delete <id>")
		return
	}

	// Use the stored logged-in username
	userName := loggedInUsername

	if userName == "" {
		logger.Info("You must be logged in to delete a task.")
		return
	}

	id := args[0]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s?username=%s", id, userName) // Use the stored username

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
		fmt.Printf("Task %s deleted successfully for user %s.\n", id, userName)
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
	tasks       map[int]map[string]Task // Map of userName to tasks
	mutex       sync.Mutex
	idSeq       int
	reusableIds []int
}

func localTaskStore() *inMemoryTaskStore {
	return &inMemoryTaskStore{
		tasks: make(map[int]map[string]Task),
	}
}

func (store *inMemoryTaskStore) AddTask(userName, title string, description string) Task {
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

	if store.tasks[id] == nil {
		store.tasks[id] = make(map[string]Task)
	}
	store.tasks[id][userName] = task // Store task under the user

	return task
}

func (store *inMemoryTaskStore) RemoveTask(userName string, id int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if _, ok := store.tasks[id]; !ok {
		return errors.New("task not found")
	}

	if _, ok := store.tasks[id][userName]; !ok {
		return errors.New("task not found for user")
	}

	delete(store.tasks[id], userName)
	if len(store.tasks[id]) == 0 {
		delete(store.tasks, id) // Remove task if no users are left
	}

	store.reusableIds = append(store.reusableIds, id)
	sort.Ints(store.reusableIds)
	return nil
}

func (store *inMemoryTaskStore) ListTasks(userName string) []Task {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	var taskList []Task
	for _, userTasks := range store.tasks {
		if task, exists := userTasks[userName]; exists {
			taskList = append(taskList, task)
		}
	}

	return taskList
}

func (store *inMemoryTaskStore) GetTask(userName string, id int) (Task, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if userTasks, exists := store.tasks[id]; exists {
		if task, exists := userTasks[userName]; exists {
			return task, nil
		}
	}
	return Task{}, errors.New("task not found for user")
}

func (store *inMemoryTaskStore) CompleteTask(userName string, id int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if userTasks, exists := store.tasks[id]; exists {
		if task, exists := userTasks[userName]; exists {
			task.Completed = true
			userTasks[userName] = task
			return nil
		}
	}
	return errors.New("task not found for user")
}

type jsonTaskStore struct {
	filePath    string
	mutex       sync.Mutex
	tasks       map[string]map[int]Task // Map of userName to tasks
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
		tasks:       make(map[string]map[int]Task), // Initialize the map for user-specific tasks
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

	// Write an empty JSON object to the file
	_, err = file.WriteString("{}")
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

	tasks := make(map[string]map[int]Task) // Match the type used in saveToFile
	if err := json.NewDecoder(file).Decode(&tasks); err != nil {
		return err
	}

	store.tasks = tasks

	// Reset reusableIds and track used IDs
	store.reusableIds = []int{}
	usedIds := make(map[int]bool)

	// Determine the highest ID to update the sequence
	highestID := 0

	for _, userTasks := range tasks {
		for id := range userTasks {
			usedIds[id] = true // Mark ID as used
			if id > highestID {
				highestID = id // Update the highest ID
			}
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

func (store *jsonTaskStore) AddTask(userName, title string, description string) Task {
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

	// Store task under the user
	if store.tasks[userName] == nil {
		store.tasks[userName] = make(map[int]Task)
	}
	store.tasks[userName][task.ID] = task

	if err := store.saveToFile(); err != nil {
		logger.Error("Failed to save JSON file", "error", err)
	}

	return task
}

func (store *jsonTaskStore) RemoveTask(userName string, id int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if userTasks, exists := store.tasks[userName]; exists {
		if _, ok := userTasks[id]; !ok {
			return errors.New("task not found for user")
		}

		delete(userTasks, id)
		if len(userTasks) == 0 {
			delete(store.tasks, userName) // Remove user if no tasks are left
		}

		store.reusableIds = append(store.reusableIds, id)

		if err := store.saveToFile(); err != nil {
			logger.Error("Error saving to file after deletion", "error", err)
			return err
		}

		logger.Info("Task deleted and file updated", "taskID", id, "userName", userName)
		return nil
	}

	return errors.New("user not found")
}

func (store *jsonTaskStore) ListTasks(userName string) []Task {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	taskList := make([]Task, 0)

	if userTasks, exists := store.tasks[userName]; exists {
		for _, task := range userTasks {
			taskList = append(taskList, task)
		}
	}

	return taskList
}

func (store *jsonTaskStore) GetTask(userName string, id int) (Task, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if userTasks, exists := store.tasks[userName]; exists {
		if task, exists := userTasks[id]; exists {
			return task, nil
		}
	}
	return Task{}, errors.New("task not found for user")
}

func (store *jsonTaskStore) CompleteTask(userName string, id int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if userTasks, exists := store.tasks[userName]; exists {
		if task, exists := userTasks[id]; exists {
			task.Completed = true
			userTasks[id] = task

			if err := store.saveToFile(); err != nil {
				logger.Error("Error saving to file", "error", err)
				return err
			}

			logger.Info("Task marked as complete and saved to file", "taskID", id, "userName", userName)
			return nil
		}
	}
	return errors.New("task not found for user")
}

// Function to save users to a JSON file
func (store *UserStore) saveUsersToFile() error {
	file, err := os.Create("users.json")
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
	if err := encoder.Encode(store.users); err != nil {
		logger.Error("Failed to encode users to file", "error", err)
		return err
	}
	logger.Info("Users saved to file", "users", store.users) // Log the saved users
	return nil
}

// Modify the existing AddUser method to save to memory and then to file
func (store *UserStore) AddUser(username, password string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if _, exists := store.users[username]; exists {
		return errors.New("user already exists")
	}

	store.users[username] = User{Username: username, Password: password}

	if err := store.saveUsersToFile(); err != nil {
		return err
	}
	return nil
}

func handleAddUser(args []string) {
	if len(args) != 2 {
		logger.Info("Usage: addUser <username> <password>")
		return
	}

	username := args[0]
	password := args[1]
	if err := userStore.AddUser(username, password); err != nil {
		logger.Error("Failed to add user", "error", err)
		return
	}

	logger.Info("User added successfully", "username", username)
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

func (store *UserStore) ListUsers() []User {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	users := make([]User, 0, len(store.users))
	for _, user := range store.users {
		users = append(users, user)
	}
	return users
}

func listUsersHandler(w http.ResponseWriter, _ *http.Request) {
	users := userStore.ListUsers()
	writeJSONResponse(w, http.StatusOK, users)
}

func handleListUsers() {
	resp, err := http.Get("http://localhost:8080/users/list")
	if err != nil {
		logger.Error("Failed to list users", "error", err)
		return
	}
	defer safeClose(resp.Body)

	var users []User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		logger.Error("Failed to decode users response", "error", err)
		return
	}

	if len(users) == 0 {
		logger.Info("No users found")
		return
	}

	for _, user := range users {
		logger.Info("User found", "username", user.Username)
	}
}

func usernameExists(userName string) bool {
	_, exists := userStore.users[userName]
	if !exists {
		logger.Error("Username does not exist", "userName", userName)
		fmt.Printf("Error: Username '%s' does not exist.\n", userName)
	}
	return exists
}

func handleRegister(scanner *bufio.Scanner) {
	fmt.Print("Enter username: ")
	scanner.Scan()
	username := scanner.Text()

	fmt.Print("Enter password: ")
	scanner.Scan()
	password := scanner.Text()

	if err := userStore.AddUser(username, password); err != nil {
		fmt.Println("Registration failed:", err)
		isLoggedIn = false // Set login status to false
	} else {
		fmt.Println("Registration successful! You can now log in.")
		// Prompt for login immediately after registration
		handleLogin(scanner)
	}
}

func loginOrRegister(scanner *bufio.Scanner) {
	fmt.Println("Welcome to the Task Manager!")
	fmt.Println("Would you like to (1) Login or (2) Register?")

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		if input == "1" {
			handleLogin(scanner)
			break
		} else if input == "2" {
			handleRegister(scanner)
			break
		} else {
			fmt.Println("Invalid option. Please enter 1 to Login or 2 to Register.")
		}
	}
}

func handleLogin(scanner *bufio.Scanner) {
	fmt.Print("Enter username: ")
	scanner.Scan()
	username := scanner.Text()

	fmt.Print("Enter password: ")
	scanner.Scan()
	password := scanner.Text()

	if err := userStore.CheckPassword(username, password); err != nil {
		fmt.Println("Login failed:", err)
		isLoggedIn = false // Set login status to false
	} else {
		fmt.Println("Login successful!")
		isLoggedIn = true           // Set login status to true
		loggedInUsername = username // Store the logged-in username
	}
}

func (store *UserStore) CheckPassword(username, password string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	user, exists := store.users[username]
	if !exists {
		return errors.New("user not found")
	}

	if user.Password != password {
		return errors.New("invalid password")
	}

	return nil
}

//check api
//remove add user
//fix list so it shows the tasks isntead of the username
