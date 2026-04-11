package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// fakeTx stubs pgx.Tx. Only Commit/Rollback are called by WithTx — any other method
// would panic because the embedded interface is nil, which is the desired signal
// that WithTx is reaching into internals it shouldn't.
type fakeTx struct {
	pgx.Tx
	commitErr   error
	rollbackErr error
	committed   bool
	rolledBack  bool
}

func (f *fakeTx) Commit(context.Context) error {
	f.committed = true
	return f.commitErr
}

func (f *fakeTx) Rollback(context.Context) error {
	f.rolledBack = true
	return f.rollbackErr
}

type fakeBeginner struct {
	tx     *fakeTx
	err    error
	called int
}

func (b *fakeBeginner) Begin(context.Context) (pgx.Tx, error) {
	b.called++
	if b.err != nil {
		return nil, b.err
	}
	return b.tx, nil
}

func TestWithTx_Commit(t *testing.T) {
	tx := &fakeTx{}
	b := &fakeBeginner{tx: tx}

	err := WithTx(context.Background(), b, func(pgx.Tx) error {
		return nil
	})

	require.NoError(t, err)
	require.True(t, tx.committed, "expected commit")
	require.False(t, tx.rolledBack, "expected no rollback")
}

func TestWithTx_RollbackOnError(t *testing.T) {
	tx := &fakeTx{}
	b := &fakeBeginner{tx: tx}
	boom := errors.New("boom")

	err := WithTx(context.Background(), b, func(pgx.Tx) error {
		return boom
	})

	require.ErrorIs(t, err, boom)
	require.False(t, tx.committed)
	require.True(t, tx.rolledBack)
}

func TestWithTx_RollbackOnPanic(t *testing.T) {
	tx := &fakeTx{}
	b := &fakeBeginner{tx: tx}

	require.PanicsWithValue(t, "boom", func() {
		_ = WithTx(context.Background(), b, func(pgx.Tx) error {
			panic("boom")
		})
	})
	require.True(t, tx.rolledBack)
	require.False(t, tx.committed)
}

func TestWithTx_BeginError(t *testing.T) {
	boom := errors.New("no conn")
	b := &fakeBeginner{err: boom}

	err := WithTx(context.Background(), b, func(pgx.Tx) error {
		t.Fatal("fn must not be called when Begin fails")
		return nil
	})

	require.ErrorIs(t, err, boom)
	require.Equal(t, 1, b.called)
}
