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

	http.HandleFunc("/tasks", store.handleTasks)
	http.HandleFunc("/tasks/", store.handleTasksById)

	fmt.Println("Listening on :8080")
	http.ListenAndServe(":8080", nil)

	if len(os.Args) < 2 {
		fmt.Println("Usage: [add|list|complete|delete] [options]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "add":
		addCmd := flag.NewFlagSet("add", flag.ExitOnError)
		title := addCmd.String("title", "", "Title of the task")
		desc := addCmd.String("description", "", "Description of the task")

		addCmd.Parse(os.Args[2:])
		if *title == "" {
			fmt.Println("title is required")
		}
		task := store.AddTask(*title, *desc)
		//print struct with field names
		fmt.Printf("Added task: %+v\n", task)

	case "list":
		tasks := store.ListTasks()
		for _, task := range tasks {
			fmt.Printf("%+v\n", task)
		}

	case "complete":
		completeCmd := flag.NewFlagSet("complete", flag.ExitOnError)
		completeId := completeCmd.Int("id", 0, "ID of the task to complete")

		completeCmd.Parse(os.Args[2:])
		if *completeId == 0 {
			fmt.Println("id is required")
			return
		}
		fmt.Printf("Completed task: %+v\n", store.CompleteTask(*completeId))

	case "delete":
		deleteCmd := flag.NewFlagSet("delete", flag.ExitOnError)
		deleteId := deleteCmd.Int("id", 0, "ID of the task to delete")

		deleteCmd.Parse(os.Args[2:])
		if *deleteId == 0 {
			fmt.Println("id is required")
			return
		}
		fmt.Printf("Deleted task: %d\n", *deleteId)

	default:
		fmt.Println("expected 'add', 'list', 'complete' or 'delete' subcommands")
	}

}

type Task struct {
	ID          int
	Title       string
	Description string
	Completed   bool
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

func (store *inMemoryTaskStore) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tasks := store.ListTasks()
		json.NewEncoder(w).Encode(tasks)
	case http.MethodPost:
		task := &Task{}
		if err := json.NewDecoder(r.Body).Decode(task); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		newTask := store.AddTask(task.Title, task.Description)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(newTask)
	default:
		http.Error(w, "Only GET and POST methods are supported on this endpoint", http.StatusMethodNotAllowed)
	}
}

func (store *inMemoryTaskStore) handleTasksById(w http.ResponseWriter, r *http.Request) {
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
