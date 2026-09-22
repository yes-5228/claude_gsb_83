package cleaningtask

import (
	"time"

	"github.com/drainage/desilting/internal/shared/date"
)

// 班组归属变更类型。
const (
	AssignmentInitial  = "initial"
	AssignmentReassign = "reassign"
)

// InitialAssignmentDate 是任务初始派工的归属生效日期。
var InitialAssignmentDate = date.MustParse("1900-01-01")

// TeamAssignment 班组派工 / 改派记录，同时作为工作量归属的时间边界。
type TeamAssignment struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	TaskID        uint      `gorm:"index:idx_team_assignment_task_seq,unique,priority:1;not null" json:"taskId"`
	Sequence      int       `gorm:"index:idx_team_assignment_task_seq,unique,priority:2;not null" json:"sequence"`
	TeamName      string    `gorm:"size:64;index;not null" json:"teamName"`
	ChangeType    string    `gorm:"size:16;index;not null;default:initial" json:"changeType"`
	EffectiveDate date.Date `gorm:"type:date;index;not null" json:"effectiveDate"`
	Reason        string    `gorm:"size:255" json:"reason"`
	OperatorName  string    `gorm:"size:32" json:"operatorName"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// TableName 指定表名。
func (TeamAssignment) TableName() string {
	return "team_assignments"
}

// TeamWorkloadReport 班组月度工作量月报主表。月报发布后为不可变快照。
type TeamWorkloadReport struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Month       string    `gorm:"size:7;uniqueIndex;not null" json:"month"`
	PublishedAt time.Time `json:"publishedAt"`
	PublishedBy string    `gorm:"size:32" json:"publishedBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// TableName 指定表名。
func (TeamWorkloadReport) TableName() string {
	return "team_workload_reports"
}

// TeamWorkloadSnapshot 班组月报中的工作量明细快照。
type TeamWorkloadSnapshot struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	ReportID        uint      `gorm:"index:idx_team_workload_snapshot,unique,priority:1;not null" json:"reportId"`
	Month           string    `gorm:"size:7;index:idx_team_workload_snapshot,unique,priority:2;not null" json:"month"`
	TeamName        string    `gorm:"size:64;index:idx_team_workload_snapshot,unique,priority:3;not null" json:"teamName"`
	RecordCount     int64     `json:"recordCount"`
	PersonnelCount  int64     `json:"personnelCount"`
	ActualWorkHours float64   `json:"actualWorkHours"`
	LengthM         float64   `json:"lengthM"`
	SludgeVolumeM3  float64   `json:"sludgeVolumeM3"`
	WaterVolumeM3   float64   `json:"waterVolumeM3"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// TableName 指定表名。
func (TeamWorkloadSnapshot) TableName() string {
	return "team_workload_snapshots"
}
