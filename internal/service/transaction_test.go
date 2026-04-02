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
	tagRepo     *mocks.MockTagRepo
	svc         *TransactionService
}

func (s *TransactionServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = mocks.NewMockTransactionRepo(s.ctrl)
	s.accountRepo = mocks.NewMockAccountRepoForTx(s.ctrl)
	s.tagRepo = mocks.NewMockTagRepo(s.ctrl)
	s.svc = &TransactionService{repo: s.repo, accounts: s.accountRepo, tags: s.tagRepo}
}

func (s *TransactionServiceSuite) TearDownTest() {
	s.ctrl.Finish()
}

func TestTransactionServiceSuite(t *testing.T) {
	suite.Run(t, new(TransactionServiceSuite))
}

// --- Create ---

func (s *TransactionServiceSuite) TestCreate_PersonalAccount_SingleShare() {
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
	// single member → one share equal to full amount (required for analytics JOIN on transaction_shares)
	s.repo.EXPECT().GetMemberDefaults(ctx, req.AccountID).Return([]model.AccountMember{
		{UserID: userID, DefaultShare: 1.0},
	}, nil)
	s.repo.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, tx model.Transaction) (model.Transaction, error) {
		s.Require().Len(tx.Shares, 1, "single-member account should produce one share for full amount")
		s.Equal(userID, tx.Shares[0].UserID)
		s.Equal(100.0, tx.Shares[0].Amount)
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

func (s *TransactionServiceSuite) TestGetByID_Success() {
	ctx := context.Background()
	userID := "user-1"
	txID := "tx-1"

	s.repo.EXPECT().GetByID(ctx, txID).Return(model.Transaction{ID: txID, AccountID: "acc-1"}, nil)
	s.accountRepo.EXPECT().IsMember(ctx, "acc-1", userID).Return(true, nil)
	s.tagRepo.EXPECT().ListForTransaction(ctx, txID).Return(nil, nil)

	tx, err := s.svc.GetByID(ctx, userID, txID)
	s.NoError(err)
	s.Equal(txID, tx.ID)
}

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

func (s *TransactionServiceSuite) TestUpdate_Success() {
	ctx := context.Background()
	userID := "user-1"
	txID := "tx-1"
	newAmount := 200.00

	s.repo.EXPECT().GetByID(ctx, txID).Return(model.Transaction{
		ID: txID, AccountID: "acc-1", Amount: 100.00, CreatedBy: userID,
	}, nil)
	s.accountRepo.EXPECT().IsMember(ctx, "acc-1", userID).Return(true, nil)
	// Amount changed without explicit shares → recalcShares calls GetMemberDefaults
	s.repo.EXPECT().GetMemberDefaults(ctx, "acc-1").Return(nil, nil)
	s.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, tx model.Transaction) (model.Transaction, error) {
		s.Equal(200.00, tx.Amount)
		s.Require().Len(tx.Shares, 1)
		s.Equal(userID, tx.Shares[0].UserID)
		s.Equal(200.00, tx.Shares[0].Amount)
		return tx, nil
	})
	s.tagRepo.EXPECT().ListForTransaction(ctx, txID).Return(nil, nil)

	tx, err := s.svc.Update(ctx, userID, txID, model.UpdateTransactionReq{Amount: &newAmount})
	s.NoError(err)
	s.Equal(200.00, tx.Amount)
}

func (s *TransactionServiceSuite) TestUpdate_AmountChanged_SharedAccount_RecalcShares() {
	ctx := context.Background()
	userID := "user-1"
	txID := "tx-1"
	newAmount := 300.00

	s.repo.EXPECT().GetByID(ctx, txID).Return(model.Transaction{
		ID: txID, AccountID: "acc-1", Amount: 100.00, CreatedBy: userID,
		Shares: []model.TransactionShare{
			{UserID: "user-1", Amount: 50.00},
			{UserID: "user-2", Amount: 50.00},
		},
	}, nil)
	s.accountRepo.EXPECT().IsMember(ctx, "acc-1", userID).Return(true, nil)
	s.repo.EXPECT().GetMemberDefaults(ctx, "acc-1").Return([]model.AccountMember{
		{UserID: "user-1", DefaultShare: 0.5},
		{UserID: "user-2", DefaultShare: 0.5},
	}, nil)
	s.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, tx model.Transaction) (model.Transaction, error) {
		s.Equal(300.00, tx.Amount)
		s.Require().Len(tx.Shares, 2)
		s.Equal(150.00, tx.Shares[0].Amount)
		s.Equal(150.00, tx.Shares[1].Amount)
		return tx, nil
	})
	s.tagRepo.EXPECT().ListForTransaction(ctx, txID).Return(nil, nil)

	tx, err := s.svc.Update(ctx, userID, txID, model.UpdateTransactionReq{Amount: &newAmount})
	s.NoError(err)
	s.Equal(300.00, tx.Amount)
}

func (s *TransactionServiceSuite) TestUpdate_AmountChanged_CustomShares_ScalesProportionally() {
	ctx := context.Background()
	userID := "user-1"
	txID := "tx-1"
	newAmount := 200.00

	s.repo.EXPECT().GetByID(ctx, txID).Return(model.Transaction{
		ID: txID, AccountID: "acc-1", Amount: 100.00, CreatedBy: userID,
		Shares: []model.TransactionShare{
			{UserID: "user-1", Amount: 70.00, IsCustom: true},
			{UserID: "user-2", Amount: 30.00, IsCustom: true},
		},
	}, nil)
	s.accountRepo.EXPECT().IsMember(ctx, "acc-1", userID).Return(true, nil)
	// Custom shares: no GetMemberDefaults call, scales proportionally
	s.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, tx model.Transaction) (model.Transaction, error) {
		s.Equal(200.00, tx.Amount)
		s.Require().Len(tx.Shares, 2)
		s.Equal(140.00, tx.Shares[0].Amount)
		s.Equal(60.00, tx.Shares[1].Amount)
		return tx, nil
	})
	s.tagRepo.EXPECT().ListForTransaction(ctx, txID).Return(nil, nil)

	tx, err := s.svc.Update(ctx, userID, txID, model.UpdateTransactionReq{Amount: &newAmount})
	s.NoError(err)
	s.Equal(200.00, tx.Amount)
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

func (s *TransactionServiceSuite) TestList_LoadsTags() {
	ctx := context.Background()
	userID := "user-1"

	s.repo.EXPECT().List(ctx, userID, gomock.Any()).Return([]model.Transaction{
		{ID: "tx-1", AccountID: "acc-1"},
		{ID: "tx-2", AccountID: "acc-1"},
	}, nil)
	s.tagRepo.EXPECT().ListForTransaction(ctx, "tx-1").Return([]model.Tag{{ID: "t1", Name: "food"}}, nil)
	s.tagRepo.EXPECT().ListForTransaction(ctx, "tx-2").Return(nil, nil)

	txs, err := s.svc.List(ctx, userID, model.TransactionFilter{})
	s.NoError(err)
	s.Len(txs, 2)
	s.Len(txs[0].Tags, 1)
	s.Equal("food", txs[0].Tags[0].Name)
}

func (s *TransactionServiceSuite) TestList_RepoError_Propagated() {
	ctx := context.Background()
	repoErr := errors.New("db error")

	s.repo.EXPECT().List(ctx, "user-1", gomock.Any()).Return(nil, repoErr)

	_, err := s.svc.List(ctx, "user-1", model.TransactionFilter{})
	s.ErrorIs(err, repoErr)
}

func (s *TransactionServiceSuite) TestCreate_DefaultCurrencyAmount_Stored() {
	ctx := context.Background()
	userID := "user-1"
	defCur := "RUB"
	defAmt := 9000.00
	req := model.CreateTransactionReq{
		AccountID:             "acc-1",
		Type:                  model.TransactionTypeExpense,
		Amount:                100.00,
		Currency:              "USD",
		Date:                  time.Now(),
		IncludeInBalance:      true,
		DefaultCurrency:       &defCur,
		DefaultCurrencyAmount: &defAmt,
	}

	s.accountRepo.EXPECT().IsMember(ctx, req.AccountID, userID).Return(true, nil)
	s.repo.EXPECT().GetMemberDefaults(ctx, req.AccountID).Return([]model.AccountMember{
		{UserID: userID, DefaultShare: 1.0},
	}, nil)
	s.repo.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, tx model.Transaction) (model.Transaction, error) {
		s.Require().NotNil(tx.DefaultCurrency)
		s.Equal("RUB", *tx.DefaultCurrency)
		s.Require().NotNil(tx.DefaultCurrencyAmount)
		s.Equal(9000.00, *tx.DefaultCurrencyAmount)
		tx.ID = "tx-dc"
		return tx, nil
	})

	tx, err := s.svc.Create(ctx, userID, req)
	s.NoError(err)
	s.Equal("tx-dc", tx.ID)
}

func (s *TransactionServiceSuite) TestUpdate_DefaultCurrencyAmount_Updated() {
	ctx := context.Background()
	userID := "user-1"
	txID := "tx-1"
	defCur := "RUB"
	oldAmt := 8000.00
	newAmt := 9500.00

	s.repo.EXPECT().GetByID(ctx, txID).Return(model.Transaction{
		ID:                    txID,
		AccountID:             "acc-1",
		Amount:                100.00,
		Currency:              "USD",
		DefaultCurrency:       &defCur,
		DefaultCurrencyAmount: &oldAmt,
	}, nil)
	s.accountRepo.EXPECT().IsMember(ctx, "acc-1", userID).Return(true, nil)
	s.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, tx model.Transaction) (model.Transaction, error) {
		s.Require().NotNil(tx.DefaultCurrencyAmount)
		s.Equal(9500.00, *tx.DefaultCurrencyAmount)
		return tx, nil
	})
	s.tagRepo.EXPECT().ListForTransaction(ctx, txID).Return(nil, nil)

	_, err := s.svc.Update(ctx, userID, txID, model.UpdateTransactionReq{DefaultCurrencyAmount: &newAmt})
	s.NoError(err)
}
