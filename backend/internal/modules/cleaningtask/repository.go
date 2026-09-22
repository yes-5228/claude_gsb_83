package cleaningtask

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/shared/refx"
)

// ErrNotFound 任务不存在。
var ErrNotFound = errors.New("清淤任务不存在")

// ErrStateConflict 并发操作导致状态已变化。
var ErrStateConflict = errors.New("任务状态已变化，请刷新后重试")

// ErrAssignmentNotFound 班组派工记录不存在。
var ErrAssignmentNotFound = errors.New("班组派工记录不存在")

// ErrSameTeam 接手班组与原班组相同。
var ErrSameTeam = errors.New("接手班组不能与原班组相同")

// Repository 清淤任务数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 构造仓储。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// DB 暴露底层连接，供 service 做反向引用检查。
func (r *Repository) DB() *gorm.DB {
	return r.db
}

// Create 新增任务。
func (r *Repository) Create(ctx context.Context, task *CleaningTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// CreateWithInitialAssignment 新增任务并写入初始派工，保证派工历史与任务同生同灭。
func (r *Repository) CreateWithInitialAssignment(ctx context.Context, task *CleaningTask) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		teamName := strings.TrimSpace(task.TeamName)
		if teamName == "" {
			teamName = "未指定班组"
		}
		assignment := &TeamAssignment{
			TaskID:        task.ID,
			Sequence:      1,
			TeamName:      teamName,
			ChangeType:    AssignmentInitial,
			EffectiveDate: InitialAssignmentDate,
			Reason:        "任务派工",
		}
		return tx.Create(assignment).Error
	})
}

// LatestAssignment 查询任务当前班组归属。
func (r *Repository) LatestAssignment(ctx context.Context, taskID uint) (*TeamAssignment, error) {
	var assignment TeamAssignment
	err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("effective_date DESC, sequence DESC").
		First(&assignment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAssignmentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &assignment, nil
}

// ListAssignments 查询任务的派工 / 改派历史。
func (r *Repository) ListAssignments(ctx context.Context, taskID uint) ([]TeamAssignment, error) {
	items := make([]TeamAssignment, 0)
	err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("sequence ASC").
		Find(&items).Error
	return items, err
}

// ReassignTeam 在事务中追加改派记录并更新任务当前班组。
func (r *Repository) ReassignTeam(ctx context.Context, taskID uint, current, target string, req ReassignRequest) (*CleaningTask, *TeamAssignment, error) {
	var resultTask *CleaningTask
	var resultAssignment *TeamAssignment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task CleaningTask
		if err := lockForUpdate(tx).First(&task, taskID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		var latest TeamAssignment
		if err := lockForUpdate(tx).
			Where("task_id = ?", taskID).
			Order("sequence DESC").
			First(&latest).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAssignmentNotFound
			}
			return err
		}
		if latest.TeamName != current {
			return ErrStateConflict
		}
		if latest.TeamName == target {
			return ErrSameTeam
		}

		assignment := &TeamAssignment{
			TaskID:        taskID,
			Sequence:      latest.Sequence + 1,
			TeamName:      target,
			ChangeType:    AssignmentReassign,
			EffectiveDate: date.Today(),
			Reason:        strings.TrimSpace(req.Reason),
			OperatorName:  strings.TrimSpace(req.OperatorName),
		}
		if err := tx.Create(assignment).Error; err != nil {
			return err
		}
		updateResult := tx.Model(&CleaningTask{}).
			Where("id = ? AND team_name = ?", taskID, current).
			Updates(map[string]any{"team_name": target, "updated_at": time.Now()})
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			return ErrStateConflict
		}
		if err := tx.First(&task, taskID).Error; err != nil {
			return err
		}
		resultTask = &task
		resultAssignment = assignment
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return resultTask, resultAssignment, nil
}

// DeleteWithAssignments 删除任务时同步删除派工历史。
func (r *Repository) DeleteWithAssignments(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ?", id).Delete(&CleaningTask{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return tx.Where("task_id = ?", id).Delete(&TeamAssignment{}).Error
	})
}

// Save 保存任务全部字段。
func (r *Repository) Save(ctx context.Context, task *CleaningTask) error {
	return r.db.WithContext(ctx).Save(task).Error
}

// Delete 物理删除任务。
func (r *Repository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&CleaningTask{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// FindByID 按主键查询任务。
func (r *Repository) FindByID(ctx context.Context, id uint) (*CleaningTask, error) {
	var task CleaningTask
	err := r.db.WithContext(ctx).First(&task, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// MaxCodeWithPrefix 查询某个前缀下已使用的最大任务编号，用于生成流水号。
func (r *Repository) MaxCodeWithPrefix(ctx context.Context, prefix string) (string, error) {
	var code string
	err := r.db.WithContext(ctx).Model(&CleaningTask{}).
		Where("code LIKE ?", prefix+"-%").
		Order("code DESC").
		Limit(1).
		Pluck("code", &code).Error
	return code, err
}

// Transition 在指定前置状态下更新任务状态，避免并发下出现非法流转。
func (r *Repository) Transition(ctx context.Context, id uint, from, to string, extra map[string]any) error {
	return r.TransitionTx(ctx, nil, id, from, to, extra)
}

// TransitionTx 在给定事务中执行状态流转，供验收模块与验收记录写入保持原子性。
//
// tx 可以为 nil，此时直接使用仓储自身的连接。
func (r *Repository) TransitionTx(ctx context.Context, tx *gorm.DB, id uint, from, to string, extra map[string]any) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	updates := map[string]any{
		"status":     to,
		"updated_at": time.Now(),
	}
	for key, value := range extra {
		updates[key] = value
	}
	result := db.WithContext(ctx).Model(&CleaningTask{}).
		Where("id = ? AND status = ?", id, from).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrStateConflict
	}
	return nil
}

// List 分页查询任务。
func (r *Repository) List(ctx context.Context, query ListQuery) ([]CleaningTask, int64, error) {
	query.Page.Normalize()
	var total int64
	if err := r.filtered(ctx, query).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	tasks := make([]CleaningTask, 0)
	err := r.filtered(ctx, query).
		Order("plan_start_date DESC, id DESC").
		Offset(query.Page.Offset()).
		Limit(query.Page.PageSize).
		Find(&tasks).Error
	if err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func (r *Repository) filtered(ctx context.Context, query ListQuery) *gorm.DB {
	tx := r.db.WithContext(ctx).Model(&CleaningTask{})
	if keyword := strings.ToLower(strings.TrimSpace(query.Keyword)); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where(
			"LOWER(code) LIKE ? OR LOWER(title) LIKE ? OR LOWER(team_name) LIKE ? OR LOWER(leader_name) LIKE ?",
			like, like, like, like,
		)
	}
	if query.Status != "" {
		tx = tx.Where("status = ?", query.Status)
	}
	if query.Priority != "" {
		tx = tx.Where("priority = ?", query.Priority)
	}
	if query.Source != "" {
		tx = tx.Where("source = ?", query.Source)
	}
	if query.PipeSegmentID > 0 {
		tx = tx.Where("pipe_segment_id = ?", query.PipeSegmentID)
	}
	if query.District != "" {
		subQuery := r.db.WithContext(ctx).Table(refx.TablePipeSegments).
			Select("id").
			Where("district = ?", query.District)
		tx = tx.Where("pipe_segment_id IN (?)", subQuery)
	}
	if query.PlanFrom != nil {
		tx = tx.Where("plan_start_date >= ?", query.PlanFrom.Time)
	}
	if query.PlanTo != nil {
		tx = tx.Where("plan_start_date <= ?", query.PlanTo.Time)
	}
	return tx
}

// HasRecords 任务下是否已经有清淤记录。
func (r *Repository) HasRecords(ctx context.Context, taskID uint) (bool, error) {
	return refx.HasRecordsForTask(ctx, r.db, taskID)
}

// HasAcceptance 任务下是否已经有验收记录。
func (r *Repository) HasAcceptance(ctx context.Context, taskID uint) (bool, error) {
	return refx.HasAcceptanceForTask(ctx, r.db, taskID)
}

// CountByStatus 统计各状态任务数量。
func (r *Repository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Status string
		Total  int64
	}
	rows := make([]row, 0)
	err := r.db.WithContext(ctx).Model(&CleaningTask{}).
		Select("status, COUNT(*) AS total").
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, item := range rows {
		result[item.Status] = item.Total
	}
	return result, nil
}

// lockForUpdate 在支持行锁的数据库上加锁；SQLite 依赖写事务串行化。
func lockForUpdate(tx *gorm.DB) *gorm.DB {
	if tx.Dialector != nil && tx.Dialector.Name() == "sqlite" {
		return tx
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}
