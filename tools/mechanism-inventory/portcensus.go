package main

// 端口两口径清点，搬自 `.scratch/mechanism-reinventory-r26/tool/`（第二十六轮起沿用，
// r27 对 r25 发布树做过复现校验）。搬过来时把「打印」换成「交回结构」，判定逻辑一字未改
// ——两口径的可比性靠的正是它没变。
//
// 判据 A（基线口径，与历轮可比）：internal/*/ports 声明的接口，其名字在 internal/** 路径含
// /adapters/ 或 /platform/ 的非测试 .go 正文里以整词出现过，即记「有生产实现」。已知虚高：
// 被适配器当依赖引用也算。
//
// 判据 B（精确口径）：go/types.Implements——生产包（Tests=false，测试替身不入内）里存在具体
// 命名类型 T 或 *T 完整实现该接口，才记「有生产实现」。
//
// 两个口径都留着是因为它们各自会错，且错的方向相反：A 会把「被引用」当成「被实现」而虚高，
// B 会把「装配期才成形的组合」漏掉而虚低。只报一个数就没人看得见这段差。

import (
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// PortEntry 是一个端口接口及其两口径判定。
type PortEntry struct {
	Context      string
	Name         string
	ImplementedA bool
	ImplementedB bool
	ImplementerB string
	SameSubtreeA bool
}

// PortCensus 是一次端口清点的结果。
type PortCensus struct {
	Ports        []PortEntry
	ScannedFiles int
	LoadErrors   []string
}

// MissingA 交回基线口径下无生产实现的端口。
func (census PortCensus) MissingA() []PortEntry {
	return census.filter(func(p PortEntry) bool { return !p.ImplementedA })
}

// MissingB 交回精确口径下无生产实现的端口。
func (census PortCensus) MissingB() []PortEntry {
	return census.filter(func(p PortEntry) bool { return !p.ImplementedB })
}

func (census PortCensus) filter(keep func(PortEntry) bool) []PortEntry {
	var out []PortEntry
	for _, p := range census.Ports {
		if keep(p) {
			out = append(out, p)
		}
	}
	return out
}

type portIface struct {
	entry PortEntry
	iface *types.Interface
}

// CensusPorts 在 dir（仓库根）上做两口径清点。
func CensusPorts(dir string) (PortCensus, error) {
	census := PortCensus{}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedFiles | packages.NeedCompiledGoFiles,
		Dir:   dir,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "./internal/...", "./cmd/...")
	if err != nil {
		return census, err
	}
	for _, p := range pkgs {
		for _, e := range p.Errors {
			census.LoadErrors = append(census.LoadErrors, p.PkgPath+": "+e.Error())
		}
	}

	var ports []portIface
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
			ports = append(ports, portIface{
				entry: PortEntry{Context: parts[0], Name: name},
				iface: iface,
			})
		}
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].entry.Context != ports[j].entry.Context {
			return ports[i].entry.Context < ports[j].entry.Context
		}
		return ports[i].entry.Name < ports[j].entry.Name
	})

	var bodies []string
	perCtxBodies := map[string][]string{}
	root := filepath.Join(dir, "internal")
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
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
		census.ScannedFiles++
		body := string(raw)
		bodies = append(bodies, body)
		rel := strings.TrimPrefix(filepath.ToSlash(strings.TrimPrefix(path, root)), "/")
		perCtxBodies[strings.SplitN(rel, "/", 2)[0]] = append(perCtxBodies[strings.SplitN(rel, "/", 2)[0]], body)
		return nil
	})
	if err != nil {
		return census, err
	}

	cached := map[string]*regexp.Regexp{}
	wordRe := func(name string) *regexp.Regexp {
		r, ok := cached[name]
		if !ok {
			r = regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
			cached[name] = r
		}
		return r
	}
	appears := func(name string, set []string) bool {
		r := wordRe(name)
		for _, b := range set {
			if r.MatchString(b) {
				return true
			}
		}
		return false
	}
	for i := range ports {
		ports[i].entry.ImplementedA = appears(ports[i].entry.Name, bodies)
		ports[i].entry.SameSubtreeA = appears(ports[i].entry.Name, perCtxBodies[ports[i].entry.Context])
	}

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
			for i := range ports {
				if ports[i].entry.ImplementedB {
					continue
				}
				if types.Implements(named, ports[i].iface) ||
					types.Implements(types.NewPointer(named), ports[i].iface) {
					ports[i].entry.ImplementedB = true
					ports[i].entry.ImplementerB = p.PkgPath + "." + name
				}
			}
		}
	}

	for _, p := range ports {
		census.Ports = append(census.Ports, p.entry)
	}
	return census, nil
}
