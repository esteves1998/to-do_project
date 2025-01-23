package main

var isLoggedIn bool
var loggedInUsername string
var taskStore TaskStore
var userStore UserStore

func main() {
	InitializeLogger()

	initializeUserStore()

	storeType := parseStoreType()

	initializeTaskStore(storeType)

	go startServer()
	runCLI()
}
