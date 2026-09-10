package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/co-wallet/backend/internal/model"
)

// Allocate names without changing the source report or any existing account.
// Reserve source names first so a generated suffix cannot steal another source's name.
func importAccountNames(ctx context.Context, repo importRepo, p model.ImportPreview, lock bool) (map[string]string, error) {
	existing, err := repo.AccountNames(ctx, p.UserID, lock)
	if err != nil {
		return nil, err
	}
	// Remove one occurrence per deleted account; equally named preserved accounts
	// must still reserve their names. The manifest includes archived accounts too.
	if p.Mode == model.ImportReplace && p.Replacement != nil {
		deletedNames := map[string]int{}
		for _, account := range p.Replacement.Accounts {
			deletedNames[account.Name]++
		}
		preserved := make([]string, 0, len(existing))
		for _, name := range existing {
			if deletedNames[name] > 0 {
				deletedNames[name]--
			} else {
				preserved = append(preserved, name)
			}
		}
		existing = preserved
	}
	key := func(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
	used, reserved := map[string]bool{}, map[string]bool{}
	for _, name := range existing {
		used[key(name)] = true
	}
	for _, a := range p.Report.Accounts {
		reserved[key(a.Name)] = true
	}
	names := make(map[string]string, len(p.Report.Accounts))
	nextSuffix := map[string]int{}
	for _, a := range p.Report.Accounts {
		name := strings.TrimSpace(a.Name)
		base := []rune(name)
		if used[key(name)] {
			for n := max(1, nextSuffix[key(a.Name)]); ; n++ {
				suffix := fmt.Sprintf(" (%d)", n)
				limit := min(len(base), 100-len(suffix))
				name = string(base[:limit]) + suffix
				if reserved[key(name)] {
					continue
				}
				if !used[key(name)] {
					nextSuffix[key(a.Name)] = n + 1
					break
				}
			}
		}
		used[key(name)] = true
		names[a.ID] = name
	}
	return names, nil
}
