package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
)

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type UserStore struct {
	users map[string]User
	mutex sync.Mutex
}

func initializeUserStore() {
	if _, err := os.Stat("users.json"); os.IsNotExist(err) {
		if err := createEmptyJSONFile("users.json"); err != nil {
			logger.Error("Failed to create empty users.json file", "error", err)
			os.Exit(1)
		}
	} else {
		if err := loadUsersFromFile(); err != nil {
			logger.Error("Failed to load users from file", "error", err)
			os.Exit(1)
		}
	}
}

func (store *UserStore) AddUser(username, password string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if _, exists := store.users[username]; exists {
		return errors.New("user already exists")
	}

	store.users[username] = User{Username: username, Password: password}

	if err := store.saveUsersToFile(); err != nil {
		return err
	}
	return nil
}

func (store *UserStore) ListUsers() []User {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	users := make([]User, 0, len(store.users))
	for _, user := range store.users {
		users = append(users, user)
	}
	return users
}

func loadUsersFromFile() error {
	file, err := os.Open("users.json")
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			logger.Error("Error closing file:", "error", err)
		}
	}(file)

	users := make(map[string]User)
	if err := json.NewDecoder(file).Decode(&users); err != nil {
		return err
	}

	userStore.users = users

	return nil
}

func (store *UserStore) saveUsersToFile() error {
	file, err := os.Create("users.json")
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
	if err := encoder.Encode(store.users); err != nil {
		logger.Error("Failed to encode users to file", "error", err)
		return err
	}
	logger.Info("Users saved to file", "users", store.users) // Log the saved users
	return nil
}

func handleListUsers() {
	resp, err := http.Get("http://localhost:8080/users/list")
	if err != nil {
		logger.Error("Failed to list users", "error", err)
		return
	}
	defer safeClose(resp.Body)

	var users []User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		logger.Error("Failed to decode users response", "error", err)
		return
	}

	if len(users) == 0 {
		logger.Info("No users found")
		return
	}

	for _, user := range users {
		logger.Info("User found", "username", user.Username)
	}
}

// Registration and Login

func loginOrRegister(scanner *bufio.Scanner) {
	fmt.Println("Welcome to the Task Manager!")
	fmt.Println("Would you like to (1) Login or (2) Register?")

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		if input == "1" {
			handleLogin(scanner)
			break
		} else if input == "2" {
			handleRegister(scanner)
			break
		} else {
			fmt.Println("Invalid option. Please enter 1 to Login or 2 to Register.")
		}
	}
}

func handleRegister(scanner *bufio.Scanner) {
	fmt.Print("Enter username: ")
	scanner.Scan()
	username := scanner.Text()

	fmt.Print("Enter password: ")
	scanner.Scan()
	password := scanner.Text()

	if err := userStore.AddUser(username, password); err != nil {
		fmt.Println("Registration failed:", err)
		isLoggedIn = false // Set login status to false
	} else {
		fmt.Println("Registration successful! You can now log in.")
		handleLogin(scanner)
	}
}

func handleLogin(scanner *bufio.Scanner) {
	fmt.Print("Enter username: ")
	scanner.Scan()
	username := scanner.Text()

	fmt.Print("Enter password: ")
	scanner.Scan()
	password := scanner.Text()

	if err := userStore.CheckPassword(username, password); err != nil {
		fmt.Println("Login failed:", err)
		isLoggedIn = false
	} else {
		fmt.Println("Login successful!")
		isLoggedIn = true
		loggedInUsername = username
	}
}

func usernameExists(userName string) bool {
	_, exists := userStore.users[userName]
	if !exists {
		logger.Error("Username does not exist", "userName", userName)
		fmt.Printf("Error: Username '%s' does not exist.\n", userName)
	}
	return exists
}

func (store *UserStore) CheckPassword(username, password string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	user, exists := store.users[username]
	if !exists {
		return errors.New("user not found")
	}

	if user.Password != password {
		return errors.New("invalid password")
	}

	return nil
}
