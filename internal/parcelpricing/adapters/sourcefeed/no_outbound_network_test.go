package sourcefeed_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 本文件把票 06 的红线「本票不写任何出网代码」做进结构：CFETS 段开工前置是部署侧登记出网能力，
// 在那之前，来源连接器实现与受控喂价口两处不得出现网络客户端的导入。只靠评审把守时，两种状态
// （没写出网代码 / 写了但没人翻到）在绿灯下长同一张脸；有了这道门，出网连接器开工那一天要动的
// 第一处就是这里——那一动本身就是「前置已满足」的可见记录。
//
// 只看导入不看符号：`http.Client` 一类符号要先导入 net/http 才写得出来，拦导入即拦全部。
// 测试文件不在此列——测试可以用 httptest 之类造替身，红线管的是随产品发出的字节。

// outboundNetworkImports 是被拦的导入路径。`net` 一并拦：拨号是出网的另一条路，不经 http。
var outboundNetworkImports = map[string]bool{
	"net":      true,
	"net/http": true,
}

// TestNoOutboundNetworkCodeShipsWithTheFileConnector 遍历本包与 cmd/parcel-pricing-feed 的非测试
// Go 文件，任一导入了出网包即红，报文点名文件与导入路径。
func TestNoOutboundNetworkCodeShipsWithTheFileConnector(t *testing.T) {
	directories := []string{
		".",
		filepath.Join("..", "..", "..", "..", "cmd", "parcel-pricing-feed"),
	}
	for _, directory := range directories {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatalf("读目录 %s：%v", directory, err)
		}
		checked := 0
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			checked++
			path := filepath.Join(directory, name)
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("解析 %s：%v", path, err)
			}
			for _, spec := range file.Imports {
				imported, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatalf("%s 的导入路径 %s 解不开：%v", path, spec.Path.Value, err)
				}
				if outboundNetworkImports[imported] {
					t.Errorf("%s 导入了 %s——票 06 红线：出网连接器开工前置是部署侧登记出网能力，仓内不得先出现出网代码", path, imported)
				}
			}
		}
		if checked == 0 {
			t.Fatalf("%s 下没有非测试 Go 文件——门禁扫了个空目录，路径写错了", directory)
		}
	}
}
