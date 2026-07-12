// Get references to the input field, add button, and the to-do list
const newTaskInput = document.getElementById("new-task");
const addTaskButton = document.getElementById("add-task");
const todoList = document.getElementById("todo-list");

// Add a new task when the 'Add Task' button is clicked
addTaskButton.addEventListener("click", function () {
	// Get the task text from the input field
	const taskText = newTaskInput.value;

	// Ensure the task isn't empty
	if (taskText.trim() === "") {
		alert("Please enter a task!");
		return;
	}

	// Create a new list item (li) to represent the task
	const li = document.createElement("li");
	li.textContent = taskText;

	// Create a 'Remove' button and append it to the task (li)
	const removeButton = document.createElement("button");
	removeButton.textContent = "Remove";
	removeButton.style.background = "#ff6666";

	// Attach the 'Remove' button's click event to delete the task
	removeButton.addEventListener("click", function () {
		todoList.removeChild(li); // Remove the task when clicked
	});

	li.appendChild(removeButton); // Add the button to the task

	// Add the newly created task (li) to the to-do list (ul)
	todoList.appendChild(li);

	// Clear the input field for a new task
	newTaskInput.value = "";
});

// Allow the user to press 'Enter' to add a task
newTaskInput.addEventListener("keyup", function (event) {
	// If the 'Enter' key is pressed, trigger the 'Add Task' button
	if (event.key === "Enter") {
		addTaskButton.click();
	}
});

// Example of a function to add a sample task automatically
function addSampleTask(taskText) {
	const li = document.createElement("li");
	li.textContent = taskText;

	const removeButton = document.createElement("button");
	removeButton.textContent = "Remove";
	removeButton.style.background = "#ff6666";

	removeButton.addEventListener("click", function () {
		todoList.removeChild(li);
	});

	li.appendChild(removeButton);
	todoList.appendChild(li);
}

// Add a sample task for demonstration when the page loads
window.onload = function () {
	addSampleTask("Sample Task: Learn JavaScript!");
};
