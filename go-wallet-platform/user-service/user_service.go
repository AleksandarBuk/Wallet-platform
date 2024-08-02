package main

import (
    "errors"
    "sync"

    "github.com/nats-io/nats.go"
)

type UserService struct {
    users map[string]User
    nc    *nats.Conn
    mutex sync.RWMutex
}

func NewUserService(nc *nats.Conn) *UserService {
    return &UserService{
        users: make(map[string]User),
        nc:    nc,
    }
}

func (s *UserService) CreateUser(user User) error {
    s.mutex.Lock()
    defer s.mutex.Unlock()

    if _, exists := s.users[user.Email]; exists {
        return errors.New("user already exists")
    }

    s.users[user.Email] = user
    return nil
}

func (s *UserService) GetUser(email string) (User, error) {
    s.mutex.RLock()
    defer s.mutex.RUnlock()

    user, exists := s.users[email]
    if !exists {
        return User{}, errors.New("user not found")
    }

    return user, nil
}

func (s *UserService) UpdateBalance(email string, amount float64) error {
    s.mutex.Lock()
    defer s.mutex.Unlock()

    user, exists := s.users[email]
    if !exists {
        return errors.New("user not found")
    }

    user.Balance += amount
    s.users[email] = user
    return nil
}

func (s *UserService) GetBalance(email string) (float64, error) {
    s.mutex.RLock()
    defer s.mutex.RUnlock()

    user, exists := s.users[email]
    if !exists {
        return 0, errors.New("user not found")
    }

    return user.Balance, nil
}