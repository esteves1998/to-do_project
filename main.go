package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sync"
)

func main() {
	store := localTaskStore()

	if len(os.Args) < 2 {
		fmt.Println("Usage: [add|list|complete|delete] [options]")
		os.Exit(1)
	}

	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	title := addCmd.String("title", "", "Title of the task")
	desc := addCmd.String("description", "", "Description of the task")

	completeCmd := flag.NewFlagSet("complete", flag.ExitOnError)
	completeId := completeCmd.Int("id", 0, "ID of the task to complete")

	deleteCmd := flag.NewFlagSet("delete", flag.ExitOnError)
	deleteId := deleteCmd.Int("id", 0, "ID of the task to delete")

	if len(os.Args) < 1 {
		fmt.Println("expected 'add', 'list', 'complete' or 'delete' subcommands")
	}

	switch os.Args[1] {
	case "add":
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
		completeCmd.Parse(os.Args[2:])
		if *completeId == 0 {
			fmt.Println("id is required")
			return
		}
		fmt.Printf("Completed task: %+v\n", store.CompleteTask(*completeId))

	case "delete":
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
