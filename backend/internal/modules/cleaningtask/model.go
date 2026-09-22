// Package cleaningtask 清淤任务登记模块：负责把管段台账上的清淤需求登记成可跟踪的任务。
package cleaningtask

import (
	"time"

	"github.com/drainage/desilting/internal/shared/date"
)

// 任务优先级。
const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

// 任务来源。
const (
	SourcePlan       = "plan"
	SourceInspection = "inspection"
	SourceComplaint  = "complaint"
	SourceFlood      = "flood"
)

// 清淤方式。
const (
	MethodHighPressure = "high_pressure"
	MethodWinch        = "winch"
	MethodGrab         = "grab"
	MethodManual       = "manual"
	MethodRobot        = "robot"
)

// TeamReassignment 班组改派记录。
//
// 一条记录表示某任务在 EffectiveDate 当天 0 点起，由 FromTeamName 移交 ToTeamName 负责。
// 清淤工作量不直接改写在记录上，而是按「作业日期时点生效的改派」动态归属：
// 作业日期早于生效日的工作量归原班组，达到或晚于生效日的归接手班组，
// 从而跨作业日期改派时同一批工作量不会被两个班组各算一遍。
type TeamReassignment struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	TaskID        uint      `gorm:"index;not null" json:"taskId"`
	FromTeamName  string    `gorm:"size:64;not null" json:"fromTeamName"`
	ToTeamName    string    `gorm:"size:64;index;not null" json:"toTeamName"`
	Reason        string    `gorm:"size:255;not null" json:"reason"`
	EffectiveDate date.Date `gorm:"type:date;index;not null" json:"effectiveDate"`
	OperatorName  string    `gorm:"size:32" json:"operatorName"`
	CreatedAt     time.Time `json:"createdAt"`
}

// TableName 指定表名。
func (TeamReassignment) TableName() string {
	return "team_reassignments"
}

// CleaningTask 清淤任务。
type CleaningTask struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Code          string     `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Title         string     `gorm:"size:128;not null" json:"title"`
	PipeSegmentID uint       `gorm:"index;not null" json:"pipeSegmentId"`
	Priority      string     `gorm:"size:16;index;not null;default:normal" json:"priority"`
	Source        string     `gorm:"size:16;index;not null;default:plan" json:"source"`
	Method        string     `gorm:"size:24" json:"method"`
	PlanStartDate date.Date  `gorm:"type:date;index;not null" json:"planStartDate"`
	PlanEndDate   date.Date  `gorm:"type:date;index;not null" json:"planEndDate"`
	TeamName      string     `gorm:"size:64" json:"teamName"`
	LeaderName    string     `gorm:"size:32" json:"leaderName"`
	LeaderPhone   string     `gorm:"size:32" json:"leaderPhone"`
	Status        string     `gorm:"size:16;index;not null;default:pending" json:"status"`
	Description   string     `gorm:"type:text" json:"description"`
	StartedAt     *time.Time `json:"startedAt"`
	FinishedAt    *time.Time `json:"finishedAt"`
	AcceptedAt    *time.Time `json:"acceptedAt"`
	CancelReason  string     `gorm:"size:255" json:"cancelReason"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// TableName 指定表名。
func (CleaningTask) TableName() string {
	return "cleaning_tasks"
}
