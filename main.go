package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
)

func main() {
	store := localTaskStore()
	go startHTTPServer(store)

	if len(os.Args) < 2 {
		fmt.Println("Usage: [add|list|complete|delete] [options]")
		os.Exit(1)
	}

	// CLI command processing
	switch os.Args[1] {
	case "add":
		handleAddCommand(store)
	case "list":
		handleListCommand(store)
	case "complete":
		handleCompleteCommand(store)
	case "delete":
		handleDeleteCommand(store)
	default:
		fmt.Println("expected 'add', 'list', 'complete' or 'delete' subcommands")
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

func handleAddCommand(store *InMemoryTaskStore) {
	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	title := addCmd.String("title", "", "Title of the task")
	desc := addCmd.String("description", "", "Description of the task")

	addCmd.Parse(os.Args[2:])

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

func handleCompleteCommand(store *InMemoryTaskStore) {
	completeCmd := flag.NewFlagSet("complete", flag.ExitOnError)
	completeId := completeCmd.Int("id", 0, "ID of the task to complete")

	completeCmd.Parse(os.Args[2:])

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

func handleDeleteCommand(store *InMemoryTaskStore) {
	deleteCmd := flag.NewFlagSet("delete", flag.ExitOnError)
	deleteId := deleteCmd.Int("id", 0, "ID of the task to delete")

	deleteCmd.Parse(os.Args[2:])

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
