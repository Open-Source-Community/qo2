# To-Do List Application in Python

# Define an empty list to store tasks
todo_list = []

# Function to display the menu options to the user
def display_menu():
    print("\nTo-Do List Menu:")
    print("1. View To-Do List")
    print("2. Add Task")
    print("3. Remove Task")
    print("4. Clear All Tasks")
    print("5. Exit")

# Function to display all tasks in the to-do list
def view_tasks():
    if len(todo_list) == 0:
        print("\nYour to-do list is empty!")
    else:
        print("\nYour To-Do List:")
        for i, task in enumerate(todo_list, start=1):
            print(f"{i}. {task}")

# Function to add a new task to the to-do list
def add_task():
    task = input("\nEnter the task you want to add: ").strip()
    
    # Ensure the task isn't empty
    if task == "":
        print("Task cannot be empty!")
    else:
        todo_list.append(task)
        print(f"Task '{task}' added to the list.")

# Function to remove a task from the to-do list
def remove_task():
    if len(todo_list) == 0:
        print("\nYour to-do list is empty!")
        return
    
    view_tasks()
    try:
        task_num = int(input("\nEnter the number of the task to remove: "))
        if 1 <= task_num <= len(todo_list):
            removed_task = todo_list.pop(task_num - 1)
            print(f"Task '{removed_task}' removed from the list.")
        else:
            print("Invalid task number!")
    except ValueError:
        print("Please enter a valid number.")

# Function to clear all tasks in the to-do list
def clear_all_tasks():
    confirm = input("\nAre you sure you want to clear all tasks? (y/n): ")
    if confirm.lower() == 'y':
        todo_list.clear()
        print("All tasks have been cleared.")
    else:
        print("Clear action canceled.")

# Main function to run the to-do list application
def run_todo_app():
    while True:
        # Display the menu to the user
        display_menu()
        
        # Get user's choice
        choice = input("\nEnter your choice (1-5): ").strip()

        # Execute the appropriate function based on the user's choice
        if choice == '1':
            view_tasks()  # View to-do list
        elif choice == '2':
            add_task()    # Add new task
        elif choice == '3':
            remove_task() # Remove a task
        elif choice == '4':
            clear_all_tasks() # Clear all tasks
        elif choice == '5':
            print("Exiting the To-Do List Application. Goodbye!")
            break
        else:
            print("Invalid choice! Please enter a number between 1 and 5.")

# Run the to-do list application
if __name__ == "__main__":
    run_todo_app()
