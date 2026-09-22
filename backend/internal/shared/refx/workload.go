package refx

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/shared/num"
)

// 班组归属与月报相关表名。
const (
	TableTeamAssignments       = "team_assignments"
	TableTeamWorkloadReports   = "team_workload_reports"
	TableTeamWorkloadSnapshots = "team_workload_snapshots"
)

// TeamWorkload 班组工作量。所有班组统计、任务详情和看板必须共用该结构与查询口径。
type TeamWorkload struct {
	TeamName        string  `json:"teamName"`
	RecordCount     int64   `json:"recordCount"`
	PersonnelCount  int64   `json:"personnelCount"`
	ActualWorkHours float64 `json:"actualWorkHours"`
	LengthM         float64 `json:"lengthM"`
	SludgeVolumeM3  float64 `json:"sludgeVolumeM3"`
	WaterVolumeM3   float64 `json:"waterVolumeM3"`
}

// TaskTeamWorkload 单个任务按班组拆分后的工作量。
type TaskTeamWorkload struct {
	TaskID          uint    `json:"taskId"`
	TeamName        string  `json:"teamName"`
	RecordCount     int64   `json:"recordCount"`
	PersonnelCount  int64   `json:"personnelCount"`
	ActualWorkHours float64 `json:"actualWorkHours"`
	LengthM         float64 `json:"lengthM"`
	SludgeVolumeM3  float64 `json:"sludgeVolumeM3"`
	WaterVolumeM3   float64 `json:"waterVolumeM3"`
}

// MonthRange 返回业务月份 [月初, 次月初)。
func MonthRange(month string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01", month, time.UTC)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return start, start.AddDate(0, 1, 0), nil
}

// AttributedTeamExpr 返回按清淤作业日期解析责任班组的 SQL 表达式。
//
// 每条派工记录在生效日期当天 00:00 起生效；每条清淤记录只会命中一个班组，
// 因此即使一次改派跨越多个作业日期，也不会在两个班组重复统计同一批工作量。
func AttributedTeamExpr(recordAlias string) string {
	return `(
		SELECT a.team_name
		FROM ` + TableTeamAssignments + ` AS a
		WHERE a.task_id = ` + recordAlias + `.task_id
		  AND a.effective_date <= ` + recordAlias + `.cleaned_at
		ORDER BY a.effective_date DESC, a.sequence DESC
		LIMIT 1
	)`
}

// TeamWorkloadQuery 班组工作量查询条件。
type TeamWorkloadQuery struct {
	Month  string
	TaskID uint
}

func teamWorkloadQuery(ctx context.Context, db *gorm.DB, query TeamWorkloadQuery) *gorm.DB {
	tx := db.WithContext(ctx).Table(TableCleaningRecords + " AS r")
	if query.TaskID > 0 {
		tx = tx.Where("r.task_id = ?", query.TaskID)
	}
	if strings.TrimSpace(query.Month) != "" {
		start, end, err := MonthRange(query.Month)
		if err == nil {
			tx = tx.Where("r.cleaned_at >= ? AND r.cleaned_at < ?", start, end)
		}
	}
	teamExpr := "COALESCE(" + AttributedTeamExpr("r") + ", '未指定班组') AS team_name"
	return tx.Select(teamExpr + `,
		COUNT(*) AS record_count,
		COALESCE(SUM(r.personnel_count), 0) AS personnel_count,
		COALESCE(SUM(r.actual_work_hours), 0) AS actual_work_hours,
		COALESCE(SUM(r.length_m), 0) AS length_m,
		COALESCE(SUM(r.sludge_volume_m3), 0) AS sludge_volume_m3,
		COALESCE(SUM(r.water_volume_m3), 0) AS water_volume_m3`).
		Group("team_name").
		Order("team_name ASC")
}

// TeamWorkloads 按实际作业日期归属口径汇总全部班组工作量。
func TeamWorkloads(ctx context.Context, db *gorm.DB, month string) ([]TeamWorkload, error) {
	items := make([]TeamWorkload, 0)
	if _, _, err := MonthRange(month); err != nil {
		return nil, err
	}
	if err := teamWorkloadQuery(ctx, db, TeamWorkloadQuery{Month: month}).Scan(&items).Error; err != nil {
		return nil, err
	}
	roundTeamWorkloads(items)
	return items, nil
}

// TeamWorkloadsForTask 按同一归属口径拆分单个任务的班组工作量。
func TeamWorkloadsForTask(ctx context.Context, db *gorm.DB, taskID uint) ([]TeamWorkload, error) {
	items := make([]TaskTeamWorkload, 0)
	if err := teamWorkloadQuery(ctx, db, TeamWorkloadQuery{TaskID: taskID}).Scan(&items).Error; err != nil {
		return nil, err
	}
	result := make([]TeamWorkload, 0, len(items))
	for _, item := range items {
		result = append(result, TeamWorkload{
			TeamName:        item.TeamName,
			RecordCount:     item.RecordCount,
			PersonnelCount:  item.PersonnelCount,
			ActualWorkHours: item.ActualWorkHours,
			LengthM:         item.LengthM,
			SludgeVolumeM3:  item.SludgeVolumeM3,
			WaterVolumeM3:   item.WaterVolumeM3,
		})
	}
	roundTeamWorkloads(result)
	return result, nil
}

// AttributedTeamNameForRecord 返回某条清淤记录按实际作业日期归属的班组。
func AttributedTeamNameForRecord(ctx context.Context, db *gorm.DB, taskID uint, cleanedAt time.Time) (string, error) {
	var teamName string
	err := db.WithContext(ctx).Table(TableTeamAssignments).
		Select("team_name").
		Where("task_id = ? AND effective_date <= ?", taskID, cleanedAt).
		Order("effective_date DESC, sequence DESC").
		Limit(1).
		Scan(&teamName).Error
	if teamName == "" {
		teamName = "未指定班组"
	}
	return teamName, err
}

// TeamWorkloadsOrSnapshot 已发布月份读取不可变快照，未发布月份按实时归属口径统计。
func TeamWorkloadsOrSnapshot(ctx context.Context, db *gorm.DB, month string) ([]TeamWorkload, bool, error) {
	if _, _, err := MonthRange(month); err != nil {
		return nil, false, err
	}
	var reportID uint
	err := db.WithContext(ctx).Table(TableTeamWorkloadReports).
		Select("id").Where("month = ?", month).Scan(&reportID).Error
	if err != nil {
		return nil, false, err
	}
	if reportID > 0 {
		items := make([]TeamWorkload, 0)
		err = db.WithContext(ctx).Table(TableTeamWorkloadSnapshots).
			Select(`team_name, record_count, personnel_count, actual_work_hours, length_m, sludge_volume_m3, water_volume_m3`).
			Where("report_id = ?", reportID).
			Order("team_name ASC").
			Scan(&items).Error
		if err != nil {
			return nil, false, err
		}
		roundTeamWorkloads(items)
		return items, true, nil
	}
	items, err := TeamWorkloads(ctx, db, month)
	return items, false, err
}

// TaskWorkDate 任务与实际作业日期的组合。
type TaskWorkDate struct {
	TaskID   uint
	WorkDate time.Time
}

type teamAssignmentRow struct {
	TaskID        uint
	TeamName      string
	EffectiveDate time.Time
}

// AttributedTeamNames 批量解析多条清淤记录在各自作业日期生效的班组。
func AttributedTeamNames(ctx context.Context, db *gorm.DB, items []TaskWorkDate) (map[uint]map[time.Time]string, error) {
	result := make(map[uint]map[time.Time]string)
	taskIDSet := make(map[uint]struct{})
	for _, item := range items {
		if item.TaskID == 0 {
			continue
		}
		taskIDSet[item.TaskID] = struct{}{}
		if result[item.TaskID] == nil {
			result[item.TaskID] = make(map[time.Time]string)
		}
	}
	if len(taskIDSet) == 0 {
		return result, nil
	}

	taskIDs := make([]uint, 0, len(taskIDSet))
	for taskID := range taskIDSet {
		taskIDs = append(taskIDs, taskID)
	}
	assignments := make([]teamAssignmentRow, 0)
	err := db.WithContext(ctx).Table(TableTeamAssignments).
		Select("task_id, team_name, effective_date").
		Where("task_id IN ?", taskIDs).
		Order("task_id ASC, effective_date ASC, sequence ASC").
		Scan(&assignments).Error
	if err != nil {
		return nil, err
	}

	byTask := make(map[uint][]teamAssignmentRow)
	for _, assignment := range assignments {
		byTask[assignment.TaskID] = append(byTask[assignment.TaskID], assignment)
	}
	for _, item := range items {
		teamName := "未指定班组"
		for _, assignment := range byTask[item.TaskID] {
			if assignment.EffectiveDate.After(item.WorkDate) {
				break
			}
			teamName = assignment.TeamName
		}
		result[item.TaskID][item.WorkDate] = teamName
	}
	return result, nil
}

func roundTeamWorkloads(items []TeamWorkload) {
	for i := range items {
		items[i].ActualWorkHours = num.Round2(items[i].ActualWorkHours)
		items[i].LengthM = num.Round2(items[i].LengthM)
		items[i].SludgeVolumeM3 = num.Round2(items[i].SludgeVolumeM3)
		items[i].WaterVolumeM3 = num.Round2(items[i].WaterVolumeM3)
	}
}
