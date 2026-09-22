package cleaningtask

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/shared/refx"
)

// FindWorkloadReport 查询已发布的月报主表。
func (r *Repository) FindWorkloadReport(ctx context.Context, month string) (*TeamWorkloadReport, error) {
	var report TeamWorkloadReport
	err := r.db.WithContext(ctx).Where("month = ?", month).First(&report).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// ListWorkloadSnapshots 查询月报快照明细。
func (r *Repository) ListWorkloadSnapshots(ctx context.Context, reportID uint) ([]refx.TeamWorkload, error) {
	type row struct {
		TeamName        string
		RecordCount     int64
		PersonnelCount  int64
		ActualWorkHours float64
		LengthM         float64
		SludgeVolumeM3  float64
		WaterVolumeM3   float64
	}
	rows := make([]row, 0)
	err := r.db.WithContext(ctx).Model(&TeamWorkloadSnapshot{}).
		Select("team_name, record_count, personnel_count, actual_work_hours, length_m, sludge_volume_m3, water_volume_m3").
		Where("report_id = ?", reportID).
		Order("team_name ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]refx.TeamWorkload, 0, len(rows))
	for _, item := range rows {
		items = append(items, refx.TeamWorkload{
			TeamName:        item.TeamName,
			RecordCount:     item.RecordCount,
			PersonnelCount:  item.PersonnelCount,
			ActualWorkHours: item.ActualWorkHours,
			LengthM:         item.LengthM,
			SludgeVolumeM3:  item.SludgeVolumeM3,
			WaterVolumeM3:   item.WaterVolumeM3,
		})
	}
	return items, nil
}

// PublishedReportMonthsBetween 查询区间内已经发布月报的月份，用于阻止改派改写历史月报。
func (r *Repository) PublishedReportMonthsBetween(ctx context.Context, fromMonth, toMonth string) ([]string, error) {
	months := make([]string, 0)
	err := r.db.WithContext(ctx).Model(&TeamWorkloadReport{}).
		Where("month >= ? AND month <= ?", fromMonth, toMonth).
		Order("month ASC").
		Pluck("month", &months).Error
	return months, err
}

// HasRecordForTaskSince 判断任务在指定日期及以后是否已有清淤记录。
func (r *Repository) HasRecordForTaskSince(ctx context.Context, taskID uint, since time.Time) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table(refx.TableCleaningRecords).
		Where("task_id = ? AND cleaned_at >= ?", taskID, since).
		Count(&count).Error
	return count > 0, err
}

// CreateWorkloadReport 发布月报：把实时统计固化为不可变快照。
func (r *Repository) CreateWorkloadReport(ctx context.Context, month string, items []refx.TeamWorkload, publishedBy string) (*TeamWorkloadReport, error) {
	now := time.Now()
	report := &TeamWorkloadReport{Month: month, PublishedAt: now, PublishedBy: publishedBy}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(report).Error; err != nil {
			return err
		}
		snapshots := make([]TeamWorkloadSnapshot, 0, len(items))
		for _, item := range items {
			snapshots = append(snapshots, TeamWorkloadSnapshot{
				ReportID:        report.ID,
				Month:           month,
				TeamName:        item.TeamName,
				RecordCount:     item.RecordCount,
				PersonnelCount:  item.PersonnelCount,
				ActualWorkHours: item.ActualWorkHours,
				LengthM:         item.LengthM,
				SludgeVolumeM3:  item.SludgeVolumeM3,
				WaterVolumeM3:   item.WaterVolumeM3,
			})
		}
		if len(snapshots) > 0 {
			if err := tx.Create(&snapshots).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}
