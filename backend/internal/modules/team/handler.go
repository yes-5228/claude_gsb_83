package team

import (
	"github.com/gofiber/fiber/v2"

	"github.com/drainage/desilting/internal/httpx"
)

// Handler 班组工作量 HTTP 接口。
type Handler struct {
	svc *Service
}

// NewHandler 构造处理器。
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Workload 某月班组工作量统计。
func (h *Handler) Workload(c *fiber.Ctx) error {
	resp, err := h.svc.Workload(c.UserContext(), c.Query("month"))
	if err != nil {
		return err
	}
	return httpx.OK(c, resp)
}

// Reports 已出具的月报清单。
func (h *Handler) Reports(c *fiber.Ctx) error {
	items, err := h.svc.ListReports(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, items)
}

// Publish 出具（封账）某月工作量月报。
func (h *Handler) Publish(c *fiber.Ctx) error {
	var req PublishMonthRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	report, err := h.svc.Publish(c.UserContext(), req)
	if err != nil {
		return err
	}
	return httpx.Message(c, report.Month+" 月班组工作量月报已出具并封账", report)
}
