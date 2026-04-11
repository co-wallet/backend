package taghandler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenameTagReq_Validate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  string
		wantName string
	}{
		{name: "valid", input: "groceries", wantName: "groceries"},
		{name: "trimmed", input: "  travel  ", wantName: "travel"},
		{name: "empty", input: "", wantErr: "name is required"},
		{name: "whitespace only", input: "   ", wantErr: "name is required"},
		{name: "too long", input: strings.Repeat("a", maxTagNameLen+1), wantErr: "name must be at most 64 characters"},
		{name: "at limit", input: strings.Repeat("a", maxTagNameLen), wantName: strings.Repeat("a", maxTagNameLen)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := renameTagReq{Name: tt.input}
			err := req.validate()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantName, req.Name)
		})
	}
}
