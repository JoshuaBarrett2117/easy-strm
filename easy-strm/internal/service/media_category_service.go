package service

import (
	"fmt"
	"strings"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

// MediaCategoryService 媒体分类服务。
// 负责媒体分类规则的归一化与持久化编排，避免 Controller 直接访问 DAO。
type MediaCategoryService struct {
	mediaCategoryDAO *dao.MediaCategoryDAO
}

// NewMediaCategoryService 创建媒体分类服务实例。
func NewMediaCategoryService(mediaCategoryDAO *dao.MediaCategoryDAO) *MediaCategoryService {
	return &MediaCategoryService{mediaCategoryDAO: mediaCategoryDAO}
}

// GetAll 获取全部媒体分类规则。
func (s *MediaCategoryService) GetAll() ([]*domain.MediaCategory, error) {
	return s.mediaCategoryDAO.GetAll()
}

// Create 创建媒体分类规则，并在入库前归一化匹配规则。
func (s *MediaCategoryService) Create(category *domain.MediaCategory) error {
	if err := validateMediaCategory(category); err != nil {
		return err
	}
	category.NormalizeMatchRules()
	return s.mediaCategoryDAO.Create(category)
}

// Update 更新媒体分类规则，并在入库前归一化匹配规则。
func (s *MediaCategoryService) Update(category *domain.MediaCategory) error {
	if err := validateMediaCategory(category); err != nil {
		return err
	}
	category.NormalizeMatchRules()
	return s.mediaCategoryDAO.Update(category)
}

func validateMediaCategory(category *domain.MediaCategory) error {
	if category == nil {
		return fmt.Errorf("媒体分类不能为空")
	}
	if strings.TrimSpace(category.Name) == "" || strings.TrimSpace(category.TargetPath) == "" {
		return fmt.Errorf("媒体分类名称和目标路径不能为空")
	}
	if category.MediaType != "movie" && category.MediaType != "tv" {
		return fmt.Errorf("媒体类型必须为 movie 或 tv")
	}
	if len(category.MatchRules) == 0 || string(category.MatchRules) == "null" {
		return fmt.Errorf("匹配规则不能为空")
	}
	if category.ID < 0 {
		return fmt.Errorf("媒体分类 ID 无效")
	}
	return nil
}

// Delete 删除媒体分类规则。
func (s *MediaCategoryService) Delete(id int) error {
	return s.mediaCategoryDAO.Delete(id)
}
