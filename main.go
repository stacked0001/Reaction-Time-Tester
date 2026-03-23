package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	// Initialize game state
	initGame()

	// Auth routes
	http.HandleFunc("/register", handleRegister)
	http.HandleFunc("/login", handleLogin)

	// WebSocket route
	http.HandleFunc("/ws", handleWebSocket)

	// Serve static files
	http.Handle("/", http.FileServer(http.Dir("./static")))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server running on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
