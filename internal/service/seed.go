package service

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

func SeedAdmin(ctx context.Context, users *repository.UserRepository, username, email, password string) error {
	count, err := users.Count(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	if password == "" {
		password = uuid.New().String()
		log.Printf("[INIT] No ADMIN_PASSWORD set. Generated admin password: %s", password)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	admin := &model.User{
		Username:        username,
		Email:           email,
		PasswordHash:    string(hash),
		DefaultCurrency: "RUB",
		IsAdmin:         true,
		IsActive:        true,
	}
	if err = users.Create(ctx, admin); err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	log.Printf("[INIT] Admin account created: %s (%s)", username, email)
	return nil
}
