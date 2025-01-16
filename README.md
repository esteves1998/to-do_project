# Task Manager

Task Manager is a simple application written in Go that allows you to manage tasks through both a command-line interface (CLI) and a RESTful API.

## Features

- Add tasks with a title and description.
- List all tasks.
- Mark tasks as completed.
- Delete tasks.
- Interactive CLI for managing tasks.
- RESTful API for external integrations.

## Requirements

- [Go](https://golang.org/) installed on your machine.

## Running the Application

1. Build and run the program:
   ```bash
   go run main.go
   ```

2. The REST API server will start at `http://localhost:8080`, and the CLI will be ready for interactive commands.

## REST API Endpoints

The application exposes the following RESTful endpoints:

### List All Tasks
- **GET** `/tasks`
- **Response:**
  ```json
  [
    {
      "id": 1,
      "title": "Buy groceries",
      "description": "Milk, eggs, bread, and butter",
      "completed": false
    },
    {
      "id": 2,
      "title": "Prepare presentation",
      "description": "Slides for the team meeting",
      "completed": false
    }
  ]
  ```

### Add a Task
- **POST** `/tasks`
- **Request Body:**
  ```json
  {
    "title": "New Task",
    "description": "Task description"
  }
  ```
- **Response:**
  ```json
  {
    "id": 3,
    "title": "New Task",
    "description": "Task description",
    "completed": false
  }
  ```

### Mark a Task as Completed
- **PUT** `/tasks/:id`
- **Request Body:**
  ```json
  {
    "id": 1
  }
  ```
- **Response:** Status `200 OK`

### Delete a Task
- **DELETE** `/tasks/:id`
- **Request Body:**
  ```json
  {
    "id": 1
  }
  ```
- **Response:** Status `200 OK`

## CLI Commands

The CLI allows you to interact with the application interactively. Below are the supported commands:

### Add a Task
```
add "Buy groceries" "Milk, eggs, bread, and butter"
```
**Output:**
```
Task added successfully.
```

### List All Tasks
```
list
```
**Output:**
```
ID: 1, Title: Buy groceries, Description: Milk, eggs, bread, and butter, Completed: false
ID: 2, Title: Prepare presentation, Description: Slides for the team meeting, Completed: false
```

### List a Task
```
get <id>
```
**Output:**
```
ID: <id>, Title: Buy groceries, Description: Milk, eggs, bread, and butter, Completed: false
```

### Complete a Task
```
complete <id>
```
**Output:**
```
Task 1 marked as completed.
```

### Delete a Task
```
delete <id>
```
**Output:**
```
Task 1 deleted successfully.
```

### Display Help
```
help
```
**Output:**
```
Commands:
  add <title> <description>    Add a new task
  list                         List all tasks
  complete <id>                Mark a task as completed
  delete <id>                  Delete a task
  help                         Show this help message
  exit                         Exit the program
```

### Exit the Program
```
exit
```
**Output:**
```
Exiting Task Manager.
```

## Notes

- Ensure the server is running at `http://localhost:8080` for the CLI to interact with the REST API.
