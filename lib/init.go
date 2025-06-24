package lib

import (
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"net"
	"net/http"
	"os"
)

func Start() {
	validateStartupFlags()
	r := mux.NewRouter()

	// Serve an CSS file
	r.HandleFunc("/static/css/style.css", cssHandler)

	// Routes
	r.HandleFunc("/", homeHandler).Methods("GET")
	r.HandleFunc("/upload-destinations", uploadDestinationsHandler).Methods("POST")
	r.HandleFunc("/upload-file", uploadFileHandler).Methods("POST")
	r.HandleFunc("/execute", executeHandler).Methods("POST")
	r.HandleFunc("/reset", resetHandler).Methods("POST")
	r.HandleFunc("/ws", websocketHandler)

	// Create the uploads directory
	err := os.MkdirAll("uploads", 0755)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Listen on:", StartFlags.ListenAddr+":"+fmt.Sprint(StartFlags.ListenPort))
	log.Fatal(http.ListenAndServe(StartFlags.ListenAddr+":"+fmt.Sprint(StartFlags.ListenPort), r))
}

// ValidateStartupFlags to test startup values
func validateStartupFlags() {
	if StartFlags.ListenPort < 1 || StartFlags.ListenPort > 65535 {
		log.Fatal("Invalid port number")
	}

	if len(StartFlags.ListenAddr) == 0 {
		return
	}

	if addr := net.ParseIP(StartFlags.ListenAddr); addr == nil {
		log.Fatal("Invalid IP address")
	}
}
