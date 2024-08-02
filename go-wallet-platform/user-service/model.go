package main

type User struct {
    ID       string  `json:"id"`
    Username string  `json:"username"`
    Email    string  `json:"email"`
    Balance  float64 `json:"balance"`
}