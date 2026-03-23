package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

const usersFile = "users.txt"

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// In-memory user store — always works, even on ephemeral filesystems.
var (
	usersMu sync.RWMutex
	users   = make(map[string]string) // username -> bcrypt hash
)

// loadUsersFromFile loads users.txt into memory at startup (best-effort).
func loadUsersFromFile() {
	f, err := os.Open(usersFile)
	if err != nil {
		return // file doesn't exist yet, that's fine
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 2)
		if len(parts) == 2 {
			users[parts[0]] = parts[1]
		}
	}
}

// saveUserToFile appends to users.txt as a best-effort backup.
func saveUserToFile(username, hash string) {
	f, err := os.OpenFile(usersFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return // no persistent disk (e.g. Render free tier) — skip silently
	}
	defer f.Close()
	f.WriteString(username + ":" + hash + "\n")
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	if req.Username == "" || req.Password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}

	usersMu.Lock()
	defer usersMu.Unlock()

	if _, exists := users[req.Username]; exists {
		http.Error(w, "username already taken", http.StatusConflict)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	users[req.Username] = string(hash)
	saveUserToFile(req.Username, string(hash))

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "registered"})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	usersMu.RLock()
	hash, exists := users[req.Username]
	usersMu.RUnlock()

	if !exists {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "ok", "username": req.Username})
}
