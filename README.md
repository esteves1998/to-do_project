# Task Manager

Task Manager is a simple command-line application written in Go to manage tasks. You can add, list, complete, and delete tasks interactively.

## Features

- Add tasks with a title and description.
- List all tasks.
- Mark tasks as completed.
- Delete tasks.
- Interactive mode for continuous usage.

## Example Commands

### Add a Task
```
  add <title> <description>
```
**Example:**
```
  add "Buy groceries" "Milk, eggs, bread, and butter"
```
**Output:**
```
Added task: {ID:1 Title:Buy groceries Description:Milk, eggs, bread, and butter Completed:false}
```

### List All Tasks
```
  list
```

**Output:**
```
{ID:1 Title:Buy groceries Description:Milk, eggs, bread, and butter Completed:false}
{ID:2 Title:Prepare presentation Description:Slides for the team meeting Completed:false}
```

### Complete a Task
```
  complete <id>
```

**Output:**
```
Completed task: <id>
```

### Delete a Task
```
  delete <id>
```
**Output:**
```
Deleted task: <id>
```

### Exit the Program
```
  exit
```

**Output:**
```
Exiting Task Manager.
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

