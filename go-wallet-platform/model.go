// model.go
package main

import "time"

type Transaction struct {
    ID        string    `json:"id"`
    UserID    string    `json:"user_id"`
    Amount    float64   `json:"amount"`
    Type      string    `json:"type"`
    ToUserID  string    `json:"to_user_id,omitempty"`
    Timestamp time.Time `json:"timestamp"`
}