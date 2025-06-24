package lib

import (
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"os"
)

func Start() {
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
	os.MkdirAll("uploads", 0755)

	fmt.Println("Listen on:", StartFlags.ListenAddr+":"+fmt.Sprintf(StartFlags.ListenPort))
	log.Fatal(http.ListenAndServe(StartFlags.ListenAddr+":"+fmt.Sprintf(StartFlags.ListenPort), r))
}
