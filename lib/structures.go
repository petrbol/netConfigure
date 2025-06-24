package lib

import (
	"embed"
	"github.com/gorilla/websocket"
	"net/http"
	"sync"
)

// StartFlags application startup flags var
var StartFlags FlagStruct

// FlagStruct application startup flags structure
type FlagStruct struct {
	ListenAddr string
	ListenPort string
}

// Destination target host
type Destination struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}

// TaskResult result of scp/ssh
type TaskResult struct {
	Host    string `json:"host"`
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error"`
}

// ProgressUpdate progress update messages
type ProgressUpdate struct {
	Type    string `json:"type"` // "scp", "ssh", or "completion"
	Host    string `json:"host"`
	Status  string `json:"status"`
	Output  string `json:"output"`
	Error   string `json:"error"`
	Success bool   `json:"success"`
}

// CompletionSummary completion summary results
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

// Embed static files into the binary
//
//go:embed templates/index.html
//go:embed static/css/style.css
var staticFiles embed.FS

// Display results
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Main destination & files declaration
var destinations []Destination
var uploadedFileName string
var uploadedFilePath string

// Handle progress messages
var wsConnections []*websocket.Conn
var connMutex sync.Mutex
