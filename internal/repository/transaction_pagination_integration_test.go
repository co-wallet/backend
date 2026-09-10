package repository_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

func TestTransactionPaginationWithIdenticalDates(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	var account string
	require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO accounts(owner_id,name,currency,access_mode,kind,initial_balance_date)
		VALUES($1,'Pagination','RUB','personal','spending',CURRENT_DATE) RETURNING id`, f.user).Scan(&account))
	// A bulk import creates many operations with the same date and timestamp.
	_, err := f.pool.Exec(ctx, `INSERT INTO transactions(id,account_id,type,amount,currency,date,created_by,created_at)
		SELECT ('00000000-0000-0000-0000-' || lpad(i::text,12,'0'))::uuid,$1,'expense',1,'RUB',CURRENT_DATE,$2,now()
		FROM generate_series(1,125) AS i`, account, f.user)
	require.NoError(t, err)
	repo := repository.NewTransactionRepository(f.pool)
	var ids []string
	for page := 1; page <= 4; page++ {
		rows, listErr := repo.List(ctx, f.user, model.TransactionFilter{Page: page, Limit: 50})
		require.NoError(t, listErr)
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		if page == 4 {
			require.Empty(t, rows)
		}
	}
	expected := make([]string, 0, 125)
	for i := 125; i >= 1; i-- {
		expected = append(expected, fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
	}
	require.Equal(t, expected, ids, "pages must cover every transaction exactly once in stable order")
}
