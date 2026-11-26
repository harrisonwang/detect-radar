package handler

import (
	"detect-radar/internal/model"
	"detect-radar/internal/service"
	"detect-radar/internal/util"
	"errors"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type ScanHandler struct {
	service *service.ScanService
	journal *service.Journal
}

func NewScanHandler(s *service.ScanService, journal *service.Journal) *ScanHandler {
	return &ScanHandler{service: s, journal: journal}
}

func (h *ScanHandler) CreateScan(c *fiber.Ctx) error {
	var req model.ScanRequest
	if err := c.BodyParser(&req); err != nil {
		return util.InvalidParameter(c, "请求内容无效："+err.Error())
	}

	if req.ScanID != "" && !util.IsValidScanID(req.ScanID) {
		return util.InvalidParameter(c, "scan_id 格式非法（仅允许字母/数字/-/_，≤64 字符）")
	}

	ip := c.IP()

	var tcpRTT, tcpRTTVar string
	if c.IsProxyTrusted() {
		tcpRTT, tcpRTTVar = c.Get("X-TCP-RTT"), c.Get("X-TCP-RTTVAR")
	}
	result, err := h.service.Analyze(c.Context(), &req, ip, c.Protocol(), tcpRTT, tcpRTTVar, c.Get("User-Agent"))
	if err != nil {
		return storageUnavailable(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(result)
}

func (h *ScanHandler) GetScan(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return util.MissingParameter(c, "缺少扫描标识")
	}
	result, ok := h.service.GetWithScenario(id, c.Query("scenario"))
	if !ok {
		return util.ResourceNotFound(c, "扫描 '"+id+"' 不存在或已过期")
	}
	return c.JSON(result)
}

func (h *ScanHandler) UpdateDNS(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" || !util.IsValidScanID(id) {
		return util.InvalidParameter(c, "scan_id 格式非法")
	}
	var body struct {
		Leaked bool `json:"leaked"`
	}
	if err := c.BodyParser(&body); err != nil {
		return util.InvalidParameter(c, "请求内容无效："+err.Error())
	}
	result, ok, err := h.service.UpdateDNSLeak(id, body.Leaked)
	if errors.Is(err, service.ErrAmbiguousScan) {
		return ambiguousScan(c)
	}
	if err != nil {
		return storageUnavailable(c, err)
	}
	if !ok {
		return util.ResourceNotFound(c, "扫描 '"+id+"' 不存在或已过期")
	}
	return c.JSON(result)
}

func ambiguousScan(c *fiber.Ctx) error {
	return util.Error(c, fiber.StatusConflict, "扫描标识冲突", "该扫描标识已被重复使用，无法关联 DNS 结果，请重新检测")
}

func (h *ScanHandler) SubmitFeedback(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" || !util.IsValidScanID(id) {
		return util.InvalidParameter(c, "scan_id 格式非法")
	}
	if len(c.Body()) > 4096 {
		return util.InvalidParameter(c, "请求体过大")
	}
	var body struct {
		Category   string `json:"category"`
		Note       string `json:"note"`
		ReplyEmail string `json:"reply_email"`
	}
	if err := c.BodyParser(&body); err != nil {
		return util.InvalidParameter(c, "请求内容无效："+err.Error())
	}
	if !service.ValidFeedbackCategory(body.Category) {
		return util.InvalidParameter(c, "category 非法（须为 false_positive/missed_detection/data_wrong/other）")
	}
	note, ok := service.NormalizeFeedbackNote(body.Note)
	if !ok {
		return util.InvalidParameter(c, "note 过长（≤500 字）")
	}
	replyEmail := strings.TrimSpace(body.ReplyEmail)
	if replyEmail != "" && !util.IsValidEmailAddress(replyEmail) {
		return util.InvalidParameter(c, "联系邮箱格式无效，请填写单个邮箱地址")
	}

	resp, _ := h.service.Get(id)
	if err := h.journal.FeedbackEvent(id, body.Category, note, replyEmail, resp); err != nil {
		return storageUnavailable(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func storageUnavailable(c *fiber.Ctx, err error) error {
	log.Printf("[遥测] 请求 %s 保存失败：%v", c.Path(), err)
	return util.RFC7807Error(c, util.ProblemTypeServiceUnavailable, "服务暂不可用", fiber.StatusServiceUnavailable, "暂时无法保存结果，请稍后重试")
}
