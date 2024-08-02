package main

import (
    "encoding/json"
    "net/http"
)

type UserHandler struct {
    userService *UserService
}

func NewUserHandler(userService *UserService) *UserHandler {
    return &UserHandler{userService: userService}
}

func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
    var user User
    if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    if err := h.userService.CreateUser(user); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(user)
}

func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
    email := r.URL.Query().Get("email")
    user, err := h.userService.GetUser(email)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }

    json.NewEncoder(w).Encode(user)
}

func (h *UserHandler) UpdateBalance(w http.ResponseWriter, r *http.Request) {
    var update struct {
        Email  string  `json:"email"`
        Amount float64 `json:"amount"`
    }
    if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    if err := h.userService.UpdateBalance(update.Email, update.Amount); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}

func (h *UserHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
    var request struct {
        Email string `json:"email"`
    }
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    balance, err := h.userService.GetBalance(request.Email)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    response := struct {
        Email   string  `json:"email"`
        Balance float64 `json:"balance"`
    }{
        Email:   request.Email,
        Balance: balance,
    }
    json.NewEncoder(w).Encode(response)
}