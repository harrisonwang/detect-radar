
(function () {
  var V4_TIMEOUT_MS = 6000;
  var V6_HARD_TIMEOUT_MS = 2500;
  var V6_AFTER_V4_MS = 1200;
  var HK_MO_TW_RE = /香港|澳门|澳門|台湾|臺灣|hong\s*kong|macau|macao|taiwan/i;

  var isLoon = typeof $loon !== 'undefined';
  var isStash =
    !isLoon &&
    typeof $environment !== 'undefined' &&
    Boolean($environment && $environment['stash-version']);
  var clientTag = isLoon ? 'loon' : isStash ? 'stash' : 'surge';

  function parseArgument(raw) {
    var out = {};
    if (typeof raw !== 'string' || !raw.trim()) return out;
    var parts = raw.split('&');
    for (var i = 0; i < parts.length; i++) {
      var item = parts[i];
      var eq = item.indexOf('=');
      if (eq <= 0) continue;
      var k = item.slice(0, eq).trim();
      var v = item.slice(eq + 1).trim();
      try {
        k = decodeURIComponent(k);
        v = decodeURIComponent(v);
      } catch (_) {}
      if (k) out[k] = v;
    }
    return out;
  }

  function parseBool(val, fallback) {
    if (val === undefined || val === null || val === '') return fallback;
    var s = String(val).trim().toLowerCase();
    if (s === '1' || s === 'true' || s === 'yes') return true;
    if (s === '0' || s === 'false' || s === 'no') return false;
    return fallback;
  }

  function resolveLoonContext() {
    if (!isLoon || typeof $environment === 'undefined' || !$environment || !$environment.params) {
      return { displayName: '', routeNode: '' };
    }
    var params = $environment.params;
    var infoName = '';
    if (params.nodeInfo && typeof params.nodeInfo === 'object' && params.nodeInfo.name) {
      infoName = String(params.nodeInfo.name).trim();
    } else if (typeof params.nodeInfo === 'string') {
      infoName = params.nodeInfo.trim();
    }
    var rawNode = '';
    if (typeof params.node === 'string') {
      rawNode = params.node.trim();
    } else if (params.node && typeof params.node === 'object' && params.node.name) {
      rawNode = String(params.node.name).trim();
    }
    return {
      displayName: infoName || rawNode,
      routeNode: rawNode || infoName,
    };
  }

  var args = parseArgument(typeof $argument !== 'undefined' ? $argument : '');
  var loonCtx = resolveLoonContext();
  var customTitle = (args.title || '').trim();
  var panelTitle =
    customTitle ||
    (isLoon && loonCtx.displayName ? 'DetectRadar · ' + loonCtx.displayName : 'DetectRadar 出口');
  var enableIPv6 = parseBool(args.ipv6, true);
  var hideIP = parseBool(args.hide_ip, false);

  function maskIP(ip) {
    var s = String(ip || '').trim();
    if (!s) return '未知';
    if (s.indexOf(':') !== -1) {
      var v6Parts = s.split(':').filter(Boolean);
      return (v6Parts.slice(0, 2).join(':') || '::') + ':*';
    }
    var v4Parts = s.split('.');
    if (v4Parts.length === 4) {
      return v4Parts[0] + '.' + v4Parts[1] + '.*.*';
    }
    return s;
  }

  function displayAddress(ip) {
    return hideIP ? maskIP(ip) : String(ip || '').trim() || '未知';
  }

  function isHosting(info) {
    if (!info) return false;
    return Boolean(
      info.is_hosting ||
        info.usage_type === 'hosting' ||
        info.usage_type_raw === 'datacenter'
    );
  }

  function isMainlandChina(info) {
    if (!info) return false;
    var cc = String(info.country || '').trim().toUpperCase();
    if (cc !== 'CN') return false;
    var haystack = [info.region, info.country_name, info.city].filter(Boolean).join(' ');
    return !HK_MO_TW_RE.test(haystack);
  }

  function effectiveCountryCode(info) {
    if (!info) return '';
    var cc = String(info.country || '').trim().toUpperCase();
    if (!cc || cc === 'LOCAL') return '';
    if (cc === 'CN') {
      var haystack = [info.region, info.country_name, info.city].filter(Boolean).join(' ');
      if (/香港|hong\s*kong/i.test(haystack)) return 'HK';
      if (/澳门|澳門|macau|macao/i.test(haystack)) return 'MO';
      if (/台湾|臺灣|taiwan/i.test(haystack)) return 'TW';
    }
    return /^[A-Z]{2}$/.test(cc) ? cc : '';
  }

  function countryFlag(code) {
    if (!code || !/^[A-Z]{2}$/.test(code)) return '';
    return String.fromCodePoint(
      0x1f1e6 + code.charCodeAt(0) - 65,
      0x1f1e6 + code.charCodeAt(1) - 65
    );
  }

  function formatLocation(info) {
    if (!info) return '未知位置';
    if (info.usage_type === 'private' || info.country === 'LOCAL') {
      return '内网地址 (LOCAL)';
    }
    var rawParts = [info.country_name || info.country, info.region, info.city];
    var seen = {};
    var parts = [];
    for (var i = 0; i < rawParts.length; i++) {
      var p = String(rawParts[i] || '').trim();
      if (!p) continue;
      var k = p.toLowerCase();
      if (seen[k]) continue;
      seen[k] = true;
      parts.push(p);
    }
    var loc = parts.join(' · ') || '未知位置';
    var flag = countryFlag(effectiveCountryCode(info));
    return flag ? flag + ' ' + loc : loc;
  }

  function normalizeASN(raw) {
    var s = String(raw || '').trim();
    if (!s) return '';
    if (/^\d+$/.test(s)) return 'AS' + s;
    if (/^as\d+$/i.test(s)) return 'AS' + s.slice(2);
    return s;
  }

  function formatOperator(info) {
    if (!info) return '未知';
    var op = String(info.isp || info.org || '').trim() || '未知';
    var asn = normalizeASN(info.asn);
    return asn ? op + ' (' + asn + ')' : op;
  }

  function formatTypeLine(info) {
    var base;
    if (!info) {
      base = '类型未定';
    } else if (info.usage_type === 'private' || info.country === 'LOCAL') {
      base = '内网地址';
    } else if (isHosting(info)) {
      base = '机房 IP';
    } else if (info.usage_type === 'isp') {
      base = info.is_mobile ? '移动网络' : '普通宽带';
    } else {
      base = '类型未定';
    }

    var tags = [];
    if (info && info.is_vpn) tags.push('VPN');
    if (info && info.is_proxy) tags.push('代理');
    if (info && info.is_tor) tags.push('Tor');
    if (info && info.is_relay) tags.push('Relay');
    if (!tags.length) return base;
    return base + ' [' + tags.join(' / ') + ']';
  }

  function escapeHTML(str) {
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function buildStatusStyle(primary, ipv6Leaked) {
    var hasProxyFlag = Boolean(
      primary &&
        (primary.is_vpn ||
          primary.is_proxy ||
          primary.is_tor ||
          primary.is_relay ||
          primary.is_anonymous)
    );
    if (ipv6Leaked || hasProxyFlag) {
      return { color: '#EF4444', icon: 'exclamationmark.triangle.fill', label: '需留意' };
    }
    if (isHosting(primary)) {
      return { color: '#F59E0B', icon: 'server.rack', label: '机房 IP' };
    }
    if (primary && primary.usage_type === 'isp') {
      return { color: '#22C55E', icon: 'checkmark.shield.fill', label: primary.is_mobile ? '移动网络' : '普通宽带' };
    }
    return { color: '#64748B', icon: 'network', label: '类型未定' };
  }

  function renderResult(v4, v6) {
    if (!v4 && !v6) {
      var errMsg = '检测失败：请检查当前节点连通性';
      if (isLoon) {
        return {
          title: panelTitle,
          htmlMessage:
            '<div style="font-family:-apple-system,sans-serif;font-size:14px;line-height:1.6;padding:8px 4px;">' +
            '<div style="color:#EF4444;font-weight:600;">' +
            escapeHTML(errMsg) +
            '</div>' +
            '<div style="margin-top:8px;font-size:12px;opacity:.65;">detectradar.com</div>' +
            '</div>',
        };
      }
      if (isStash) {
        return {
          title: panelTitle,
          content: errMsg,
          icon: 'exclamationmark.triangle.fill',
          backgroundColor: '#EF4444',
          url: 'https://detectradar.com/',
        };
      }
      return {
        title: panelTitle,
        content: errMsg,
        icon: 'exclamationmark.triangle.fill',
        'icon-color': '#EF4444',
      };
    }

    var primary = v4 || v6;
    var ipv6Leaked = Boolean(
      v4 && v6 && !isMainlandChina(v4) && isMainlandChina(v6) && !isHosting(v6)
    );
    var style = buildStatusStyle(primary, ipv6Leaked);

    var lines = [
      '出口 IP: ' + displayAddress(primary.ip),
      '地区: ' + formatLocation(primary),
      '运营商: ' + formatOperator(primary),
      '类型: ' + formatTypeLine(primary),
    ];

    if (typeof primary.human_ratio === 'number' && isFinite(primary.human_ratio)) {
      var pct = Math.max(0, Math.min(100, primary.human_ratio)).toFixed(1);
      lines.push('人类流量: ' + pct + '%（网段近30天）');
    }

    if (enableIPv6) {
      if (v4 && v6) {
        var v6Addr = displayAddress(v6.ip);
        if (ipv6Leaked) {
          lines.push('IPv6: 泄露（经国内宽带直连 ' + v6Addr + '）');
        } else {
          lines.push('IPv6: ' + v6Addr + '（未泄露）');
        }
      } else if (v4) {
        lines.push('IPv6: 未连通');
      } else {
        lines.push('IPv4: 未连通（仅 IPv6 出口）');
      }
    }

    if (isLoon) {
      var htmlLines = '';
      for (var i = 0; i < lines.length; i++) {
        htmlLines += '<div style="margin:4px 0;">' + escapeHTML(lines[i]) + '</div>';
      }
      return {
        title: panelTitle,
        htmlMessage:
          '<div style="font-family:-apple-system,BlinkMacSystemFont,sans-serif;font-size:14px;line-height:1.55;padding:6px 2px;">' +
          '<div style="display:flex;align-items:center;gap:6px;margin-bottom:8px;font-weight:600;color:' +
          style.color +
          ';">' +
          '<span>●</span><span>' +
          escapeHTML(ipv6Leaked ? '发现 IPv6 泄露' : style.label) +
          '</span></div>' +
          htmlLines +
          '<div style="margin-top:10px;font-size:12px;opacity:.6;">完整诊断（DNS / WebRTC / 指纹）：detectradar.com</div>' +
          '</div>',
      };
    }

    var textContent = lines.join('\n');
    if (isStash) {
      return {
        title: panelTitle,
        content: textContent,
        icon: style.icon,
        backgroundColor: style.color,
        url: 'https://detectradar.com/',
      };
    }

    return {
      title: panelTitle,
      content: textContent,
      icon: style.icon,
      'icon-color': style.color,
    };
  }

  var timers = [];
  function trackTimer(t) {
    timers.push(t);
    return t;
  }
  function clearAllTimers() {
    for (var i = 0; i < timers.length; i++) {
      clearTimeout(timers[i]);
    }
    timers = [];
  }

  function fetchIP(url, timeoutMs, cb) {
    var doneFlag = false;
    var timer = trackTimer(setTimeout(function () {
      if (doneFlag) return;
      doneFlag = true;
      cb(null);
    }, timeoutMs));

    var opts = {
      url: url,
      timeout: Math.max(1, Math.ceil(timeoutMs / 1000)),
    };
    if (isLoon && loonCtx.routeNode) {
      opts.node = loonCtx.routeNode;
    }

    try {
      $httpClient.get(opts, function (error, response, data) {
        if (doneFlag) return;
        doneFlag = true;
        clearTimeout(timer);

        var status =
          response && (response.status || response.statusCode || 200);
        if (error || status < 200 || status >= 300 || !data) {
          cb(null);
          return;
        }
        try {
          var parsed = typeof data === 'string' ? JSON.parse(data) : data;
          if (!parsed || !parsed.ip) {
            cb(null);
            return;
          }
          cb(parsed);
        } catch (_) {
          cb(null);
        }
      });
    } catch (_) {
      if (doneFlag) return;
      doneFlag = true;
      clearTimeout(timer);
      cb(null);
    }
  }

  var v4Done = false;
  var v6Done = !enableIPv6;
  var v4Result = null;
  var v6Result = null;
  var finished = false;

  function maybeFinish() {
    if (finished) return;
    if (!v4Done || !v6Done) return;
    finished = true;
    clearAllTimers();
    $done(renderResult(v4Result, v6Result));
  }

  fetchIP('https://4.detectradar.com/api/v1/ip?from=' + clientTag, V4_TIMEOUT_MS, function (res) {
    v4Done = true;
    v4Result = res;
    if (!v6Done) {
      trackTimer(setTimeout(function () {
        v6Done = true;
        maybeFinish();
      }, V6_AFTER_V4_MS));
    }
    maybeFinish();
  });

  if (enableIPv6) {
    fetchIP('https://6.detectradar.com/api/v1/ip?from=' + clientTag, V6_HARD_TIMEOUT_MS, function (res) {
      v6Done = true;
      v6Result = res;
      maybeFinish();
    });
  }
})();
