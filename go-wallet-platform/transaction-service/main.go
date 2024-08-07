package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "time"
    
    "github.com/google/uuid"
    "github.com/gorilla/mux"
    _ "github.com/lib/pq"
    "github.com/nats-io/nats.go"
)

type TransactionService struct {
    nc *nats.Conn
    db *sql.DB
}

type Transaction struct {
    ID        string    `json:"id"`
    UserID    string    `json:"user_id"`
    Amount    float64   `json:"amount"`
    Type      string    `json:"type"`
    ToUserID  string    `json:"to_user_id,omitempty"`
    Timestamp time.Time `json:"timestamp"`
}

// NewTransactionService creates a new instance of TransactionService
func NewTransactionService(nc *nats.Conn, db *sql.DB) *TransactionService {
    return &TransactionService{
        nc: nc,
        db: db,
    }
}

// AddMoney adds a specified amount of money to a user's account
func (s *TransactionService) AddMoney(userEmail string, amount float64) (float64, error) {
    tx, err := s.db.Begin()
    if err != nil {
        return 0, err
    }
    defer tx.Rollback()

    var userID string
    err = tx.QueryRow("SELECT id FROM users WHERE email = $1", userEmail).Scan(&userID)
    if err != nil {
        return 0, fmt.Errorf("user not found: %v", err)
    }

    transactionID := uuid.New().String()
    _, err = tx.Exec(`
        INSERT INTO transactions (id, user_id, amount, type, timestamp)
        VALUES ($1, $2, $3, $4, $5)
    `, transactionID, userID, amount, "credit", time.Now())
    if err != nil {
        return 0, err
    }

    var newBalance float64
    err = tx.QueryRow("UPDATE users SET balance = balance + $1 WHERE id = $2 RETURNING balance", amount, userID).Scan(&newBalance)
    if err != nil {
        return 0, err
    }

    if err := tx.Commit(); err != nil {
        return 0, err
    }

    return newBalance, nil
}

// TransferMoney transfers a specified amount of money from one user to another
func (s *TransactionService) TransferMoney(fromEmail, toEmail string, amount float64) error {
    // Begin a new database transaction
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    // Ensure the transaction is rolled back in case of an error
    defer tx.Rollback()

    var fromUserID, toUserID string

    // Retrieve the sender's user ID based on their email
    err = tx.QueryRow("SELECT id FROM users WHERE email = $1", fromEmail).Scan(&fromUserID)
    if err != nil {
        return fmt.Errorf("sender not found: %v", err)
    }

    // Retrieve the recipient's user ID based on their email
    err = tx.QueryRow("SELECT id FROM users WHERE email = $1", toEmail).Scan(&toUserID)
    if err != nil {
        return fmt.Errorf("recipient not found: %v", err)
    }

    var senderBalance float64

    // Retrieve the sender's current balance
    err = tx.QueryRow("SELECT balance FROM users WHERE id = $1", fromUserID).Scan(&senderBalance)
    if err != nil {
        return fmt.Errorf("failed to get sender's balance: %v", err)
    }

    // Check if the sender has sufficient balance to transfer the specified amount
    if senderBalance < amount {
        return fmt.Errorf("insufficient balance")
    }

    // Create a debit transaction for the sender
    debitID := uuid.New().String()
    _, err = tx.Exec(`
        INSERT INTO transactions (id, user_id, amount, type, to_user_id, timestamp)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, debitID, fromUserID, -amount, "transfer", toUserID, time.Now())
    if err != nil {
        return err
    }

    // Create a credit transaction for the recipient
    creditID := uuid.New().String()
    _, err = tx.Exec(`
        INSERT INTO transactions (id, user_id, amount, type, to_user_id, timestamp)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, creditID, toUserID, amount, "transfer", fromUserID, time.Now())
    if err != nil {
        return err
    }

    // Update the sender's balance by deducting the transferred amount
    _, err = tx.Exec("UPDATE users SET balance = balance - $1 WHERE id = $2", amount, fromUserID)
    if err != nil {
        return err
    }

    // Update the recipient's balance by adding the transferred amount
    _, err = tx.Exec("UPDATE users SET balance = balance + $1 WHERE id = $2", amount, toUserID)
    if err != nil {
        return err
    }

    // Commit the transaction to make all changes permanent
    return tx.Commit()
}

// handleAddMoney handles HTTP requests to add money to a user's account
func handleAddMoney(w http.ResponseWriter, r *http.Request, s *TransactionService) {
    var req struct {
        UserEmail string  `json:"user_email"`
        Amount    float64 `json:"amount"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    updatedBalance, err := s.AddMoney(req.UserEmail, req.Amount)
    if err != nil {
        http.Error(w, fmt.Sprintf("Error adding money: %v", err), http.StatusInternalServerError)
        return
    }

    response := struct {
        UpdatedBalance float64 `json:"updated_balance"`
    }{
        UpdatedBalance: updatedBalance,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

// handleTransferMoney handles HTTP requests to transfer money from one user to another
func handleTransferMoney(w http.ResponseWriter, r *http.Request, s *TransactionService) {
    var req struct {
        FromEmail string  `json:"from_email"`
        ToEmail   string  `json:"to_email"`
        Amount    float64 `json:"amount"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    if err := s.TransferMoney(req.FromEmail, req.ToEmail, req.Amount); err != nil {
        http.Error(w, fmt.Sprintf("Error transferring money: %v", err), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// main is the entry point of the application. It sets up the connections to NATS and PostgreSQL,
// initializes the transaction service, sets up the HTTP routes, and starts the HTTP server.
func main() {
    db, err := sql.Open("postgres", "host=localhost dbname=wallet_platform user=mainuser password=mainuserpass sslmode=disable")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    nc, err := nats.Connect(nats.DefaultURL)
    if err != nil {
        log.Fatal(err)
    }
    defer nc.Close()

    service := NewTransactionService(nc, db)

    r := mux.NewRouter()
    r.HandleFunc("/transactions/add", func(w http.ResponseWriter, r *http.Request) {
        handleAddMoney(w, r, service)
    }).Methods("POST")
    r.HandleFunc("/transactions/transfer", func(w http.ResponseWriter, r *http.Request) {
        handleTransferMoney(w, r, service)
    }).Methods("POST")

    log.Println("Transaction service is running on :8081")
    log.Fatal(http.ListenAndServe(":8081", r))
}