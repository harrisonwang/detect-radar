package crawler

import "testing"

// Zscaler 的层级不固定（cloud→continent→city→[]，外加扁平的 svpnIPs），递归收集器要能
// 钻到任意深度、从每个 {"range":...} 叶子取 CIDR，v4 / v6 都收。
func TestCollectZscalerRanges(t *testing.T) {
	doc := map[string]any{
		"zscaler.net": map[string]any{
			"continent : EMEA": map[string]any{
				"city : Amsterdam": []any{
					map[string]any{"range": "159.254.250.0/23", "hostname": ""},
					map[string]any{"range": "2a03:eec0:3900::/40"},
				},
			},
		},
		"svpnIPs": []any{ // 顶层的扁平列表，结构与上面不同，也要能收进来
			map[string]any{"range": "165.225.0.0/17"},
		},
	}

	var out []Range
	collectZscalerRanges(doc, "zscaler.net", &out)

	got := map[string]bool{}
	for _, r := range out {
		got[r.CIDR] = true
		if r.Provider != "zscaler" || r.Service != "zscaler.net" {
			t.Errorf("字段未正确填充: %+v", r)
		}
	}

	if len(out) != 3 {
		t.Fatalf("应收集到 3 条（含 v6），实际 %d 条: %+v", len(out), out)
	}
	for _, want := range []string{"159.254.250.0/23", "165.225.0.0/17", "2a03:eec0:3900::/40"} {
		if !got[want] {
			t.Errorf("缺少期望的 CIDR %s", want)
		}
	}
}
