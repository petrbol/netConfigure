package lib

import (
	"encoding/json"
	"fmt"
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

func cssHandler(w http.ResponseWriter, r *http.Request) {
	cssContent, err := staticFiles.ReadFile("static/css/style.css")
	if err != nil {
		http.Error(w, "CSS file not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/css")
	w.Write(cssContent)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	// Read embedded HTML template
	htmlContent, err := staticFiles.ReadFile("templates/index.html")
	if err != nil {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.New("index").Parse(string(htmlContent))
	if err != nil {
		http.Error(w, "Template parsing error", http.StatusInternalServerError)
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

func broadcastProgress(update ProgressUpdate) {
	message, _ := json.Marshal(update)

	connMutex.Lock()
	defer connMutex.Unlock()

	activeConnections := []*websocket.Conn{}
	for _, conn := range wsConnections {
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			// if the connection is dead, do not add to the list
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
			// if the connection is dead, do not add to the list
			conn.Close()
		} else {
			activeConnections = append(activeConnections, conn)
		}
	}
	wsConnections = activeConnections
}
