package analytics

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
)

func TestSummaryResponse_JSON(t *testing.T) {
	s := model.AnalyticsSummary{Balance: 100.5, Expenses: 50.25, Income: 200.75}
	raw, err := json.Marshal(toSummaryResponse(s))
	assert.NoError(t, err)

	var got map[string]any
	assert.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, 100.5, got["balance"])
	assert.Equal(t, 50.25, got["expenses"])
	assert.Equal(t, 200.75, got["income"])
}

func TestCategoryStatResponses_JSON(t *testing.T) {
	stats := []model.CategoryStat{
		{CategoryID: "c-1", CategoryName: "Food", Icon: ptr.To("🍔"), Amount: 42.0},
		{CategoryID: "c-2", CategoryName: "Other", Amount: 10.0},
	}
	raw, err := json.Marshal(toCategoryStatResponses(stats))
	assert.NoError(t, err)

	var got []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &got))
	assert.Len(t, got, 2)
	assert.Equal(t, "c-1", got[0]["categoryId"])
	assert.Equal(t, "Food", got[0]["categoryName"])
	assert.Equal(t, "🍔", got[0]["icon"])
	assert.Equal(t, 42.0, got[0]["amount"])
	assert.NotContains(t, got[1], "icon", "nil icon must be omitted")
}

func TestTagStatResponses_JSON(t *testing.T) {
	stats := []model.TagStat{{TagID: "t-1", TagName: "travel", Amount: 300.0}}
	raw, err := json.Marshal(toTagStatResponses(stats))
	assert.NoError(t, err)

	var got []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &got))
	assert.Len(t, got, 1)
	assert.Equal(t, "t-1", got[0]["tagId"])
	assert.Equal(t, "travel", got[0]["tagName"])
	assert.Equal(t, 300.0, got[0]["amount"])
}
