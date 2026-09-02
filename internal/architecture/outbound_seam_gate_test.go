package architecture

import (
	"strings"
	"testing"
)

// ADR-0090 决定三：出向端口交回的是已经分好格的封闭代数，不是 `*http.Response`，也不是裸
// error；平台层不得根据 HTTP 状态码代答。状态码在出向侧什么都不保证——公开文档里有恒回 200、
// 成败在响应体 `code` 上的渠道，那是该记录举的实证反例。把状态码解读成业务答案的那一步归各家
// 适配器，它是适配器唯一不可替代的职责。
//
// 写成门禁而不是留给注释与评审，是因为这条的破法看不出来：**顺手在出向缝里读一下状态码**在
// 编译期不报，在测试里也不报——它答出来的恰好就是当前唯一走得到的那一格正确答案。等第二家渠道
// 进来（比如一家恒 200 的）才发现，那时错误的读法已经被两条链继承走了。
func TestTheOutboundSeamDoesNotSeeTransport(t *testing.T) {
	t.Parallel()

	for _, file := range selectSources(t, "出向缝不得看见传输层", isOutboundSeam) {
		for _, imported := range file.imports {
			if strings.HasPrefix(imported, "net/http") || strings.HasPrefix(imported, "github.com/go-chi") {
				t.Errorf("%s：出向缝导入传输层 %q", file.path, imported)
			}
		}
	}
}

// TestTheOutboundSeamFilterCoversOnlyTheSharedPackage 直接测分类谓词。
//
// 理由与本包其余几条同一个：上一条门禁今天扫的包本来就不导入传输层，它**恒绿**，所以它绿
// 证明不了谓词选对了包。谓词一旦选空，`selectSources` 会 fatal；但谓词若选**多**了或选**偏**
// 了，门禁照样绿，而那时它守的已经不是出向缝。
func TestTheOutboundSeamFilterCoversOnlyTheSharedPackage(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		// ADR-0090 决定七指定的那个位置，及其子包。
		modulePath + "/internal/platform/outbound":          true,
		modulePath + "/internal/platform/outbound/internal": true,
		// 兄弟包不在其内。子串匹配会把它们一并圈进来，而它们没有理由受这条约束——一个专门
		// 放 HTTP 传输的兄弟包被禁止导入 net/http，只会逼下一个人去绕过门禁而不是遵守它。
		modulePath + "/internal/platform/outboundhttp": false,
		modulePath + "/internal/platform/outboxintent": false,
		// 各家渠道适配器**恰恰要**读状态码，那是它们不可替代的职责。圈进来会把 ADR-0090
		// 交给适配器的那一步禁掉，门禁于是从守规则变成破规则。
		modulePath + "/internal/parcelshipment/adapters/channel": false,
	}

	for pkg, want := range cases {
		if got := isOutboundSeam(pkg); got != want {
			t.Errorf("isOutboundSeam(%q) = %v，想要 %v", pkg, got, want)
		}
	}
}

// isOutboundSeam 按段匹配 ADR-0090 决定七指定的那一个共用出向包及其子包。
//
// 按段不按子串，与本包其余分类谓词同一口径：`outboundhttp` 这类兄弟包含同一个子串，一次
// 子串命中就把它们一并圈进本约束。
func isOutboundSeam(pkg string) bool {
	segments, ok := internalSegments(pkg)
	if !ok || len(segments) < 2 {
		return false
	}
	return segments[0] == "platform" && segments[1] == "outbound"
}
