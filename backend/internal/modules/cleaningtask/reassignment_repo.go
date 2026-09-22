package cleaningtask

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrTeamChanged 改派时任务当前班组已与操作人看到的不一致（并发改派）。
var ErrTeamChanged = errors.New("任务班组已被其他操作变更，请刷新后重试")

// ReassignmentRepository 班组改派记录数据访问。
type ReassignmentRepository struct {
	db *gorm.DB
}

// NewReassignmentRepository 构造改派记录仓储。
func NewReassignmentRepository(db *gorm.DB) *ReassignmentRepository {
	return &ReassignmentRepository{db: db}
}

// DB 暴露底层连接。
func (r *ReassignmentRepository) DB() *gorm.DB {
	return r.db
}

// Transaction 在事务内执行改派写入与任务班组更新，保证两者同时成功或失败。
func (r *ReassignmentRepository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// CreateInTx 在给定事务中写入一条改派记录。
func (r *ReassignmentRepository) CreateInTx(ctx context.Context, tx *gorm.DB, item *TeamReassignment) error {
	return tx.WithContext(ctx).Create(item).Error
}

// ChangeTeamInTx 在给定事务中按「当前班组快照」条件更新任务班组。
//
// WHERE 条件带上 team_name = fromTeamName：两个人同时改派同一条任务时，
// 先提交的一方更新成功，后提交的一方因当前班组已变化导致 RowsAffected=0，
// 从而只有一次改派生效。
func (r *ReassignmentRepository) ChangeTeamInTx(ctx context.Context, tx *gorm.DB, id uint, fromTeamName, toTeamName string) error {
	result := tx.WithContext(ctx).Model(&CleaningTask{}).
		Where("id = ? AND team_name = ?", id, fromTeamName).
		Updates(map[string]any{
			"team_name":  toTeamName,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTeamChanged
	}
	return nil
}

// ListByTask 查询某任务的改派记录，按生效先后排序。
func (r *ReassignmentRepository) ListByTask(ctx context.Context, taskID uint) ([]TeamReassignment, error) {
	items := make([]TeamReassignment, 0)
	err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("effective_date ASC, id ASC").
		Find(&items).Error
	return items, err
}

// ListByTaskIDs 批量查询多个任务的改派记录，供任务列表/详情批量补齐。
func (r *ReassignmentRepository) ListByTaskIDs(ctx context.Context, taskIDs []uint) (map[uint][]TeamReassignment, error) {
	result := make(map[uint][]TeamReassignment, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	items := make([]TeamReassignment, 0)
	err := r.db.WithContext(ctx).
		Where("task_id IN ?", taskIDs).
		Order("effective_date ASC, id ASC").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	for i := range items {
		result[items[i].TaskID] = append(result[items[i].TaskID], items[i])
	}
	return result, nil
}

// LatestByTask 查询某任务最近一次生效的改派，没有时返回 nil。
func (r *ReassignmentRepository) LatestByTask(ctx context.Context, taskID uint) (*TeamReassignment, error) {
	var item TeamReassignment
	err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("effective_date DESC, id DESC").
		Limit(1).
		Find(&item).Error
	if err != nil {
		return nil, err
	}
	if item.ID == 0 {
		return nil, nil
	}
	return &item, nil
}
