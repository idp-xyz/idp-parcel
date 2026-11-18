package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// 仓配履约的依赖方向门禁。
//
// ADR-0375 决定三：其他上下文不以仓配履约为前提。仓配可以经自己的消费侧适配器调用别的上下文，反方向只留决定三点名的
// 几处——专为消费仓配事实而立的结算采用、计价评价与全程追踪投影，它们落在各自上下文的 adapters/warehousefulfillment。
// ADR-0375 的 Consequences 记着这一向没有门禁守：`TestBusinessModulesDoNotReachIntoEachOther` 认的是适配器的位置，
// 不认方向，`internal/<消费方>/adapters/<提供方>` 这个形状对任何上下文都放行，于是 `internal/parcelshipment/adapters/
// warehousefulfillment` 一落地，小包托运就依赖了仓配，而那条规则照绿。
//
// 本门禁守下列规则，都只看 `internal/`：
//   - 导入：除仓配自己与点名的消费侧适配器位置（含子包）之外，没有包导入 `internal/warehousefulfillment` 或它的子包；
//   - 目录：`adapters/warehousefulfillment` 这个目录名只许出现在点名的位置。目录那一条是导入那一条的伴生：目录先建出来、
//     导入后到，红在建目录那一刻，比红在第一行导入早。
//
// `cmd/` 不在此限：把仓配的路与端点装进进程是组合根的职责（ADR-0375 决定四），它本来就要同时认识两边。
//
// 守不住的几格，如实写在这里：
//   - 只认非测试源文件里的包路径导入（`loadSources` 排掉 `_test.go`）。别的上下文的测试文件导入仓配不在网内。
//   - 只认包依赖，不认数据与信封：别的上下文去读仓配的库表、或把信封类型串写死在自己代码里，包路径上看不见。
//   - 只认 `internal/` 目录名里的 `adapters/warehousefulfillment`。同样内容放进别的目录名，目录那一条抓不到；
//     它的导入仍然逃不过导入那一条。

// warehouseFulfillmentContext 是仓配履约上下文在 `internal/` 下的目录名。
const warehouseFulfillmentContext = "warehousefulfillment"

// warehouseFulfillmentConsumerAdapters 是 ADR-0375 决定三点名的消费侧适配器位置，相对 `internal/`、斜杠分隔。
// 加一处要先有新的 ADR，而不是先改这里。
var warehouseFulfillmentConsumerAdapters = []string{
	"settlementaccounting/adapters/warehousefulfillment",
	"parcelpricing/adapters/warehousefulfillment",
	"visibilityexception/adapters/warehousefulfillment",
}

// isOrUnder 按路径段判 relative 是 position 本身还是它的子路径。不用裸前缀：`adapters/warehousefulfillmentutil`
// 这样的兄弟目录用前缀会被当成例外位置，白拿豁免，而那是一个没人发现得了的假阴性。
func isOrUnder(relative, position string) bool {
	return relative == position || strings.HasPrefix(relative, position+"/")
}

// reachesWarehouseFulfillment 判一次导入是不是伸进仓配履约上下文，含它的子包。按段匹配：`warehousefulfillmentx` 是另一个
// 目录，不是仓配。
func reachesWarehouseFulfillment(imported string) bool {
	segments, inInternal := internalSegments(imported)
	return inInternal && segments[0] == warehouseFulfillmentContext
}

// mayReachWarehouseFulfillment 判导入方有没有资格导入仓配：仓配自己，或点名的消费侧适配器位置及其子包。不在 `internal/` 下的
// 导入方（`cmd/` 等）不归本门禁管，一律放行。
func mayReachWarehouseFulfillment(importer string) bool {
	segments, inInternal := internalSegments(importer)
	if !inInternal {
		return true
	}
	relative := strings.Join(segments, "/")
	if isOrUnder(relative, warehouseFulfillmentContext) {
		return true
	}
	return slices.ContainsFunc(warehouseFulfillmentConsumerAdapters, func(position string) bool {
		return isOrUnder(relative, position)
	})
}

// warehouseFulfillmentImportViolation 是门禁的判定本身：一个包的一次导入是不是违规。纯函数，自测对着内存里的样例调它。
func warehouseFulfillmentImportViolation(importer, imported string) bool {
	return reachesWarehouseFulfillment(imported) && !mayReachWarehouseFulfillment(importer)
}

// misplacedWarehouseFulfillmentAdapters 是目录那一条的判定本身：入参是相对 `internal/` 的目录清单（斜杠分隔），交回违规的那几个。
// 末两段是 `adapters/warehousefulfillment` 且不是点名位置之一，即违规——不要求它恰好在 `<上下文>/adapters/` 那一层，
// 嵌得更深或直接挂在 `internal/adapters/` 下的同名目录一样红。
func misplacedWarehouseFulfillmentAdapters(directories []string) []string {
	var misplaced []string
	for _, directory := range directories {
		segments := strings.Split(directory, "/")
		if len(segments) < 2 || segments[len(segments)-1] != warehouseFulfillmentContext || segments[len(segments)-2] != "adapters" {
			continue
		}
		if !slices.Contains(warehouseFulfillmentConsumerAdapters, directory) {
			misplaced = append(misplaced, directory)
		}
	}
	return misplaced
}

// internalDirectories 列出 internalRoot 下全部目录，相对路径、斜杠分隔。什么都不跳过：同名目录藏在哪一层都该被看见。
func internalDirectories(t *testing.T, internalRoot string) []string {
	t.Helper()

	var directories []string
	walkErr := filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(internalRoot, path)
		if err != nil {
			return err
		}
		if relative != "." {
			directories = append(directories, filepath.ToSlash(relative))
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("遍历 %s：%v", internalRoot, walkErr)
	}
	return directories
}

// misplacedWarehouseFulfillmentAdaptersUnder 是目录那一条从磁盘到判定的整条链，生产与自测走同一条。
func misplacedWarehouseFulfillmentAdaptersUnder(t *testing.T, internalRoot string) []string {
	t.Helper()

	directories := internalDirectories(t, internalRoot)
	// 判定对「没有这个目录」恒绿，所以先证明遍历确实走到了适配器目录这一层；目录约定一变、遍历扫空，不能静默绿。
	if !slices.ContainsFunc(directories, func(directory string) bool { return filepath.Base(directory) == "adapters" }) {
		t.Fatalf("%s 下一个 adapters 目录都没遍历到；目录那一条的扫描面已经失效，会永远空过", internalRoot)
	}
	return misplacedWarehouseFulfillmentAdapters(directories)
}

// TestNoPackageOutsideWarehouseFulfillmentImportsIt 守导入那一条：仓配之外只有点名的消费侧适配器位置够得着它（ADR-0375 决定三）。
func TestNoPackageOutsideWarehouseFulfillmentImportsIt(t *testing.T) {
	t.Parallel()

	inInternal := func(pkg string) bool {
		_, ok := internalSegments(pkg)
		return ok
	}
	for _, file := range selectSources(t, "仓配履约只被它自己与点名的消费侧适配器导入", inInternal) {
		for _, imported := range file.imports {
			if warehouseFulfillmentImportViolation(file.pkg, imported) {
				t.Errorf("%s：导入 %q；其他上下文不以仓配履约为前提，只有 %v 这些消费侧适配器位置例外（ADR-0375 决定三）",
					file.path, imported, warehouseFulfillmentConsumerAdapters)
			}
		}
	}
}

// TestWarehouseFulfillmentAdaptersExistOnlyAtTheDesignatedPositions 守目录那一条：`adapters/warehousefulfillment` 只许
// 出现在点名的位置（ADR-0375 决定三）。
func TestWarehouseFulfillmentAdaptersExistOnlyAtTheDesignatedPositions(t *testing.T) {
	t.Parallel()

	for _, directory := range misplacedWarehouseFulfillmentAdaptersUnder(t, filepath.Join(repositoryRoot(t), "internal")) {
		t.Errorf("internal/%s：仓配的消费侧适配器只许落在 %v（ADR-0375 决定三）；别的位置一建出来，那个上下文就依赖了仓配",
			directory, warehouseFulfillmentConsumerAdapters)
	}
}

// TestTheWarehouseFulfillmentExemptionsAreExactlyTheNamedPositions 把例外名单钉到 ADR-0375 决定三点名的那几处。名单变宽是
// 一次依赖方向的决定，要先有记录，这条在改名单的当场变红。
func TestTheWarehouseFulfillmentExemptionsAreExactlyTheNamedPositions(t *testing.T) {
	t.Parallel()

	want := []string{
		"settlementaccounting/adapters/warehousefulfillment",
		"parcelpricing/adapters/warehousefulfillment",
		"visibilityexception/adapters/warehousefulfillment",
	}
	if !reflect.DeepEqual(warehouseFulfillmentConsumerAdapters, want) {
		t.Fatalf("例外位置 = %v，ADR-0375 决定三点名的是 %v；改名单要先有新的记录", warehouseFulfillmentConsumerAdapters, want)
	}
}

// TestTheWarehouseFulfillmentImportGateCanActuallyCatchAViolation 证导入那一条真能红，也证例外没有放宽。
//
// 与本包其余门禁同一约定：一个从来不会失败的门禁比没有门禁更坑人，因为它还会取信。今天的树上没有任何仓配的包，真实扫描
// 必然是绿的，光看绿说明不了判定对不对，所以这里用内存里的样例逼出判定的两侧。
func TestTheWarehouseFulfillmentImportGateCanActuallyCatchAViolation(t *testing.T) {
	t.Parallel()

	const (
		warehouse         = modulePath + "/internal/warehousefulfillment"
		warehouseDomain   = modulePath + "/internal/warehousefulfillment/domain"
		warehousePorts    = modulePath + "/internal/warehousefulfillment/ports"
		warehouseSibling  = modulePath + "/internal/warehousefulfillmentx/domain"
		sharedContextPath = modulePath + "/internal/parcelshipment/domain"
	)
	for name, test := range map[string]struct {
		importer string
		imported string
		want     bool
	}{
		"小包托运的应用层导入仓配领域包": {
			importer: modulePath + "/internal/parcelshipment/application", imported: warehouseDomain, want: true,
		},
		"导入仓配上下文根包同样算": {
			importer: modulePath + "/internal/parcelshipment/application", imported: warehouse, want: true,
		},
		"别的上下文自己的 warehousefulfillment 适配器不在例外里": {
			importer: modulePath + "/internal/parcelshipment/adapters/warehousefulfillment", imported: warehousePorts, want: true,
		},
		"网络路由的 HTTP 适配器导入仓配": {
			importer: modulePath + "/internal/networkrouting/adapters/http", imported: warehouseDomain, want: true,
		},
		"共享技术设施导入仓配": {
			importer: modulePath + "/internal/platform/dispatch", imported: warehouseDomain, want: true,
		},
		"例外上下文的应用层不是例外位置": {
			importer: modulePath + "/internal/settlementaccounting/application", imported: warehousePorts, want: true,
		},
		"例外上下文的适配器根目录不是例外位置": {
			importer: modulePath + "/internal/settlementaccounting/adapters", imported: warehousePorts, want: true,
		},
		"名字以例外位置开头的兄弟适配器不白拿豁免": {
			importer: modulePath + "/internal/settlementaccounting/adapters/warehousefulfillmentutil", imported: warehousePorts, want: true,
		},
		"名字以仓配开头的兄弟上下文不算仓配自己": {
			importer: modulePath + "/internal/warehousefulfillmentx/domain", imported: warehouseDomain, want: true,
		},

		"结算的仓配适配器是例外": {
			importer: modulePath + "/internal/settlementaccounting/adapters/warehousefulfillment", imported: warehousePorts, want: false,
		},
		"计价的仓配适配器是例外": {
			importer: modulePath + "/internal/parcelpricing/adapters/warehousefulfillment", imported: warehousePorts, want: false,
		},
		"全程追踪的仓配适配器是例外": {
			importer: modulePath + "/internal/visibilityexception/adapters/warehousefulfillment", imported: warehousePorts, want: false,
		},
		"例外位置的子包同样是例外": {
			importer: modulePath + "/internal/settlementaccounting/adapters/warehousefulfillment/asof", imported: warehouseDomain, want: false,
		},
		"仓配自己的根包导入自己的子包": {
			importer: warehouse, imported: warehouseDomain, want: false,
		},
		"仓配自己的应用层导入自己的领域包": {
			importer: modulePath + "/internal/warehousefulfillment/application", imported: warehouseDomain, want: false,
		},
		"仓配自己的适配器导入自己的端口": {
			importer: modulePath + "/internal/warehousefulfillment/adapters/postgres", imported: warehousePorts, want: false,
		},
		"cmd 不受本门禁约束": {
			importer: modulePath + "/cmd/parcel-dispatch", imported: warehouseDomain, want: false,
		},
		"导入的是名字相近的兄弟目录，不是仓配": {
			importer: modulePath + "/internal/parcelshipment/application", imported: warehouseSibling, want: false,
		},
		"导入别的上下文不归本门禁": {
			importer: modulePath + "/internal/parcelshipment/application", imported: sharedContextPath, want: false,
		},
		"导入标准库不归本门禁": {
			importer: modulePath + "/internal/parcelshipment/application", imported: "fmt", want: false,
		},
		"别的模块里同名的 internal 路径不是本仓的仓配": {
			importer: modulePath + "/internal/parcelshipment/application", imported: "example.com/other/internal/warehousefulfillment/domain", want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := warehouseFulfillmentImportViolation(test.importer, test.imported); got != test.want {
				t.Fatalf("%s 导入 %s 判为违规 = %v，want %v", test.importer, test.imported, got, test.want)
			}
		})
	}
}

// TestTheWarehouseFulfillmentDirectoryGateCanActuallyCatchAViolation 证目录那一条真能红，也证点名的位置不被抓。
func TestTheWarehouseFulfillmentDirectoryGateCanActuallyCatchAViolation(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		directories []string
		want        []string
	}{
		"小包托运下的仓配适配器": {
			directories: []string{"parcelshipment", "parcelshipment/adapters", "parcelshipment/adapters/warehousefulfillment"},
			want:        []string{"parcelshipment/adapters/warehousefulfillment"},
		},
		"各个别的上下文下的仓配适配器一并抓到": {
			directories: []string{
				"networkrouting/adapters/warehousefulfillment",
				"nodeoperations/adapters/warehousefulfillment",
				"partycommercial/adapters/warehousefulfillment",
				"accessidentity/adapters/warehousefulfillment",
			},
			want: []string{
				"networkrouting/adapters/warehousefulfillment",
				"nodeoperations/adapters/warehousefulfillment",
				"partycommercial/adapters/warehousefulfillment",
				"accessidentity/adapters/warehousefulfillment",
			},
		},
		"共享技术设施下的同名目录": {
			directories: []string{"platform/adapters/warehousefulfillment"},
			want:        []string{"platform/adapters/warehousefulfillment"},
		},
		"嵌得更深的同名目录": {
			directories: []string{"parcelshipment/inner/adapters/warehousefulfillment"},
			want:        []string{"parcelshipment/inner/adapters/warehousefulfillment"},
		},
		"直接挂在 internal 下的 adapters 里": {
			directories: []string{"adapters/warehousefulfillment"},
			want:        []string{"adapters/warehousefulfillment"},
		},
		"仓配上下文自己下面叫这个名字的适配器也不行": {
			directories: []string{"warehousefulfillment/adapters/warehousefulfillment"},
			want:        []string{"warehousefulfillment/adapters/warehousefulfillment"},
		},
		"例外位置的子目录里再套一层同名目录不是例外": {
			directories: []string{"settlementaccounting/adapters/warehousefulfillment/adapters/warehousefulfillment"},
			want:        []string{"settlementaccounting/adapters/warehousefulfillment/adapters/warehousefulfillment"},
		},

		"点名的位置不被抓": {
			directories: []string{
				"settlementaccounting/adapters/warehousefulfillment",
				"parcelpricing/adapters/warehousefulfillment",
				"visibilityexception/adapters/warehousefulfillment",
			},
			want: nil,
		},
		"点名位置的子目录不被抓": {
			directories: []string{
				"settlementaccounting/adapters/warehousefulfillment/asof",
				"parcelpricing/adapters/warehousefulfillment/inbox",
				"visibilityexception/adapters/warehousefulfillment/inbox",
			},
			want: nil,
		},
		"仓配自己的目录与它自己的适配器不被抓": {
			directories: []string{
				"warehousefulfillment",
				"warehousefulfillment/domain",
				"warehousefulfillment/adapters",
				"warehousefulfillment/adapters/postgres",
				"warehousefulfillment/adapters/parcelshipment",
			},
			want: nil,
		},
		"名字相近的兄弟目录不被抓": {
			directories: []string{
				"parcelshipment/adapters/warehousefulfillmentutil",
				"parcelshipment/adapters/warehouse",
				"parcelshipment/application/warehousefulfillment",
			},
			want: nil,
		},
		"别的上下文的跨上下文适配器不归这一条": {
			directories: []string{"parcelshipment/adapters/partycommercial", "parcelshipment/adapters/postgres"},
			want:        nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := misplacedWarehouseFulfillmentAdapters(test.directories); !slices.Equal(got, test.want) {
				t.Fatalf("违规目录 = %v，want %v", got, test.want)
			}
		})
	}
}

// TestTheWarehouseFulfillmentDirectoryGateSeesAViolationOnDisk 把目录那一条从磁盘遍历到判定整条链各走一遍：纯函数自测证不了
// 遍历真的走到了那一层，而遍历漏层是另一种静默变绿。
func TestTheWarehouseFulfillmentDirectoryGateSeesAViolationOnDisk(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, directory := range []string{
		"parcelshipment/adapters/warehousefulfillment",
		"parcelshipment/adapters/postgres",
		"settlementaccounting/adapters/warehousefulfillment/asof",
		"parcelpricing/adapters/warehousefulfillment",
		"visibilityexception/adapters/warehousefulfillment",
		"warehousefulfillment/adapters/postgres",
		"networkrouting/deeper/adapters/warehousefulfillment",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(directory)), 0o755); err != nil {
			t.Fatalf("造目录 %s：%v", directory, err)
		}
	}

	got := misplacedWarehouseFulfillmentAdaptersUnder(t, root)
	slices.Sort(got)
	want := []string{
		"networkrouting/deeper/adapters/warehousefulfillment",
		"parcelshipment/adapters/warehousefulfillment",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("磁盘上的违规目录 = %v，want %v", got, want)
	}
}
