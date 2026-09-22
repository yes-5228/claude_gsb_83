package team_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/drainage/desilting/internal/modules/cleaningrecord"
	"github.com/drainage/desilting/internal/modules/cleaningtask"
	"github.com/drainage/desilting/internal/modules/dashboard"
	"github.com/drainage/desilting/internal/modules/team"
	"github.com/drainage/desilting/internal/shared/date"
	"github.com/drainage/desilting/internal/shared/refx"
	"github.com/drainage/desilting/internal/testsupport"
)

func record(t *testing.T, fixture *testsupport.Fixture, taskID uint, at date.Date, sludge float64, people int) {
	t.Helper()
	_, err := fixture.Records.Create(context.Background(), cleaningrecord.SaveRequest{
		TaskID: taskID, CleanedAt: at, LengthM: 50, SludgeVolumeM3: sludge,
		WaterVolumeM3: 5, PersonnelCount: people, Method: cleaningtask.MethodHighPressure,
		Weather: cleaningrecord.WeatherSunny, RecorderName: "记录人",
	})
	if err != nil {
		t.Fatalf("录入清淤记录失败: %v", err)
	}
}

func find(items []refx.TeamWorkload, name string) (refx.TeamWorkload, bool) {
	for i := range items {
		if items[i].TeamName == name {
			return items[i], true
		}
	}
	return refx.TeamWorkload{}, false
}

// TestMultipleReassignmentsAttribution 连续多次改派时，每条作业记录按作业日期
// 精确归属到当时班组，全量统计既不遗漏也不重复。
func TestMultipleReassignmentsAttribution(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()
	svc := team.NewService(team.NewRepository(fixture.DB))
	fixture.Tasks.SetClosedMonthGuard(svc)

	task := fixture.CreateTask(t, fixture.Segment.ID, "连续改派任务")
	d0 := date.Today().AddDays(-6)
	d1 := date.Today().AddDays(-4)
	d2 := date.Today().AddDays(-2)
	record(t, fixture, task.ID, d0, 10, 2) // 测试班组
	record(t, fixture, task.ID, d1, 20, 4) // 乙班
	record(t, fixture, task.ID, d2, 30, 6) // 丙班

	mustReassign := func(to string, eff date.Date) {
		if _, err := fixture.Tasks.Reassign(ctx, task.ID, cleaningtask.ReassignRequest{
			ToTeamName: to, Reason: "交接", EffectiveDate: eff,
		}); err != nil {
			t.Fatalf("改派给 %s 失败: %v", to, err)
		}
	}
	mustReassign("乙班", date.Today().AddDays(-5))
	mustReassign("丙班", date.Today().AddDays(-3))

	// 班组工作量统计（team 模块）。
	resp, err := svc.Workload(ctx, "")
	if err != nil {
		t.Fatalf("查询工作量失败: %v", err)
	}
	expect := map[string]float64{"测试班组": 10, "乙班": 20, "丙班": 30}
	gotPeople := map[string]int64{}
	var total float64
	for _, w := range resp.Items {
		total += w.SludgeVolumeM3
		gotPeople[w.TeamName] = w.PersonDays
	}
	for name, want := range expect {
		w, ok := find(resp.Items, name)
		if !ok || w.SludgeVolumeM3 != want {
			t.Fatalf("班组 %s 期望 %v 方，实际 %+v ok=%v", name, want, w, ok)
		}
	}
	if total != 60 {
		t.Fatalf("三班组合计应等于总清淤量 60（不重不漏），实际 %v", total)
	}
	if gotPeople["乙班"] != 4 || gotPeople["丙班"] != 6 {
		t.Fatalf("工日归属不正确: %v", gotPeople)
	}

	// 看板与统计同一口径。
	dash := dashboard.NewService(fixture.DB)
	dw, err := dash.TeamWorkload(ctx, "")
	if err != nil {
		t.Fatalf("看板班组工作量查询失败: %v", err)
	}
	var dashTotal float64
	for _, w := range dw.Items {
		dashTotal += w.SludgeVolumeM3
	}
	if dashTotal != 60 {
		t.Fatalf("看板合计应为 60，实际 %v", dashTotal)
	}
	// 看板最近记录 SQL（含归属子查询）能正常执行，且班组按作业日期归属。
	recent, err := dash.RecentRecords(ctx, 10)
	if err != nil {
		t.Fatalf("看板最近记录查询失败: %v", err)
	}
	byDate := map[string]string{}
	for _, r := range recent {
		byDate[r.CleanedAt.String()] = r.TeamName
	}
	if byDate[d1.String()] != "乙班" || byDate[d2.String()] != "丙班" || byDate[d0.String()] != "测试班组" {
		t.Fatalf("最近记录班组归属不正确: %v", byDate)
	}
}

// TestPublishFreezesMonth 出具月报后封账状态可查，重复出具幂等。
func TestPublishFreezesMonth(t *testing.T) {
	fixture := testsupport.NewFixture(t)
	ctx := context.Background()
	svc := team.NewService(team.NewRepository(fixture.DB))

	month := date.Today().AddDays(-40).String()[:7]
	if _, err := svc.Publish(ctx, team.PublishMonthRequest{Month: month, PublishedBy: "甲"}); err != nil {
		t.Fatalf("首次出具月报失败: %v", err)
	}
	resp, err := svc.Workload(ctx, month)
	if err != nil {
		t.Fatalf("查询月报失败: %v", err)
	}
	if !resp.Published {
		t.Fatal("月报出具后应为封账状态")
	}
	if _, err := svc.Publish(ctx, team.PublishMonthRequest{Month: month, Remark: fmt.Sprintf("补注 %s", month)}); err != nil {
		t.Fatalf("重复出具应幂等成功: %v", err)
	}
}
