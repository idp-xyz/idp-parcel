package networkhttp_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// updateContractFixtures 由 `go test ./internal/networkrouting/adapters/http/ -update` 打开：
// 把端点此刻真答出的响应体写回 testdata/，而不是与之比对。
var updateContractFixtures = flag.Bool("update", false, "把端点答出的响应体写回 testdata/ 契约夹具")

// assertContractFixture 把端点答出的 JSON 与 testdata/<name> 逐字节比对（只抹平行尾）。
//
// 这份文件是前后端之间唯一的共享物（票 admin-web-audit-followups/04）：管理台 apps/admin-web
// 的契约测试读的是同一个路径，对着它校自己手写的 TS 响应类型。所以它必须是端点**真答出**
// 的字节整理成的缩进 JSON，不是手写的样例——手写一份就等于为同一形状立第二个口径，Go 这侧
// 改一个键时它不会跟上。比对失败时提示带 -update 重跑：写回之后 git diff 把变了什么摆到
// 眼前，前端测试随之决定类型要不要跟。
func assertContractFixture(t *testing.T, name string, actual []byte) {
	t.Helper()
	// json.Encoder 会在末尾多给一个换行，Indent 又原样保留末尾空白；先裁掉再统一补一个。
	var indented bytes.Buffer
	if err := json.Indent(&indented, bytes.TrimSpace(actual), "", "  "); err != nil {
		t.Fatalf("响应不是 JSON：%v\n%s", err, actual)
	}
	indented.WriteByte('\n')

	path := filepath.Join("testdata", name)
	if *updateContractFixtures {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, indented.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("契约夹具 %s 读不到（%v）；带 -update 跑一次生成，连夹具一起提交", path, err)
	}
	// 仓内 .gitattributes 对 .json 只有 text=auto，Windows 检出会成 CRLF；行尾不是契约。
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(want, indented.Bytes()) {
		t.Fatalf("端点答出的响应体与契约夹具 %s 不一致。\n答出：\n%s夹具：\n%s"+
			"若形状变更是有意的，带 -update 重跑并连夹具一起提交；apps/admin-web 的契约测试会告诉你 TS 类型要不要跟。",
			path, indented.Bytes(), want)
	}
}
