package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"text/template"

	"github.com/google/uuid"
)

// Errors
var (
	ErrJsonInvalid         = errors.New("Invalid JSON Required Todo")
	ErrTodoEmpty           = errors.New("Invalid JSON, No Todo Found")
	ErrTodoNotFound        = errors.New("Todo Not Found")
	ErrInternalServerError = errors.New("Something went wrong with the operation")
)

type Todo struct {
	Id   string `json:"id"`
	Todo string `json:"todo"`
}

type UserTodoRequest struct {
	Todo string `json:"todo"`
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Println("[-] Health Check")
	w.WriteHeader(http.StatusNoContent)
}

func createTodo(w http.ResponseWriter, r *http.Request) {
	// Since the UUID generation could panic I need to recover from that
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Panic Averted on Create Todo, %v", r)
		}
	}()
	fmt.Println("[-] Create Todo")

	var userTodoRequest UserTodoRequest
	if err := json.NewDecoder(r.Body).Decode(&userTodoRequest); err != nil {
		http.Error(w, ErrJsonInvalid.Error(), http.StatusBadRequest)
		return
	}

	if userTodoRequest.Todo == "" {
		http.Error(w, ErrTodoEmpty.Error(), http.StatusBadRequest)
		return
	}

	id := uuid.New().String()

	todosDb.mu.Lock()
	defer todosDb.mu.Unlock()

	createdTodo := Todo{
		Id:   id,
		Todo: userTodoRequest.Todo,
	}
	todosDb.todos[id] = createdTodo
	w.WriteHeader(http.StatusAccepted)
}

func getTodos(w http.ResponseWriter, r *http.Request) {
	defer todosDb.mu.RUnlock()
	fmt.Println("[-] Get All Todos")

	todosDb.mu.RLock()

	if len(todosDb.todos) == 0 {
		http.Error(
			w,
			ErrTodoEmpty.Error(),
			http.StatusNotFound,
		)
		return
	}
	todos := make([]Todo, 0, len(todosDb.todos))

	for _, todo := range todosDb.todos {
		todos = append(todos, todo)
	}

	jsonTodos, err := json.Marshal(todos)

	if err != nil {
		http.Error(
			w,
			ErrInternalServerError.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonTodos)
}

func getTodoById(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	fmt.Println("[-] Get todo by id %v", id)

	todosDb.mu.RLock()
	todo, ok := todosDb.todos[id]
	todosDb.mu.RUnlock()

	if !ok {
		http.Error(
			w,
			ErrTodoNotFound.Error(),
			http.StatusNotFound,
		)
		return
	}

	todoJson, err := json.Marshal(todo)
	if err != nil {
		http.Error(
			w,
			ErrInternalServerError.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(todoJson)
}

func patchTodoById(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	fmt.Println("[-] Patch todo by id %v", id)

	todosDb.mu.Lock()

	defer todosDb.mu.Unlock()

	if _, ok := todosDb.todos[id]; !ok {
		http.Error(
			w,
			ErrTodoNotFound.Error(),
			http.StatusBadRequest,
		)
		return
	}

	var userTodoRequest UserTodoRequest

	if err := json.NewDecoder(r.Body).Decode(&userTodoRequest); err != nil {
		http.Error(
			w,
			ErrJsonInvalid.Error(),
			http.StatusBadRequest,
		)
		return
	}
	updatedTodo := Todo{
		Id:   id,
		Todo: userTodoRequest.Todo,
	}

	todosDb.todos[id] = updatedTodo
	jsonUpdatedTodo, err := json.Marshal(updatedTodo)

	if err != nil {
		http.Error(
			w,
			ErrInternalServerError.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write(jsonUpdatedTodo)
}

func deleteTodoById(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	fmt.Println("[-] Delete todo by id %v", id)

	todosDb.mu.Lock()
	defer todosDb.mu.Unlock()

	if _, ok := todosDb.todos[id]; !ok {
		http.Error(
			w,
			ErrTodoNotFound.Error(),
			http.StatusBadRequest,
		)
		return
	}

	delete(todosDb.todos, id)

	w.WriteHeader(http.StatusNoContent)

}

// Compile templates on start of the application
var templates = template.Must(template.ParseFiles("public/upload.html"))

const fileUploadFolder = "files"

func createFileUploadDir(fileUploadFolder string) {
	err := os.MkdirAll(fileUploadFolder, 0755)
	if err != nil {
		fmt.Println("[-] Error creating directory:", err)
	}
}

func handleGetFile(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "upload.html", nil)
}

func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	const TENMB = 10 << 20

	// Max upload size limit 10 mb files
	r.ParseMultipartForm(TENMB)

	file, header, err := r.FormFile("myFile")
	if err != nil {
		http.Error(
			w,
			ErrInternalServerError.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	defer file.Close()
	fmt.Println("[-] Uploaded File Name", header.Filename)
	fmt.Println("[-] Uploaded File Size", header.Size)
	fmt.Println("[-] Uploaded File Mime Header", header.Header)

	// create the file
	dst, err := os.Create(fmt.Sprintf("%v/%v", fileUploadFolder, header.Filename))
	defer dst.Close()

	if err != nil {
		fmt.Println("[-] Error in File Creation %v", err.Error())
		http.Error(
			w,
			ErrInternalServerError.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	if _, err := io.Copy(dst, file); err != nil {
		fmt.Println("[-] Error in Copying File form src to dst %v", err.Error())
		http.Error(
			w,
			ErrInternalServerError.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	fmt.Println("File uploaded successfully")
	w.WriteHeader(http.StatusNoContent)
}

type TodosDB struct {
	mu    sync.RWMutex
	todos map[string]Todo
}

var todosDb TodosDB

func main() {
	todosDb = TodosDB{
		todos: make(map[string]Todo),
	}
	mux := http.NewServeMux()

	createFileUploadDir(fileUploadFolder)

	// Add handlers
	mux.HandleFunc("GET /health-check", healthCheck)
	mux.HandleFunc("GET /todos", getTodos)
	mux.HandleFunc("POST /todos", createTodo)
	mux.HandleFunc("GET /todos/{id}", getTodoById)
	mux.HandleFunc("PATCH /todos/{id}", patchTodoById)
	mux.HandleFunc("DELETE /todos/{id}", deleteTodoById)

	// File Uplaod
	mux.HandleFunc("GET /file", handleGetFile)
	mux.HandleFunc("POST /file", handleFileUpload)

	// Start the server
	fmt.Println("Started server at :8080 ...")
	http.ListenAndServe(":8080", mux)
}
