package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
)

// Destination represents a target host
type Destination struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}

// TaskResult represents the result of an operation
type TaskResult struct {
	Host    string `json:"host"`
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error"`
}

// ProgressUpdate represents a progress update
type ProgressUpdate struct {
	Type    string `json:"type"` // "scp", "ssh", or "completion"
	Host    string `json:"host"`
	Status  string `json:"status"`
	Output  string `json:"output"`
	Error   string `json:"error"`
	Success bool   `json:"success"`
}

// CompletionSummary represents the final completion status
type CompletionSummary struct {
	Type             string `json:"type"` // "completion"
	TotalHosts       int    `json:"total_hosts"`
	SuccessfulSCP    int    `json:"successful_scp"`
	FailedSCP        int    `json:"failed_scp"`
	SuccessfulSSH    int    `json:"successful_ssh"`
	FailedSSH        int    `json:"failed_ssh"`
	Status           string `json:"status"`
	AllOperations    int    `json:"all_operations"`
	FailedOperations int    `json:"failed_operations"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var destinations []Destination
var uploadedFileName string
var uploadedFilePath string

func main() {
	r := mux.NewRouter()

	// Static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	// Routes
	r.HandleFunc("/", homeHandler).Methods("GET")
	r.HandleFunc("/upload-destinations", uploadDestinationsHandler).Methods("POST")
	r.HandleFunc("/upload-file", uploadFileHandler).Methods("POST")
	r.HandleFunc("/execute", executeHandler).Methods("POST")
	r.HandleFunc("/reset", resetHandler).Methods("POST")
	r.HandleFunc("/ws", websocketHandler)

	// Create the necessary directories
	os.MkdirAll("uploads", 0755)
	os.MkdirAll("templates", 0755)
	os.MkdirAll("static/css", 0755)
	os.MkdirAll("static/js", 0755)
	os.MkdirAll("static/images", 0755)

	fmt.Println("Server starting on :8080")
	fmt.Println("Make sure to create the following structure:")
	fmt.Println("  templates/index.html")
	fmt.Println("  static/css/style.css")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	// Reset everything
	destinations = []Destination{}
	uploadedFileName = ""
	uploadedFilePath = ""

	// Clean up uploads directory
	os.RemoveAll("uploads")
	os.MkdirAll("uploads", 0755)

	tmpl.Execute(w, nil)
}

func uploadDestinationsHandler(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("destinationFile")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to read file: " + err.Error(),
		})
		return
	}
	defer file.Close()

	var newDestinations []Destination
	if err := json.NewDecoder(file).Decode(&newDestinations); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to parse JSON: " + err.Error(),
		})
		return
	}

	// Set the default port if not specified
	for i := range newDestinations {
		if newDestinations[i].Port == 0 {
			newDestinations[i].Port = 22
		}
	}

	destinations = newDestinations
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"destinations": destinations,
	})
}

func uploadFileHandler(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("uploadFile")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to read file: " + err.Error(),
		})
		return
	}
	defer file.Close()

	uploadedFileName = header.Filename
	uploadedFilePath = filepath.Join("uploads", uploadedFileName)

	dst, err := os.Create(uploadedFilePath)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to create file: " + err.Error(),
		})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to save file: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"filename": uploadedFileName,
	})
}

var wsConnections []*websocket.Conn
var connMutex sync.Mutex

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer func() {
		conn.Close()
		// Remove connection from slice when it closes
		connMutex.Lock()
		for i, c := range wsConnections {
			if c == conn {
				wsConnections = append(wsConnections[:i], wsConnections[i+1:]...)
				break
			}
		}
		connMutex.Unlock()
	}()

	connMutex.Lock()
	wsConnections = append(wsConnections, conn)
	connMutex.Unlock()

	// Keep the connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func broadcastProgress(update ProgressUpdate) {
	message, _ := json.Marshal(update)

	connMutex.Lock()
	defer connMutex.Unlock()

	activeConnections := []*websocket.Conn{}
	for _, conn := range wsConnections {
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			// Connection is dead, don't add to active list
			conn.Close()
		} else {
			activeConnections = append(activeConnections, conn)
		}
	}
	wsConnections = activeConnections
}

func broadcastCompletion(summary CompletionSummary) {
	message, _ := json.Marshal(summary)

	connMutex.Lock()
	defer connMutex.Unlock()

	activeConnections := []*websocket.Conn{}
	for _, conn := range wsConnections {
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			// Connection is dead, don't add to active list
			conn.Close()
		} else {
			activeConnections = append(activeConnections, conn)
		}
	}
	wsConnections = activeConnections
}

func executeHandler(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")
	targetDir := r.FormValue("targetDir")
	command := r.FormValue("command")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

	// Execute tasks asynchronously
	go func() {
		var wg sync.WaitGroup
		var mu sync.Mutex

		// Counters for completion summary
		totalHosts := len(destinations)
		successfulSCP := 0
		failedSCP := 0
		successfulSSH := 0
		failedSSH := 0

		hasFileToUpload := uploadedFilePath != ""
		hasCommandToExecute := command != ""

		// Calculate total operations
		totalOperations := 0
		if hasFileToUpload {
			totalOperations += totalHosts
		}
		if hasCommandToExecute {
			totalOperations += totalHosts
		}

		if totalOperations == 0 {
			// Nothing to do
			broadcastCompletion(CompletionSummary{
				Type:             "completion",
				TotalHosts:       totalHosts,
				Status:           "No operations to perform",
				AllOperations:    0,
				FailedOperations: 0,
			})
			return
		}

		for _, dest := range destinations {
			wg.Add(1)
			go func(destination Destination) {
				defer wg.Done()

				if hasFileToUpload {
					// SCP file transfer
					broadcastProgress(ProgressUpdate{
						Type:   "scp",
						Host:   destination.Address,
						Status: "Starting file transfer...",
					})

					scpCmd := exec.Command("sshpass", "-p", password, "scp",
						"-P", strconv.Itoa(destination.Port),
						"-o", "StrictHostKeyChecking=no",
						"-O", //legacy mode
						uploadedFilePath,
						fmt.Sprintf("%s@%s:%s/", username, destination.Address, targetDir))

					scpOutput, scpErr := scpCmd.CombinedOutput()

					mu.Lock()
					if scpErr != nil {
						failedSCP++
						mu.Unlock()
						broadcastProgress(ProgressUpdate{
							Type:    "scp",
							Host:    destination.Address,
							Status:  "File transfer failed",
							Error:   scpErr.Error() + "\n" + string(scpOutput),
							Success: false,
						})
					} else {
						successfulSCP++
						mu.Unlock()
						broadcastProgress(ProgressUpdate{
							Type:    "scp",
							Host:    destination.Address,
							Status:  "File transfer completed",
							Output:  string(scpOutput),
							Success: true,
						})
					}
				}

				// SSH command execution
				if hasCommandToExecute {
					broadcastProgress(ProgressUpdate{
						Type:   "ssh",
						Host:   destination.Address,
						Status: "Executing command...",
					})

					sshCmd := exec.Command("sshpass", "-p", password, "ssh",
						"-p", strconv.Itoa(destination.Port),
						"-o", "StrictHostKeyChecking=no",
						fmt.Sprintf("%s@%s", username, destination.Address),
						command)

					sshOutput, sshErr := sshCmd.CombinedOutput()

					mu.Lock()
					if sshErr != nil {
						failedSSH++
						mu.Unlock()
						broadcastProgress(ProgressUpdate{
							Type:    "ssh",
							Host:    destination.Address,
							Status:  "Command execution failed",
							Error:   sshErr.Error() + "\n" + string(sshOutput),
							Success: false,
						})
					} else {
						successfulSSH++
						mu.Unlock()
						broadcastProgress(ProgressUpdate{
							Type:    "ssh",
							Host:    destination.Address,
							Status:  "Command executed successfully",
							Output:  string(sshOutput),
							Success: true,
						})
					}
				}
			}(dest)
		}

		// Wait for all operations to complete
		wg.Wait()

		// Calculate totals and send completion summary
		totalFailed := failedSCP + failedSSH
		totalSuccessful := successfulSCP + successfulSSH

		var status string
		if totalFailed == 0 {
			status = fmt.Sprintf("All operations completed successfully! (%d/%d)", totalSuccessful, totalOperations)
		} else if totalSuccessful == 0 {
			status = fmt.Sprintf("All operations failed! (%d/%d)", totalFailed, totalOperations)
		} else {
			status = fmt.Sprintf("Operations completed with %d failures and %d successes", totalFailed, totalSuccessful)
		}

		// Send completion summary
		broadcastCompletion(CompletionSummary{
			Type:             "completion",
			TotalHosts:       totalHosts,
			SuccessfulSCP:    successfulSCP,
			FailedSCP:        failedSCP,
			SuccessfulSSH:    successfulSSH,
			FailedSSH:        failedSSH,
			Status:           status,
			AllOperations:    totalOperations,
			FailedOperations: totalFailed,
		})
	}()
}

func resetHandler(w http.ResponseWriter, r *http.Request) {
	// Reset everything
	destinations = []Destination{}
	uploadedFileName = ""
	uploadedFilePath = ""

	// Clean up uploads directory
	os.RemoveAll("uploads")
	os.MkdirAll("uploads", 0755)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}
