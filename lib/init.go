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

	// Serve an embedded CSS file
	r.HandleFunc("/static/css/style.css", func(w http.ResponseWriter, r *http.Request) {
		cssContent, err := staticFiles.ReadFile("static/css/style.css")
		if err != nil {
			http.Error(w, "CSS file not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		w.Write(cssContent)
	})

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
