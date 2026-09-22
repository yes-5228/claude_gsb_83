package cleaningtask

import "github.com/drainage/desilting/internal/shared/refx"

// ReassignRequest 改派班组请求体。
type ReassignRequest struct {
	CurrentTeamName string `json:"currentTeamName" label:"原班组" validate:"required,max=64"`
	TargetTeamName  string `json:"targetTeamName" label:"接手班组" validate:"required,max=64"`
	Reason          string `json:"reason" label:"改派原因" validate:"required,max=255"`
	OperatorName    string `json:"operatorName" label:"操作人" validate:"max=32"`
}

// TeamAssignmentItem 改派记录。
type TeamAssignmentItem struct {
	ID            uint   `json:"id"`
	Sequence      int    `json:"sequence"`
	TeamName      string `json:"teamName"`
	ChangeType    string `json:"changeType"`
	EffectiveDate string `json:"effectiveDate"`
	Reason        string `json:"reason"`
	OperatorName  string `json:"operatorName"`
	CreatedAt     string `json:"createdAt"`
}

// ReassignResponse 改派结果。
type ReassignResponse struct {
	Task       *CleaningTask       `json:"task"`
	Assignment TeamAssignmentItem  `json:"assignment"`
	Workloads  []refx.TeamWorkload `json:"workloads"`
}

// PublishReportRequest 发布班组月度工作量月报。
type PublishReportRequest struct {
	Month       string `json:"month" label:"月份" validate:"required,len=7"`
	PublishedBy string `json:"publishedBy" label:"发布人" validate:"max=32"`
}

// TeamWorkloadReportResponse 班组月度工作量。
type TeamWorkloadReportResponse struct {
	Month       string              `json:"month"`
	Published   bool                `json:"published"`
	PublishedAt string              `json:"publishedAt,omitempty"`
	PublishedBy string              `json:"publishedBy,omitempty"`
	Items       []refx.TeamWorkload `json:"items"`
}
