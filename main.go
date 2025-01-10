package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

func main() {
	store := taskStore()

	http.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tasks := store.ListTasks()
			json.NewEncoder(w).Encode(tasks)

		case http.MethodPost:
			var task Task
			if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
				http.Error(w, "Invalid input", http.StatusBadRequest)
				return
			}
			addedTask := store.AddTask(task.Title, task.Description)
			json.NewEncoder(w).Encode(addedTask)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/tasks/", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Path[len("/tasks/"):]
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPut:
			if err := store.CompleteTask(id); err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			fmt.Fprintf(w, "Task %d marked as completed", id)

		case http.MethodDelete:
			if err := store.RemoveTask(id); err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			fmt.Fprintf(w, "Task %d deleted", id)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	fmt.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
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

func handleAdd(args []string, store *inMemoryTaskStore) {
	if len(args) < 2 {
		fmt.Println("Usage: add <title> <description>")
		return
	}
	title := args[0]
	description := strings.Join(args[1:], " ")
	task := store.AddTask(title, description)
	fmt.Printf("Added task: %+v\n", task)
}

func handleList(_ []string, store *inMemoryTaskStore) {
	tasks := store.ListTasks()
	if len(tasks) == 0 {
		fmt.Println("No tasks available.")
		return
	}
	for _, task := range tasks {
		fmt.Printf("%+v\n", task)
	}
}

func handleComplete(args []string, store *inMemoryTaskStore) {
	if len(args) < 1 {
		fmt.Println("Usage: complete <id>")
		return
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Println("Invalid ID.")
		return
	}
	err = store.CompleteTask(id)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Completed task: %d\n", id)
	}
}

func handleDelete(args []string, store *inMemoryTaskStore) {
	if len(args) < 1 {
		fmt.Println("Usage: delete <id>")
		return
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Println("Invalid ID.")
		return
	}
	err = store.RemoveTask(id)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Deleted task: %d\n", id)
	}
}

type Task struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Completed   bool   `json:"completed"`
}

type inMemoryTaskStore struct {
	tasks map[int]Task
	mutex sync.Mutex
	idSeq int
}

func taskStore() *inMemoryTaskStore {
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
