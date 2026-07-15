package service

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// 结果反馈的四类语义。误报标注只能来自真实用户，是校准规则表的唯一现场信号，
// 分类固定为枚举，便于运营侧按类聚合统计误报率/漏检率。
const (
	FeedbackFalsePositive   = "false_positive"   // 我是正常网络，被误判
	FeedbackMissedDetection = "missed_detection" // 在用代理/VPN，但没检出
	FeedbackDataWrong       = "data_wrong"       // 位置/运营商/ASN 等数据不对
	FeedbackOther           = "other"
)

// feedbackNoteMaxRunes 备注字数上限（按 Unicode 码点计，中文一字算一个）
const feedbackNoteMaxRunes = 500

// ValidFeedbackCategory 校验反馈分类是否在枚举内
func ValidFeedbackCategory(category string) bool {
	switch category {
	case FeedbackFalsePositive, FeedbackMissedDetection, FeedbackDataWrong, FeedbackOther:
		return true
	}
	return false
}

// NormalizeFeedbackNote 归一化用户备注：剔除控制字符（制表/换行归并为空格），去首尾空白，
// 再按码点数校验 ≤ feedbackNoteMaxRunes。超长返回 ok=false（交由 handler 判 400）。
func NormalizeFeedbackNote(note string) (string, bool) {
	var b strings.Builder
	for _, r := range note {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r):
			// 丢弃其余控制字符
		default:
			b.WriteRune(r)
		}
	}
	cleaned := strings.TrimSpace(b.String())
	if utf8.RuneCountInString(cleaned) > feedbackNoteMaxRunes {
		return "", false
	}
	return cleaned, true
}
