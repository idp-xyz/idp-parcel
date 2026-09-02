package main

import (
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// 非业务目录：它们在 internal/ 下与限界上下文同级，但不表达任何领域语言。名单写死是有意的
// ——判「这个目录是不是一个上下文」需要领域知识，清点工具没有也不该替它拍；名单变长时由改
// 目录的人在这里补一行，那比让工具猜要诚实。
var nonBusinessDirs = map[string]bool{
	"platform":     true,
	"architecture": true,
}

// Outbox 投递适配器按类型名认，不按文件名。文件叫什么是作者自由，而这个名字形状是
// ADR-0043 的落库形态在代码里的稳定痕迹。
var outboxHandoffDecl = regexp.MustCompile(`(?m)^type\s+Outbox\w*Handoff\b`)

// ContextCount 是一个上下文子树的文件面清点。
type ContextCount struct {
	Name          string
	Production    int
	Test          int
	Application   int
	Postgres      int
	HTTP          int
	OutboxHandoff int
	Business      bool
}

// CrossSeam 是一处跨上下文消费缝：消费方在自己的子树里为某个提供方写的适配器。
// 缝按 internal/<消费方>/adapters/<提供方>/ 的目录形状认，提供方必须是另一个上下文
// ——同层的 postgres、http、identity 等是技术适配器，不是缝。
type CrossSeam struct {
	Consumer string
	Provider string
	Files    int
}

// ModuleCount 用于迁移这类「一个目录一族文件」的清点。
type ModuleCount struct {
	Name  string
	Files int
}

// FileCensus 是一次文件面清点的全部结果。
type FileCensus struct {
	Contexts   []ContextCount
	CrossSeams []CrossSeam
	Migrations []ModuleCount
	CmdProd    int
	CmdTest    int
}

// BusinessContexts 交回业务上下文那一部分，顺序同 Contexts。
func (census FileCensus) BusinessContexts() []ContextCount {
	var out []ContextCount
	for _, ctx := range census.Contexts {
		if ctx.Business {
			out = append(out, ctx)
		}
	}
	return out
}

// Totals 交回各列合计。业务与非业务一并计入——文档里那几个「全仓」数就是这个口径。
func (census FileCensus) Totals() ContextCount {
	total := ContextCount{Name: "合计"}
	for _, ctx := range census.Contexts {
		total.Production += ctx.Production
		total.Test += ctx.Test
		total.Application += ctx.Application
		total.Postgres += ctx.Postgres
		total.HTTP += ctx.HTTP
		total.OutboxHandoff += ctx.OutboxHandoff
	}
	return total
}

// CrossSeamTotals 交回缝组数与缝内生产文件数。组数按「消费方—提供方」配对算，不是消费方个数
// ——同一个消费方对两个提供方是两条缝。
func (census FileCensus) CrossSeamTotals() (groups, files int) {
	for _, seam := range census.CrossSeams {
		groups++
		files += seam.Files
	}
	return groups, files
}

// CensusFiles 清点 internal/、cmd/ 与 migrations/ 三处。root 是仓库根。
func CensusFiles(root fs.FS) (FileCensus, error) {
	contexts := map[string]*ContextCount{}
	seams := map[string]*CrossSeam{}
	census := FileCensus{}

	// 先认出有哪些上下文，跨上下文缝的判定要用这份名单：只有提供方也是一个上下文时，
	// adapters/<x>/ 才算缝。
	entries, err := fs.ReadDir(root, "internal")
	if err != nil {
		return census, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		contexts[entry.Name()] = &ContextCount{
			Name:     entry.Name(),
			Business: !nonBusinessDirs[entry.Name()],
		}
	}

	err = fs.WalkDir(root, "internal", func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		parts := strings.Split(p, "/")
		if len(parts) < 3 {
			return nil
		}
		ctx := contexts[parts[1]]
		if ctx == nil {
			return nil
		}
		if strings.HasSuffix(p, "_test.go") {
			ctx.Test++
			return nil
		}
		ctx.Production++

		switch {
		case contains(parts, "application"):
			ctx.Application++
		case matchesAdapter(parts, "postgres"):
			ctx.Postgres++
			body, readErr := fs.ReadFile(root, p)
			if readErr != nil {
				return readErr
			}
			if outboxHandoffDecl.Match(body) {
				ctx.OutboxHandoff++
			}
		case matchesAdapter(parts, "http"):
			ctx.HTTP++
		}

		if provider, ok := crossContextProvider(parts, contexts); ok {
			key := parts[1] + "→" + provider
			seam := seams[key]
			if seam == nil {
				seam = &CrossSeam{Consumer: parts[1], Provider: provider}
				seams[key] = seam
			}
			seam.Files++
		}
		return nil
	})
	if err != nil {
		return census, err
	}

	if err := countTree(root, "cmd", &census.CmdProd, &census.CmdTest); err != nil {
		return census, err
	}

	migrations, err := fs.ReadDir(root, "migrations")
	if err != nil {
		return census, err
	}
	for _, entry := range migrations {
		if !entry.IsDir() {
			continue
		}
		files, err := fs.ReadDir(root, path.Join("migrations", entry.Name()))
		if err != nil {
			return census, err
		}
		count := 0
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".sql") {
				count++
			}
		}
		census.Migrations = append(census.Migrations, ModuleCount{Name: entry.Name(), Files: count})
	}

	for _, ctx := range contexts {
		census.Contexts = append(census.Contexts, *ctx)
	}
	for _, seam := range seams {
		census.CrossSeams = append(census.CrossSeams, *seam)
	}
	sort.Slice(census.Contexts, func(i, j int) bool { return census.Contexts[i].Name < census.Contexts[j].Name })
	sort.Slice(census.CrossSeams, func(i, j int) bool {
		if census.CrossSeams[i].Consumer != census.CrossSeams[j].Consumer {
			return census.CrossSeams[i].Consumer < census.CrossSeams[j].Consumer
		}
		return census.CrossSeams[i].Provider < census.CrossSeams[j].Provider
	})
	sort.Slice(census.Migrations, func(i, j int) bool { return census.Migrations[i].Name < census.Migrations[j].Name })
	return census, nil
}

func countTree(root fs.FS, dir string, prod, test *int) error {
	return fs.WalkDir(root, dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		if strings.HasSuffix(p, "_test.go") {
			*test++
		} else {
			*prod++
		}
		return nil
	})
}

func contains(parts []string, want string) bool {
	for _, p := range parts[:len(parts)-1] {
		if p == want {
			return true
		}
	}
	return false
}

// matchesAdapter 要求 adapters 与目标名紧邻。松一格（只问路径里有没有出现过 postgres）
// 会把 adapters/postgres/ 之外碰巧同名的目录也算进来，而那类误计正是手工重盘反复出错的那种。
func matchesAdapter(parts []string, kind string) bool {
	for i := 0; i+1 < len(parts)-1; i++ {
		if parts[i] == "adapters" && parts[i+1] == kind {
			return true
		}
	}
	return false
}

func crossContextProvider(parts []string, contexts map[string]*ContextCount) (string, bool) {
	for i := 0; i+1 < len(parts)-1; i++ {
		if parts[i] != "adapters" {
			continue
		}
		provider := parts[i+1]
		if provider == parts[1] {
			continue
		}
		if ctx, ok := contexts[provider]; ok && ctx.Business {
			return provider, true
		}
	}
	return "", false
}
