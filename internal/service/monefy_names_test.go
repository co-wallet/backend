package service

import (
	"context"
	"strings"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/co-wallet/backend/internal/model"
	"go.uber.org/mock/gomock"
)

func (s *ImportSuite) TestConfigureUniqueAccountNames() {
	for _, tc := range []struct {
		name                   string
		existing, source, want []string
		mode                   model.ImportMode
	}{
		{name: "no conflict", source: []string{"Cash"}, want: []string{"Cash"}},
		{name: "case and spaces", existing: []string{" CASH ", "Cash (1)"}, source: []string{"Cash"}, want: []string{"Cash (2)"}},
		{name: "reserve source names", existing: []string{"Cash"}, source: []string{"Cash", "Cash (1)", "Cash"}, want: []string{"Cash (2)", "Cash (1)", "Cash (3)"}},
		{name: "unicode limit", existing: []string{strings.Repeat("я", 100)}, source: []string{strings.Repeat("я", 100)}, want: []string{strings.Repeat("я", 96) + " (1)"}},
		{name: "replacement ignores old names", mode: model.ImportReplace, existing: []string{"Cash"}, source: []string{"Cash", "Cash"}, want: []string{"Cash", "Cash (1)"}},
	} {
		s.Run(tc.name, func() {
			s.names = tc.existing
			p := model.ImportPreview{ID: s.id, UserID: s.user, Mode: tc.mode}
			kinds := map[string]model.AccountKind{}
			for i, name := range tc.source {
				id := string(rune('a' + i))
				p.Report.Accounts = append(p.Report.Accounts, monefy.Account{ID: id, Name: name})
				kinds[id] = model.AccountKindSpending
			}
			s.store.EXPECT().Load(s.user, s.id).Return(p, nil)
			s.repo.EXPECT().Catalog(gomock.Any(), false).Return(nil, nil)
			if tc.mode == model.ImportReplace {
				s.repo.EXPECT().Replacement(gomock.Any(), s.user).Return(model.ImportReplacement{}, nil)
			}
			s.store.EXPECT().Save(gomock.Any()).Return(nil)
			got, err := s.svc.Configure(context.Background(), s.user, s.id, kinds, nil, nil, nil)
			s.Require().NoError(err)
			for i, name := range tc.want {
				s.Equal(name, got.Accounts[i].Name)
				s.Equal(tc.source[i], got.Report.Accounts[i].Name)
				s.Equal(tc.source[i], p.Report.Accounts[i].Name)
			}
		})
	}
}

func (s *ImportSuite) TestConfirmRejectsChangedAccountNamesBeforeWrite() {
	s.names = []string{"Cash", "Cash (1)"}
	s.confirmStart(model.ImportPreview{UserID: s.user, Report: monefy.Report{Accounts: []monefy.Account{{ID: "a", Name: "Cash"}}}, Accounts: []model.ImportAccount{{SourceID: "a", Name: "Cash (1)"}}})
	_, err := s.svc.Confirm(context.Background(), s.user, s.id, false, false)
	s.ErrorContains(err, "account_names_changed")
	s.ErrorIs(err, apperr.ErrConflict)
}

func (s *ImportSuite) TestConfigureAccountNamesReadFailure() {
	s.namesErr = apperr.ErrConflict
	s.store.EXPECT().Load(s.user, s.id).Return(model.ImportPreview{}, nil)
	s.repo.EXPECT().Catalog(gomock.Any(), false).Return(nil, nil)
	_, err := s.svc.Configure(context.Background(), s.user, s.id, nil, nil, nil, nil)
	s.ErrorIs(err, apperr.ErrConflict)
}

func (s *ImportSuite) TestAvailabilityAllowsExistingData() {
	got, err := s.svc.Availability(context.Background(), s.user)
	s.Require().NoError(err)
	s.Empty(got.Reasons)
	_, err = s.svc.Availability(context.Background(), "invalid")
	s.ErrorIs(err, apperr.ErrUnauthorized)
}
