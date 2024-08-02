package main

import (
    "database/sql"
    "log"
    "net/http"

    "github.com/gorilla/mux"
    _ "github.com/lib/pq"
    "github.com/nats-io/nats.go"
)

func main() {
	// Connect to NATS
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", "host=localhost dbname=wallet_platform user=mainuser password=mainuserpass sslmode=disable")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	userService := NewUserService(db, nc)
    userHandler := NewUserHandler(userService)

    r := mux.NewRouter()
    r.HandleFunc("/users", userHandler.CreateUser).Methods("POST")
    r.HandleFunc("/users", userHandler.GetUser).Methods("GET")
    r.HandleFunc("/users/balance", userHandler.UpdateBalance).Methods("PUT")
    r.HandleFunc("/balance", userHandler.GetBalance).Methods("GET")

    setupNATSSubscriptions(nc, userService)

    log.Println("Starting user service on :8080")
    log.Fatal(http.ListenAndServe(":8080", r))
}

func setupNATSSubscriptions(nc *nats.Conn, userService *UserService) {
	nc.Subscribe("user.update_balance", func(m *nats.Msg) {
		userService.HandleBalanceUpdate(m)
	})
}