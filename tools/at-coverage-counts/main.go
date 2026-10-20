// at-coverage-counts 重算 .scratch/at-coverage-inventory.md「总览」那两张计数表：按桶的数量，
// 与按 AT 前缀的总数和逐桶数。数取自同一份文件里各分桶节的表行，工具只重写一对标记之间。
//
// 计数从行数算而不由票手改，是为了让点名 AT 的票只移行：每张票都改同两张计数表时两票同批
// 必撞，而手改的数又会悄悄落后于行——各桶的行就在同一份文件里，数本来就算得出来。
// 计数在进 main 时重算。
//
// 它**只数行，不分桶**。一条 AT 该进哪个桶是领票时对照 UC 原文的判断，留在行的位置上。
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
	inventoryPath = ".scratch/at-coverage-inventory.md"
	countsBegin   = "<!-- 总览两表：生成物，勿手改，由 tools/at-coverage-counts 从下面各分桶节的行数生成 -->"
	countsEnd     = "<!-- 总览两表完 -->"
)

func main() {
	dir := flag.String("dir", ".", "仓库根")
	check := flag.Bool("check", false, "只核对：重算结果与文件不同即非零退出，不写盘")
	flag.Parse()

	if err := run(*dir, *check); err != nil {
		fmt.Fprintln(os.Stderr, "at-coverage-counts:", err)
		if errors.Is(err, markedregion.ErrStale) {
			os.Exit(1)
		}
		os.Exit(2)
	}
}

func run(dir string, check bool) error {
	path := filepath.Join(dir, filepath.FromSlash(inventoryPath))
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	census, err := Count(string(content))
	if err != nil {
		return fmt.Errorf("%s：%w", path, err)
	}
	out, err := markedregion.Replace(string(content), countsBegin, countsEnd, Render(census))
	if err != nil {
		return fmt.Errorf("%s：%w", path, err)
	}
	return markedregion.Sync(path, out, check)
}
