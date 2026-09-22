package team

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/shared/refx"
)

// monthLen 月份字符串 YYYY-MM 的长度。
const monthLen = 7

// Service 班组工作量统计与月报。
type Service struct {
	repo *Repository
	db   *gorm.DB
}

// NewService 构造服务。
func NewService(repo *Repository) *Service {
	return &Service{repo: repo, db: repo.DB()}
}

// WorkloadResponse 某月班组工作量（含该月封账状态）。
type WorkloadResponse struct {
	Month     string              `json:"month"`
	From      date.Date           `json:"from"`
	To        date.Date           `json:"to"`
	Items     []refx.TeamWorkload `json:"items"`
	Published bool                `json:"published"`
	Report    *WorkloadReport     `json:"report"`
}

// Workload 统计指定月份（YYYY-MM，默认当月）各班组按统一口径归属的工作量。
func (s *Service) Workload(ctx context.Context, month string) (*WorkloadResponse, error) {
	month, from, to, err := normalizeMonth(month)
	if err != nil {
		return nil, err
	}
	items, err := refx.TeamWorkloadBetween(ctx, s.db, &from, &to)
	if err != nil {
		return nil, httpx.WrapInternal("统计班组工作量失败", err)
	}
	report, err := s.repo.FindByMonth(ctx, month)
	if err != nil {
		return nil, httpx.WrapInternal("查询月报失败", err)
	}
	resp := &WorkloadResponse{
		Month:  month,
		From:   from,
		To:     to,
		Items:  items,
		Report: report,
	}
	if report != nil {
		resp.Published = report.Published
	}
	return resp, nil
}

// PublishMonthRequest 出具月报请求体。
type PublishMonthRequest struct {
	Month       string `json:"month" label:"月份" validate:"required,len=7"`
	Remark      string `json:"remark" label:"备注" validate:"max=255"`
	PublishedBy string `json:"publishedBy" label:"出具人" validate:"max=32"`
}

// Publish 出具（封账）某月工作量月报。月份不允许晚于当前月。
// 重复出具为幂等操作，但只刷新备注/出具人，不改变封账后的归属数据。
func (s *Service) Publish(ctx context.Context, req PublishMonthRequest) (*WorkloadReport, error) {
	month, _, _, err := normalizeMonth(strings.TrimSpace(req.Month))
	if err != nil {
		return nil, err
	}
	today := date.Today()
	currentMonth := fmt.Sprintf("%04d-%02d", today.Year(), int(today.Month()))
	if month > currentMonth {
		return nil, httpx.Validation("不能为未来月份出具月报")
	}
	report := &WorkloadReport{
		Month:       month,
		Published:   true,
		Remark:      strings.TrimSpace(req.Remark),
		PublishedBy: strings.TrimSpace(req.PublishedBy),
	}
	if err := s.repo.Upsert(ctx, report); err != nil {
		return nil, httpx.WrapInternal("出具月报失败", err)
	}
	return report, nil
}

// ListReports 查询已出具的月报清单。
func (s *Service) ListReports(ctx context.Context) ([]WorkloadReport, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, httpx.WrapInternal("查询月报清单失败", err)
	}
	return items, nil
}

// CheckReassignAllowed 实现 cleaningtask.ClosedMonthGuard：
// 判断在 effectiveDate 对 taskID 改派是否会改变任何已封账月份的工作量归属。
//
// 改派只可能影响「作业日期 >= 生效日」的历史记录，因此只需检查区间
// [生效日, 生效日所在月月末] 内，是否存在已封账月份含有该任务的清淤记录。
func (s *Service) CheckReassignAllowed(ctx context.Context, taskID uint, effective date.Date) error {
	months, err := s.repo.PublishedMonths(ctx)
	if err != nil {
		return httpx.WrapInternal("检查已封账月报失败", err)
	}
	for _, month := range months {
		from, to, err := monthBounds(month)
		if err != nil {
			continue
		}
		// 生效日晚于该月月末：这个月的作业早已全部归属，改派不影响它。
		if effective.After(to) {
			continue
		}
		// 区间下界取生效日与月初的较大者。
		start := from
		if effective.After(start) {
			start = effective
		}
		var count int64
		err = s.db.WithContext(ctx).Table(refx.TableCleaningRecords).
			Where("task_id = ? AND cleaned_at >= ? AND cleaned_at <= ?", taskID, start.Time, to.Time).
			Count(&count).Error
		if err != nil {
			return httpx.WrapInternal("检查改派对月报的影响失败", err)
		}
		if count > 0 {
			return httpx.InvalidState(fmt.Sprintf(
				"%s 月班组工作量月报已出具并封账，该月存在本任务的作业记录，改派会改变其归属，不能改派",
				month,
			))
		}
	}
	return nil
}

// normalizeMonth 校验并归一化月份字符串，返回 (YYYY-MM, 月初, 月末)。
func normalizeMonth(month string) (string, date.Date, date.Date, error) {
	month = strings.TrimSpace(month)
	if month == "" {
		today := date.Today()
		month = fmt.Sprintf("%04d-%02d", today.Year(), int(today.Month()))
	}
	from, to, err := monthBounds(month)
	if err != nil {
		return "", date.Date{}, date.Date{}, httpx.Validation("月份格式不正确，应为 YYYY-MM")
	}
	return month, from, to, nil
}

// monthBounds 返回某月的月初与月末日期。
func monthBounds(month string) (date.Date, date.Date, error) {
	if len(month) != monthLen || month[4] != '-' {
		return date.Date{}, date.Date{}, fmt.Errorf("非法月份 %q", month)
	}
	parsed, err := time.Parse("2006-01", month)
	if err != nil {
		return date.Date{}, date.Date{}, err
	}
	loc := time.UTC
	from := date.New(time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, loc))
	to := date.New(from.AddDate(0, 1, -1))
	return from, to, nil
}
