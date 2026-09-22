package refx

import (
	"context"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/shared/num"
)

// 班组改派与月报相关的表名，与各模块 model 的 TableName() 保持一致。
const (
	TableTeamReassignments   = "team_reassignments"
	TableTeamWorkloadReports = "team_workload_reports"
)

// TeamWorkload 单个班组在某个时间范围内的工作量。
//
// 口径唯一来源：按每条清淤记录的实际作业日期（cleaned_at）归属到
// 「该作业日期时点上生效的班组」。班组工作量统计、任务详情、看板都通过本文件的
// 查询构造得到，保证三处口径一致，且跨作业日期改派时同一批工作量不会重复计算。
type TeamWorkload struct {
	TeamName       string  `json:"teamName"`
	RecordCount    int64   `json:"recordCount"`
	PersonDays     int64   `json:"personDays"`
	SludgeVolumeM3 float64 `json:"sludgeVolumeM3"`
	CleanedLengthM float64 `json:"cleanedLengthM"`
}

// attributedTeamExpr 返回「某条清淤记录按作业日期归属的班组」的 SQL 表达式。
//
// recordAlias 是清淤记录表在外层查询中的别名。归属规则：
//   - 取生效日不晚于作业日期的最近一次改派的接手班组；
//   - 作业日期早于所有改派（改派前的历史工作量）时，取最早一次改派记录的原班组；
//   - 任务从未改派过时，取任务当前班组。
//
// 因此无论任务改派过几次、改派是否跨越作业日期，每条记录恰好归属到一个班组。
func attributedTeamExpr(recordAlias string) string {
	return `COALESCE(
		(SELECT h.to_team_name FROM ` + TableTeamReassignments + ` h
			WHERE h.task_id = ` + recordAlias + `.task_id AND h.effective_date <= ` + recordAlias + `.cleaned_at
			ORDER BY h.effective_date DESC, h.id DESC LIMIT 1),
		(SELECT h.from_team_name FROM ` + TableTeamReassignments + ` h
			WHERE h.task_id = ` + recordAlias + `.task_id
			ORDER BY h.effective_date ASC, h.id ASC LIMIT 1),
		t.team_name)`
}

// attributedRecordsSelect 返回「带归属班组的清淤记录」派生表，供各统计查询复用。
func attributedRecordsSelect() string {
	return `SELECT r.id AS record_id, r.task_id AS task_id, r.cleaned_at AS cleaned_at,
			r.length_m AS length_m, r.sludge_volume_m3 AS sludge_volume_m3,
			r.personnel_count AS personnel_count,
			` + attributedTeamExpr("r") + ` AS team_name
		FROM ` + TableCleaningRecords + ` r
		INNER JOIN ` + TableCleaningTasks + ` t ON t.id = r.task_id`
}

func roundWorkload(items []TeamWorkload) []TeamWorkload {
	for i := range items {
		items[i].SludgeVolumeM3 = num.Round2(items[i].SludgeVolumeM3)
		items[i].CleanedLengthM = num.Round2(items[i].CleanedLengthM)
	}
	return items
}

// TeamWorkloadBetween 按归属班组汇总给定作业日期区间内的工作量（含首尾日期）。
// from / to 为 nil 时分别表示不限制下界 / 上界。未指定班组的工作量不计入（team_name 为空）。
func TeamWorkloadBetween(ctx context.Context, db *gorm.DB, from, to *date.Date) ([]TeamWorkload, error) {
	tx := db.WithContext(ctx).Table("(" + attributedRecordsSelect() + ") AS w").
		Select(`w.team_name,
			COUNT(*) AS record_count,
			COALESCE(SUM(w.personnel_count), 0) AS person_days,
			COALESCE(SUM(w.sludge_volume_m3), 0) AS sludge_volume_m3,
			COALESCE(SUM(w.length_m), 0) AS cleaned_length_m`).
		Where("w.team_name <> ''")
	if from != nil {
		tx = tx.Where("w.cleaned_at >= ?", from.Time)
	}
	if to != nil {
		tx = tx.Where("w.cleaned_at <= ?", to.Time)
	}
	items := make([]TeamWorkload, 0)
	err := tx.Group("w.team_name").
		Order("sludge_volume_m3 DESC, w.team_name ASC").
		Scan(&items).Error
	if err != nil {
		return nil, err
	}
	return roundWorkload(items), nil
}

// TaskTeamWorkload 某个任务下按归属班组拆分的工作量。
type TaskTeamWorkload struct {
	TeamWorkload
	TaskID uint `json:"taskId"`
}

// TeamWorkloadByTaskIDs 批量查询多个任务各自按班组拆分的工作量，避免任务详情 N+1 查询。
// 返回值以任务 ID 为键；没有清淤记录的任务不在 map 中。
func TeamWorkloadByTaskIDs(ctx context.Context, db *gorm.DB, taskIDs []uint) (map[uint][]TeamWorkload, error) {
	result := make(map[uint][]TeamWorkload, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	rows := make([]TaskTeamWorkload, 0)
	err := db.WithContext(ctx).Table("("+attributedRecordsSelect()+") AS w").
		Select(`w.task_id,
			w.team_name,
			COUNT(*) AS record_count,
			COALESCE(SUM(w.personnel_count), 0) AS person_days,
			COALESCE(SUM(w.sludge_volume_m3), 0) AS sludge_volume_m3,
			COALESCE(SUM(w.length_m), 0) AS cleaned_length_m`).
		Where("w.task_id IN ?", taskIDs).
		Where("w.team_name <> ''").
		Group("w.task_id, w.team_name").
		Order("w.task_id ASC, sludge_volume_m3 DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].SludgeVolumeM3 = num.Round2(rows[i].SludgeVolumeM3)
		rows[i].CleanedLengthM = num.Round2(rows[i].CleanedLengthM)
		result[rows[i].TaskID] = append(result[rows[i].TaskID], rows[i].TeamWorkload)
	}
	return result, nil
}

// RecordAttributedTeamExpr 暴露按作业日期归属班组的 SQL 表达式，
// 供看板等直接 JOIN 清淤记录的查询复用，确保与统计口径完全一致。
// recordAlias 为清淤记录表别名，且查询中需以 t 作为清淤任务表别名。
func RecordAttributedTeamExpr(recordAlias string) string {
	return attributedTeamExpr(recordAlias)
}
