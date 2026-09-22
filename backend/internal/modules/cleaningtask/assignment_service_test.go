package cleaningtask_test

import (
	"context"
	"testing"

	"github.com/drainage/desilting/internal/httpx"
	"github.com/drainage/desilting/internal/modules/cleaningrecord"
	"github.com/drainage/desilting/internal/modules/cleaningtask"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/testsupport"
)

func createRecordAt(t *testing.T, fixture *testsupport.Fixture, taskID uint, day date.Date, sludge float64) {
	t.Helper()
	_, err := fixture.Records.Create(context.Background(), cleaningrecord.SaveRequest{
		TaskID:         taskID,
		CleanedAt:      day,
		LengthM:        100,
		SludgeVolumeM3: sludge,
		WaterVolumeM3:  10,
		PersonnelCount: 3,
		Method:         cleaningtask.MethodHighPressure,
		Weather:        cleaningrecord.WeatherSunny,
		RecorderName:   "测试记录人",
	})
	testsupport.RequireNoError(t, err)
}

func TestReassignSplitsWorkloadByActualWorkDate(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	task := fixture.CreateTask(t, fixture.Segment.ID, "跨日期改派的任务")
	today := date.Today()

	createRecordAt(t, fixture, task.ID, today.AddDays(-2), 10)
	createRecordAt(t, fixture, task.ID, today.AddDays(-1), 20)

	result, err := fixture.Tasks.Reassign(context.Background(), task.ID, cleaningtask.ReassignRequest{
		CurrentTeamName: "测试班组",
		TargetTeamName:  "城西养护二班",
		Reason:          "原班组设备调度冲突",
	})
	testsupport.RequireNoError(t, err)
	createRecordAt(t, fixture, task.ID, today, 30)
	if result.Task.TeamName != "城西养护二班" {
		t.Fatalf("改派后当前班组应为城西养护二班，实际 %s", result.Task.TeamName)
	}

	detail, err := fixture.Tasks.Detail(context.Background(), task.ID)
	testsupport.RequireNoError(t, err)
	if len(detail.Assignments) != 2 {
		t.Fatalf("任务详情应包含初始派工和改派两条记录，实际 %d 条", len(detail.Assignments))
	}
	if len(detail.TeamWorkloads) != 2 {
		t.Fatalf("任务详情班组工作量应与改派响应同一口径，实际 %d 组", len(detail.TeamWorkloads))
	}

	totalRecords := int64(0)
	for _, item := range detail.TeamWorkloads {
		totalRecords += item.RecordCount
	}
	if totalRecords != 3 {
		t.Fatalf("同批工作量不能重复计算，期望总记录数 3，实际 %d", totalRecords)
	}
}

func TestReassignRejectsCompletedTask(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	task := fixture.TaskReadyForAcceptance(t, fixture.Segment.ID, "已完工报验的任务")

	_, err := fixture.Tasks.Reassign(context.Background(), task.ID, cleaningtask.ReassignRequest{
		CurrentTeamName: "测试班组",
		TargetTeamName:  "接手班组",
		Reason:          "尝试改派",
	})
	testsupport.RequireAppError(t, err, httpx.CodeInvalidState)
}

func TestStaleReassignOnlyOneChangeTakesEffect(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	task := fixture.CreateTask(t, fixture.Segment.ID, "并发改派的任务")

	_, err := fixture.Tasks.Reassign(context.Background(), task.ID, cleaningtask.ReassignRequest{
		CurrentTeamName: "测试班组",
		TargetTeamName:  "班组 B",
		Reason:          "第一次改派",
	})
	testsupport.RequireNoError(t, err)

	_, err = fixture.Tasks.Reassign(context.Background(), task.ID, cleaningtask.ReassignRequest{
		CurrentTeamName: "测试班组",
		TargetTeamName:  "班组 C",
		Reason:          "基于旧页面的并发改派",
	})
	testsupport.RequireAppError(t, err, httpx.CodeInvalidState)

	reloaded := fixture.Reload(t, task.ID)
	if reloaded.TeamName != "班组 B" {
		t.Fatalf("只有一次改派应生效，期望班组 B，实际 %s", reloaded.TeamName)
	}
}

func TestPublishedMonthlyReportCannotBeChangedByReassign(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	task := fixture.CreateTask(t, fixture.Segment.ID, "月报发布后的改派任务")
	otherTask := fixture.CreateTask(t, fixture.Segment.ID, "月报发布后无当月工作量的任务")
	month := date.Today().Time.Format("2006-01")

	_, err := fixture.Tasks.PublishTeamWorkloadReport(context.Background(), cleaningtask.PublishReportRequest{
		Month:       month,
		PublishedBy: "统计员",
	})
	testsupport.RequireNoError(t, err)

	_, err = fixture.Tasks.PublishTeamWorkloadReport(context.Background(), cleaningtask.PublishReportRequest{Month: month})
	testsupport.RequireAppError(t, err, httpx.CodeConflict)

	createRecordAt(t, fixture, task.ID, date.Today(), 5)
	_, err = fixture.Tasks.Reassign(context.Background(), task.ID, cleaningtask.ReassignRequest{
		CurrentTeamName: "测试班组",
		TargetTeamName:  "月报发布后接手班组",
		Reason:          "尝试影响已出月报",
	})
	testsupport.RequireAppError(t, err, httpx.CodeInvalidState)

	_, err = fixture.Tasks.Reassign(context.Background(), otherTask.ID, cleaningtask.ReassignRequest{
		CurrentTeamName: "测试班组",
		TargetTeamName:  "无当月工作量接手班组",
		Reason:          "不影响已发布月报",
	})
	testsupport.RequireNoError(t, err)
}
