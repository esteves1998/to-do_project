package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

func main() {
	store := localTaskStore()

	if len(os.Args) > 1 && isCLICommand(os.Args[1]) {
		handleCLICommand(store, os.Args[1:])
		return
	}

	go startHTTPServer(store)
	fmt.Println("Server running on port 8080. Enter commands (add, list, complete, delete, or exit):")

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "exit" {
			fmt.Println("Exiting...")
			break
		}

		args := strings.Fields(input)
		if len(args) == 0 {
			continue
		}

		switch args[0] {
		case "add":
			handleAddCommand(store, args[1:])
		case "list":
			handleListCommand(store)
		case "complete":
			handleCompleteCommand(store, args[1:])
		case "delete":
			handleDeleteCommand(store, args[1:])
		default:
			fmt.Println("Unknown command. Supported commands: add, list, complete, delete, exit")
		}
	}
}

func startHTTPServer(store *InMemoryTaskStore) {
	http.HandleFunc("/tasks", store.handleTasks)
	http.HandleFunc("/tasks/", store.handleTasksById)

	fmt.Println("Listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}

func isCLICommand(cmd string) bool {
	switch cmd {
	case "add", "list", "complete", "delete":
		return true
	}
	return false
}

func handleCLICommand(store *InMemoryTaskStore, args []string) {
	switch args[0] {
	case "add":
		handleAddCommand(store, args[1:])
	case "list":
		handleListCommand(store)
	case "complete":
		handleCompleteCommand(store, args[1:])
	case "delete":
		handleDeleteCommand(store, args[1:])
	default:
		fmt.Println("Unknown command. Supported commands: add, list, complete, delete")
	}
}

func handleAddCommand(store *InMemoryTaskStore, args []string) {
	addCmd := flag.NewFlagSet("add", flag.ContinueOnError)
	title := addCmd.String("title", "", "Title of the task")
	desc := addCmd.String("description", "", "Description of the task")

	if err := addCmd.Parse(args); err != nil {
		fmt.Printf("Error parsing args: %v\n", err)
		return
	}

	if *title == "" {
		fmt.Println("title is required")
		return
	}

	task := store.AddTask(*title, *desc)
	fmt.Printf("Added task: %+v\n", task)
}

func handleListCommand(store *InMemoryTaskStore) {
	tasks := store.ListTasks()
	for _, task := range tasks {
		fmt.Printf("%+v\n", task)
	}
}

func handleCompleteCommand(store *InMemoryTaskStore, args []string) {
	completeCmd := flag.NewFlagSet("complete", flag.ContinueOnError)
	completeId := completeCmd.Int("id", 0, "ID of the task to complete")

	if err := completeCmd.Parse(args); err != nil {
		fmt.Printf("Error parsing flags: %v\n", err)
		return
	}

	if *completeId == 0 {
		fmt.Println("id is required")
		return
	}

	if err := store.CompleteTask(*completeId); err != nil {
		fmt.Printf("Error completing task: %v\n", err)
		return
	}
	fmt.Printf("Completed task with ID: %d\n", *completeId)
}

func handleDeleteCommand(store *InMemoryTaskStore, args []string) {
	deleteCmd := flag.NewFlagSet("delete", flag.ContinueOnError)
	deleteId := deleteCmd.Int("id", 0, "ID of the task to delete")

	if err := deleteCmd.Parse(args); err != nil {
		fmt.Printf("Error parsing flags: %v\n", err)
		return
	}

	if *deleteId == 0 {
		fmt.Println("id is required")
		return
	}

	if err := store.RemoveTask(*deleteId); err != nil {
		fmt.Printf("Error deleting task: %v\n", err)
		return
	}
	fmt.Printf("Deleted task with ID: %d\n", *deleteId)
}

// Task definition
type Task struct {
	ID          int
	Title       string
	Description string
	Completed   bool
}

// InMemoryTaskStore definition
type InMemoryTaskStore struct {
	tasks map[int]Task
	mutex sync.Mutex
	idSeq int
}

func localTaskStore() *InMemoryTaskStore {
	return &InMemoryTaskStore{
		tasks: make(map[int]Task),
	}
}

func (store *InMemoryTaskStore) AddTask(title string, description string) Task {
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

func (store *InMemoryTaskStore) RemoveTask(id int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if _, ok := store.tasks[id]; !ok {
		return errors.New("task not found")
	}

	delete(store.tasks, id)
	return nil
}

func (store *InMemoryTaskStore) ListTasks() []Task {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	taskList := make([]Task, 0, len(store.tasks))

	for _, task := range store.tasks {
		taskList = append(taskList, task)
	}

	return taskList
}

func (store *InMemoryTaskStore) CompleteTask(id int) error {
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

// HTTP Handlers
func (store *InMemoryTaskStore) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tasks := store.ListTasks()
		if err := json.NewEncoder(w).Encode(tasks); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	case http.MethodPost:
		task := &Task{}
		if err := json.NewDecoder(r.Body).Decode(task); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		newTask := store.AddTask(task.Title, task.Description)
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(newTask); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	default:
		http.Error(w, "Only GET and POST methods are supported on this endpoint", http.StatusMethodNotAllowed)
	}
}

func (store *InMemoryTaskStore) handleTasksById(w http.ResponseWriter, r *http.Request) {
	taskId := r.URL.Path[len("/tasks/"):]
	id, err := strconv.Atoi(taskId)

	if err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		if err := store.CompleteTask(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := store.RemoveTask(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Only PATCH and DELETE methods are supported on this endpoint", http.StatusMethodNotAllowed)
	}
}
