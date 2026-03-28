package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service/mocks"
)

type TransactionServiceSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	repo        *mocks.MockTransactionRepo
	accountRepo *mocks.MockAccountRepoForTx
	svc         *TransactionService
}

func (s *TransactionServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = mocks.NewMockTransactionRepo(s.ctrl)
	s.accountRepo = mocks.NewMockAccountRepoForTx(s.ctrl)
	s.svc = &TransactionService{repo: s.repo, accounts: s.accountRepo}
}

func (s *TransactionServiceSuite) TearDownTest() {
	s.ctrl.Finish()
}

func TestTransactionServiceSuite(t *testing.T) {
	suite.Run(t, new(TransactionServiceSuite))
}

// --- Create ---

func (s *TransactionServiceSuite) TestCreate_PersonalAccount_NoShares() {
	ctx := context.Background()
	userID := "user-1"
	req := model.CreateTransactionReq{
		AccountID:        "acc-1",
		Type:             model.TransactionTypeExpense,
		Amount:           100.00,
		Currency:         "RUB",
		Date:             time.Now(),
		IncludeInBalance: true,
	}

	s.accountRepo.EXPECT().IsMember(ctx, req.AccountID, userID).Return(true, nil)
	// single member → no split
	s.repo.EXPECT().GetMemberDefaults(ctx, req.AccountID).Return([]model.AccountMember{
		{UserID: userID, DefaultShare: 1.0},
	}, nil)
	s.repo.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, tx model.Transaction) (model.Transaction, error) {
		s.Nil(tx.Shares, "single-member account should produce no shares")
		s.Equal(userID, tx.CreatedBy)
		tx.ID = "tx-1"
		return tx, nil
	})

	tx, err := s.svc.Create(ctx, userID, req)
	s.NoError(err)
	s.Equal("tx-1", tx.ID)
}

func (s *TransactionServiceSuite) TestCreate_SharedAccount_AutoShares() {
	ctx := context.Background()
	userID := "user-1"
	req := model.CreateTransactionReq{
		AccountID: "acc-shared",
		Type:      model.TransactionTypeExpense,
		Amount:    100.00,
		Currency:  "RUB",
		Date:      time.Now(),
	}

	s.accountRepo.EXPECT().IsMember(ctx, req.AccountID, userID).Return(true, nil)
	s.repo.EXPECT().GetMemberDefaults(ctx, req.AccountID).Return([]model.AccountMember{
		{UserID: "user-1", DefaultShare: 0.5},
		{UserID: "user-2", DefaultShare: 0.5},
	}, nil)
	s.repo.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, tx model.Transaction) (model.Transaction, error) {
		s.Require().Len(tx.Shares, 2)
		s.Equal(50.00, tx.Shares[0].Amount)
		s.Equal(50.00, tx.Shares[1].Amount)
		s.False(tx.Shares[0].IsCustom)
		tx.ID = "tx-2"
		return tx, nil
	})

	tx, err := s.svc.Create(ctx, userID, req)
	s.NoError(err)
	s.Equal("tx-2", tx.ID)
}

func (s *TransactionServiceSuite) TestCreate_CustomShares() {
	ctx := context.Background()
	userID := "user-1"
	req := model.CreateTransactionReq{
		AccountID: "acc-shared",
		Type:      model.TransactionTypeExpense,
		Amount:    100.00,
		Currency:  "RUB",
		Date:      time.Now(),
		Shares: []model.ShareReq{
			{UserID: "user-1", Amount: 70.00},
			{UserID: "user-2", Amount: 30.00},
		},
	}

	s.accountRepo.EXPECT().IsMember(ctx, req.AccountID, userID).Return(true, nil)
	s.repo.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, tx model.Transaction) (model.Transaction, error) {
		s.Require().Len(tx.Shares, 2)
		s.True(tx.Shares[0].IsCustom)
		s.Equal(70.00, tx.Shares[0].Amount)
		tx.ID = "tx-3"
		return tx, nil
	})

	tx, err := s.svc.Create(ctx, userID, req)
	s.NoError(err)
	s.Equal("tx-3", tx.ID)
}

func (s *TransactionServiceSuite) TestCreate_InvalidShares_ReturnsValidationError() {
	ctx := context.Background()
	userID := "user-1"
	req := model.CreateTransactionReq{
		AccountID: "acc-shared",
		Type:      model.TransactionTypeExpense,
		Amount:    100.00,
		Currency:  "RUB",
		Date:      time.Now(),
		Shares: []model.ShareReq{
			{UserID: "user-1", Amount: 60.00},
			{UserID: "user-2", Amount: 30.00}, // sum = 90, not 100
		},
	}

	s.accountRepo.EXPECT().IsMember(ctx, req.AccountID, userID).Return(true, nil)

	_, err := s.svc.Create(ctx, userID, req)
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *TransactionServiceSuite) TestCreate_NotMember_ReturnsForbidden() {
	ctx := context.Background()
	userID := "user-1"
	req := model.CreateTransactionReq{
		AccountID: "acc-1",
		Type:      model.TransactionTypeExpense,
		Amount:    100.00,
		Currency:  "RUB",
		Date:      time.Now(),
	}

	s.accountRepo.EXPECT().IsMember(ctx, req.AccountID, userID).Return(false, nil)

	_, err := s.svc.Create(ctx, userID, req)
	s.True(errors.Is(err, apperr.ErrForbidden))
}

func (s *TransactionServiceSuite) TestCreate_TransferMissingToAccount_ReturnsValidationError() {
	ctx := context.Background()
	req := model.CreateTransactionReq{
		AccountID: "acc-1",
		Type:      model.TransactionTypeTransfer,
		Amount:    100.00,
		Currency:  "RUB",
		Date:      time.Now(),
		// ToAccountID intentionally nil
	}

	_, err := s.svc.Create(ctx, "user-1", req)
	s.True(errors.Is(err, apperr.ErrValidation))
}

// --- GetByID ---

func (s *TransactionServiceSuite) TestGetByID_NotMember_ReturnsForbidden() {
	ctx := context.Background()
	userID := "user-1"
	txID := "tx-99"

	s.repo.EXPECT().GetByID(ctx, txID).Return(model.Transaction{
		ID:        txID,
		AccountID: "acc-1",
	}, nil)
	s.accountRepo.EXPECT().IsMember(ctx, "acc-1", userID).Return(false, nil)

	_, err := s.svc.GetByID(ctx, userID, txID)
	s.True(errors.Is(err, apperr.ErrForbidden))
}

func (s *TransactionServiceSuite) TestGetByID_NotFound_ReturnsNotFound() {
	ctx := context.Background()

	s.repo.EXPECT().GetByID(ctx, "tx-missing").Return(model.Transaction{}, apperr.ErrNotFound)

	_, err := s.svc.GetByID(ctx, "user-1", "tx-missing")
	s.True(errors.Is(err, apperr.ErrNotFound))
}

// --- Update ---

func (s *TransactionServiceSuite) TestUpdate_AmountNegative_ReturnsValidationError() {
	ctx := context.Background()
	userID := "user-1"
	txID := "tx-1"
	neg := -50.00

	s.repo.EXPECT().GetByID(ctx, txID).Return(model.Transaction{
		ID:        txID,
		AccountID: "acc-1",
		Amount:    100.00,
	}, nil)
	s.accountRepo.EXPECT().IsMember(ctx, "acc-1", userID).Return(true, nil)

	_, err := s.svc.Update(ctx, userID, txID, model.UpdateTransactionReq{Amount: &neg})
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *TransactionServiceSuite) TestUpdate_CustomShares_InvalidSum_ReturnsValidationError() {
	ctx := context.Background()
	userID := "user-1"
	txID := "tx-1"

	s.repo.EXPECT().GetByID(ctx, txID).Return(model.Transaction{
		ID:        txID,
		AccountID: "acc-1",
		Amount:    100.00,
	}, nil)
	s.accountRepo.EXPECT().IsMember(ctx, "acc-1", userID).Return(true, nil)

	_, err := s.svc.Update(ctx, userID, txID, model.UpdateTransactionReq{
		Shares: []model.ShareReq{
			{UserID: "u1", Amount: 40.00},
			{UserID: "u2", Amount: 40.00}, // sum = 80, not 100
		},
	})
	s.True(errors.Is(err, apperr.ErrValidation))
}

// --- Delete ---

func (s *TransactionServiceSuite) TestDelete_NotMember_ReturnsForbidden() {
	ctx := context.Background()
	userID := "user-1"
	txID := "tx-1"

	s.repo.EXPECT().GetByID(ctx, txID).Return(model.Transaction{
		ID:        txID,
		AccountID: "acc-1",
	}, nil)
	s.accountRepo.EXPECT().IsMember(ctx, "acc-1", userID).Return(false, nil)

	err := s.svc.Delete(ctx, userID, txID)
	s.True(errors.Is(err, apperr.ErrForbidden))
}

func (s *TransactionServiceSuite) TestDelete_Success() {
	ctx := context.Background()
	userID := "user-1"
	txID := "tx-1"

	s.repo.EXPECT().GetByID(ctx, txID).Return(model.Transaction{
		ID:        txID,
		AccountID: "acc-1",
	}, nil)
	s.accountRepo.EXPECT().IsMember(ctx, "acc-1", userID).Return(true, nil)
	s.repo.EXPECT().Delete(ctx, txID).Return(nil)

	err := s.svc.Delete(ctx, userID, txID)
	s.NoError(err)
}

// --- List ---

func (s *TransactionServiceSuite) TestList_DefaultsApplied() {
	ctx := context.Background()
	userID := "user-1"

	s.repo.EXPECT().List(ctx, userID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, f model.TransactionFilter) ([]model.Transaction, error) {
			s.Equal(50, f.Limit)
			s.Equal(1, f.Page)
			return []model.Transaction{}, nil
		},
	)

	txs, err := s.svc.List(ctx, userID, model.TransactionFilter{})
	s.NoError(err)
	s.Empty(txs)
}

func (s *TransactionServiceSuite) TestList_RepoError_Propagated() {
	ctx := context.Background()
	repoErr := errors.New("db error")

	s.repo.EXPECT().List(ctx, "user-1", gomock.Any()).Return(nil, repoErr)

	_, err := s.svc.List(ctx, "user-1", model.TransactionFilter{})
	s.ErrorIs(err, repoErr)
}
