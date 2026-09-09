package preview_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/importer/preview"
	"github.com/co-wallet/backend/internal/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestStoreLifecycle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	s, err := preview.New(dir)
	require.NoError(t, err)
	p := model.ImportPreview{ID: uuid.NewString(), UserID: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(preview.TTL)}
	require.NoError(t, s.Save(p))
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	path := filepath.Join(dir, p.UserID+"_"+p.ID+".json")
	info, err = os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	// Reopening the store simulates a backend restart.
	s, err = preview.New(dir)
	require.NoError(t, err)
	got, err := s.Load(p.UserID, p.ID)
	require.NoError(t, err)
	require.Equal(t, p, got)
	_, err = s.Load(uuid.NewString(), p.ID)
	require.ErrorIs(t, err, apperr.ErrNotFound)
	_, err = s.Load(p.UserID, "../../outside")
	require.ErrorIs(t, err, apperr.ErrNotFound)
	require.NoError(t, s.Delete(p.UserID, p.ID))
	require.NoError(t, s.Delete(p.UserID, p.ID))
	_, err = s.Load(p.UserID, p.ID)
	require.ErrorIs(t, err, apperr.ErrNotFound)
}
func TestExpirationAndQuota(t *testing.T) {
	dir := t.TempDir()
	s, err := preview.New(dir)
	require.NoError(t, err)
	p := model.ImportPreview{ID: uuid.NewString(), UserID: uuid.NewString(), ExpiresAt: time.Now().Add(-time.Hour)}
	require.NoError(t, s.Save(p))
	_, err = s.Load(p.UserID, p.ID)
	require.ErrorIs(t, err, apperr.ErrNotFound)
	path := filepath.Join(dir, p.UserID+"_"+p.ID+".json")
	old := time.Now().Add(-25 * time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))
	require.NoError(t, s.Sweep())
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
	p.ExpiresAt = time.Now().Add(preview.TTL)
	for range 5 {
		p.ID = uuid.NewString()
		require.NoError(t, s.Save(p))
	}
	p.ID = uuid.NewString()
	require.ErrorIs(t, s.Save(p), apperr.ErrConflict)
}
