package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

func main() {
	store := localTaskStore()
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Task Manager")
	fmt.Println("Commands:")
	fmt.Println("  add <title> <description>")
	fmt.Println("  list")
	fmt.Println("  complete <id>")
	fmt.Println("  delete <id>")
	fmt.Println("  exit")

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		parts := strings.Fields(input)

		if len(parts) == 0 {
			continue
		}

		cmd := parts[0]
		args := parts[1:]

		switch cmd {
		case "add":
			if len(args) < 2 {
				fmt.Println("Usage: add <title> <description>")
				continue
			}
			title := args[0]
			description := strings.Join(args[1:], " ")
			task := store.AddTask(title, description)
			fmt.Printf("Added task: %+v\n", task)

		case "list":
			tasks := store.ListTasks()
			if len(tasks) == 0 {
				fmt.Println("No tasks available.")
			} else {
				for _, task := range tasks {
					fmt.Printf("%+v\n", task)
				}
			}

		case "complete":
			if len(args) < 1 {
				fmt.Println("Usage: complete <id>")
				continue
			}
			id, err := strconv.Atoi(args[0])
			if err != nil {
				fmt.Println("Invalid ID.")
				continue
			}
			err = store.CompleteTask(id)
			if err != nil {
				fmt.Println("Error:", err)
			} else {
				fmt.Printf("Completed task: %d\n", id)
			}

		case "delete":
			if len(args) < 1 {
				fmt.Println("Usage: delete <id>")
				continue
			}
			id, err := strconv.Atoi(args[0])
			if err != nil {
				fmt.Println("Invalid ID.")
				continue
			}
			err = store.RemoveTask(id)
			if err != nil {
				fmt.Println("Error:", err)
			} else {
				fmt.Printf("Deleted task: %d\n", id)
			}

		case "exit":
			fmt.Println("Exiting Task Manager.")
			os.Exit(0)

		default:
			fmt.Println("Unknown command. Available commands: add, list, complete, delete, exit")
		}
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
