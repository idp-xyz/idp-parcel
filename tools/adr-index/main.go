// adr-index 从 docs/adr 下各份 ADR 生成 docs/adr/README.md 里的两个列表（已接受 / 已被取代）。
//
// 每份一行：标题取 H1，注记取 Status 行里索引注记标记之后的原文，取代的去向取 Status 行
// `Superseded by` 的链接。README 其余部分是手写的说明，工具只重写两对标记之间。
//
// 列表取自各 ADR 自己的 Status 行，是为了让带新 ADR 或部分停用的票只改 ADR 文件本身：手写
// 列表时每张票都往同一个末尾追加，两票同批必撞；两侧同改一行的部分停用注记，自动解冲突会给
// 同一份 ADR 留下新旧两行。列表在进 main 时重生成。
//
// 与 tools/mechanism-inventory 不同，本工具只用标准库，所以住在主模块里，
// 根目录的 go vet / go test 直接扫得到它。
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"go.idp.xyz/idp-parcel/tools/internal/markedregion"
)

const (
	acceptedBegin   = "<!-- 已接受决策列表：生成物，勿手改，由 tools/adr-index 生成 -->"
	acceptedEnd     = "<!-- 已接受决策列表完 -->"
	supersededBegin = "<!-- 已被取代决策列表：生成物，勿手改，由 tools/adr-index 生成 -->"
	supersededEnd   = "<!-- 已被取代决策列表完 -->"
)

func main() {
	dir := flag.String("dir", ".", "仓库根")
	check := flag.Bool("check", false, "只核对：生成结果与 README 不同即非零退出，不写盘")
	flag.Parse()

	if err := run(*dir, *check); err != nil {
		fmt.Fprintln(os.Stderr, "adr-index:", err)
		if errors.Is(err, markedregion.ErrStale) {
			os.Exit(1)
		}
		os.Exit(2)
	}
}

func run(dir string, check bool) error {
	adrDir := filepath.Join(dir, "docs", "adr")
	records, err := ReadRecords(os.DirFS(adrDir))
	if err != nil {
		return err
	}
	accepted, superseded := Render(records)

	readme := filepath.Join(adrDir, "README.md")
	content, err := os.ReadFile(readme)
	if err != nil {
		return err
	}
	out, err := markedregion.Replace(string(content), acceptedBegin, acceptedEnd, accepted)
	if err != nil {
		return fmt.Errorf("%s：%w", readme, err)
	}
	out, err = markedregion.Replace(out, supersededBegin, supersededEnd, superseded)
	if err != nil {
		return fmt.Errorf("%s：%w", readme, err)
	}
	return markedregion.Sync(readme, out, check)
}
