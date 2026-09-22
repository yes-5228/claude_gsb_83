// Package team 班组工作量统计与月报模块。
//
// 工作量归属口径统一收敛在 shared/refx：按每条清淤记录的实际作业日期归属到
// 当时生效的班组。班组工作量统计、任务详情、看板共用同一套查询，保证口径一致。
//
// 月报是按月封账的快照：某月一旦出具月报，改派就不能再改变该月的工作量归属，
// 跨月改派因此无法改写已经出具的月报。
package team

import "time"

// WorkloadReport 班组工作量月报（按月封账）。
type WorkloadReport struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Month       string    `gorm:"size:7;uniqueIndex;not null" json:"month"` // YYYY-MM
	Published   bool      `gorm:"not null;default:false" json:"published"`
	Remark      string    `gorm:"size:255" json:"remark"`
	PublishedBy string    `gorm:"size:32" json:"publishedBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// TableName 指定表名。
func (WorkloadReport) TableName() string {
	return "team_workload_reports"
}
