package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/nats-io/nats.go"
)

type User struct {
	ID      string  `json:"id"`
	Email   string  `json:"email"`
	Balance float64 `json:"balance"`
}

type UserService struct {
	db *sql.DB
	nc *nats.Conn
}

func NewUserService(db *sql.DB, nc *nats.Conn) *UserService {
	return &UserService{db: db, nc: nc}
}

func (s *UserService) CreateUser(user User) error {
	_, err := s.db.Exec("INSERT INTO users (id, email, balance) VALUES ($1, $2, $3)", user.ID, user.Email, user.Balance)
	return err
}

func (s *UserService) GetUser(email string) (User, error) {
	var user User
	err := s.db.QueryRow("SELECT id, email, balance FROM users WHERE email = $1", email).Scan(&user.ID, &user.Email, &user.Balance)
	return user, err
}

func (s *UserService) UpdateBalance(email string, amount float64) error {
	_, err := s.db.Exec("UPDATE users SET balance = balance + $1 WHERE email = $2", amount, email)
	return err
}

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

func NewUserHandler(service *UserService) *UserHandler {
	return &UserHandler{service: service}
}

func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.service.CreateUser(user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	user, err := h.service.GetUser(email)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(user)
}

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
	w.WriteHeader(http.StatusOK)
}

func (h *UserHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	user, err := h.service.GetUser(email)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]float64{"balance": user.Balance})
}