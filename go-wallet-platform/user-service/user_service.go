package main

import (
    "database/sql"
    "encoding/json"
    "net/http"
    "strconv"
    "strings"
    "sync"
    "errors"
    "github.com/lib/pq"
    "github.com/nats-io/nats.go"
)

var ErrUserAlreadyExists = errors.New("user already exists")

type User struct {
	ID      string  `json:"id"`
	Email   string  `json:"email"`
	Balance float64 `json:"balance"`
}

type UserService struct {
    db    *sql.DB
    nc    *nats.Conn
    mutex sync.RWMutex
    users map[string]User
}

// NewUserService creates a new UserService instance
func NewUserService(db *sql.DB, nc *nats.Conn) *UserService {
	return &UserService{
		db:    db,
		nc:    nc,
		users: make(map[string]User),
	}
}

// CreateUser adds a new user to the database and in-memory map
func (s *UserService) CreateUser(user User) error {
    _, err := s.db.Exec("INSERT INTO users (id, email, balance) VALUES ($1, $2, $3)", user.ID, user.Email, user.Balance)
    if err != nil {
        if pqErr, ok := err.(*pq.Error); ok {
            if pqErr.Code == "23505" { // unique_violation
                return ErrUserAlreadyExists
            }
        }
        return err
    }
    s.mutex.Lock()
    s.users[user.Email] = user
    s.mutex.Unlock()
    return nil
}

// GetUser retrieves a user from the in-memory map or database
func (s *UserService) GetUser(email string) (User, error) {
	s.mutex.RLock()
	user, exists := s.users[email]
	s.mutex.RUnlock()
	if exists {
		return user, nil
	}

	var dbUser User
	err := s.db.QueryRow("SELECT id, email, balance FROM users WHERE email = $1", email).Scan(&dbUser.ID, &dbUser.Email, &dbUser.Balance)
	if err != nil {
		return User{}, err
	}

	s.mutex.Lock()
	s.users[email] = dbUser
	s.mutex.Unlock()

	return dbUser, nil
}

// UpdateBalance updates the balance of a user in the database and in-memory map
func (s *UserService) UpdateBalance(email string, amount float64) error {
    user, err := s.GetUser(email)
    if err != nil {
        return err
    }

    user.Balance += amount

    _, err = s.db.Exec("UPDATE users SET balance = $1 WHERE email = $2", user.Balance, email)
    if err != nil {
        return err
    }

    s.mutex.Lock()
    s.users[email] = user
    s.mutex.Unlock()
    return nil
}

// HandleBalanceUpdate handles balance update messages from NATS
func (s *UserService) HandleBalanceUpdate(m *nats.Msg) {
	parts := strings.Split(string(m.Data), ":")
	if len(parts) != 2 {
		m.Respond([]byte("invalid request format"))
		return
	}

	email, amountStr := parts[0], parts[1]
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		m.Respond([]byte("invalid amount"))
		return
	}

	if err := s.UpdateBalance(email, amount); err != nil {
		m.Respond([]byte("failed to update balance"))
		return
	}

	user, err := s.GetUser(email)
	if err != nil {
		m.Respond([]byte("failed to fetch updated user"))
		return
	}

	response, _ := json.Marshal(user)
	m.Respond(response)
}

type UserHandler struct {
    service *UserService
}

// NewUserHandler creates a new UserHandler instance
func NewUserHandler(service *UserService) *UserHandler {
	return &UserHandler{service: service}
}

// CreateUser handles HTTP requests to create a new user
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
    var user User
    if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    if err := h.service.CreateUser(user); err != nil {
        if err == ErrUserAlreadyExists {
            http.Error(w, "User already exists", http.StatusConflict)
        } else {
            http.Error(w, err.Error(), http.StatusInternalServerError)
        }
        return
    }
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(user)
}

// GetUser handles HTTP requests to retrieve a user
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
    email := r.URL.Query().Get("email")
    user, err := h.service.GetUser(email)
    if err != nil {
        http.Error(w, "User not found", http.StatusNotFound)
        return
    }
    json.NewEncoder(w).Encode(user)
}

// UpdateBalance handles HTTP requests to update a user's balance
func (h *UserHandler) UpdateBalance(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email  string  `json:"email"`
        Amount float64 `json:"amount"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    if err := h.service.UpdateBalance(req.Email, req.Amount); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    user, _ := h.service.GetUser(req.Email)
    json.NewEncoder(w).Encode(map[string]float64{"balance": user.Balance})
}

// GetBalance handles HTTP requests to retrieve a user's balance
func (h *UserHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
    email := r.URL.Query().Get("email")
    user, err := h.service.GetUser(email)
    if err != nil {
        http.Error(w, "User not found", http.StatusNotFound)
        return
    }
    json.NewEncoder(w).Encode(map[string]float64{"balance": user.Balance})
}

