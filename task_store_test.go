package main

import (
	"strconv"
	"sync"
	"testing"
)

func TestAddTaskConcurrent(t *testing.T) {
	store := localTaskStore()

	// Number of goroutines to run in parallel
	const numGoroutines = 100

	// Wait group to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// A slice to store task IDs for validation
	taskIDs := make([]int, 0, numGoroutines)
	var mu sync.Mutex // Correctly initialize the mutex

	// Run multiple goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(title string) {
			defer wg.Done()
			task := store.AddTask(title, "description")
			mu.Lock()
			taskIDs = append(taskIDs, task.ID)
			mu.Unlock()
		}(strconv.Itoa(i))
	}
	wg.Wait()

	// Validate that all task IDs are unique and sequential
	idMap := make(map[int]bool)
	for _, id := range taskIDs {
		if idMap[id] {
			t.Fatalf("Duplicate task ID detected: %d", id)
		}
		idMap[id] = true
	}

	// Validate the total number of tasks added
	if len(taskIDs) != numGoroutines {
		t.Fatalf("Expected %d tasks, but got %d", numGoroutines, len(taskIDs))
	}
}

func TestCompleteTaskConcurrent(t *testing.T) {
	store := localTaskStore()

	// Add a number of tasks to complete later
	const numTasks = 100
	for i := 0; i < numTasks; i++ {
		store.AddTask(strconv.Itoa(i), "description")
	}

	// Number of goroutines to run in parallel
	const numGoroutines = 100

	// Wait group to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// A slice to store task IDs for validation
	taskIDs := make([]int, 0, numGoroutines)
	var mu sync.Mutex // Mutex to safely access the taskIDs slice

	// Run multiple goroutines to complete tasks
	for i := 0; i < numGoroutines; i++ {
		go func(taskID int) {
			defer wg.Done()
			err := store.CompleteTask(taskID)
			if err != nil {
				t.Errorf("Failed to complete task %d: %v", taskID, err)
			}
			mu.Lock()
			taskIDs = append(taskIDs, taskID)
			mu.Unlock()
		}(i + 1) // Pass task IDs starting from 1
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Validate that all task IDs are completed and unique
	idMap := make(map[int]bool)
	for _, id := range taskIDs {
		if idMap[id] {
			t.Fatalf("Duplicate task ID detected: %d", id)
		}
		idMap[id] = true
	}

	// Validate that all tasks have been marked as completed
	for i := 1; i <= numTasks; i++ {
		task, err := store.GetTask(i)
		if err != nil {
			t.Errorf("Failed to get task %d: %v", i, err)
			continue
		}
		if !task.Completed {
			t.Errorf("Task %d was not completed", i)
		}
	}
}
func TestListTasksConcurrent(t *testing.T) {
	store := localTaskStore()

	// Add some tasks to the store
	numTasks := 10
	for i := 0; i < numTasks; i++ {
		store.AddTask(strconv.Itoa(i), "description")
	}

	// Number of goroutines to run in parallel
	const numGoroutines = 100

	// Wait group to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// A slice to store task lists for validation
	var taskLists [][]Task
	var mu sync.Mutex

	// Run multiple goroutines to list tasks concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			tasks := store.ListTasks()
			mu.Lock()
			taskLists = append(taskLists, tasks)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(taskLists) == 0 {
		t.Fatalf("No task lists were retrieved")
	}

	for _, tasks := range taskLists {
		if len(tasks) != numTasks {
			t.Fatalf("Expected %d tasks, but got %d", numTasks, len(tasks))
		}

		idMap := make(map[int]bool)
		for _, task := range tasks {
			if idMap[task.ID] {
				t.Fatalf("Duplicate task ID detected: %d", task.ID)
			}
			idMap[task.ID] = true
		}
	}

}
