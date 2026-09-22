package cleaningtask

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/shared/date"
)

// ClosedMonthGuard 已封账月报的保护能力，由班组工作量（team）模块实现。
//
// 改派前由该接口判断是否会改变已出月报月份的工作量归属；会改变时返回错误阻止改派，
// 从而保证跨月改派不能改写已经出具的月报。
type ClosedMonthGuard interface {
	CheckReassignAllowed(ctx context.Context, taskID uint, effectiveDate date.Date) error
}

// ReassignService 任务改派班组业务逻辑。
type ReassignService struct {
	repo  *ReassignmentRepository
	tasks *Repository
	guard ClosedMonthGuard
}

// NewReassignService 构造改派服务。
func NewReassignService(repo *ReassignmentRepository, tasks *Repository) *ReassignService {
	return &ReassignService{repo: repo, tasks: tasks}
}

// SetClosedMonthGuard 注入月报封账保护（在路由装配阶段、team 模块构造后调用）。
func (s *ReassignService) SetClosedMonthGuard(guard ClosedMonthGuard) {
	s.guard = guard
}

// Reassign 把任务从当前班组改派给接手班组，并记录原班组、接手班组、原因与时间。
//
// 规则：
//   - 只有待开工、清淤中的任务可以改派；已完工报验（含报验后退回整改）的任务不允许改派；
//   - 任务必须已指定实施班组，接手班组不能为空且不能与当前班组相同；
//   - 生效日期不能晚于今天，也不能早于最近一次改派的生效日期（不允许倒补交接）；
//   - 不允许改变任何已封账月份的工作量归属；
//   - 任务班组按「当前班组快照」条件更新，并发改派同一任务时只有一次生效。
func (s *ReassignService) Reassign(ctx context.Context, id uint, req ReassignRequest) (*TeamReassignment, error) {
	task, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	if task.Status != StatusPending && task.Status != StatusInProgress {
		return nil, httpx.InvalidState(StatusReassignBlockedMessage(task.Status))
	}
	// 完工报验后即使因整改退回清淤中，也不允许再改派：报验时的工作量归属已固化。
	hasAcceptance, err := s.tasks.HasAcceptance(ctx, id)
	if err != nil {
		return nil, httpx.WrapInternal("检查验收记录失败", err)
	}
	if hasAcceptance {
		return nil, httpx.InvalidState("该任务已完工报验并产生验收记录，不能再改派班组")
	}

	fromTeam := strings.TrimSpace(task.TeamName)
	if fromTeam == "" {
		return nil, httpx.InvalidState("任务尚未指定实施班组，请先在任务编辑中补填班组，无需走改派")
	}
	toTeam := strings.TrimSpace(req.ToTeamName)
	if toTeam == "" {
		return nil, httpx.Validation("接手班组不能为空")
	}
	if toTeam == fromTeam {
		return nil, httpx.Validation("接手班组与当前班组相同，无需改派")
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return nil, httpx.Validation("改派原因不能为空")
	}

	effective := req.EffectiveDate
	if effective.IsZero() {
		effective = date.Today()
	}
	if effective.After(date.Today()) {
		return nil, httpx.Validation("改派生效日期不能晚于今天")
	}
	latest, err := s.repo.LatestByTask(ctx, id)
	if err != nil {
		return nil, httpx.WrapInternal("查询最近改派记录失败", err)
	}
	if latest != nil && effective.Before(latest.EffectiveDate) {
		return nil, httpx.Validation("改派生效日期不能早于最近一次改派的生效日期（" + latest.EffectiveDate.String() + "）")
	}

	if s.guard != nil {
		if err := s.guard.CheckReassignAllowed(ctx, id, effective); err != nil {
			return nil, err
		}
	}

	item := &TeamReassignment{
		TaskID:        id,
		FromTeamName:  fromTeam,
		ToTeamName:    toTeam,
		Reason:        reason,
		EffectiveDate: effective,
		OperatorName:  strings.TrimSpace(req.OperatorName),
		CreatedAt:     time.Now(),
	}
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if err := s.repo.ChangeTeamInTx(ctx, tx, id, fromTeam, toTeam); err != nil {
			return err
		}
		return s.repo.CreateInTx(ctx, tx, item)
	})
	if err != nil {
		if errors.Is(err, ErrTeamChanged) {
			return nil, httpx.InvalidState("任务班组已被其他人改派，请刷新后重试")
		}
		return nil, httpx.WrapInternal("改派班组失败", err)
	}
	return item, nil
}

// ListByTask 查询某任务的改派记录。
func (s *ReassignService) ListByTask(ctx context.Context, taskID uint) ([]TeamReassignment, error) {
	if _, err := s.tasks.FindByID(ctx, taskID); err != nil {
		return nil, notFound(err)
	}
	items, err := s.repo.ListByTask(ctx, taskID)
	if err != nil {
		return nil, httpx.WrapInternal("查询改派记录失败", err)
	}
	return items, nil
}

// StatusReassignBlockedMessage 返回各状态下不允许改派的提示文案。
func StatusReassignBlockedMessage(status string) string {
	switch status {
	case StatusCompleted:
		return "任务已完工报验，等待验收，不能再改派班组"
	case StatusAccepted:
		return "任务已验收完成，不能再改派班组"
	case StatusCancelled:
		return "任务已取消，不能再改派班组"
	default:
		return "任务当前状态不允许改派班组"
	}
}
