import type { Locale } from '../i18n';

export interface FAQItem {
	question: string;
	answer: string;
}

export const ABOUT_FAQS_ZH: FAQItem[] = [
	{
		question: '什么是 DNS 泄露？测出阿里 DNS 或 Cloudflare 1.1.1.1 算泄露吗？',
		answer:
			'DNS 泄露是指网络请求已切到代理或海外出口，但域名解析却绕过出口发给了跨地区或本机宽带/移动运营商的本地 DNS。如果测出的是阿里 DNS、腾讯 DNSPod、Cloudflare、Google、Quad9、OpenDNS、NextDNS、AdGuard 等公共 DNS 或常规机房 DNS，DetectRadar 会将其归为公共或常规解析服务，不判为运营商泄露。',
	},
	{
		question: '机房 IP 和住宅 IP（普通宽带 IP）有什么区别？怎么看当前是什么类型？',
		answer:
			'住宅 IP 是普通家庭或办公宽带用的地址，机房 IP 则是云服务器或数据中心的地址。因为机房 IP 经常多人共用或被批量租用，在登录账号、付款或风控较严的网站上更容易弹验证码或被拦截。在 DetectRadar 首页的「网络身份」卡片里，可以直接看机房或普通网络标签、运营商、ASN 及 Cloudflare Radar 近 30 天网段人类流量占比，同时核对有没有被列入公开黑名单或开放常见代理端口。',
	},
	{
		question: '什么是 WebRTC 泄露和 IPv6 泄露？为什么开了代理还会暴露真实地址？',
		answer:
			'WebRTC 是浏览器自带的音视频直连功能，有时会绕开代理直接读出本机真实的公网或内网 IP；IPv6 泄露则通常是因为宽带同时开了 IPv4 和 IPv6，但代理只接管了 IPv4，结果 IPv6 流量直接走了本地宽带。可以分别打开 DetectRadar 的 WebRTC 泄露检测页和 IPv6 泄露检测页，逐项查看两条线路的实际出口。',
	},
	{
		question: '为什么出口 IP 已经在海外，网站还是能识别出环境异常？',
		answer:
			'很多网站不只看出口 IP 在哪，还会同时核对浏览器时区、常用语言、DNS 归属地、WebRTC 地址，以及显卡和操作系统是否相符。比如出口 IP 在美国，浏览器却还是东八区时间和中文语言，一眼就会露馅。DetectRadar 会把这些项目汇总在同一份报告里，方便快速找出哪一项没对上。',
	},
	{
		question: '什么是浏览器指纹检测？在线检测会泄露或保存我的个人隐私吗？',
		answer:
			'浏览器指纹是指网页通过读取绘图（Canvas）、音频接口（Audio）、显卡型号（WebGL）以及语言、时区等细节拼出的环境特征；DetectRadar 用这些特征来检查浏览器接口有没有被改写、显卡与系统是否相符、有没有带自动化脚本标记。检测时，Canvas 绘图结果在浏览器本地算成不可逆哈希后再提交（不上传原始画面），Audio 仅核对原生接口是否被改写，WebGL 仅提交显卡渲染器型号用于核对操作系统，不会用来追踪个人身份；分享截图也在本地生成并自动隐藏完整 IP。',
	},
];

export const ABOUT_FAQS_EN: FAQItem[] = [
	{
		question: 'What is a DNS leak? Do Cloudflare 1.1.1.1 or Google 8.8.8.8 count as leaks?',
		answer:
			'A DNS leak happens when your traffic routes through a proxy or VPN, yet DNS lookups bypass the tunnel and go to a cross-region or local residential/mobile ISP resolver. Public DNS resolvers (Cloudflare, Google, Quad9, OpenDNS, NextDNS, AdGuard, AliDNS, DNSPod, etc.) and datacenter resolvers are exempt and are not flagged as ISP leaks.',
	},
	{
		question: 'What is the difference between a datacenter IP and a residential IP?',
		answer:
			'Residential IPs belong to consumer broadband or mobile ISPs, while datacenter IPs belong to cloud or hosting providers. Because datacenter ranges are often shared or automated, strict sites may trigger extra challenges. On the DetectRadar home page, the Network Identity card displays ISP vs. datacenter classification, ASN, ISP/organization, Cloudflare Radar 30-day ASN human traffic ratio, public blocklists, and open proxy ports.',
	},
	{
		question: 'What are WebRTC leaks and IPv6 leaks?',
		answer:
			'WebRTC can establish direct peer-to-peer connections (via STUN/ICE) that reveal local or real public IP addresses outside a browser proxy. An IPv6 leak occurs on dual-stack networks when a proxy only tunnels IPv4, allowing IPv6 traffic to exit directly through your local ISP. Use the dedicated WebRTC Leak Test and IPv6 Leak Test pages to inspect each stack independently.',
	},
	{
		question: 'Why do sites flag an environment even when the exit IP looks clean?',
		answer:
			'Sites often cross-check your exit IP against browser timezone, preferred language, DNS resolvers, WebRTC candidates, and WebGL renderer vs. OS consistency. DetectRadar checks these signals together in one report so mismatches are easy to spot.',
	},
	{
		question: 'Does the browser fingerprint check expose or store personal data?',
		answer:
			'Canvas rendering output is hashed locally in your browser into an irreversible hash before submission (raw pixels are never uploaded). Audio checks inspect only whether native AudioContext methods are overridden, and WebGL submits only the GPU renderer string to verify consistency with your declared OS. Shareable text summaries and PNG cards are rendered locally with full IP addresses masked automatically.',
	},
];

export const ABOUT_FAQS = ABOUT_FAQS_ZH;

export function getAboutFaqs(locale: Locale = 'zh-CN'): FAQItem[] {
	return locale === 'en' ? ABOUT_FAQS_EN : ABOUT_FAQS_ZH;
}
