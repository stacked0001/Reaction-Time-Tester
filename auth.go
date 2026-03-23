package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const usersFile = "users.txt"

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loadUsers reads username:hashed_password pairs from file.
func loadUsers() (map[string]string, error) {
	users := make(map[string]string)

	f, err := os.Open(usersFile)
	if os.IsNotExist(err) {
		return users, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			users[parts[0]] = parts[1]
		}
	}
	return users, scanner.Err()
}

// saveUser appends a new username:hashed_password line to file.
func saveUser(username, hashedPassword string) error {
	f, err := os.OpenFile(usersFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(username + ":" + hashedPassword + "\n")
	return err
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

	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Password) == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}

	users, err := loadUsers()
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	if _, exists := users[req.Username]; exists {
		http.Error(w, "username already taken", http.StatusConflict)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	if err := saveUser(req.Username, string(hash)); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

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

	users, err := loadUsers()
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	hash, exists := users[req.Username]
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
