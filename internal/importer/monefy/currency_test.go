package monefy_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransactionHistoricalBaseAmount(t *testing.T) {
	for _, tt := range []struct{ name, sql, amount string }{
		{"inverse latest revision", "", "3000.000"},
		{"direct preferred", `INSERT INTO CurrencyRate VALUES('direct',949,643,2000000,638396640000000000,638396640000000000,NULL)`, "1999.998"},
		{"future not used", `DELETE FROM CurrencyRate WHERE Id NOT IN ('future','removed')`, ""},
		{"conflict not guessed", `UPDATE CurrencyRate SET CreatedOn=638399232000000000 WHERE Id='revision-1'`, ""},
		{"missing base", `UPDATE Currency SET IsBase=0`, ""},
		{"ambiguous base", `UPDATE Currency SET IsBase=1`, ""},
		{"expense stays positive", `UPDATE "Transaction" SET CategoryId='food' WHERE Id='income'`, "3000.000"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := parse(t, tt.sql)
			tx := r.Transactions[2]
			if tt.amount == "" {
				require.Nil(t, tx.DefaultCurrencyAmount)
			} else {
				require.NotNil(t, tx.DefaultCurrencyAmount)
				require.Equal(t, "RUB", tx.DefaultCurrency)
				require.Equal(t, tt.amount, tx.DefaultCurrencyAmount.String())
				require.Equal(t, "historical", tx.BaseAmountSource)
			}
		})
	}
}
