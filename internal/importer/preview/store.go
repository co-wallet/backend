// Package preview stores private, expiring import snapshots on a single server.
package preview

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/google/uuid"
)

const TTL = 24 * time.Hour
const MaxSnapshotBytes = 128 << 20
const maxTotalBytes = 512 << 20

type Store struct {
	dir string
	mu  sync.Mutex
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(user, id string) (string, error) {
	if _, err := uuid.Parse(user); err != nil {
		return "", apperr.ErrUnauthorized
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", apperr.ErrNotFound
	}
	// Canonical UUIDs only: never allow path components from the request.
	if uuid.MustParse(user).String() != user || uuid.MustParse(id).String() != id {
		return "", apperr.ErrNotFound
	}
	return filepath.Join(s.dir, user+"_"+id+".json"), nil
}

func (s *Store) Save(p model.ImportPreview) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(p.UserID, p.ID)
	if err != nil {
		return err
	}
	if err = s.sweep(time.Now()); err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(data) > MaxSnapshotBytes {
		return apperr.ErrValidation
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	var size int64
	count := 0
	for _, entry := range entries {
		info, e := entry.Info()
		if e != nil {
			return e
		}
		size += info.Size()
		if strings.HasPrefix(entry.Name(), p.UserID+"_") {
			count++
		}
	}
	if count >= 5 || size+int64(len(data)) > maxTotalBytes {
		return apperr.ErrConflict
	}
	f, err := os.CreateTemp(s.dir, ".pending-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }() // Временный файл уже переименован при успешной записи.
	_, writeErr := f.Write(data)
	err = errors.Join(writeErr, f.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	// Reconfigured previews retain the source expiration rather than extending TTL.
	created := p.ExpiresAt.Add(-TTL)
	return os.Chtimes(path, created, created)
}

func (s *Store) Load(user, id string) (model.ImportPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var p model.ImportPreview
	path, err := s.path(user, id)
	if err != nil {
		return p, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, apperr.ErrNotFound
	}
	if err != nil {
		return p, err
	}
	err = json.NewDecoder(io.LimitReader(f, MaxSnapshotBytes+1)).Decode(&p)
	err = errors.Join(err, f.Close())
	if err != nil {
		return model.ImportPreview{}, err
	}
	if p.ID != id || p.UserID != user {
		return model.ImportPreview{}, apperr.ErrNotFound
	}
	if !time.Now().Before(p.ExpiresAt) {
		return model.ImportPreview{}, apperr.ErrNotFound
	}
	return p, nil
}

func (s *Store) Delete(user, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(user, id)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) Sweep() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sweep(time.Now())
}

func (s *Store) sweep(now time.Time) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		if now.Sub(info.ModTime()) >= TTL || strings.HasPrefix(entry.Name(), ".pending-") {
			if e = os.Remove(filepath.Join(s.dir, entry.Name())); e != nil {
				return e
			}
		}
	}
	return nil
}
