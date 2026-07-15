package handler

import (
	"detect-radar/internal/model"
	"detect-radar/internal/service"
	"detect-radar/internal/util"

	"github.com/gofiber/fiber/v2"
)

type ScanHandler struct {
	service *service.ScanService
	journal *service.Journal
}

func NewScanHandler(s *service.ScanService, journal *service.Journal) *ScanHandler {
	return &ScanHandler{service: s, journal: journal}
}

// CreateScan POST /scans
// 客户端提交采集到的全部信号，服务端汇总出口 IP 信息、一致性、泄露和指纹，返回统一 verdict
func (h *ScanHandler) CreateScan(c *fiber.Ctx) error {
	var req model.ScanRequest
	if err := c.BodyParser(&req); err != nil {
		return util.InvalidParameter(c, "Invalid request body: "+err.Error())
	}

	// scan_id 若提供须合法（用于 GET /scans/:id 取回，且作为内存 store 的 key）
	if req.ScanID != "" && !util.IsValidScanID(req.ScanID) {
		return util.InvalidParameter(c, "scan_id 格式非法（仅允许字母/数字/-/_，≤64 字符）")
	}

	// 出口 IP 恒取自连接本身（浏览器真实出口 IP），不接受调用方指定——既避免把判定结果
	// 当任意 IP 的免费查询接口，也防止 RTT 遥测里 server_tcp_ms（量的是请求方连接）与
	// intel（描述被查 IP）张冠李戴、污染 Phase 2 标定。反代后须配 trusted_proxies + proxy_header。
	ip := c.IP()

	// nginx 透传的「出口 IP↔服务器」TCP RTT（µs 整数字符串；本地无 nginx 时为空）。
	// nginx 环回反代到 127.0.0.1:8080，Go 侧读 TCP_INFO 只会得到 ~0，真值只有 nginx 看得到。
	//
	// 仅当对端属于 trusted_proxies（即请求确实经 nginx 而来）才采信：这两个头是客户端可写的，
	// 直连后端即可伪造。生产虽有 nginx 的 proxy_set_header 覆写与防火墙兜底，但 RTT 数据要用于
	// 标定检测阈值，被污染的样本会直接毒化标定结果，故在此加一道信任门。
	var tcpRTT, tcpRTTVar string
	if c.IsProxyTrusted() {
		tcpRTT, tcpRTTVar = c.Get("X-TCP-RTT"), c.Get("X-TCP-RTTVAR")
	}
	result := h.service.Analyze(c.Context(), &req, ip, c.Protocol(), tcpRTT, tcpRTTVar)
	h.journal.ScanEvent(result, c.Get("User-Agent"))
	return c.Status(fiber.StatusCreated).JSON(result)
}

// GetScan GET /scans/:id
func (h *ScanHandler) GetScan(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return util.MissingParameter(c, "scan id is required")
	}
	result, ok := h.service.Get(id)
	if !ok {
		return util.ResourceNotFound(c, "scan '"+id+"' not found or has expired.")
	}
	return c.JSON(result)
}

// UpdateDNS POST /scans/:id/dns
// 异步 DNS 泄露结果回传后端，更新该扫描的 DNS 判定并重新评分（评分始终纯后端产出）。
func (h *ScanHandler) UpdateDNS(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" || !util.IsValidScanID(id) {
		return util.InvalidParameter(c, "scan_id 格式非法")
	}
	var body struct {
		Leaked bool `json:"leaked"`
	}
	if err := c.BodyParser(&body); err != nil {
		return util.InvalidParameter(c, "Invalid request body: "+err.Error())
	}
	result, ok := h.service.UpdateDNSLeak(id, body.Leaked)
	if !ok {
		return util.ResourceNotFound(c, "scan '"+id+"' not found or has expired.")
	}
	return c.JSON(result)
}

// SubmitFeedback POST /scans/:id/feedback
// 结果反馈通道：用户是误报/漏检标注的唯一来源，反馈落遥测流水（feedback 事件）。
// 扫描仍在内存 store（30 分钟内）时附带当时评分现场；过期也照收——过期扫描的反馈
// 仍是有效数据，靠 scan_id 与先前 scan 行关联。校验通过返回 204，不回显任何内容。
func (h *ScanHandler) SubmitFeedback(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" || !util.IsValidScanID(id) {
		return util.InvalidParameter(c, "scan_id 格式非法")
	}
	// 反馈体都是极小 JSON，超过 2KB 视为异常输入直接拒绝（全局 BodyLimit 之外的收紧）
	if len(c.Body()) > 2048 {
		return util.InvalidParameter(c, "请求体过大")
	}
	var body struct {
		Category string `json:"category"`
		Note     string `json:"note"`
	}
	if err := c.BodyParser(&body); err != nil {
		return util.InvalidParameter(c, "Invalid request body: "+err.Error())
	}
	if !service.ValidFeedbackCategory(body.Category) {
		return util.InvalidParameter(c, "category 非法（须为 false_positive/missed_detection/data_wrong/other）")
	}
	note, ok := service.NormalizeFeedbackNote(body.Note)
	if !ok {
		return util.InvalidParameter(c, "note 过长（≤500 字）")
	}

	// 扫描仍在内存 store 时取回作为评分现场；已过期 Get 返回 nil，反馈照落
	resp, _ := h.service.Get(id)
	h.journal.FeedbackEvent(id, body.Category, note, resp)
	return c.SendStatus(fiber.StatusNoContent)
}
