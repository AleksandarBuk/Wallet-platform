package main

import (
    "encoding/json"
    "log"
    "net/http"
    "strings"
    "strconv"
	"fmt"
    "github.com/gorilla/mux"
    "github.com/nats-io/nats.go"
)

func main() {
    // Connect to NATS
    nc, err := nats.Connect(nats.DefaultURL)
    if err != nil {
        log.Fatal(err)
    }
    defer nc.Close()

    userService := NewUserService(nc)
    userHandler := NewUserHandler(userService)

    r := mux.NewRouter()
    r.HandleFunc("/users", userHandler.CreateUser).Methods("POST")
    r.HandleFunc("/users", userHandler.GetUser).Methods("GET")
    r.HandleFunc("/users/balance", userHandler.UpdateBalance).Methods("PUT")
    r.HandleFunc("/balance", userHandler.GetBalance).Methods("GET")

    nc.Subscribe("user.update_balance", func(m *nats.Msg) {
		parts := strings.Split(string(m.Data), ":")
		if len(parts) != 2 {
			m.Respond([]byte("invalid request: expected format 'email:amount'"))
			return
		}
	
		email := parts[0]
		amount, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			m.Respond([]byte(fmt.Sprintf("invalid amount: %v", err)))
			return
		}
	
		err = userService.UpdateBalance(email, amount)
		if err != nil {
			m.Respond([]byte(fmt.Sprintf("failed to update balance: %v", err)))
			return
		}
	
		updatedUser, err := userService.GetUser(email)
		if err != nil {
			m.Respond([]byte(fmt.Sprintf("balance updated but failed to fetch user: %v", err)))
			return
		}
	
		response, err := json.Marshal(struct {
			Email   string  `json:"email"`
			Balance float64 `json:"balance"`
		}{
			Email:   updatedUser.Email,
			Balance: updatedUser.Balance,
		})
		if err != nil {
			m.Respond([]byte(fmt.Sprintf("balance updated but failed to serialize response: %v", err)))
			return
		}
	
		m.Respond(response)
	})

    log.Println("Starting user service on :8080")
    log.Fatal(http.ListenAndServe(":8080", r))
}