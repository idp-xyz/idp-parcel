package cataloguepage_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// accountsSpec 是测试用的一册：时刻缺省序、组合标识（文本 + 整数）、一个封闭词表维、一个不设词表的维、
// 一个册子选择器——共用件要接住的形状在这一册里各出现一次。
func accountsSpec() cataloguepage.Spec {
	return cataloguepage.Spec{
		Name: "SYN-accounts",
		Sorts: []cataloguepage.SortDimension{
			{Name: "registeredAt", Kind: cataloguepage.Instant},
			{Name: "accountId", Kind: cataloguepage.Text},
			{Name: "revision", Kind: cataloguepage.Integer},
		},
		DefaultSort: cataloguepage.Sort{Field: "registeredAt", Descending: true},
		Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Integer},
		Filters: []cataloguepage.FilterDimension{
			{Name: "status", Vocabulary: []string{"REGISTERED", "EFFECTIVE", "DEACTIVATED"}},
			{Name: "customerPartyId"},
		},
		Selectors: []string{"family"},
	}
}

var accounts = cataloguepage.MustCatalogue(accountsSpec())

func TestAWellFormedDeclarationIsAccepted(t *testing.T) {
	if _, err := cataloguepage.NewCatalogue(accountsSpec()); err != nil {
		t.Fatalf("合规声明被拒：%v", err)
	}
}

// Covers: 声明是各册写死的常量，错在这里就是编程错误——每一种都要在装配时报出来，不能等到某个请求
// 恰好走到那一格才露出来；尤其是筛选维或选择器占了 after / sort / q，解码时那个参数的含义就说不清了。
func TestAnIllFormedDeclarationIsRefused(t *testing.T) {
	cases := map[string]func(*cataloguepage.Spec){
		"册名为空":        func(spec *cataloguepage.Spec) { spec.Name = "" },
		"没有可排维":       func(spec *cataloguepage.Spec) { spec.Sorts = nil },
		"可排维名为空":      func(spec *cataloguepage.Spec) { spec.Sorts[1].Name = "" },
		"可排维重名":       func(spec *cataloguepage.Spec) { spec.Sorts[1].Name = "registeredAt" },
		"可排维值形状未知":    func(spec *cataloguepage.Spec) { spec.Sorts[1].Kind = "decimal" },
		"缺省序不在可排维里":   func(spec *cataloguepage.Spec) { spec.DefaultSort.Field = "name" },
		"没有标识":        func(spec *cataloguepage.Spec) { spec.Identity = nil },
		"标识段值形状未知":    func(spec *cataloguepage.Spec) { spec.Identity[1] = "" },
		"筛选维名为空":      func(spec *cataloguepage.Spec) { spec.Filters[1].Name = "" },
		"筛选维占了 q":     func(spec *cataloguepage.Spec) { spec.Filters[1].Name = "q" },
		"筛选维占了 sort":  func(spec *cataloguepage.Spec) { spec.Filters[1].Name = "sort" },
		"筛选维占了 after": func(spec *cataloguepage.Spec) { spec.Filters[1].Name = "after" },
		"筛选维重名":       func(spec *cataloguepage.Spec) { spec.Filters[1].Name = "status" },
		"词表里有空值":      func(spec *cataloguepage.Spec) { spec.Filters[0].Vocabulary = []string{"EFFECTIVE", ""} },
		"选择器名为空":      func(spec *cataloguepage.Spec) { spec.Selectors = []string{""} },
		"选择器占了 sort":  func(spec *cataloguepage.Spec) { spec.Selectors = []string{"sort"} },
		"选择器与筛选维重名":   func(spec *cataloguepage.Spec) { spec.Selectors = []string{"status"} },
	}
	for name, spoil := range cases {
		t.Run(name, func(t *testing.T) {
			spec := accountsSpec()
			spoil(&spec)
			if _, err := cataloguepage.NewCatalogue(spec); err == nil {
				t.Fatal("不合规的声明被接受")
			}
		})
	}
}

func TestMustCataloguePanicsOnAnIllFormedDeclaration(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("不合规的声明没有 panic")
		}
		if !strings.Contains(recovered.(error).Error(), "cataloguepage") {
			t.Fatalf("panic 值应说明来自本包，实得 %v", recovered)
		}
	}()
	spec := accountsSpec()
	spec.Name = ""
	cataloguepage.MustCatalogue(spec)
}

func TestTheSortParameterSpellsDescendingWithALeadingMinus(t *testing.T) {
	if got := (cataloguepage.Sort{Field: "registeredAt", Descending: true}).String(); got != "-registeredAt" {
		t.Fatalf("倒序写法：得 %q", got)
	}
	if got := (cataloguepage.Sort{Field: "accountId"}).String(); got != "accountId" {
		t.Fatalf("顺序写法：得 %q", got)
	}
}
