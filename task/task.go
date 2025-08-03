package task

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

type Status string

const (
	StatusCreated   Status = "created"
	StatusProcessing Status = "processing"
	StatusDone      Status = "done"
	StatusError     Status = "error"
)

type Task struct {
	ID          string   `json:"id"`
	Status      Status   `json:"status"`
	FileURLs    []string `json:"file_urls"`
	ResultURL   string   `json:"result_url,omitempty"`
	ErrorDetails string   `json:"error_details,omitempty"`
	mutex       sync.Mutex
}

func NewTask() *Task {
	return &Task{
		ID:       uuid.New().String(),
		Status:   StatusCreated,
		FileURLs: []string{},
	}
}

func (t *Task) AddFile(url string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.FileURLs = append(t.FileURLs, url)
}

func (t *Task) Process(allowedExtensions []string) {
	t.mutex.Lock()
	t.Status = StatusProcessing
	t.mutex.Unlock()
	log.Printf("Processing task %s", t.ID)

	zipFileName := fmt.Sprintf("%s.zip", t.ID)
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		log.Printf("Failed to create zip file for task %s: %v", t.ID, err)
		t.setError(fmt.Sprintf("failed to create zip file: %v", err))
		return
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	var errors []string

	for _, fileURL := range t.FileURLs {
		log.Printf("Processing file %s for task %s", fileURL, t.ID)
		if !isAllowedExtension(fileURL, allowedExtensions) {
			log.Printf("File extension not allowed for %s", fileURL)
			errors = append(errors, fmt.Sprintf("file extension not allowed: %s", fileURL))
			continue
		}

		resp, err := http.Get(fileURL)
		if err != nil {
			log.Printf("Failed to download file %s: %v", fileURL, err)
			errors = append(errors, fmt.Sprintf("failed to download file: %s, error: %v", fileURL, err))
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("Failed to download file %s, status: %s", fileURL, resp.Status)
			errors = append(errors, fmt.Sprintf("failed to download file: %s, status: %s", fileURL, resp.Status))
			continue
		}

		fileName := filepath.Base(fileURL)
		zipEntry, err := zipWriter.Create(fileName)
		if err != nil {
			log.Printf("Failed to create zip entry for %s: %v", fileName, err)
			errors = append(errors, fmt.Sprintf("failed to create zip entry for %s: %v", fileName, err))
			continue
		}

		_, err = io.Copy(zipEntry, resp.Body)
		if err != nil {
			log.Printf("Failed to write to zip entry for %s: %v", fileName, err)
			errors = append(errors, fmt.Sprintf("failed to write to zip entry for %s: %v", fileName, err))
		}
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	if len(errors) > 0 {
		t.ErrorDetails = strings.Join(errors, "; ")
	}

	t.Status = StatusDone
	t.ResultURL = fmt.Sprintf("/archives/%s", zipFileName)
	log.Printf("Finished processing task %s", t.ID)
}

func (t *Task) setError(errStr string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.Status = StatusError
	t.ErrorDetails = errStr
}

func isAllowedExtension(fileURL string, allowedExtensions []string) bool {
	u, err := url.Parse(fileURL)
	if err != nil {
		return false
	}

	ext := strings.ToLower(filepath.Ext(u.Path))

	// Если в пути нет расширения, попробуем найти его в параметрах
	if ext == "" {
		q := u.Query()
		for _, vals := range q {
			for _, val := range vals {
				if e := strings.ToLower(filepath.Ext(val)); e != "" {
					ext = e
					break
				}
			}
			if ext != "" {
				break
			}
		}
	}

	for _, allowedExt := range allowedExtensions {
		if ext == allowedExt {
			return true
		}
	}
	return false
}
