package team

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Register 注册班组工作量路由，并返回 service 供改派封账保护装配使用。
func Register(router fiber.Router, db *gorm.DB) *Service {
	svc := NewService(NewRepository(db))
	handler := NewHandler(svc)

	group := router.Group("/team-workload")
	group.Get("", handler.Workload)
	group.Get("/reports", handler.Reports)
	group.Post("/reports", handler.Publish)

	return svc
}
