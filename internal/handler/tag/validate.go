package taghandler

import (
	"fmt"
	"strings"
)

const maxTagNameLen = 64

type renameTagReq struct {
	Name string `json:"name"`
}

func (r *renameTagReq) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(r.Name) > maxTagNameLen {
		return fmt.Errorf("name must be at most %d characters", maxTagNameLen)
	}
	return nil
}
