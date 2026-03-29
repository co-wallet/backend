package accounthandler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateAccountReq_Validate(t *testing.T) {
	validReq := func() createAccountReq {
		return createAccountReq{
			Name:               "My Card",
			Type:               "personal",
			Currency:           "USD",
			InitialBalance:     0,
			InitialBalanceDate: "2024-01-15",
		}
	}

	tests := []struct {
		name    string
		modify  func(*createAccountReq)
		wantErr string
	}{
		{
			name:    "valid",
			modify:  func(_ *createAccountReq) {},
			wantErr: "",
		},
		{
			name:    "name trimmed to empty",
			modify:  func(r *createAccountReq) { r.Name = "   " },
			wantErr: "name is required",
		},
		{
			name:    "empty name",
			modify:  func(r *createAccountReq) { r.Name = "" },
			wantErr: "name is required",
		},
		{
			name:    "currency too short",
			modify:  func(r *createAccountReq) { r.Currency = "US" },
			wantErr: "currency must be a 3-letter ISO code",
		},
		{
			name:    "currency too long",
			modify:  func(r *createAccountReq) { r.Currency = "USDT" },
			wantErr: "currency must be a 3-letter ISO code",
		},
		{
			name:    "currency lowercased is normalized",
			modify:  func(r *createAccountReq) { r.Currency = "usd" },
			wantErr: "",
		},
		{
			name:    "invalid type",
			modify:  func(r *createAccountReq) { r.Type = "family" },
			wantErr: "type must be 'personal' or 'shared'",
		},
		{
			name:    "type shared is valid",
			modify:  func(r *createAccountReq) { r.Type = "shared" },
			wantErr: "",
		},
		{
			name:    "negative initial balance is valid",
			modify:  func(r *createAccountReq) { r.InitialBalance = -500 },
			wantErr: "",
		},
		{
			name:    "zero initial balance is valid",
			modify:  func(r *createAccountReq) { r.InitialBalance = 0 },
			wantErr: "",
		},
		{
			name:    "positive initial balance is valid",
			modify:  func(r *createAccountReq) { r.InitialBalance = 1000.50 },
			wantErr: "",
		},
		{
			name:    "missing initial balance date",
			modify:  func(r *createAccountReq) { r.InitialBalanceDate = "" },
			wantErr: "initialBalanceDate is required",
		},
		{
			name:    "invalid date format",
			modify:  func(r *createAccountReq) { r.InitialBalanceDate = "15-01-2024" },
			wantErr: "initialBalanceDate must be YYYY-MM-DD",
		},
		{
			name:    "invalid date value",
			modify:  func(r *createAccountReq) { r.InitialBalanceDate = "2024-13-01" },
			wantErr: "initialBalanceDate must be YYYY-MM-DD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validReq()
			tt.modify(&req)
			err := req.validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}
		})
	}
}

func TestUpdateAccountReq_Validate(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name    string
		req     updateAccountReq
		wantErr string
	}{
		{
			name:    "all nil fields valid",
			req:     updateAccountReq{},
			wantErr: "",
		},
		{
			name:    "valid name",
			req:     updateAccountReq{Name: strPtr("New Name")},
			wantErr: "",
		},
		{
			name:    "name trimmed to empty",
			req:     updateAccountReq{Name: strPtr("   ")},
			wantErr: "name cannot be empty",
		},
		{
			name:    "empty name string",
			req:     updateAccountReq{Name: strPtr("")},
			wantErr: "name cannot be empty",
		},
		{
			name:    "valid date",
			req:     updateAccountReq{InitialBalanceDate: strPtr("2024-06-01")},
			wantErr: "",
		},
		{
			name:    "invalid date format",
			req:     updateAccountReq{InitialBalanceDate: strPtr("01/06/2024")},
			wantErr: "initialBalanceDate must be YYYY-MM-DD",
		},
		{
			name:    "invalid date value",
			req:     updateAccountReq{InitialBalanceDate: strPtr("2024-00-01")},
			wantErr: "initialBalanceDate must be YYYY-MM-DD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}
		})
	}
}

func TestAddMemberReq_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     addMemberReq
		wantErr string
	}{
		{
			name:    "valid",
			req:     addMemberReq{Username: "alice", DefaultShare: 0.5},
			wantErr: "",
		},
		{
			name:    "empty username",
			req:     addMemberReq{Username: "", DefaultShare: 0.5},
			wantErr: "username is required",
		},
		{
			name:    "whitespace username",
			req:     addMemberReq{Username: "  ", DefaultShare: 0.5},
			wantErr: "username is required",
		},
		{
			name:    "share below 0",
			req:     addMemberReq{Username: "alice", DefaultShare: -0.1},
			wantErr: "defaultShare must be between 0 and 1",
		},
		{
			name:    "share above 1",
			req:     addMemberReq{Username: "alice", DefaultShare: 1.1},
			wantErr: "defaultShare must be between 0 and 1",
		},
		{
			name:    "share = 0 valid",
			req:     addMemberReq{Username: "alice", DefaultShare: 0},
			wantErr: "",
		},
		{
			name:    "share = 1 valid",
			req:     addMemberReq{Username: "alice", DefaultShare: 1},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}
		})
	}
}

func TestUpdateMemberReq_Validate(t *testing.T) {
	tests := []struct {
		name    string
		share   float64
		wantErr string
	}{
		{"valid 0.5", 0.5, ""},
		{"valid 0", 0, ""},
		{"valid 1", 1, ""},
		{"negative", -0.01, "defaultShare must be between 0 and 1"},
		{"above 1", 1.01, "defaultShare must be between 0 and 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := updateMemberReq{DefaultShare: tt.share}
			err := req.validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}
		})
	}
}
