package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DirectoryValidator handles directory validation logic
type DirectoryValidator struct{}

// Validate checks if the directory exists and prompts the user for a valid path if it doesn't.
func (dv *DirectoryValidator) Validate(dirPath string) (string, error) {
	const maxRetries = 3
	reader := bufio.NewReader(os.Stdin)

	for i := 0; i < maxRetries; i++ {
		if _, err := os.Stat(dirPath); err == nil {
			return dirPath, nil
		}

		fmt.Println("Invalid directory. Please enter a valid directory path:")
		newPath, _ := reader.ReadString('\n')
		dirPath = strings.TrimSpace(newPath)
	}

	return "", errors.New("maximum retries reached for directory validation")
}

// FileDeleter handles file deletion logic
type FileDeleter struct {
	Extension string
}

// DeleteFilesWithTimeout deletes files with a timeout and retries on failure.
func (fd *FileDeleter) DeleteFilesWithTimeout(dirPath string, files []os.DirEntry, workerCount, maxRetries int, timeout time.Duration) error {
	type FileTask struct {
		FileName string
		Retries  int
	}

	fileChan := make(chan FileTask, len(files))
	errorChan := make(chan error, len(files))
	var wg sync.WaitGroup

	// Worker function
	worker := func() {
		defer wg.Done()
		for task := range fileChan {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			filePath := filepath.Join(dirPath, task.FileName)
			errChan := make(chan error, 1)

			// Attempt to delete the file
			go func() {
				errChan <- os.Remove(filePath)
			}()

			select {
			case <-ctx.Done():
				// Timeout occurred
				if task.Retries < maxRetries {
					task.Retries++
					fileChan <- task
				} else {
					errorChan <- fmt.Errorf("timeout deleting file after %d retries: %s", maxRetries, filePath)
				}
			case err := <-errChan:
				// File deletion completed
				if err != nil {
					if task.Retries < maxRetries {
						task.Retries++
						fileChan <- task
					} else {
						errorChan <- fmt.Errorf("failed to delete file after %d retries: %s, %v", maxRetries, filePath, err)
					}
				} else {
					fmt.Printf("Deleted file: %s\n", filePath)
				}
			}
		}
	}

	// Start worker goroutines
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker()
	}

	// Send initial file tasks to the channel
	go func() {
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), fd.Extension) {
				fileChan <- FileTask{FileName: file.Name(), Retries: 0}
			}
		}
		close(fileChan)
	}()

	// Wait for all workers to finish
	wg.Wait()
	close(errorChan)

	// Collect errors
	var errors []string
	for err := range errorChan {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors occurred during file deletion: %s", strings.Join(errors, "; "))
	}
	return nil
}

// Application orchestrates the logic
type Application struct {
	Validator *DirectoryValidator
	Deleter   *FileDeleter
}

// Run executes the application logic
func (app *Application) Run(args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: <program> <directory_path>")
		return
	}

	dirPath := args[0]
	validDir, err := app.Validator.Validate(dirPath)
	if err != nil {
		fmt.Println("Error validating directory:", err)
		return
	}

	files, err := os.ReadDir(validDir)
	if err != nil {
		fmt.Println("Error reading directory:", err)
		return
	}

	fmt.Printf("Total files in directory: %d\n", len(files))

	if err := app.Deleter.DeleteFilesWithTimeout(validDir, files, 5, 3, time.Second); err != nil {
		fmt.Println("Error deleting files:", err)
		return
	}

	fmt.Println("All files with the specified extension deleted successfully.")
}

func main() {
	validator := &DirectoryValidator{}
	deleter := &FileDeleter{Extension: ".rdp"}
	app := &Application{
		Validator: validator,
		Deleter:   deleter,
	}

	args := os.Args[1:] // Skip the executable path
	app.Run(args)
}