package main

import "time"

type Transaction struct {
    ID        string    `json:"id"`
    UserID    string    `json:"user_id"`
    Amount    float64   `json:"amount"`
    Type      string    `json:"type"` // "credit", "debit", or "transfer"
    ToUserID  string    `json:"to_user_id,omitempty"` // For transfers
    Timestamp time.Time `json:"timestamp"`
}