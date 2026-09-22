package team

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrNotFound 月报不存在。
var ErrNotFound = errors.New("班组工作量月报不存在")

// Repository 月报数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 构造仓储。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// DB 暴露底层连接。
func (r *Repository) DB() *gorm.DB {
	return r.db
}

// FindByMonth 查询某月月报，不存在时返回 nil。
func (r *Repository) FindByMonth(ctx context.Context, month string) (*WorkloadReport, error) {
	var report WorkloadReport
	err := r.db.WithContext(ctx).Where("month = ?", month).First(&report).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// List 查询全部月报，按月份倒序。
func (r *Repository) List(ctx context.Context) ([]WorkloadReport, error) {
	items := make([]WorkloadReport, 0)
	err := r.db.WithContext(ctx).Order("month DESC").Find(&items).Error
	return items, err
}

// PublishedMonths 返回所有已封账月份的列表（升序）。
func (r *Repository) PublishedMonths(ctx context.Context) ([]string, error) {
	months := make([]string, 0)
	err := r.db.WithContext(ctx).Model(&WorkloadReport{}).
		Where("published = ?", true).
		Order("month ASC").
		Pluck("month", &months).Error
	return months, err
}

// Upsert 新建或更新某月月报记录。
func (r *Repository) Upsert(ctx context.Context, report *WorkloadReport) error {
	existing, err := r.FindByMonth(ctx, report.Month)
	if err != nil {
		return err
	}
	now := time.Now()
	if existing == nil {
		report.CreatedAt = now
		report.UpdatedAt = now
		return r.db.WithContext(ctx).Create(report).Error
	}
	existing.Published = report.Published
	existing.Remark = report.Remark
	existing.PublishedBy = report.PublishedBy
	existing.UpdatedAt = now
	return r.db.WithContext(ctx).Save(existing).Error
}
