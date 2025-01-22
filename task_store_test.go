package main

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

func TestConcurrentAccessMemoryStore(t *testing.T) {
	store := localTaskStore()
	totalUsers := 10
	tasksPerUser := 100
	wg := &sync.WaitGroup{}

	for i := 0; i < totalUsers; i++ {
		userName := fmt.Sprintf("user%d", i)
		wg.Add(1)

		go func(userName string) {
			defer wg.Done()
			for j := 0; j < tasksPerUser; j++ {
				taskTitle := fmt.Sprintf("Task %d", j)
				taskDesc := fmt.Sprintf("Description for task %d", j)
				store.AddTask(userName, taskTitle, taskDesc)
			}

			// List tasks to ensure they were added
			tasks := store.ListTasks(userName)
			if len(tasks) != tasksPerUser {
				t.Errorf("Expected %d tasks for %s, got %d", tasksPerUser, userName, len(tasks))
			}
		}(userName)
	}

	wg.Wait()
}

func TestConcurrentAccessJSONStore(t *testing.T) {
	filePath := "test_tasks.json"
	if err := os.WriteFile(filePath, []byte("{}"), 0664); err != nil {
		t.Fatal("Failed to clean to test file", err)
	}

	store := newJSONTaskStore("test_tasks.json")
	totalUsers := 10
	tasksPerUser := 100
	wg := &sync.WaitGroup{}

	for i := 0; i < totalUsers; i++ {
		userName := fmt.Sprintf("user%d", i)
		wg.Add(1)

		go func(userName string) {
			defer wg.Done()
			for j := 0; j < tasksPerUser; j++ {
				taskTitle := fmt.Sprintf("Task %d", j)
				taskDesc := fmt.Sprintf("Description for task %d", j)
				store.AddTask(userName, taskTitle, taskDesc)
			}

			// List tasks to ensure they were added
			tasks := store.ListTasks(userName)
			if len(tasks) != tasksPerUser {
				t.Errorf("Expected %d tasks for %s, got %d", tasksPerUser, userName, len(tasks))
			}
		}(userName)
	}

	wg.Wait()
}

func TestConcurrentTaskCompletionMemoryStore(t *testing.T) {
	store := localTaskStore()

	totalUsers := 5
	tasksPerUser := 50
	wg := &sync.WaitGroup{}

	for i := 0; i < totalUsers; i++ {
		userName := fmt.Sprintf("user%d", i)
		wg.Add(1)

		go func(userName string) {
			defer wg.Done()

			for j := 0; j < tasksPerUser; j++ {
				taskTitle := fmt.Sprintf("Task %d", j)
				taskDesc := fmt.Sprintf("Description for task %d", j)
				task := store.AddTask(userName, taskTitle, taskDesc)

				if err := store.CompleteTask(userName, task.ID); err != nil {
					t.Errorf("Failed to complete task %d for user %s: %v", task.ID, userName, err)
				}
			}
		}(userName)
	}

	wg.Wait()
}

func TestConcurrentTaskCompletionJSONStore(t *testing.T) {
	filePath := "test_complete_tasks.json"
	if err := os.WriteFile(filePath, []byte("{}"), 0664); err != nil {
		t.Fatal("Failed to clean to test file", err)
	}

	store := newJSONTaskStore("test_complete_tasks.json")

	totalUsers := 5
	tasksPerUser := 50
	wg := &sync.WaitGroup{}

	for i := 0; i < totalUsers; i++ {
		userName := fmt.Sprintf("user%d", i)
		wg.Add(1)

		go func(userName string) {
			defer wg.Done()

			for j := 0; j < tasksPerUser; j++ {
				taskTitle := fmt.Sprintf("Task %d", j)
				taskDesc := fmt.Sprintf("Description for task %d", j)
				task := store.AddTask(userName, taskTitle, taskDesc)

				if err := store.CompleteTask(userName, task.ID); err != nil {
					t.Errorf("Failed to complete task %d for user %s: %v", task.ID, userName, err)
				}
			}
		}(userName)
	}

	wg.Wait()
}
