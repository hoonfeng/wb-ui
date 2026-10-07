// 公共判定内核（resource_ref_policy.go）的单测。
//
// 这份测试守的是 Q5-B 的核心主张——**一份判定、两处执行**：
//
//  1. 类别判定 / 放行规则 / resolver 询问序列**各只有一份实现**（真值表直接打在
//     内核上，任何一侧改规则都必须改内核、因此必然在这里被看见）；
//  2. 两条执行路径（loadExternalResource 的「取内容」路径 与 MediaResourceAllowed
//     的「只判定」路径）在同一引用、同一策略下**结论一致**；
//  3. 唯一的有意差异是**能力边界**而非策略差异：媒体通道没有网络栈 ⇒
//     http(s)/blob 恒拒——它由 `hasNetworkChannel` 参数显式表达，单独断言。
//
// 端到端（四配置 × 矩阵页的逐格等级）由 cmd/psai -media 负责；本轮重构的验收口径是
// 「探针逐格零变化」，见 docs/media-format-verification-plan.md §9.9 挂账登记。

package webkit

import (
	"errors"
	"testing"
)

func TestClassifyResourceRefKinds(t *testing.T) {
	cases := []struct {
		ref  string
		want resourceRefKind
	}{
		{"data:audio/wav;base64,UklGRiQAAABXQVZF", refKindInline},
		{"DATA:text/plain,hi", refKindInline},
		{"  data:text/plain,hi  ", refKindInline},
		{"http://example.com/a.png", refKindNetwork},
		{"https://example.com/a.png", refKindNetwork},
		{"blob:null/1234", refKindNetwork},
		{"file:///C:/tmp/a.png", refKindExternal},
		{"a.png", refKindExternal},
		{"", refKindExternal},
	}
	for _, c := range cases {
		if got := classifyResourceRef(c.ref); got != c.want {
			t.Errorf("classifyResourceRef(%q) = %v，期望 %v", c.ref, got, c.want)
		}
	}
}

func TestIsPolicyFreeResourceRefMatchesClassifier(t *testing.T) {
	// data: 的放行规则只有一份实现：IsPolicyFreeResourceRef 必须与分类器同源
	//（宿主侧 app/mediaaccess.go 也用它，因此宿主层不会出现第二份）。
	for _, ref := range []string{"data:x,1", "DATA:x,1", "  data:  ", "file:///a", "a", "http://x/y", ""} {
		want := classifyResourceRef(ref) == refKindInline
		if got := IsPolicyFreeResourceRef(ref); got != want {
			t.Errorf("IsPolicyFreeResourceRef(%q) = %v，期望 %v", ref, got, want)
		}
	}
}

func TestResourceRefAllowedTruthTable(t *testing.T) {
	refs := map[string]resourceRefKind{
		"data:text/plain,hi":   refKindInline,
		"http://example.com/a": refKindNetwork,
		"https://example.com/a": refKindNetwork,
		"blob:null/1":          refKindNetwork,
		"file:///C:/tmp/a.png": refKindExternal,
		"a.png":                refKindExternal,
	}
	for _, pol := range []ResourcePolicy{DenyExternal, AllowHostResolved, AllowAll} {
		for _, hasNet := range []bool{true, false} {
			wv := &WebView{mode: ModeToolkit}
			wv.SetResourcePolicy(pol)
			for ref, kind := range refs {
				var want bool
				switch kind {
				case refKindInline:
					want = true // 与策略解耦，任何策略、任何通道都放行
				case refKindNetwork:
					want = hasNet && pol == AllowAll // 无网络通道 ⇒ 恒拒
				default:
					want = pol == AllowAll
				}
				if got := wv.resourceRefAllowed(ref, hasNet); got != want {
					t.Errorf("policy=%s hasNetwork=%v ref=%q：得到 %v，期望 %v",
						pol, hasNet, ref, got, want)
				}
			}
		}
	}
	// nil WebView：一律拒绝。
	var nilWV *WebView
	if nilWV.resourceRefAllowed("data:x,1", true) {
		t.Fatal("nil WebView 应一律拒绝")
	}
}

func TestResolverRefForms(t *testing.T) {
	wv := &WebView{mode: ModeToolkit}
	// 无文档 URL（LoadHTML 直出内容）：只有原样一种形态——与两侧既有行为一致。
	if got := wv.resolverRefForms("a.png"); len(got) != 1 || got[0] != "a.png" {
		t.Fatalf("无基准时应只有原样形态，得到 %v", got)
	}
	// 有文档 URL：原样 + 绝对化两种形态（宿主可能只认其中一种）。
	wv.setDocumentURL("file:///C:/app/index.html")
	got := wv.resolverRefForms("img/a.png")
	if len(got) != 2 || got[0] != "img/a.png" || got[1] != "file:///C:/app/img/a.png" {
		t.Fatalf("有基准时应给原样 + 绝对化，得到 %v", got)
	}
	// 已带 scheme 的引用不参与绝对化 ⇒ 只有一种形态。
	if got := wv.resolverRefForms("file:///C:/x/a.png"); len(got) != 1 {
		t.Fatalf("绝对引用不应再绝对化，得到 %v", got)
	}
	if got := wv.resolverRefForms("data:text/plain,x"); len(got) != 1 {
		t.Fatalf("data: 不应绝对化，得到 %v", got)
	}
	if got := wv.resolverRefForms(""); got != nil {
		t.Fatalf("空引用应返回 nil，得到 %v", got)
	}
}

// TestPolicyVerdictConsistentAcrossBothPaths 是「两处执行、同一判定」的可验证形式：
// 同一引用、同一策略下，取内容路径被**策略门禁**拒绝 ⟺ 布尔判定为 false。
//
// 只在两者共同覆盖的类别上断言（data: / file / 相对路径）——http(s)/blob 是有意
// 差异，见 TestMediaPathHasNoNetworkChannel。
func TestPolicyVerdictConsistentAcrossBothPaths(t *testing.T) {
	refs := []string{
		"data:text/plain,hello",
		"file:///C:/tmp/wbui-definitely-missing.png",
		"wbui-definitely-missing.png",
	}
	for _, pol := range []ResourcePolicy{DenyExternal, AllowHostResolved, AllowAll} {
		for _, withResolver := range []bool{false, true} {
			wv := &WebView{mode: ModeToolkit}
			wv.SetResourcePolicy(pol)
			if withResolver {
				// resolver 不提供任何引用（命中即放行是另一条路径，单独断言）。
				wv.SetResourceResolver(func(string) (string, bool) { return "", false })
			}
			for _, ref := range refs {
				_, err := wv.loadExternalResource(ref, PurposeImage)
				blocked := errors.Is(err, ErrExternalResourceBlocked)
				allowed := wv.MediaResourceAllowed(ref)
				if allowed == blocked {
					t.Errorf("策略 %s（resolver=%v）、引用 %q：布尔判定 allowed=%v 与门禁 blocked=%v 不一致（err=%v）",
						pol, withResolver, ref, allowed, blocked, err)
				}
			}
		}
	}
}

// TestMediaPathHasNoNetworkChannel 守**有意差异**：同一 http(s)/blob 引用下，有网络
// 通道的取内容路径在 AllowAll 放行，而媒体路径恒拒——因为媒体没有 fetchResource。
// 这条差异由 hasNetworkChannel 参数表达，不是第二份策略规则。
func TestMediaPathHasNoNetworkChannel(t *testing.T) {
	wv := &WebView{mode: ModeBrowser} // ModeBrowser → 默认 AllowAll
	for _, ref := range []string{"http://example.com/a.png", "https://example.com/a.png", "blob:null/1"} {
		if !wv.resourceRefAllowed(ref, true) {
			t.Errorf("有网络通道且 AllowAll 时应放行 %q", ref)
		}
		if wv.MediaResourceAllowed(ref) {
			t.Errorf("媒体通道没有网络栈：%q 必须恒拒", ref)
		}
	}
}

// TestResolverHitAllowedOnBothPaths：resolver 命中即放行，两条路径同一语义（且媒体
// 侧的询问序列与取内容侧共用 resolverRefForms）。
func TestResolverHitAllowedOnBothPaths(t *testing.T) {
	const fileURL = "file:///C:/app/img/a.png"
	wv := &WebView{mode: ModeToolkit} // 默认 DenyExternal：没有 resolver 就一律拒
	wv.setDocumentURL("file:///C:/app/index.html")
	wv.SetResourceResolver(func(ref string) (string, bool) {
		if ref == "img/a.png" || ref == fileURL {
			return "<bytes>", true
		}
		return "", false
	})
	if !wv.MediaResourceAllowed("img/a.png") {
		t.Fatal("resolver 命中的相对引用应放行（原样形态命中）")
	}
	content, err := wv.loadExternalResource("img/a.png", PurposeImage)
	if err != nil || content != "<bytes>" {
		t.Fatalf("resolver 命中应取到内容，得到 content=%q err=%v", content, err)
	}
	// 只有绝对化形态能命中的情形（宿主只认绝对 URL）：两条路径也应一致放行。
	const relOnlyAbs = "img/b.png"
	wv.SetResourceResolver(func(ref string) (string, bool) {
		if ref == "file:///C:/app/img/b.png" {
			return "<bytes-b>", true
		}
		return "", false
	})
	if !wv.MediaResourceAllowed(relOnlyAbs) {
		t.Fatal("resolver 只认绝对 URL 时，媒体侧也应经绝对化形态命中")
	}
	if content, err := wv.loadExternalResource(relOnlyAbs, PurposeImage); err != nil || content != "<bytes-b>" {
		t.Fatalf("resolver 只认绝对 URL 时取内容应命中，得到 content=%q err=%v", content, err)
	}
}
