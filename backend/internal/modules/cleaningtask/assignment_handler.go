package cleaningtask

import (
	"github.com/gofiber/fiber/v2"

	"github.com/drainage/desilting/internal/httpx"
)

// Reassign 改派班组。
func (h *Handler) Reassign(c *fiber.Ctx) error {
	id, err := httpx.PathID(c, "id", "任务")
	if err != nil {
		return err
	}
	var req ReassignRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	result, err := h.svc.Reassign(c.UserContext(), id, req)
	if err != nil {
		return err
	}
	return httpx.Message(c, "任务已改派给新班组", result)
}

// TeamWorkloadReport 查询班组月度工作量。
func (h *Handler) TeamWorkloadReport(c *fiber.Ctx) error {
	month := c.Query("month")
	if month == "" {
		month = c.Query("currentMonth")
	}
	result, err := h.svc.TeamWorkloadReport(c.UserContext(), month)
	if err != nil {
		return err
	}
	return httpx.OK(c, result)
}

// PublishTeamWorkloadReport 发布班组月度工作量月报。
func (h *Handler) PublishTeamWorkloadReport(c *fiber.Ctx) error {
	var req PublishReportRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	result, err := h.svc.PublishTeamWorkloadReport(c.UserContext(), req)
	if err != nil {
		return err
	}
	return httpx.Message(c, "班组月度工作量月报已发布", result)
}
