// 端口两口径复点（第二十六轮）。只读清点，不改任何仓内文件。
//
// 判据 A（基线口径，与 r24/r25 可比）：internal/*/ports 包声明的接口，其名字在
// internal/** 路径含 /adapters/ 或 /platform/ 的非测试 .go 文件正文里以整词出现过，
// 即记「有生产实现」。已知虚高：被适配器当依赖引用也算（r25 第四节）。
//
// 判据 B（精确口径，r25 未做、本轮补齐）：go/types.Implements——internal/... 与
// cmd/... 的生产包（Tests=false，测试替身不入内）里存在具体命名类型 T 或 *T 完整
// 实现该接口，才记「有生产实现」。
package main

import (
	"flag"
	"fmt"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type port struct {
	Context string
	Name    string
	Iface   *types.Interface
}

func main() {
	dir := flag.String("dir", ".", "仓库根")
	mode := flag.String("mode", "ab", "a=只判据A；ab=两口径")
	flag.Parse()

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedFiles | packages.NeedCompiledGoFiles,
		Dir:   *dir,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "./internal/...", "./cmd/...")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	loadErrors := 0
	for _, p := range pkgs {
		for _, e := range p.Errors {
			loadErrors++
			fmt.Fprintln(os.Stderr, "pkg error:", p.PkgPath, e)
		}
	}

	// 1) 收集 internal/<ctx>/ports 的接口声明。
	var ports []port
	for _, p := range pkgs {
		rest, ok := strings.CutPrefix(p.PkgPath, "go.idp.xyz/idp-parcel/internal/")
		if !ok {
			continue
		}
		parts := strings.Split(rest, "/")
		if len(parts) != 2 || parts[1] != "ports" {
			continue
		}
		scope := p.Types.Scope()
		for _, name := range scope.Names() {
			tn, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			iface, ok := tn.Type().Underlying().(*types.Interface)
			if !ok {
				continue
			}
			ports = append(ports, port{Context: parts[0], Name: name, Iface: iface})
		}
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Context != ports[j].Context {
			return ports[i].Context < ports[j].Context
		}
		return ports[i].Name < ports[j].Name
	})

	// 2) 判据 A：扫 internal/** 的 adapters/platform 生产文件正文。
	var bodies []string                   // 全部生产文件正文
	perCtxBodies := map[string][]string{} // 上下文 -> 该上下文子树内的正文（交叉核对用）
	scanned := 0
	root := filepath.Join(*dir, "internal")
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		slash := filepath.ToSlash(path)
		if !strings.HasSuffix(slash, ".go") || strings.HasSuffix(slash, "_test.go") {
			return nil
		}
		if !strings.Contains(slash, "/adapters/") && !strings.Contains(slash, "/platform/") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		body := string(raw)
		bodies = append(bodies, body)
		rel := strings.TrimPrefix(filepath.ToSlash(strings.TrimPrefix(path, root)), "/")
		rel = filepath.ToSlash(rel)
		ctx := strings.SplitN(rel, "/", 2)[0]
		perCtxBodies[ctx] = append(perCtxBodies[ctx], body)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "walk:", err)
		os.Exit(1)
	}

	nameRe := map[string]*regexp.Regexp{}
	re := func(name string) *regexp.Regexp {
		r, ok := nameRe[name]
		if !ok {
			r = regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
			nameRe[name] = r
		}
		return r
	}
	appears := func(name string, set []string) bool {
		r := re(name)
		for _, b := range set {
			if r.MatchString(b) {
				return true
			}
		}
		return false
	}

	implementedA := map[int]bool{}     // 全局口径（r25 头条）
	implementedASame := map[int]bool{} // 同上下文子树口径（交叉核对）
	for i, pt := range ports {
		implementedA[i] = appears(pt.Name, bodies)
		implementedASame[i] = appears(pt.Name, perCtxBodies[pt.Context])
	}

	// 3) 判据 B：生产包里任一具体命名类型（或其指针）真实现该接口。
	implementedB := map[int]string{}
	if *mode == "ab" {
		for _, p := range pkgs {
			if p.Types == nil {
				continue
			}
			scope := p.Types.Scope()
			for _, name := range scope.Names() {
				tn, ok := scope.Lookup(name).(*types.TypeName)
				if !ok || tn.IsAlias() {
					continue
				}
				named, ok := tn.Type().(*types.Named)
				if !ok {
					continue
				}
				if _, isIface := named.Underlying().(*types.Interface); isIface {
					continue
				}
				for i, pt := range ports {
					if _, done := implementedB[i]; done {
						continue
					}
					if types.Implements(named, pt.Iface) ||
						types.Implements(types.NewPointer(named), pt.Iface) {
						implementedB[i] = p.PkgPath + "." + name
					}
				}
			}
		}
	}

	// 4) 汇总输出。
	type row struct{ total, missA, missASame, missB int }
	byCtx := map[string]*row{}
	for i, pt := range ports {
		r, ok := byCtx[pt.Context]
		if !ok {
			r = &row{}
			byCtx[pt.Context] = r
		}
		r.total++
		if !implementedA[i] {
			r.missA++
		}
		if !implementedASame[i] {
			r.missASame++
		}
		if _, done := implementedB[i]; !done && *mode == "ab" {
			r.missB++
		}
	}
	var ctxs []string
	for c := range byCtx {
		ctxs = append(ctxs, c)
	}
	sort.Strings(ctxs)

	fmt.Printf("dir=%s mode=%s pkgErrors=%d adapters/platform 生产文件=%d\n", *dir, *mode, loadErrors, scanned)
	fmt.Println("上下文 | 接口总数 | A缺(全局) | A缺(同上下文) | B缺(types.Implements)")
	totals := row{}
	for _, c := range ctxs {
		r := byCtx[c]
		fmt.Printf("%s | %d | %d | %d | %d\n", c, r.total, r.missA, r.missASame, r.missB)
		totals.total += r.total
		totals.missA += r.missA
		totals.missASame += r.missASame
		totals.missB += r.missB
	}
	fmt.Printf("合计 | %d | %d | %d | %d\n", totals.total, totals.missA, totals.missASame, totals.missB)

	fmt.Println("\n== 判据 A 缺（全局口径，头条） ==")
	for i, pt := range ports {
		if !implementedA[i] {
			mark := ""
			if by, done := implementedB[i]; done {
				mark = "   <- B 已实现（A 虚低：实现者 " + by + "）"
			}
			fmt.Printf("%s.%s%s\n", pt.Context, pt.Name, mark)
		}
	}

	fmt.Println("\n== 判据 A 缺（同上下文子树口径，交叉核对） ==")
	for i, pt := range ports {
		if !implementedASame[i] {
			fmt.Printf("%s.%s\n", pt.Context, pt.Name)
		}
	}
	if *mode == "ab" {
		fmt.Println("\n== 判据 B 缺（真无生产实现） ==")
		for i, pt := range ports {
			if _, done := implementedB[i]; !done {
				mark := ""
				if implementedA[i] {
					mark = "   <- A 记已实现（表面缺：被引用但无人生产）"
				}
				fmt.Printf("%s.%s%s\n", pt.Context, pt.Name, mark)
			}
		}
	}
}
