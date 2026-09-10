package service

import (
	"context"
	"testing"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service/mocks"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ImportSharesSuite struct{ ImportSuite }

func TestImportSharesSuite(t *testing.T) { suite.Run(t, new(ImportSharesSuite)) }
func (s *ImportSharesSuite) TestConfigureSharedValidationAndRounding() {
	for _, tc := range []struct {
		name     string
		members  []model.CreateAccountMemberReq
		invalid  bool
		inactive bool
	}{
		{"equal", []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: .5}, {Username: "other", DefaultShare: .5}}, false, false},
		{"unequal rounding", []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: .3333}, {Username: "other", DefaultShare: .6667}}, false, false},
		{"duplicate", []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: .5}, {Username: "owner", DefaultShare: .5}}, true, false},
		{"missing owner", []model.CreateAccountMemberReq{{Username: "other", DefaultShare: 1}}, true, false},
		{"wrong sum", []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: .5}, {Username: "other", DefaultShare: .4}}, true, false},
		{"too precise", []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: .00001}, {Username: "other", DefaultShare: .99999}}, true, false},
		{"negative", []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: -.5}}, true, false},
		{"unknown", []model.CreateAccountMemberReq{{Username: "missing", DefaultShare: 1}}, true, false},
		{"inactive", []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: .5}, {Username: "other", DefaultShare: .5}}, true, true},
		{"empty", nil, true, false},
	} {
		s.Run(tc.name, func() {
			c := gomock.NewController(s.T())
			repo, store := mocks.NewMockimportRepo(c), mocks.NewMockimportStore(c)
			svc := &ImportService{repo: repo, store: store}
			p := model.ImportPreview{ID: s.id, UserID: s.user, Report: monefy.Report{Accounts: []monefy.Account{{ID: "a", InitialBalance: 1000}}, Transactions: []monefy.Transaction{{ID: "t", AccountID: "a", Type: "expense", Amount: -1}}}}
			store.EXPECT().Load(s.user, s.id).Return(p, nil)
			repo.EXPECT().Catalog(gomock.Any(), false).Return(nil, nil)
			repo.EXPECT().AccountNames(gomock.Any(), s.user, false).Return(nil, nil)
			repo.EXPECT().GetByUsername(gomock.Any(), "owner").Return(model.User{ID: s.user, Username: "owner", IsActive: true}, nil).AnyTimes()
			repo.EXPECT().GetByUsername(gomock.Any(), "other").Return(model.User{ID: "other-id", Username: "other", IsActive: !tc.inactive}, nil).AnyTimes()
			repo.EXPECT().GetByUsername(gomock.Any(), "missing").Return(model.User{}, apperr.ErrNotFound).AnyTimes()
			store.EXPECT().Save(gomock.Any()).Return(nil)
			got, err := svc.Configure(context.Background(), s.user, s.id, map[string]model.AccountKind{"a": "deposit"}, nil, map[string]string{"a": "preset:cash|red|none"}, map[string]model.ImportAccountAccess{"a": {AccessMode: "shared", Members: tc.members}})
			s.Require().NoError(err)
			s.Equal(!tc.invalid, got.Report.CanImport())
			s.Equal(model.AccountKindDeposit, got.Accounts[0].Kind)
			s.Equal("preset:cash|red|none", got.Accounts[0].Icon)
			s.Empty(p.AccountAccess)
			if tc.invalid {
				s.Empty(got.TransactionShares)
				return
			}
			s.Len(got.Accounts[0].Members, 2)
			amounts := map[string]string{}
			for _, share := range got.TransactionShares["t"] {
				amounts[share.UserID] = share.Amount
			}
			if tc.name == "equal" {
				s.Equal("0.0005", amounts[s.user])
				s.Equal("0.0005", amounts["other-id"])
			} else {
				s.Equal("0.0003", amounts[s.user])
				s.Equal("0.0007", amounts["other-id"])
			}
		})
	}
}
func (s *ImportSharesSuite) TestConfirmationRejectsUnavailableParticipantBeforeWrite() {
	p := model.ImportPreview{Categories: []model.ImportCategory{}, Accounts: []model.ImportAccount{{AccessMode: "shared", Members: []model.ImportMember{{UserID: "other", Username: "other", DefaultShare: 1}}}}}
	s.confirmStart(p)
	s.repo.EXPECT().Catalog(gomock.Any(), true).Return(nil, nil)
	s.repo.EXPECT().Currencies(gomock.Any()).Return(nil, nil)
	s.repo.EXPECT().GetByUsername(gomock.Any(), "other").Return(model.User{ID: "other", IsActive: false}, nil)
	_, err := s.svc.Confirm(context.Background(), s.user, s.id, true, false)
	s.ErrorContains(err, "members_changed")
}
