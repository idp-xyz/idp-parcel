package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 总览只数分桶表行，所以 UC 里立了号、表里没补行的 AT 不进总数，而且没有任何东西会报：复核报告
// M23 在 9045b6fd 上实测约 160 个，那几批票只在盘点文末追记里写了「各加 N」，本工具不读追记。
// 这里把表行与 UC 的 AT 号逐一对照，让缺口在重算那一刻就响，不再等下一次复核去数。

const applicationDir = "docs/application"

// 立号的那一行：表格行的首格恰是一个 AT 号。正文里顺带提到的号（「替代 `AT-CC-109`」）不是立号，
// 一并收进来的话，提到一个已经改掉的旧号就会凭空要求一行；交接表首格并列几个号的那种行引的是
// 别处已立的号，不认也不会漏。
var declaredRow = regexp.MustCompile("^\\|\\s*`?(AT-[A-Z]+-\\d+)`?\\s*\\|")

// DeclaredATs 收 root 下 docs/application 各 .md 里立下的 AT 号，值是首次立号的文件（相对 root、斜杠分隔）。
func DeclaredATs(root string) (map[string]string, error) {
	base := filepath.Join(root, filepath.FromSlash(applicationDir))
	declared := map[string]string{}
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(content), "\n") {
			m := declaredRow.FindStringSubmatch(strings.TrimSuffix(line, "\r"))
			if m == nil {
				continue
			}
			if _, seen := declared[m[1]]; !seen {
				declared[m[1]] = filepath.ToSlash(rel)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("读不了 %s：%w", applicationDir, err)
	}
	return declared, nil
}

// CoverageError 是表行与立号对不上的两个方向：缺行的 AT 不进总数（总览低报），多出的行数的是
// UC 里已经没有的号（总览高报）。两边都列全，改的人不必再自己去对。
type CoverageError struct {
	Missing map[string]string // AT → 立号的 UC 文件
	Extra   map[string]string // AT → 所在的桶
}

func (e *CoverageError) Error() string {
	var b strings.Builder
	b.WriteString("分桶表行与 " + applicationDir + " 的 AT 号对不上；总览只数表行，照这样重算会算错，没有重算。\n")
	if len(e.Missing) > 0 {
		fmt.Fprintf(&b, "%s 里立了号、分桶表里没有行的 %d 个：\n", applicationDir, len(e.Missing))
		for _, id := range sortedATs(e.Missing) {
			fmt.Fprintf(&b, "  %s（%s）\n", id, e.Missing[id])
		}
	}
	if len(e.Extra) > 0 {
		fmt.Fprintf(&b, "分桶表里有行、%s 里已没有这个号的 %d 个：\n", applicationDir, len(e.Extra))
		for _, id := range sortedATs(e.Extra) {
			fmt.Fprintf(&b, "  %s（%s）\n", id, e.Extra[id])
		}
	}
	b.WriteString("补法：缺行的，在 " + inventoryPath + " 按「方法与口径」的分桶判据各补一行——测试已点名入①，" +
		"机制在而测试未点名入②，实例半边或闸门阻断入③，机制未实现入④，判不准的在行内注明；多出的，AT 改了号就把行改成新号，" +
		"整条删了就删行。文末追记里写的加数不算数。改完再跑本工具。")
	return b.String()
}

// Reconcile 对照立号与分桶表行，两边一致才交回 nil。
func Reconcile(declared map[string]string, census Census) error {
	missing := map[string]string{}
	for id, file := range declared {
		if _, ok := census.Rows[id]; !ok {
			missing[id] = file
		}
	}
	extra := map[string]string{}
	for id, bucket := range census.Rows {
		if _, ok := declared[id]; !ok {
			extra[id] = bucket
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	return &CoverageError{Missing: missing, Extra: extra}
}

// sortedATs 按前缀、再按号的数值排，与盘点各桶的行序同。
func sortedATs(set map[string]string) []string {
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		pi, ni := splitAT(ids[i])
		pj, nj := splitAT(ids[j])
		if pi != pj {
			return pi < pj
		}
		return ni < nj
	})
	return ids
}

func splitAT(id string) (string, int) {
	at := strings.LastIndex(id, "-")
	n, _ := strconv.Atoi(id[at+1:])
	return id[:at], n
}
