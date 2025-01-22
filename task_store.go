package main

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"sync"
)

type Task struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Completed   bool   `json:"completed"`
}

type TaskStore interface {
	AddTask(userName, title string, description string) Task
	RemoveTask(userName string, id int) error
	ListTasks(userName string) []Task
	GetTask(userName string, id int) (Task, error)
	CompleteTask(userName string, id int) error
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
