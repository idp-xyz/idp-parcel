package migrations

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
)

// 校验和对嵌入原始字节计算（checksumOf），行尾进哈希：同一份 SQL 的 CRLF 形与 LF 形
// 是两个校验和。.gitattributes 已钉 `*.sql text eol=lf`，干净检出永远是 LF 形；但本机
// 若被会写 CRLF 的工具改过，构建出的迁移器会把 CRLF 哈希记进已施加册，干净检出与 CI
// 随即在同一行上报校验和漂移——而 build/vet 对行尾全无信号，两形还曾各卡一条把演示库
// 锁死（取证见批务票 admin-remainder-mechanism-batch/05 的欠账节）。只有这里能在测试期
// 把它拦住：治本改 checksumOf 归一行尾的路已被否（全部既有校验和作废，代价不对等）。
func TestEmbeddedMigrationAssetsCarryNoCarriageReturn(t *testing.T) {
	checked := 0
	err := fs.WalkDir(assets, ".", func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(assetPath, ".sql") {
			return nil
		}
		checked++
		content, readErr := fs.ReadFile(assets, assetPath)
		if readErr != nil {
			return readErr
		}
		if offset := bytes.IndexByte(content, '\r'); offset >= 0 {
			t.Errorf("%s 含 \\r（首见于字节偏移 %d）：迁移资产必须是 LF 行尾——CRLF 会改变校验和，让已施加库在干净检出下报漂移", assetPath, offset)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("遍历嵌入迁移资产：%v", err)
	}
	if checked == 0 {
		t.Fatal("嵌入迁移资产里一个 .sql 都没走到——守卫空转等于没有守卫，先查嵌入清单")
	}
}
