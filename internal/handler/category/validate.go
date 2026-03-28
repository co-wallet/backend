package categoryhandler

import (
	"errors"
	"strings"

	"github.com/co-wallet/backend/internal/model"
)

type createCategoryReq struct {
	ParentID *string             `json:"parentId"`
	Name     string              `json:"name"`
	Type     model.CategoryType  `json:"type"`
	Icon     *string             `json:"icon"`
}

func (r *createCategoryReq) validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("name is required")
	}
	if !r.Type.IsValid() {
		return errors.New("type must be 'expense' or 'income'")
	}
	return nil
}

func (r *createCategoryReq) toModelReq() model.CreateCategoryReq {
	return model.CreateCategoryReq{
		ParentID: r.ParentID,
		Name:     strings.TrimSpace(r.Name),
		Type:     r.Type,
		Icon:     r.Icon,
	}
}

type updateCategoryReq struct {
	Name *string `json:"name"`
	Icon *string `json:"icon"`
}

func (r *updateCategoryReq) validate() error {
	if r.Name != nil && strings.TrimSpace(*r.Name) == "" {
		return errors.New("name cannot be empty")
	}
	return nil
}

func (r *updateCategoryReq) toModelReq() model.UpdateCategoryReq {
	req := model.UpdateCategoryReq{Icon: r.Icon}
	if r.Name != nil {
		name := strings.TrimSpace(*r.Name)
		req.Name = &name
	}
	return req
}
