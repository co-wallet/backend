package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/co-wallet/backend/internal/model"
)

func TestCalculateShares(t *testing.T) {
	cases := []struct {
		name        string
		total       float64
		members     []model.AccountMember
		wantAmounts []float64
		wantErr     bool
	}{
		{
			name:  "equal split two members",
			total: 100.00,
			members: []model.AccountMember{
				{UserID: "u1", DefaultShare: 0.5},
				{UserID: "u2", DefaultShare: 0.5},
			},
			wantAmounts: []float64{50.00, 50.00},
		},
		{
			name:  "unequal split, rounding absorbed by last member",
			total: 100.00,
			members: []model.AccountMember{
				{UserID: "u1", DefaultShare: 1.0 / 3},
				{UserID: "u2", DefaultShare: 1.0 / 3},
				{UserID: "u3", DefaultShare: 1.0 / 3},
			},
			// 33.33 + 33.33 + 33.34 = 100.00
			wantAmounts: []float64{33.33, 33.33, 33.34},
		},
		{
			name:  "70/30 split",
			total: 99.99,
			members: []model.AccountMember{
				{UserID: "u1", DefaultShare: 0.7},
				{UserID: "u2", DefaultShare: 0.3},
			},
			// 69.99 + 30.00 = 99.99
			wantAmounts: []float64{69.99, 30.00},
		},
		{
			name:    "no members returns error",
			total:   100.00,
			members: []model.AccountMember{},
			wantErr: true,
		},
		{
			name:  "single member gets full amount",
			total: 50.50,
			members: []model.AccountMember{
				{UserID: "u1", DefaultShare: 1.0},
			},
			wantAmounts: []float64{50.50},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shares, err := calculateShares(tc.total, tc.members)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, shares, len(tc.wantAmounts))

			var sum float64
			for i, s := range shares {
				assert.Equal(t, tc.members[i].UserID, s.UserID)
				assert.Equal(t, tc.wantAmounts[i], s.Amount)
				assert.False(t, s.IsCustom)
				sum += s.Amount
			}
			assert.InDelta(t, tc.total, sum, 0.001, "shares must sum to total")
		})
	}
}

func TestValidateCustomShares(t *testing.T) {
	cases := []struct {
		name    string
		total   float64
		shares  []model.ShareReq
		wantErr bool
	}{
		{
			name:  "exact sum",
			total: 100.00,
			shares: []model.ShareReq{
				{UserID: "u1", Amount: 60.00},
				{UserID: "u2", Amount: 40.00},
			},
		},
		{
			name:  "within tolerance (0.005 diff)",
			total: 100.00,
			shares: []model.ShareReq{
				{UserID: "u1", Amount: 99.995},
			},
		},
		{
			name:  "sum exceeds tolerance",
			total: 100.00,
			shares: []model.ShareReq{
				{UserID: "u1", Amount: 60.00},
				{UserID: "u2", Amount: 41.00},
			},
			wantErr: true,
		},
		{
			name:  "sum below tolerance",
			total: 100.00,
			shares: []model.ShareReq{
				{UserID: "u1", Amount: 98.98},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCustomShares(tc.total, tc.shares)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
