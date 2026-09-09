package repository_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
)

func TestExternalTransfers(t *testing.T) {
	dsn := os.Getenv("BALANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BALANCE_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := fmt.Sprintf("balance_test_%d", time.Now().UnixNano())
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	require.NoError(t, err)
	defer func() {
		_, cleanupErr := admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		require.NoError(t, cleanupErr)
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	sqlDB := stdlib.OpenDB(*cfg.ConnConfig)
	defer func() { require.NoError(t, sqlDB.Close()) }()
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(sqlDB, "../../migrations"))
	exec := func(query string, args ...any) {
		_, execErr := pool.Exec(ctx, query, args...)
		require.NoError(t, execErr)
	}

	const sender = "00000000-0000-0000-0000-000000000001"
	const recipient = "00000000-0000-0000-0000-000000000002"
	const member = "00000000-0000-0000-0000-000000000003"
	exec(`INSERT INTO users(id,username,email,password_hash) VALUES ($1,'sender','sender@test','x'),($2,'recipient','recipient@test','x'),($3,'member','member@test','x')`, sender, recipient, member)
	accounts := repository.NewAccountRepository(pool)
	src, err := accounts.Create(ctx, model.Account{OwnerID: sender, Name: "Source", Currency: "USD", AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, InitialBalanceDate: time.Now().AddDate(0, 0, -1)})
	require.NoError(t, err)
	dst, err := accounts.Create(ctx, model.Account{OwnerID: recipient, Name: "Destination", Currency: "EUR", AccessMode: model.AccountAccessModeShared, Kind: model.AccountKindSpending, AcceptTransfers: true, InitialBalanceDate: time.Now().AddDate(0, 0, -1)})
	require.NoError(t, err)
	require.NoError(t, accounts.AddMember(ctx, model.AccountMember{AccountID: dst.ID, UserID: recipient, DefaultShare: 0.6}))
	require.NoError(t, accounts.AddMember(ctx, model.AccountMember{AccountID: dst.ID, UserID: member, DefaultShare: 0.4}))
	hidden, err := accounts.Create(ctx, model.Account{OwnerID: recipient, Name: "Hidden", Currency: "USD", AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, InitialBalanceDate: time.Now()})
	require.NoError(t, err)
	require.False(t, hidden.AcceptTransfers)
	choices, err := accounts.ListTransferAccounts(ctx, "recipient")
	require.NoError(t, err)
	require.Len(t, choices, 1)
	require.Equal(t, dst.ID, choices[0].ID)
	choices, err = accounts.ListTransferAccounts(ctx, "recip")
	require.NoError(t, err)
	require.Empty(t, choices)
	txRepo := repository.NewTransactionRepository(pool)
	svc := service.NewTransactionService(pool, txRepo, accounts, repository.NewTagRepository(pool))
	req := model.CreateTransactionReq{AccountID: src.ID, ToAccountID: ptr.To(dst.ID), Type: model.TransactionTypeTransfer, Amount: 100, Currency: "USD", ToAmount: ptr.To(90.0), Date: time.Now(), Tags: []string{"private"}}
	// A tag write failure must roll back the transfer and both sides of its shares.
	badReq := req
	badReq.Tags = []string{strings.Repeat("x", 1000)}
	_, err = svc.Create(ctx, sender, badReq)
	require.Error(t, err)
	var partial int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM transactions`).Scan(&partial))
	require.Zero(t, partial)
	created, err := svc.Create(ctx, sender, req)
	require.NoError(t, err)
	incoming, err := svc.GetByID(ctx, recipient, created.ID)
	require.NoError(t, err)
	require.True(t, incoming.ReadOnly)
	require.Empty(t, incoming.Tags)
	require.Empty(t, incoming.Shares)
	require.Equal(t, 54.0, *incoming.RecipientAmount)
	require.Equal(t, "Destination", incoming.ToAccountName)
	listed, err := svc.List(ctx, recipient, model.TransactionFilter{})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.True(t, listed[0].ReadOnly)
	require.Equal(t, 54.0, *listed[0].RecipientAmount)
	require.ErrorIs(t, svc.Delete(ctx, recipient, created.ID), apperr.ErrForbidden)
	_, err = svc.Update(ctx, recipient, created.ID, model.UpdateTransactionReq{Amount: ptr.To(1.0)})
	require.ErrorIs(t, err, apperr.ErrForbidden)
	// Current membership shares do not rewrite historical incoming amounts.
	require.NoError(t, accounts.UpdateMemberShare(ctx, dst.ID, recipient, 0.1))
	require.NoError(t, accounts.UpdateMemberShare(ctx, dst.ID, member, 0.9))
	balances, err := accounts.ListBalancesByUser(ctx, recipient, "EUR")
	require.NoError(t, err)
	require.Equal(t, 54.0, balances[dst.ID].BalanceNative)
	require.Equal(t, 90.0, balances[dst.ID].TotalNative)
	summary, err := repository.NewAnalyticsRepository(pool).Summary(ctx, model.AnalyticsFilter{UserID: recipient, DisplayCurrency: "EUR", DateFrom: time.Now().AddDate(0, 0, -1), DateTo: time.Now().AddDate(0, 0, 1)})
	require.NoError(t, err)
	require.Equal(t, 54.0, summary.Balance)
	dst.AcceptTransfers = false
	_, err = accounts.Update(ctx, dst)
	require.NoError(t, err)
	_, err = svc.Create(ctx, sender, req)
	require.ErrorIs(t, err, apperr.ErrForbidden)
	// Existing transfers remain editable by the source, retaining the old proportions.
	_, err = svc.Update(ctx, sender, created.ID, model.UpdateTransactionReq{Amount: ptr.To(200.0), ToAmount: ptr.To(180.0)})
	require.NoError(t, err)
	balances, err = accounts.ListBalancesByUser(ctx, recipient, "EUR")
	require.NoError(t, err)
	require.Equal(t, 108.0, balances[dst.ID].BalanceNative)
	// Four-decimal personal amounts survive metadata-only edits unchanged.
	hidden.AcceptTransfers = true
	_, err = accounts.Update(ctx, hidden)
	require.NoError(t, err)
	tiny, err := svc.Create(ctx, sender, model.CreateTransactionReq{AccountID: src.ID, ToAccountID: ptr.To(hidden.ID), Type: model.TransactionTypeTransfer, Amount: 0.0001, Currency: "USD", Date: time.Now()})
	require.NoError(t, err)
	_, err = svc.Update(ctx, sender, tiny.ID, model.UpdateTransactionReq{Description: ptr.To("note")})
	require.NoError(t, err)
	tinyView, err := svc.GetByID(ctx, recipient, tiny.ID)
	require.NoError(t, err)
	require.Equal(t, 0.0001, *tinyView.RecipientAmount)
	require.NoError(t, svc.Delete(ctx, sender, tiny.ID))
	require.NoError(t, accounts.SoftDelete(ctx, dst.ID))
	_, err = svc.Create(ctx, sender, req)
	require.ErrorIs(t, err, apperr.ErrNotFound)
	require.NoError(t, svc.Delete(ctx, sender, created.ID))
	var remaining int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM transfer_shares`).Scan(&remaining))
	require.Zero(t, remaining)
}
