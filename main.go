package main

import (
	"flag"
	"fmt"
	"os"
)

var isLoggedIn bool
var loggedInUsername string
var taskStore TaskStore
var userStore UserStore

func main() {
	// Initialize the logger
	InitializeLogger()

	// Initialize the user store
	initializeUserStore()

	// Command-line argument to choose the task store type.
	storeType := flag.String("store", "memory", "Specify the task store: 'memory' or 'json'")
	flag.Parse()

	// Initialize the task store based on the provided type.
	switch *storeType {
	case "json":
		taskStore = newJSONTaskStore("tasks.json")
	case "memory":
		taskStore = localTaskStore()
	default:
		fmt.Println("Invalid store type. Use 'memory' or 'json'.")
		os.Exit(1)
	}

	// Start the server and CLI concurrently.
	go startServer()
	runCLI()
}
