package cleaningtask_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/modules/cleaningrecord"
	"github.com/drainage/desilting/internal/modules/cleaningtask"
	"github.com/drainage/desilting/internal/modules/team"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/shared/refx"
	"github.com/drainage/desilting/internal/testsupport"
)

// workloadOf 在工作量列表中找到指定班组的工作量。
func workloadOf(items []refx.TeamWorkload, teamName string) (refx.TeamWorkload, bool) {
	for i := range items {
		if items[i].TeamName == teamName {
			return items[i], true
		}
	}
	return refx.TeamWorkload{}, false
}

// addRecordAt 在指定作业日期录入一条清淤记录。
func addRecordAt(t *testing.T, fixture *testsupport.Fixture, taskID uint, at date.Date, sludge, lengthM float64, people int) {
	t.Helper()
	_, err := fixture.Records.Create(context.Background(), cleaningrecord.SaveRequest{
		TaskID:         taskID,
		CleanedAt:      at,
		LengthM:        lengthM,
		SludgeVolumeM3: sludge,
		WaterVolumeM3:  10,
		PersonnelCount: people,
		Method:         cleaningtask.MethodHighPressure,
		Weather:        cleaningrecord.WeatherSunny,
		RecorderName:   "记录人",
	})
	if err != nil {
		t.Fatalf("录入清淤记录失败: %v", err)
	}
}

// TestReassignCrossDaySplitsWorkload 跨作业日期改派：工作量按实际作业日期归属，
// 同一批工作量不会被两个班组各算一遍，且任务详情与统一统计口径一致。
func TestReassignCrossDaySplitsWorkload(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()
	task := fixture.CreateTask(t, fixture.Segment.ID, "跨日改派任务")

	day1 := date.Today().AddDays(-2)
	day2 := date.Today().AddDays(-1)
	addRecordAt(t, fixture, task.ID, day1, 10, 100, 3)
	addRecordAt(t, fixture, task.ID, day2, 20, 200, 5)

	// 在 day2 当天起由甲班改派给乙班。
	item, err := fixture.Tasks.Reassign(ctx, task.ID, cleaningtask.ReassignRequest{
		ToTeamName:    "乙班",
		Reason:        "甲班设备检修，剩余作业移交乙班",
		EffectiveDate: day2,
		OperatorName:  "调度员",
	})
	if err != nil {
		t.Fatalf("改派失败: %v", err)
	}
	if item.FromTeamName != "测试班组" || item.ToTeamName != "乙班" {
		t.Fatalf("改派记录原班组/接手班组不正确: %+v", item)
	}

	reloaded := fixture.Reload(t, task.ID)
	if reloaded.TeamName != "乙班" {
		t.Fatalf("改派后任务当前班组应为乙班，实际 %q", reloaded.TeamName)
	}

	// 全时间段按统一口径统计：甲班 10、乙班 20，合计仍是 30，没有重复计算。
	all, err := refx.TeamWorkloadBetween(ctx, fixture.DB, nil, nil)
	if err != nil {
		t.Fatalf("统计班组工作量失败: %v", err)
	}
	jia, ok := workloadOf(all, "测试班组")
	if !ok || jia.SludgeVolumeM3 != 10 || jia.RecordCount != 1 || jia.PersonDays != 3 {
		t.Fatalf("原班组工作量应为 1 条/3 工日/10 方，实际 %+v ok=%v", jia, ok)
	}
	yi, ok := workloadOf(all, "乙班")
	if !ok || yi.SludgeVolumeM3 != 20 || yi.RecordCount != 1 || yi.PersonDays != 5 {
		t.Fatalf("接手班组工作量应为 1 条/5 工日/20 方，实际 %+v ok=%v", yi, ok)
	}

	// 任务详情使用同一口径，拆分合计等于任务总量。
	detail, err := fixture.Tasks.Detail(ctx, task.ID)
	if err != nil {
		t.Fatalf("查询任务详情失败: %v", err)
	}
	if len(detail.Reassignments) != 1 {
		t.Fatalf("任务详情应包含 1 条改派记录，实际 %d", len(detail.Reassignments))
	}
	var detailSludge float64
	var detailRecords int64
	for _, w := range detail.TeamWorkload {
		detailSludge += w.SludgeVolumeM3
		detailRecords += w.RecordCount
	}
	if detailSludge != 30 || detailRecords != 2 {
		t.Fatalf("任务详情班组拆分合计应为 30 方/2 条，实际 %v 方/%v 条", detailSludge, detailRecords)
	}
	if detail.RecordTotals.SludgeVolumeM3 != 30 {
		t.Fatalf("任务清淤量汇总应为 30，实际 %v", detail.RecordTotals.SludgeVolumeM3)
	}
}

// TestReassignBlockedAfterCompletion 已完工报验或已验收的任务不允许再改派。
func TestReassignBlockedAfterCompletion(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()

	completed := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "已完工报验任务")
	_, err := fixture.Tasks.Reassign(ctx, completed.ID, cleaningtask.ReassignRequest{
		ToTeamName: "乙班", Reason: "测试", EffectiveDate: date.Today(),
	})
	if err == nil {
		t.Fatal("已完工报验的任务不应允许改派")
	}
	testsupport.RequireAppError(t, err, httpx.CodeInvalidState)

	// 验收合格后同样不允许改派。
	if _, err := fixture.Acceptances.Create(ctx, testsupport.PassRequest(completed.ID, 90)); err != nil {
		t.Fatalf("登记验收合格失败: %v", err)
	}
	accepted := fixture.Reload(t, completed.ID)
	if accepted.Status != cleaningtask.StatusAccepted {
		t.Fatalf("任务应为已验收，实际 %s", accepted.Status)
	}
	if _, err := fixture.Tasks.Reassign(ctx, accepted.ID, cleaningtask.ReassignRequest{
		ToTeamName: "乙班", Reason: "测试", EffectiveDate: date.Today(),
	}); err == nil {
		t.Fatal("已验收的任务不应允许改派")
	}
}

// TestReassignValidation 改派参数校验。
func TestReassignValidation(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()
	task := fixture.CreateTask(t, fixture.Segment.ID, "校验任务")

	cases := []struct {
		name string
		req  cleaningtask.ReassignRequest
	}{
		{"接手班组与当前相同", cleaningtask.ReassignRequest{ToTeamName: "测试班组", Reason: "x", EffectiveDate: date.Today()}},
		{"接手班组为空", cleaningtask.ReassignRequest{ToTeamName: "  ", Reason: "x", EffectiveDate: date.Today()}},
		{"原因为空", cleaningtask.ReassignRequest{ToTeamName: "乙班", Reason: "  ", EffectiveDate: date.Today()}},
		{"生效日期在未来", cleaningtask.ReassignRequest{ToTeamName: "乙班", Reason: "x", EffectiveDate: date.Today().AddDays(1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := fixture.Tasks.Reassign(ctx, task.ID, tc.req); err == nil {
				t.Fatalf("%s：应当被拒绝", tc.name)
			}
		})
	}

	// 不允许倒补：第二次改派生效日早于第一次。
	if _, err := fixture.Tasks.Reassign(ctx, task.ID, cleaningtask.ReassignRequest{
		ToTeamName: "乙班", Reason: "第一次", EffectiveDate: date.Today().AddDays(-1),
	}); err != nil {
		t.Fatalf("第一次改派失败: %v", err)
	}
	if _, err := fixture.Tasks.Reassign(ctx, task.ID, cleaningtask.ReassignRequest{
		ToTeamName: "丙班", Reason: "倒补", EffectiveDate: date.Today().AddDays(-2),
	}); err == nil {
		t.Fatal("生效日期早于最近一次改派时不应允许")
	}
}

// TestClosedMonthBlocksReassign 跨月改派不能改写已出具的月报；
// 不触及封账月份的改派仍可进行，且封账月份归属保持不变。
func TestClosedMonthBlocksReassign(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()
	teamSvc := team.NewService(team.NewRepository(fixture.DB))
	fixture.Tasks.SetClosedMonthGuard(teamSvc)

	now := time.Now().UTC()
	twoMonthsAgo := date.New(time.Date(now.Year(), now.Month()-2, 1, 0, 0, 0, 0, time.UTC))
	closedMonth := twoMonthsAgo.String()[:7]

	task := fixture.CreateTask(t, fixture.Segment.ID, "跨月改派任务")
	// 作业发生在已封账月份的 10 号，若改派生效日（5 号）成立，该作业归属会被改写。
	addRecordAt(t, fixture, task.ID, twoMonthsAgo.AddDays(10), 15, 100, 4)

	if _, err := teamSvc.Publish(ctx, team.PublishMonthRequest{Month: closedMonth, PublishedBy: "统计员"}); err != nil {
		t.Fatalf("出具月报失败: %v", err)
	}

	// 生效日落入已封账月份且早于该月作业日期：拒绝改派。
	if _, err := fixture.Tasks.Reassign(ctx, task.ID, cleaningtask.ReassignRequest{
		ToTeamName: "乙班", Reason: "尝试改写历史月份", EffectiveDate: twoMonthsAgo.AddDays(5),
	}); err == nil {
		t.Fatal("改派会改变已封账月份归属时不应允许")
	}

	// 生效日为今天（晚于封账月月末）：允许，且封账月份工作量仍归原班组。
	if _, err := fixture.Tasks.Reassign(ctx, task.ID, cleaningtask.ReassignRequest{
		ToTeamName: "乙班", Reason: "当月起交接", EffectiveDate: date.Today(),
	}); err != nil {
		t.Fatalf("不触及封账月份的改派应允许，实际: %v", err)
	}
	closed, err := teamSvc.Workload(ctx, closedMonth)
	if err != nil {
		t.Fatalf("查询封账月工作量失败: %v", err)
	}
	w, ok := workloadOf(closed.Items, "测试班组")
	if !ok || w.SludgeVolumeM3 != 15 {
		t.Fatalf("封账月份工作量不应被改派改写，实际 %+v ok=%v", closed.Items, ok)
	}
	if _, exists := workloadOf(closed.Items, "乙班"); exists {
		t.Fatal("接手班组不应在已封账月份获得工作量")
	}
}

// TestConcurrentReassignOnlyOneSucceeds 两个人同时改派同一条任务时只有一次生效：
// 后提交的一方因当前班组已变化导致乐观条件更新失败。
func TestConcurrentReassignOnlyOneSucceeds(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()
	task := fixture.CreateTask(t, fixture.Segment.ID, "并发改派任务")

	repo := cleaningtask.NewReassignmentRepository(fixture.DB)

	// run 模拟一个操作人基于「测试班组」旧快照提交改派。
	run := func(toTeam string) error {
		return fixture.DB.Transaction(func(tx *gorm.DB) error {
			return repo.ChangeTeamInTx(ctx, tx, task.ID, "测试班组", toTeam)
		})
	}
	if err := run("乙班"); err != nil {
		t.Fatalf("第一次改派应成功，实际 %v", err)
	}
	if err := run("丙班"); err != cleaningtask.ErrTeamChanged {
		t.Fatalf("第二次并发改派应返回 ErrTeamChanged，实际 %v", err)
	}

	// 丙班的改派事务已回滚，任务当前班组只保留第一次生效的结果。
	reloaded := fixture.Reload(t, task.ID)
	if reloaded.TeamName != "乙班" {
		t.Fatalf("只有第一次改派应生效，当前班组实际为 %q", reloaded.TeamName)
	}
	var reassignmentCount int64
	if err := fixture.DB.Table(refx.TableTeamReassignments).Where("task_id = ?", task.ID).Count(&reassignmentCount).Error; err != nil {
		t.Fatalf("统计改派记录失败: %v", err)
	}
	if reassignmentCount != 0 {
		t.Fatalf("乐观条件测试直接写库，不应产生改派记录，实际 %d 条", reassignmentCount)
	}
}
