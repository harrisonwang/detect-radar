package handler

import (
	"detect-radar/internal/model"
	"detect-radar/internal/service"
	"detect-radar/internal/util"
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

func projectScanResponse(resp *model.ScanResponse, lang string) *model.ScanResponse {
	if resp == nil {
		return nil
	}
	cp := *resp
	cp.ServerData.IPIntel = service.ProjectIPIntel(resp.ServerData.IPIntel, lang)
	return &cp
}

func (h *ScanHandler) CreateScan(c *fiber.Ctx) error {
	var req model.ScanRequest
	if err := c.BodyParser(&req); err != nil {
		return util.InvalidParameter(c, util.Localized(c, "请求内容无效："+err.Error(), "Invalid request body: "+err.Error()))
	}

	if req.ScanID != "" && !util.IsValidScanID(req.ScanID) {
		return util.InvalidParameter(c, util.Localized(c, "scan_id 格式非法（仅允许字母/数字/-/_，≤64 字符）", "Invalid scan_id format (alphanumeric, -, _, max 64 chars)"))
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
	return c.Status(fiber.StatusCreated).JSON(projectScanResponse(result, util.ResolveLang(c)))
}

func (h *ScanHandler) GetScan(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return util.MissingParameter(c, util.Localized(c, "缺少扫描标识", "Missing scan_id"))
	}
	result, ok := h.service.GetWithScenario(id, c.Query("scenario"))
	if !ok {
		return util.ResourceNotFound(c, util.Localized(c, "扫描 '"+id+"' 不存在或已过期", "Scan '"+id+"' not found or expired"))
	}
	return c.JSON(projectScanResponse(result, util.ResolveLang(c)))
}

func ambiguousScan(c *fiber.Ctx) error {
	return util.Error(
		c,
		fiber.StatusConflict,
		util.Localized(c, "扫描标识冲突", "Scan ID Conflict"),
		util.Localized(c, "该扫描标识已被重复使用，无法关联 DNS 结果，请重新检测", "This scan_id has already been used and cannot be bound to DNS results; please re-scan"),
	)
}

func (h *ScanHandler) SubmitFeedback(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" || !util.IsValidScanID(id) {
		return util.InvalidParameter(c, util.Localized(c, "scan_id 格式非法", "Invalid scan_id format"))
	}
	if len(c.Body()) > 4096 {
		return util.InvalidParameter(c, util.Localized(c, "请求体过大", "Request body too large"))
	}
	var body struct {
		Category   string `json:"category"`
		Note       string `json:"note"`
		ReplyEmail string `json:"reply_email"`
	}
	if err := c.BodyParser(&body); err != nil {
		return util.InvalidParameter(c, util.Localized(c, "请求内容无效："+err.Error(), "Invalid request body: "+err.Error()))
	}
	if !service.ValidFeedbackCategory(body.Category) {
		return util.InvalidParameter(c, util.Localized(c, "category 非法（须为 false_positive/missed_detection/data_wrong/other）", "Invalid category (must be false_positive, missed_detection, data_wrong, or other)"))
	}
	note, ok := service.NormalizeFeedbackNote(body.Note)
	if !ok {
		return util.InvalidParameter(c, util.Localized(c, "note 过长（≤500 字）", "Note is too long (max 500 characters)"))
	}
	replyEmail := strings.TrimSpace(body.ReplyEmail)
	if replyEmail != "" && !util.IsValidEmailAddress(replyEmail) {
		return util.InvalidParameter(c, util.Localized(c, "联系邮箱格式无效，请填写单个邮箱地址", "Invalid email format; please provide a single email address"))
	}

	resp, _ := h.service.Get(id)
	if err := h.journal.FeedbackEvent(id, body.Category, note, replyEmail, resp); err != nil {
		return storageUnavailable(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func storageUnavailable(c *fiber.Ctx, err error) error {
	log.Printf("[遥测] 请求 %s 保存失败：%v", c.Path(), err)
	return util.RFC7807Error(
		c,
		util.ProblemTypeServiceUnavailable,
		util.Localized(c, "服务暂不可用", "Service Unavailable"),
		fiber.StatusServiceUnavailable,
		util.Localized(c, "暂时无法保存结果，请稍后重试", "Temporarily unable to save results, please try again later"),
	)
}
