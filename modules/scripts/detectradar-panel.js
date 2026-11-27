
(function () {
  var V4_TIMEOUT_MS = 6000;
  var V6_HARD_TIMEOUT_MS = 2500;
  var V6_AFTER_V4_MS = 1200;

  var isLoon = typeof $loon !== 'undefined';
  var isStash =
    !isLoon &&
    typeof $environment !== 'undefined' &&
    Boolean($environment && $environment['stash-version']);
  var clientTag = isLoon ? 'loon' : isStash ? 'stash' : 'surge';

  function parseArgument(raw) {
    var out = {};
    if (raw && typeof raw === 'object') {
      for (var key in raw) {
        if (Object.prototype.hasOwnProperty.call(raw, key) && key) {
          out[String(key).trim()] = String(raw[key] == null ? '' : raw[key]).trim();
        }
      }
      return out;
    }
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

  function resolveLang(args) {
    var raw = String(args.lang || '').trim().toLowerCase();
    if (raw.indexOf('en') === 0) return 'en';
    if (raw.indexOf('zh') === 0) return 'zh';
    if (
      typeof $script !== 'undefined' &&
      $script &&
      typeof $script.name === 'string' &&
      /-en$/i.test($script.name.trim())
    ) {
      return 'en';
    }
    return 'zh';
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
  var lang = resolveLang(args);
  var isEN = lang === 'en';
  var webHomeURL = isEN ? 'https://detectradar.com/en/' : 'https://detectradar.com/';
  var webHomeLabel = isEN ? 'detectradar.com/en/' : 'detectradar.com';
  var loonCtx = resolveLoonContext();
  var customTitle = (args.title || '').trim();
  var panelTitle =
    customTitle ||
    (isLoon && loonCtx.displayName
      ? 'DetectRadar · ' + loonCtx.displayName
      : isEN
        ? 'DetectRadar Exit'
        : 'DetectRadar 出口');
  var enableIPv6 = parseBool(args.ipv6, true);
  var hideIP = parseBool(args.hide_ip, false);

  function maskIP(ip) {
    var s = String(ip || '').trim();
    if (!s) return isEN ? 'Unknown' : '未知';
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
    return hideIP ? maskIP(ip) : String(ip || '').trim() || (isEN ? 'Unknown' : '未知');
  }

  function isHosting(info) {
    if (!info) return false;
    return Boolean(
      info.is_hosting ||
        info.usage_type === 'hosting' ||
        info.usage_type_raw === 'datacenter'
    );
  }

  function isProxyOrHosting(info) {
    if (!info) return false;
    return Boolean(
      isHosting(info) ||
        info.is_vpn ||
        info.is_proxy ||
        info.is_tor ||
        info.is_relay ||
        info.is_anonymous
    );
  }

  function isAccessNetwork(info) {
    if (!info || isProxyOrHosting(info)) return false;
    var usage = String(info.usage_type || '').trim().toLowerCase();
    var raw = String(info.usage_type_raw || '').trim().toLowerCase();
    return Boolean(
      info.is_mobile ||
        usage === 'isp' ||
        raw === 'residential' ||
        raw === 'mobile' ||
        raw === 'isp' ||
        raw === 'business' ||
        raw === 'education' ||
        raw === 'government' ||
        raw === 'satellite'
    );
  }

  function effectiveCountryCode(info) {
    if (!info) return '';
    var cc = String(info.country || '').trim().toUpperCase();
    if (!cc || cc === 'LOCAL' || cc === 'XX' || cc === 'ZZ') return '';
    if (cc === 'CN') {
      var haystack = [info.region, info.country_name, info.city].filter(Boolean).join(' ');
      if (/香港|hong\s*kong/i.test(haystack)) return 'HK';
      if (/澳门|澳門|macau|macao/i.test(haystack)) return 'MO';
      if (/台湾|臺灣|taiwan/i.test(haystack)) return 'TW';
    }
    return /^[A-Z]{2}$/.test(cc) ? cc : '';
  }

  function evaluateIPv6Leak(v4, v6) {
    if (!v4 || !v6 || !isAccessNetwork(v6)) return false;
    var v4Geo = effectiveCountryCode(v4);
    var v6Geo = effectiveCountryCode(v6);
    var v4ASN = normalizeASN(v4.asn);
    var v6ASN = normalizeASN(v6.asn);
    var sameASN = Boolean(v4ASN && v6ASN && v4ASN === v6ASN);
    var v4ProxyOrHosting = isProxyOrHosting(v4);
    if (!v4Geo || !v6Geo) {
      return Boolean(v4ProxyOrHosting && !sameASN);
    }
    var crossRegion = v4Geo !== v6Geo;
    var sameRegionProxyBypass = v4Geo === v6Geo && v4ProxyOrHosting && !sameASN;
    return crossRegion || sameRegionProxyBypass;
  }

  function countryFlag(code) {
    if (!code || !/^[A-Z]{2}$/.test(code)) return '';
    return String.fromCodePoint(
      0x1f1e6 + code.charCodeAt(0) - 65,
      0x1f1e6 + code.charCodeAt(1) - 65
    );
  }

  function formatLocation(info) {
    if (!info) return isEN ? 'Unknown' : '未知位置';
    if (info.usage_type === 'private' || info.country === 'LOCAL') {
      return isEN ? 'Private Network (LOCAL)' : '内网地址 (LOCAL)';
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
    var loc = parts.join(' · ') || (isEN ? 'Unknown' : '未知位置');
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
    if (!info) return isEN ? 'Unknown' : '未知';
    var op = String(info.isp || info.org || '').trim() || (isEN ? 'Unknown' : '未知');
    var asn = normalizeASN(info.asn);
    return asn ? op + ' (' + asn + ')' : op;
  }

  function formatTypeLine(info) {
    var base;
    if (!info) {
      base = isEN ? 'Unknown' : '类型未定';
    } else if (info.usage_type === 'private' || info.country === 'LOCAL') {
      base = isEN ? 'Private Network' : '内网地址';
    } else if (isHosting(info)) {
      base = isEN ? 'Datacenter' : '机房 IP';
    } else if (info.usage_type === 'isp') {
      base = info.is_mobile
        ? isEN
          ? 'Mobile'
          : '移动网络'
        : isEN
          ? 'ISP'
          : '普通宽带';
    } else {
      base = isEN ? 'Unknown' : '类型未定';
    }

    var tags = [];
    if (info && info.is_vpn) tags.push('VPN');
    if (info && info.is_proxy) tags.push(isEN ? 'Proxy' : '代理');
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
      return { color: '#EF4444', icon: 'exclamationmark.triangle.fill', label: isEN ? 'Attention' : '需留意' };
    }
    if (isHosting(primary)) {
      return { color: '#F59E0B', icon: 'server.rack', label: isEN ? 'Datacenter' : '机房 IP' };
    }
    if (primary && primary.usage_type === 'isp') {
      return {
        color: '#22C55E',
        icon: 'checkmark.shield.fill',
        label: primary.is_mobile ? (isEN ? 'Mobile' : '移动网络') : isEN ? 'ISP' : '普通宽带',
      };
    }
    return { color: '#64748B', icon: 'network', label: isEN ? 'Unknown' : '类型未定' };
  }

  function renderResult(v4, v6) {
    if (!v4 && !v6) {
      var errMsg = isEN ? 'Check failed: verify node connectivity' : '检测失败：请检查当前节点连通性';
      if (isLoon) {
        return {
          title: panelTitle,
          htmlMessage:
            '<div style="font-family:-apple-system,sans-serif;font-size:14px;line-height:1.6;padding:8px 4px;">' +
            '<div style="color:#EF4444;font-weight:600;">' +
            escapeHTML(errMsg) +
            '</div>' +
            '<div style="margin-top:8px;font-size:12px;opacity:.65;">' +
            escapeHTML(webHomeLabel) +
            '</div>' +
            '</div>',
        };
      }
      if (isStash) {
        return {
          title: panelTitle,
          content: errMsg,
          icon: 'exclamationmark.triangle.fill',
          backgroundColor: '#EF4444',
          url: webHomeURL,
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
    var ipv6Leaked = evaluateIPv6Leak(v4, v6);
    var style = buildStatusStyle(primary, ipv6Leaked);

    var lines = [
      (isEN ? 'Exit IP: ' : '出口 IP: ') + displayAddress(primary.ip),
      (isEN ? 'Location: ' : '地区: ') + formatLocation(primary),
      (isEN ? 'ISP / Org: ' : '运营商: ') + formatOperator(primary),
      (isEN ? 'Type: ' : '类型: ') + formatTypeLine(primary),
    ];

    if (typeof primary.human_ratio === 'number' && isFinite(primary.human_ratio)) {
      var pct = Math.max(0, Math.min(100, primary.human_ratio)).toFixed(1);
      lines.push(
        isEN
          ? 'Human Traffic: ' + pct + '% (30d ASN)'
          : '人类流量: ' + pct + '%（网段近30天）'
      );
    }

    if (enableIPv6) {
      if (v4 && v6) {
        var v6Addr = displayAddress(v6.ip);
        if (ipv6Leaked) {
          lines.push(
            isEN
              ? 'IPv6: Leak (ISP bypass ' + v6Addr + ')'
              : 'IPv6: 泄露（经接入网直连 ' + v6Addr + '）'
          );
        } else {
          lines.push(
            isEN
              ? 'IPv6: ' + v6Addr + ' (No leak)'
              : 'IPv6: ' + v6Addr + '（未泄露）'
          );
        }
      } else if (v4) {
        lines.push(isEN ? 'IPv6: Unreachable' : 'IPv6: 未连通');
      } else {
        lines.push(isEN ? 'IPv4: Unreachable (IPv6-only)' : 'IPv4: 未连通（仅 IPv6 出口）');
      }
    }

    if (isLoon) {
      var htmlLines = '';
      for (var i = 0; i < lines.length; i++) {
        htmlLines += '<div style="margin:4px 0;">' + escapeHTML(lines[i]) + '</div>';
      }
      var footerText = isEN
        ? 'Full check (DNS / WebRTC / Fingerprint): detectradar.com/en/'
        : '完整诊断（DNS / WebRTC / 指纹）：detectradar.com';
      return {
        title: panelTitle,
        htmlMessage:
          '<div style="font-family:-apple-system,BlinkMacSystemFont,sans-serif;font-size:14px;line-height:1.55;padding:6px 2px;">' +
          '<div style="display:flex;align-items:center;gap:6px;margin-bottom:8px;font-weight:600;color:' +
          style.color +
          ';">' +
          '<span>●</span><span>' +
          escapeHTML(
            ipv6Leaked
              ? isEN
                ? 'IPv6 Leak Detected'
                : '发现 IPv6 泄露'
              : style.label
          ) +
          '</span></div>' +
          htmlLines +
          '<div style="margin-top:10px;font-size:12px;opacity:.6;">' +
          escapeHTML(footerText) +
          '</div>' +
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
        url: webHomeURL,
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

  fetchIP('https://4.detectradar.com/api/v1/ip?from=' + clientTag + '&lang=' + lang, V4_TIMEOUT_MS, function (res) {
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
    fetchIP('https://6.detectradar.com/api/v1/ip?from=' + clientTag + '&lang=' + lang, V6_HARD_TIMEOUT_MS, function (res) {
      v6Done = true;
      v6Result = res;
      maybeFinish();
    });
  }
})();
