export interface FAQItem {
	question: string;
	answer: string;
}

export const ABOUT_FAQS: FAQItem[] = [
	{
		question: '什么是 DNS 泄露？测出阿里 DNS 或 Cloudflare 1.1.1.1 算泄露吗？',
		answer:
			'DNS 泄露是指网络出口已经在海外，但域名解析请求却发给了中国大陆宽带运营商的本地 DNS。如果测出的是阿里 DNS、腾讯 DNSPod、Cloudflare、Google、AdGuard 等常见公共 DNS 或常规机房 DNS，DetectRadar 会将其归为公共或常规解析服务，不判为国内运营商泄露。',
	},
	{
		question: '机房 IP 和住宅 IP（普通宽带 IP）有什么区别？怎么看当前是什么类型？',
		answer:
			'住宅 IP 是普通家庭或办公宽带用的地址，机房 IP 则是云服务器或数据中心的地址。因为机房 IP 经常多人共用或被批量租用，在登录账号、付款或风控较严的网站上更容易弹验证码或被拦截。在 DetectRadar 首页的「网络身份」卡片里，可以直接看机房或普通网络标签、运营商、ASN 及 Cloudflare Radar 近 30 天网段人类流量占比，同时核对有没有被列入公开黑名单或开放常见代理端口。',
	},
	{
		question: '什么是 WebRTC 泄露和 IPv6 泄露？为什么开了代理还会暴露真实地址？',
		answer:
			'WebRTC 是浏览器自带的音视频直连功能，有时会绕开代理直接读出本机真实的公网或内网 IP；IPv6 泄露则通常是因为宽带同时开了 IPv4 和 IPv6，但代理只接管了 IPv4，结果 IPv6 流量直接走了国内宽带。可以分别打开 DetectRadar 的 WebRTC 泄露检测页和 IPv6 泄露检测页，逐项查看两条线路的实际出口。',
	},
	{
		question: '为什么出口 IP 已经在海外，网站还是能识别出环境异常？',
		answer:
			'很多网站不只看出口 IP 在哪，还会同时核对浏览器时区、常用语言、DNS 归属地、WebRTC 地址，以及显卡和操作系统是否相符。比如出口 IP 在美国，浏览器却还是东八区时间和中文语言，一眼就会露馅。DetectRadar 会把这些项目汇总在同一份报告里，方便快速找出哪一项没对上。',
	},
	{
		question: '什么是浏览器指纹检测？在线检测会泄露或保存我的个人隐私吗？',
		answer:
			'浏览器指纹是指网页通过读取绘图（Canvas）、音频（Audio）、显卡渲染（WebGL）以及语言、时区等细节拼出的环境特征；DetectRadar 用这些特征来检查浏览器有没有被插件改写、有没有带自动化脚本标记。检测时，绘图、音频、渲染结果和扩展列表都在浏览器本地算成无法还原原样的匿名哈希后再提交（不上传原始画面和插件明细，仅显卡型号原文核对系统），不会用来追踪个人身份；分享截图也在本地生成并自动隐藏完整 IP。',
	},
];
