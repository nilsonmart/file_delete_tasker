package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// // DirectoryValidator handles directory validation logic
// type DirectoryValidator struct{}

// // FileDeleter handles file deletion logic
// type FileDeleter struct {
// 	Extension string
// }

func (dv *DirectoryValidator) Validate(dirPath string) (string, error) {
	for {
		if _, err := os.Stat(dirPath); err == nil {
			return dirPath, nil
		}

		fmt.Println("Invalid directory. Please enter a valid directory path:")
		reader := bufio.NewReader(os.Stdin)
		newPath, _ := reader.ReadString('\n')
		dirPath = strings.TrimSpace(newPath)
	}
}

func (fd *FileDeleter) DeleteFilesWithTimeout(dirPath string, files []os.DirEntry, workerCount, maxRetries int, timeout time.Duration) error {
	type FileTask struct {
		FileName string
		Retries  int
	}

	fileChan := make(chan FileTask, len(files)) // Channel to pass file tasks
	errorChan := make(chan error, len(files))   // Channel to collect errors
	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range fileChan {
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()

				filePath := filepath.Join(dirPath, task.FileName)
				errChan := make(chan error, 1)

				// Attempt to delete the file with a timeout
				go func() {
					errChan <- os.Remove(filePath)
				}()

				select {
				case <-ctx.Done():
					// Timeout occurred
					if task.Retries < maxRetries {
						task.Retries++
						fileChan <- task // Re-queue the file for retry
					} else {
						errorChan <- fmt.Errorf("timeout deleting file after %d retries: %s", maxRetries, filePath)
					}
				case err := <-errChan:
					// File deletion completed
					if err != nil {
						if task.Retries < maxRetries {
							task.Retries++
							fileChan <- task // Re-queue the file for retry
						} else {
							errorChan <- fmt.Errorf("failed to delete file after %d retries: %s, %v", maxRetries, filePath, err)
						}
					} else {
						fmt.Printf("Deleted file: %s\n", filePath)
					}
				}
			}
		}()
	}

	// Send initial file tasks to the channel
	go func() {
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), fd.Extension) {
				fileChan <- FileTask{FileName: file.Name(), Retries: 0}
			}
		}
		close(fileChan) // Close the channel to signal workers no more files are coming
	}()

	// Wait for all workers to finish
	wg.Wait()
	close(errorChan) // Close the error channel after all workers are done

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

func (app *Application) Run(args []string) {
	if len(args) == 0 {
		fmt.Println("No directory path provided. Please provide a valid directory path.")
		return
	}

	if len(args) > 1 {
		for _, v := range args {
			fmt.Println("Found parameter:", v)
		}
		fmt.Println("Only one argument is allowed, the directory path to scan.")
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
	files, _ := os.ReadDir("path/to/directory")
	if err := app.Deleter.DeleteFilesWithTimeout("path/to/directory", files, 5, 3, time.Second); err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("All files deleted successfully.")
	}

	args := os.Args[1:] // Skip the executable path
	app.Run(args)
}
