package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// Game states
const (
	StateWaiting  = "waiting"  // fewer than 2 players
	StatePending  = "pending"  // countdown before GO
	StateActive   = "active"   // GO has been sent, waiting for clicks
	StateResults  = "results"  // showing round results
)

// Message types sent to/from clients
type Msg struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// Player represents a connected client
type Player struct {
	conn     *websocket.Conn
	username string
	// set when player clicks during active state
	reactionMs int64
	clicked    bool
	disqualified bool
}

// Game holds all shared state
type Game struct {
	mu      sync.Mutex
	players map[*Player]struct{}
	state   string
	goTime  time.Time // when GO was sent
}

var game *Game

func initGame() {
	game = &Game{
		players: make(map[*Player]struct{}),
		state:   StateWaiting,
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "username required", http.StatusBadRequest)
		return
	}

	websocket.Handler(func(ws *websocket.Conn) {
		p := &Player{conn: ws, username: username}

		game.mu.Lock()
		game.players[p] = struct{}{}
		count := len(game.players)
		game.mu.Unlock()

		log.Printf("%s connected (%d players)", username, count)

		// Notify everyone of new player count
		broadcast(Msg{Type: "players", Payload: count})

		// If we now have 2+ players and weren't already running, start a round
		maybeStartRound()

		// Read loop
		for {
			var raw string
			if err := websocket.Message.Receive(ws, &raw); err != nil {
				break
			}
			handleClientMessage(p, raw)
		}

		// Disconnected
		game.mu.Lock()
		delete(game.players, p)
		count = len(game.players)
		gameState := game.state
		game.mu.Unlock()

		log.Printf("%s disconnected (%d players)", username, count)
		broadcast(Msg{Type: "players", Payload: count})

		// If now fewer than 2, reset to waiting
		if count < 2 && gameState != StateWaiting {
			game.mu.Lock()
			game.state = StateWaiting
			game.mu.Unlock()
			broadcast(Msg{Type: "state", Payload: StateWaiting})
		}
	}).ServeHTTP(w, r)
}

// handleClientMessage processes a message from a client.
func handleClientMessage(p *Player, raw string) {
	var msg Msg
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return
	}

	if msg.Type == "click" {
		now := time.Now()
		game.mu.Lock()
		defer game.mu.Unlock()

		if game.state == StatePending {
			// Clicked before GO — disqualify
			p.disqualified = true
			p.clicked = true
			sendTo(p, Msg{Type: "disqualified", Payload: "clicked too early"})
			checkRoundComplete()
			return
		}

		if game.state != StateActive {
			return
		}

		if p.clicked {
			return // already registered
		}

		reactionMs := now.Sub(game.goTime).Milliseconds()

		if reactionMs < 100 {
			// Suspiciously fast — reject (likely a race, treat as too early)
			p.disqualified = true
			p.clicked = true
			sendTo(p, Msg{Type: "disqualified", Payload: "reaction too fast (<100ms)"})
		} else {
			p.clicked = true
			p.reactionMs = reactionMs
			sendTo(p, Msg{Type: "your_result", Payload: reactionMs})
		}

		checkRoundComplete()
	}
}

// checkRoundComplete checks if all players have clicked; must be called with game.mu held.
func checkRoundComplete() {
	for p := range game.players {
		if !p.clicked {
			return
		}
	}
	// All clicked — compute results
	game.state = StateResults
	sendResults()

	// Schedule next round
	go func() {
		time.Sleep(4 * time.Second)
		resetAndStart()
	}()
}

// sendResults broadcasts round results to all players; must be called with game.mu held.
func sendResults() {
	type result struct {
		Username     string `json:"username"`
		ReactionMs   int64  `json:"reaction_ms"`
		Disqualified bool   `json:"disqualified"`
	}

	var results []result
	var winner string
	var winnerMs int64 = -1

	for p := range game.players {
		r := result{
			Username:     p.username,
			ReactionMs:   p.reactionMs,
			Disqualified: p.disqualified,
		}
		results = append(results, r)

		if !p.disqualified && p.reactionMs > 0 {
			if winnerMs == -1 || p.reactionMs < winnerMs {
				winnerMs = p.reactionMs
				winner = p.username
			}
		}
	}

	payload := map[string]interface{}{
		"results": results,
		"winner":  winner,
	}

	// Broadcast without holding lock (sendTo is safe)
	for p := range game.players {
		sendTo(p, Msg{Type: "results", Payload: payload})
	}
}

// resetAndStart resets player state and starts a new round.
func resetAndStart() {
	game.mu.Lock()
	for p := range game.players {
		p.clicked = false
		p.disqualified = false
		p.reactionMs = 0
	}
	count := len(game.players)
	if count < 2 {
		game.state = StateWaiting
		game.mu.Unlock()
		broadcast(Msg{Type: "state", Payload: StateWaiting})
		return
	}
	game.state = StatePending
	game.mu.Unlock()

	broadcast(Msg{Type: "state", Payload: StatePending})
	startRoundAfterDelay()
}

// maybeStartRound starts a round if conditions are met.
func maybeStartRound() {
	game.mu.Lock()
	count := len(game.players)
	state := game.state
	game.mu.Unlock()

	if count >= 2 && state == StateWaiting {
		game.mu.Lock()
		game.state = StatePending
		game.mu.Unlock()

		broadcast(Msg{Type: "state", Payload: StatePending})
		startRoundAfterDelay()
	}
}

// startRoundAfterDelay waits a random 2–5s then sends GO.
func startRoundAfterDelay() {
	delay := time.Duration(2000+rand.Intn(3000)) * time.Millisecond
	time.Sleep(delay)

	game.mu.Lock()
	// Abort if state changed (e.g. player left)
	if game.state != StatePending {
		game.mu.Unlock()
		return
	}
	game.state = StateActive
	game.goTime = time.Now()
	game.mu.Unlock()

	broadcast(Msg{Type: "go"})

	// Auto-end round after 10 seconds in case some players never click
	go func() {
		time.Sleep(10 * time.Second)
		game.mu.Lock()
		if game.state == StateActive {
			// Force-click anyone who hasn't clicked
			for p := range game.players {
				if !p.clicked {
					p.clicked = true
					p.disqualified = true
				}
			}
			game.state = StateResults
			sendResults()
			game.mu.Unlock()
			go func() {
				time.Sleep(4 * time.Second)
				resetAndStart()
			}()
		} else {
			game.mu.Unlock()
		}
	}()
}

// broadcast sends a message to all connected players.
func broadcast(msg Msg) {
	game.mu.Lock()
	players := make([]*Player, 0, len(game.players))
	for p := range game.players {
		players = append(players, p)
	}
	game.mu.Unlock()

	for _, p := range players {
		sendTo(p, msg)
	}
}

// sendTo sends a message to a single player.
func sendTo(p *Player, msg Msg) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	if err := websocket.Message.Send(p.conn, string(data)); err != nil {
		log.Printf("send error to %s: %v", p.username, err)
	}
}
