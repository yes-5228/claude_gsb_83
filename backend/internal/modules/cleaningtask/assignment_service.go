package cleaningtask

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/shared/refx"
)

// Reassign 改派班组。已完工报验 / 已验收 / 已取消任务不允许改派；
// 改派从当天起生效，既有作业按作业日期保留给原班组。
func (s *Service) Reassign(ctx context.Context, id uint, req ReassignRequest) (*ReassignResponse, error) {
	target := strings.TrimSpace(req.TargetTeamName)
	current := strings.TrimSpace(req.CurrentTeamName)
	if current == "" {
		return nil, httpx.Validation("原班组不能为空")
	}
	if target == "" {
		return nil, httpx.Validation("接手班组不能为空")
	}
	if current == target {
		return nil, httpx.Validation("接手班组不能与原班组相同")
	}
	if strings.TrimSpace(req.Reason) == "" {
		return nil, httpx.Validation("改派原因不能为空")
	}

	task, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	if task.Status != StatusPending && task.Status != StatusInProgress {
		return nil, httpx.InvalidState("任务已经完工报验、验收或取消，不允许再改派班组")
	}

	actualCurrent := strings.TrimSpace(task.TeamName)
	if actualCurrent == "" {
		actualCurrent = "未指定班组"
	}
	if actualCurrent != current {
		return nil, httpx.InvalidState("原班组已被其他操作变更，请刷新后重试")
	}

	today := date.Today()
	month := today.Time.Format("2006-01")
	publishedMonths, err := s.repo.PublishedReportMonthsBetween(ctx, month, month)
	if err != nil {
		return nil, httpx.WrapInternal("检查班组月报失败", err)
	}
	if len(publishedMonths) > 0 {
		hasCurrentWork, err := s.repo.HasRecordForTaskSince(ctx, id, today.Time)
		if err != nil {
			return nil, httpx.WrapInternal("检查当月作业记录失败", err)
		}
		if hasCurrentWork {
			return nil, httpx.InvalidState("当月班组工作量月报已经发布，该任务已有当月工作量，不能再改派")
		}
	}

	updated, assignment, err := s.repo.ReassignTeam(ctx, id, current, target, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			return nil, httpx.NotFound("清淤任务不存在")
		case errors.Is(err, ErrStateConflict):
			return nil, httpx.InvalidState("班组改派已被其他用户先处理，请刷新后重试")
		case errors.Is(err, ErrSameTeam):
			return nil, httpx.Validation("接手班组不能与原班组相同")
		case errors.Is(err, gorm.ErrDuplicatedKey):
			return nil, httpx.Conflict("班组改派正在被其他用户处理，请刷新后重试")
		default:
			return nil, httpx.WrapInternal("改派班组失败", err)
		}
	}

	workloads, err := refx.TeamWorkloadsForTask(ctx, s.repo.DB(), id)
	if err != nil {
		return nil, httpx.WrapInternal("统计班组工作量失败", err)
	}
	return &ReassignResponse{
		Task:       updated,
		Assignment: toAssignmentItem(*assignment),
		Workloads:  workloads,
	}, nil
}

// TeamWorkloadReport 查询班组月度工作量：已发布月份返回不可变快照，否则返回实时归属统计。
func (s *Service) TeamWorkloadReport(ctx context.Context, month string) (*TeamWorkloadReportResponse, error) {
	if strings.TrimSpace(month) == "" {
		month = date.Today().Time.Format("2006-01")
	}
	normalizedMonth, err := normalizeMonth(month)
	if err != nil {
		return nil, err
	}
	report, err := s.repo.FindWorkloadReport(ctx, normalizedMonth)
	if err != nil {
		return nil, httpx.WrapInternal("查询班组月报失败", err)
	}
	response := &TeamWorkloadReportResponse{Month: normalizedMonth, Items: []refx.TeamWorkload{}}
	if report != nil {
		items, err := s.repo.ListWorkloadSnapshots(ctx, report.ID)
		if err != nil {
			return nil, httpx.WrapInternal("查询班组月报快照失败", err)
		}
		response.Published = true
		response.PublishedAt = report.PublishedAt.Format(time.RFC3339)
		response.PublishedBy = report.PublishedBy
		response.Items = items
		return response, nil
	}

	items, err := refx.TeamWorkloads(ctx, s.repo.DB(), normalizedMonth)
	if err != nil {
		return nil, httpx.WrapInternal("统计班组工作量失败", err)
	}
	response.Items = items
	return response, nil
}

// PublishTeamWorkloadReport 发布班组月度工作量月报。月报发布后快照不可再被改派改写。
func (s *Service) PublishTeamWorkloadReport(ctx context.Context, req PublishReportRequest) (*TeamWorkloadReportResponse, error) {
	month, err := normalizeMonth(req.Month)
	if err != nil {
		return nil, err
	}
	currentMonth := date.Today().Time.Format("2006-01")
	if month > currentMonth {
		return nil, httpx.Validation("不能发布未来月份的班组工作量月报")
	}
	if existing, err := s.repo.FindWorkloadReport(ctx, month); err != nil {
		return nil, httpx.WrapInternal("检查班组月报失败", err)
	} else if existing != nil {
		return nil, httpx.Conflict(month + " 班组工作量月报已经发布，已出月报不允许覆盖")
	}

	items, err := refx.TeamWorkloads(ctx, s.repo.DB(), month)
	if err != nil {
		return nil, httpx.WrapInternal("统计班组工作量失败", err)
	}
	report, err := s.repo.CreateWorkloadReport(ctx, month, items, strings.TrimSpace(req.PublishedBy))
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, httpx.Conflict(month + " 班组工作量月报已经发布，已出月报不允许覆盖")
		}
		return nil, httpx.WrapInternal("发布班组月报失败", err)
	}
	return &TeamWorkloadReportResponse{
		Month:       month,
		Published:   true,
		PublishedAt: report.PublishedAt.Format(time.RFC3339),
		PublishedBy: report.PublishedBy,
		Items:       items,
	}, nil
}

func toAssignmentItem(assignment TeamAssignment) TeamAssignmentItem {
	return TeamAssignmentItem{
		ID:            assignment.ID,
		Sequence:      assignment.Sequence,
		TeamName:      assignment.TeamName,
		ChangeType:    assignment.ChangeType,
		EffectiveDate: assignment.EffectiveDate.String(),
		Reason:        assignment.Reason,
		OperatorName:  assignment.OperatorName,
		CreatedAt:     assignment.CreatedAt.Format(time.RFC3339),
	}
}

func normalizeMonth(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if _, _, err := refx.MonthRange(trimmed); err != nil {
		return "", httpx.BadRequest("月份格式不正确，应为 YYYY-MM")
	}
	return trimmed, nil
}
