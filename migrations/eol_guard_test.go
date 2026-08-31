package migrations

import (
	"bytes"
	"io/fs"
	"path"
	"strings"
	"testing"
)

// 校验和对嵌入原始字节计算（checksumOf），行尾进哈希：同一份 SQL 的 CRLF 形与 LF 形
// 是两个校验和。.gitattributes 已钉 `*.sql text eol=lf`，干净检出永远是 LF 形；但本机
// 若被会写 CRLF 的工具改过，构建出的迁移器会把 CRLF 哈希记进已施加册，干净检出与 CI
// 随即在同一行上报校验和漂移——而 build/vet 对行尾全无信号，两形还曾各卡一条把演示库
// 锁死（取证见批务票 admin-remainder-mechanism-batch/05 的欠账节）。只有这里能在测试期
// 把它拦住：治本改 checksumOf 归一行尾的路已被否（全部既有校验和作废，代价不对等）。
// UTF-8 BOM 与行尾同罪：同样进哈希、同样对 build/vet 静默，故同门看守。
func TestEmbeddedMigrationAssetsCarryNoCarriageReturnOrBOM(t *testing.T) {
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
			// 嵌入路径以 migrations 包目录为根，补上前缀才是能照抄的仓库根相对路径。
			t.Errorf("%s 含 \\r（首见于字节偏移 %d）：迁移资产必须是 LF 行尾——CRLF 会改变校验和，"+
				"让已施加库在干净检出下报漂移。在仓库根跑 `git checkout -- %s` 归一回 LF（内容零差），"+
				"并查是什么工具写出了 CRLF（AGENTS.md 点名过 Set-Content 一类）",
				assetPath, offset, path.Join("migrations", path.Clean(assetPath)))
		}
		if bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}) {
			t.Errorf("%s 以 UTF-8 BOM 开头：BOM 同样计入校验和，成因与处置同 \\r 那条", assetPath)
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
