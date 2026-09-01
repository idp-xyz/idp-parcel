package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// 客户费用调整册的唯一创建用例门禁。
//
// CONTEXT「调整类型与唯一所有权」表规定每类调整只能由指定用例创建：计价纠错与商业让利
// 归 UC-SA-002，`UC-SA-003` 只纳入既有调整、`UC-SA-005` 不得借核销创建费用调整。
// ADR-0087 决定二要求补册时把这道门一并落下——「补册而不落这道门，就开出一条谁都能造
// 调整的路，而那比没有册更坏：册在，看起来就像守着」。
//
// **表上的种类封闭集守不住这一条。** `charge_adjustment.kind` 只开 UC-SA-002 拥有的两格，
// 那管的是「册上表达得出什么类型」；而 CONTEXT 那张表管的是「谁能写」。今天
// `settlementaccounting/application` 是一个包，所有编排同包而居，于是截单用例只要依赖
// `ChargeAdjustmentStore` 就能写一条完全合法的 `PRICING_CORRECTION`，CHECK 与领域都会收下。
//
// 为什么是扫源码而不是靠可见性：Go 没有 friend 可见性，同包内谁都够得着同一个端口类型；
// 把端口挪进未导出位置也不行——它得被 adapters 实现。**同包内的所有权只有扫源码守得住。**
//
// 守不住的那一格如实写在这里：本门禁只认**按名字**提到 `ChargeAdjustmentStore` 的引用。
// 把它经由一个类型别名或一个中间接口转手，就能绕过去。它拦的是现实中会发生的那一种——
// 另一个编排照着既有 Deps 结构抄一行依赖进去。

// chargeAdjustmentRegisterPort 是被守的端口名。
const chargeAdjustmentRegisterPort = "ChargeAdjustmentStore"

// chargeAdjustmentOwningFile 是 CONTEXT 判给 UC-SA-002 的那一个文件。换文件名要连同
// 这里一起改——那正是这道门要你停一下的时刻。
const chargeAdjustmentOwningFile = "record_charge_adjustment.go"

// TestOnlyTheOwningUseCaseReachesTheChargeAdjustmentRegister 守住：
// `settlementaccounting/application` 包内只有唯一创建用例那一个文件够得着调整册端口。
func TestOnlyTheOwningUseCaseReachesTheChargeAdjustmentRegister(t *testing.T) {
	t.Parallel()

	// 按目录过滤到 settlement-accounting 的编排包：所有权是这一个包内部的事，
	// 别处（ports 的声明、adapters 的实现、cmd 的装配）提到它都是正当的。
	directory := filepath.Join(repositoryRoot(t), "internal", "settlementaccounting", "application")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("读取编排包目录：%v", err)
	}

	var reaching []string
	sawOwningFile := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if name == chargeAdjustmentOwningFile {
			sawOwningFile = true
		}
		syntax, err := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, name), nil, 0)
		if err != nil {
			t.Fatalf("解析 %s：%v", name, err)
		}
		if namesChargeAdjustmentRegister(syntax) {
			reaching = append(reaching, name)
		}
	}
	sort.Strings(reaching)

	if !sawOwningFile {
		t.Fatalf("%s 不在编排包里——唯一创建用例换了位置，这道门跟着失效了，"+
			"改 chargeAdjustmentOwningFile 之前先确认所有权没跟着搬走", chargeAdjustmentOwningFile)
	}
	if len(reaching) != 1 || reaching[0] != chargeAdjustmentOwningFile {
		t.Fatalf("够得着调整册的文件是 %v，只该是 [%s]——"+
			"CONTEXT「调整类型与唯一所有权」表把计价纠错与商业让利判给 UC-SA-002，"+
			"别的编排要动调整只能调用它，不能自己写册（ADR-0087 决定二）",
			reaching, chargeAdjustmentOwningFile)
	}
}

// namesChargeAdjustmentRegister 判一份语法树里有没有按名字提到调整册端口。
func namesChargeAdjustmentRegister(syntax *ast.File) bool {
	found := false
	ast.Inspect(syntax, func(node ast.Node) bool {
		if found {
			return false
		}
		selector, ok := node.(*ast.SelectorExpr)
		if ok && selector.Sel != nil && selector.Sel.Name == chargeAdjustmentRegisterPort {
			found = true
			return false
		}
		if identifier, ok := node.(*ast.Ident); ok && identifier.Name == chargeAdjustmentRegisterPort {
			found = true
			return false
		}
		return true
	})
	return found
}

// TestTheChargeAdjustmentOwnershipGateCanActuallyCatchAViolation 证这道门禁真能红。
//
// 与本包其余门禁同一约定：一个从来不会失败的门禁比没有门禁更坑人，因为它还会取信。
// 这里用合成源码逼出识别器的两侧，不重复跑全仓。
func TestTheChargeAdjustmentOwnershipGateCanActuallyCatchAViolation(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		source string
		want   bool
	}{
		"另一个编排把册抄进了自己的 Deps": {
			source: `package application
type CutOffDeps struct {
	Adjustments ports.ChargeAdjustmentStore
}`,
			want: true,
		},
		"局部变量声明同样算够得着": {
			source: `package application
func leak(store ports.ChargeAdjustmentStore) {}`,
			want: true,
		},
		"只提到费用库不算": {
			source: `package application
type ConfirmDeps struct {
	Charges ports.CustomerChargeStore
}`,
			want: false,
		},
		"名字相近但不是它": {
			source: `package application
type Deps struct {
	Adjustments ports.RecoveryAdjustmentStore
}`,
			want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", test.source, 0)
			if err != nil {
				t.Fatalf("解析合成源码：%v", err)
			}
			if got := namesChargeAdjustmentRegister(syntax); got != test.want {
				t.Fatalf("认出 = %v, want %v", got, test.want)
			}
		})
	}
}
