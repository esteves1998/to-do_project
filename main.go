package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

var taskStore = localTaskStore()

func main() {
	go startServer()
	runCLI()
}

func startServer() {
	http.HandleFunc("/tasks", taskHandler)
	http.HandleFunc("/tasks/", singleTaskHandler) // For operations that require a task ID
	fmt.Printf("Starting REST API server on http://localhost:8080\n> ")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
}

func taskHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tasks := taskStore.ListTasks()
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(tasks); err != nil {
			http.Error(w, "Failed to encode tasks", http.StatusInternalServerError)
			return
		}
	case http.MethodPost:
		var task Task
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
			http.Error(w, "Invalid task data", http.StatusBadRequest)
			return
		}
		newTask := taskStore.AddTask(task.Title, task.Description)
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(newTask); err != nil {
			http.Error(w, "Failed to encode new task", http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func singleTaskHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "tasks/")

	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		if err := taskStore.CompleteTask(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if err := taskStore.RemoveTask(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		task, err := taskStore.GetTask(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(task); err != nil {
			http.Error(w, "Failed to encode task", http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
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
	}
	id := args[0]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s", id)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer safeClose(resp.Body)
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		var task Task
		if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
			fmt.Println("Error:", err)
			return
		}
		fmt.Printf("ID: %d, Title: %s, Description: %s, Completed: %v\n",
			task.ID, task.Title, task.Description, task.Completed)
	} else {
		fmt.Printf("Task with ID %s not found.\n", id)
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

func localTaskStore() *inMemoryTaskStore {
	return &inMemoryTaskStore{
		tasks: make(map[int]Task),
	}
}

func (store *inMemoryTaskStore) AddTask(title string, description string) Task {
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
