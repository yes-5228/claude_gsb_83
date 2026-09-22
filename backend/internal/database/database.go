// Package database 负责数据库连接、表结构迁移。
package database

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/drainage/desilting/internal/config"
	"github.com/drainage/desilting/internal/modules/acceptance"
	"github.com/drainage/desilting/internal/modules/cleaningrecord"
	"github.com/drainage/desilting/internal/modules/cleaningtask"
	"github.com/drainage/desilting/internal/modules/pipesegment"
)

// Open 根据配置建立数据库连接。
//
// 生产与 docker compose 使用 PostgreSQL；本地开发或单元测试可以切到 SQLite，
// 两种驱动共用同一套 model 与查询代码。
func Open(cfg *config.Config) (*gorm.DB, error) {
	dialector, err := dialectorFor(cfg)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:         gormlogger.Default.LogMode(gormlogger.Warn),
		TranslateError: true,
		NowFunc:        func() time.Time { return time.Now().In(time.Local) },
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

func dialectorFor(cfg *config.Config) (gorm.Dialector, error) {
	switch cfg.DBDriver {
	case config.DriverSQLite:
		if dir := filepath.Dir(cfg.DBPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("创建 SQLite 数据目录失败: %w", err)
			}
		}
		return sqlite.Open(cfg.DBPath), nil
	case config.DriverPostgres:
		return postgres.Open(cfg.PostgresDSN()), nil
	default:
		return nil, fmt.Errorf("不支持的数据库驱动 %q，可选值为 %s / %s",
			cfg.DBDriver, config.DriverPostgres, config.DriverSQLite)
	}
}

// Migrate 建立/更新所有业务表。
//
// 表之间的引用关系由应用层在 service 中校验，因此这里不建外键约束，
// 便于后续按模块拆库时平滑迁移。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&pipesegment.PipeSegment{},
		&cleaningtask.CleaningTask{},
		&cleaningrecord.CleaningRecord{},
		&acceptance.AcceptanceRecord{},
		&cleaningtask.TeamAssignment{},
		&cleaningtask.TeamWorkloadReport{},
		&cleaningtask.TeamWorkloadSnapshot{},
	)
}

// SeedTeamAssignments 为升级前已存在的任务补齐初始派工记录。
//
// 派工记录表启用后，新建任务会在同一事务中写入初始记录；这里只处理历史任务。
func SeedTeamAssignments(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var taskIDs []uint
		if err := tx.Model(&cleaningtask.CleaningTask{}).
			Where("id NOT IN (?)",
				tx.Model(&cleaningtask.TeamAssignment{}).Select("task_id"),
			).
			Pluck("id", &taskIDs).Error; err != nil {
			return err
		}
		if len(taskIDs) == 0 {
			return nil
		}

		type taskTeam struct {
			ID       uint
			TeamName string
		}
		tasks := make([]taskTeam, 0, len(taskIDs))
		if err := tx.Model(&cleaningtask.CleaningTask{}).
			Select("id, team_name").
			Where("id IN ?", taskIDs).
			Scan(&tasks).Error; err != nil {
			return err
		}

		assignments := make([]cleaningtask.TeamAssignment, 0, len(tasks))
		for _, task := range tasks {
			teamName := task.TeamName
			if teamName == "" {
				teamName = "未指定班组"
			}
			assignments = append(assignments, cleaningtask.TeamAssignment{
				TaskID:        task.ID,
				Sequence:      1,
				TeamName:      teamName,
				ChangeType:    cleaningtask.AssignmentInitial,
				EffectiveDate: cleaningtask.InitialAssignmentDate,
				Reason:        "历史任务初始派工迁移",
			})
		}
		if len(assignments) == 0 {
			return nil
		}
		return tx.Create(&assignments).Error
	})
}
