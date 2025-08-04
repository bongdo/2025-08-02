package handlers

import (
	"2025-08-02/config"
	"2025-08-02/task"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/mux"
)

type TaskManager struct {
	Tasks              map[string]*task.Task
	mutex              sync.Mutex
	config             *config.Config
	concurrentTaskSema chan struct{}
}

func NewTaskManager(cfg *config.Config) *TaskManager {
	return &TaskManager{
		Tasks:              make(map[string]*task.Task),
		config:             cfg,
		concurrentTaskSema: make(chan struct{}, cfg.MaxConcurrentTasks),
	}
}

// CreateTaskHandler creates a new task
// @Summary      Create a new task
// @Description  creates a new task for archiving files
// @Tags         tasks
// @Accept       json
// @Produce      json
// @Success      201 {object} task.Task
// @Failure      503 {string} string "server is busy, please try again later"
// @Router       /tasks [post]
func (tm *TaskManager) CreateTaskHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("CreateTaskHandler called")
	if len(tm.concurrentTaskSema) >= tm.config.MaxConcurrentTasks {
		log.Println("Server is busy")
		http.Error(w, "server is busy, please try again later", http.StatusServiceUnavailable)
		return
	}

	t := task.NewTask()
	log.Printf("Created new task with ID: %s", t.ID)
	tm.mutex.Lock()
	tm.Tasks[t.ID] = t
	tm.mutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(t)
}

// AddFileHandler adds a file to a task
// @Summary      Add a file to a task
// @Description  adds a file URL to a task for archiving
// @Tags         tasks
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Task ID"
// @Param        url  body      string  true  "File URL"
// @Success      202
// @Failure      400 {string} string "invalid request body"
// @Failure      404 {string} string "task not found"
// @Router       /tasks/{id}/files [post]
func (tm *TaskManager) AddFileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["id"]
	log.Printf("AddFileHandler called for task ID: %s", taskID)

	tm.mutex.Lock()
	t, ok := tm.Tasks[taskID]
	tm.mutex.Unlock()

	if !ok {
		log.Printf("Task with ID: %s not found", taskID)
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	var body struct {
		URL string `json:"url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("Invalid request body for task ID: %s", taskID)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("Adding file %s to task ID: %s", body.URL, taskID)
	t.AddFile(body.URL)

	if len(t.FileURLs) >= tm.config.MaxFilesPerTask {
		log.Printf("Task %s reached max files, starting processing", taskID)
		t.SetResultURL()
		tm.concurrentTaskSema <- struct{}{}
		go func() {
			defer func() { <-tm.concurrentTaskSema }()
			t.Process(tm.config.AllowedExtensions)
		}()
	}

	w.WriteHeader(http.StatusAccepted)
}

// GetTaskStatusHandler returns the status of a task
// @Summary      Get task status
// @Description  get the status of a task by ID
// @Tags         tasks
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Task ID"
// @Success      200 {object} task.Task
// @Failure      404 {string} string "task not found"
// @Router       /tasks/{id} [get]
func (tm *TaskManager) GetTaskStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["id"]
	log.Printf("GetTaskStatusHandler called for task ID: %s", taskID)

	tm.mutex.Lock()
	t, ok := tm.Tasks[taskID]
	tm.mutex.Unlock()

	if !ok {
		log.Printf("Task with ID: %s not found", taskID)
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}

// ServeArchiveHandler serves the archived zip file
// @Summary      Download an archived file
// @Description  downloads the zip file for a given task ID
// @Tags         archives
// @Produce      application/zip
// @Param        filename   path      string  true  "Archive filename (e.g., taskID.zip)"
// @Success      200 {file}  file "Archive file"
// @Failure      404 {string} string "archive not found"
// @Router       /archives/{filename} [get]
func (tm *TaskManager) ServeArchiveHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	filename := vars["filename"]
	log.Printf("ServeArchiveHandler called for filename: %s", filename)

	// Basic security check to prevent directory traversal
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	filePath := filename // Assuming archives are in the current directory

	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		log.Printf("Archive file %s not found", filePath)
		http.Error(w, "archive not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	http.ServeFile(w, r, filePath)
}
