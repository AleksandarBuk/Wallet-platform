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

// type Transaction struct {
//     ID        string    `json:"id"`
//     UserID    string    `json:"user_id"`
//     Amount    float64   `json:"amount"`
//     Type      string    `json:"type"`
//     ToUserID  string    `json:"to_user_id,omitempty"`
//     Timestamp time.Time `json:"timestamp"`
// }


func NewTransactionService(nc *nats.Conn, db *sql.DB) *TransactionService {
    return &TransactionService{
        nc: nc,
        db: db,
    }
}

func (s *TransactionService) AddMoney(userID string, amount float64) (float64, error) {
    tx, err := s.db.Begin()
    if err != nil {
        return 0, err
    }
    defer tx.Rollback()

    transactionID := uuid.New().String()
    _, err = tx.Exec(`
        INSERT INTO transactions (id, user_id, amount, type, timestamp)
        VALUES ($1, $2, $3, $4, $5)
    `, transactionID, userID, amount, "credit", time.Now())
    if err != nil {
        return 0, err
    }

    msg, err := s.nc.Request("user.update_balance", []byte(fmt.Sprintf("%s:%f", userID, amount)), 5*time.Second)
    if err != nil {
        return 0, err
    }

    var response struct {
        Balance float64 `json:"balance"`
    }
    if err := json.Unmarshal(msg.Data, &response); err != nil {
        return 0, fmt.Errorf("failed to unmarshal response: %v", err)
    }

    if err := tx.Commit(); err != nil {
        return 0, err
    }

    return response.Balance, nil
}

func (s *TransactionService) TransferMoney(fromUserID, toUserID string, amount float64) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    debitID := uuid.New().String()
    _, err = tx.Exec(`
        INSERT INTO transactions (id, user_id, amount, type, to_user_id, timestamp)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, debitID, fromUserID, -amount, "transfer", toUserID, time.Now())
    if err != nil {
        return err
    }

    creditID := uuid.New().String()
    _, err = tx.Exec(`
        INSERT INTO transactions (id, user_id, amount, type, to_user_id, timestamp)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, creditID, toUserID, amount, "transfer", fromUserID, time.Now())
    if err != nil {
        return err
    }

    _, err = s.nc.Request("user.update_balance", []byte(fmt.Sprintf("%s:%f", fromUserID, -amount)), 5*time.Second)
    if err != nil {
        return err
    }

    _, err = s.nc.Request("user.update_balance", []byte(fmt.Sprintf("%s:%f", toUserID, amount)), 5*time.Second)
    if err != nil {
        return err
    }

    return tx.Commit()
}

func handleAddMoney(w http.ResponseWriter, r *http.Request, s *TransactionService) {
    var req struct {
        UserID string  `json:"user_id"`
        Amount float64 `json:"amount"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    updatedBalance, err := s.AddMoney(req.UserID, req.Amount)
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

func handleTransferMoney(w http.ResponseWriter, r *http.Request, s *TransactionService) {
    var req struct {
        FromUserID string  `json:"from_user_id"`
        ToUserID   string  `json:"to_user_id"`
        Amount     float64 `json:"amount_to_transfer"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    if err := s.TransferMoney(req.FromUserID, req.ToUserID, req.Amount); err != nil {
        http.Error(w, fmt.Sprintf("Error transferring money: %v", err), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}

func main() {
    // Connect to PostgreSQL
    db, err := sql.Open("postgres", "host=localhost dbname=wallet_platform user=mainuser password=mainuserpass sslmode=disable")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Connect to NATS
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