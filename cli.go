package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func runCLI() {
	scanner := bufio.NewScanner(os.Stdin)
	logger.Info("Task Manager started (connected to REST API)")

	for {
		loginOrRegister(scanner)

		if isLoggedIn {
			break
		}
	}

	printHelp()

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		parts := strings.Fields(input)

		// If user gives a blank command do nothing
		if len(parts) == 0 {
			continue
		}

		cmd := parts[0]
		args := parts[1:]

		switch cmd {
		case "listUsers":
			handleListUsers()
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

func handleAdd(args []string) {
	if len(args) < 2 {
		logger.Info("Usage: add \"<title>\" \"<description>\"")
		return
	}

	// Use the stored logged-in username
	userName := loggedInUsername

	if userName == "" {
		logger.Info("You must be logged in to add a task.")
		return
	}

	quoteRegex := regexp.MustCompile(`"(.*?)"`)
	matches := quoteRegex.FindAllStringSubmatch(strings.Join(args, " "), -1)

	if len(matches) < 2 {
		logger.Info("Usage: add \"<title>\" \"<description>\"", "args", args)
		return
	}

	title := matches[0][1]
	description := matches[1][1]

	task := Task{
		Title:       title,
		Description: description,
	}
	resp, err := http.Post(fmt.Sprintf("http://localhost:8080/tasks?username=%s", userName), "application/json", toJSON(task))
	if err != nil {
		logger.Error("Failed to add task", "error", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		logger.Info("Task added successfully", "title", title)
	} else {
		logger.Error("Failed to add task", "error", err)
	}
}

func handleList() {
	// Use the stored logged-in username
	userName := loggedInUsername

	if userName == "" {
		logger.Info("You must be logged in to list tasks.")
		return
	}

	resp, err := http.Get(fmt.Sprintf("http://localhost:8080/tasks?username=%s", userName))
	if err != nil {
		logger.Error("Failed to list tasks", "error", err)
		return
	}
	defer safeClose(resp.Body)

	var tasks []Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		logger.Error("Failed to decode tasks response", "error", err)
		return
	}

	if len(tasks) == 0 {
		logger.Info("No tasks found for user", "userName", userName)
		return
	}

	for _, task := range tasks {
		logger.Info("Task found", "taskID", task.ID, "title", task.Title, "description", task.Description, "completed", task.Completed)
	}
}

func handleGetTaskByID(args []string) {
	if len(args) != 2 {
		logger.Info("Usage: get <username> <id>")
		return
	}

	userName := args[0]
	id := args[1]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s?username=%s", id, userName)

	resp, err := http.Get(url)
	if err != nil {
		logger.Error("Failed to get task", "id", id, "error", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusOK {
		var task Task
		if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
			logger.Error("Error decoding response:", "error", err)
			return
		}
		fmt.Printf("ID: %d, Title: %s, Description: %s, Completed: %v\n",
			task.ID, task.Title, task.Description, task.Completed)
	} else if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("Task with ID %s not found for user %s.\n", id, userName)
	} else {
		fmt.Printf("Unexpected error: %s\n", resp.Status)
	}
}

func handleComplete(args []string) {
	if len(args) < 1 {
		logger.Info("Usage: complete <id>")
		return
	}

	// Use the stored logged-in username
	userName := loggedInUsername

	if userName == "" {
		logger.Info("You must be logged in to complete a task.")
		return
	}

	id := args[0]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s?username=%s", id, userName)

	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		logger.Error("Error creating request:", "error", err)
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to complete task", "id", id, "error", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusOK {
		logger.Info("Task completed successfully", "id", id, "userName", userName)
	} else {
		logger.Error("Failed to complete task", "id", id, "error", resp.Status)
	}
}

func handleDelete(args []string) {
	if len(args) < 1 {
		logger.Info("Usage: delete <id>")
		return
	}

	// Use the stored logged-in username
	userName := loggedInUsername

	if userName == "" {
		logger.Info("You must be logged in to delete a task.")
		return
	}

	id := args[0]
	url := fmt.Sprintf("http://localhost:8080/tasks/%s?username=%s", id, userName) // Use the stored username

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		logger.Error("Error creating request:", "error", err)
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to delete task", "id", id, "error", err)
		return
	}
	defer safeClose(resp.Body)

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("Task %s deleted successfully for user %s.\n", id, userName)
	} else {
		fmt.Printf("Failed to delete task %s: %s\n", id, resp.Status)
	}
}

func printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  add \"<title>\" \"<description>\"    Add a new task for the logged-in user")
	fmt.Println("  list                                 List all tasks for the logged-in user")
	fmt.Println("  complete <id>                       Mark a task as completed for the logged-in user")
	fmt.Println("  delete <id>                         Delete a task for the logged-in user")
	fmt.Println("  help                                 Show this help message")
	fmt.Println("  exit                                 Exit the program")
	fmt.Println("  listUsers                            List all users")
}

func toJSON(task Task) *strings.Reader {
	data, _ := json.Marshal(task)
	return strings.NewReader(string(data))
}
