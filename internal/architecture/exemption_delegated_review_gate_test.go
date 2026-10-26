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

// 「免复核声明下的系统代写」一格的唯一置位者门禁。
//
// ADR-0262 决定一：租户对某一来源声明免人工复核之后，连接器的代写复核不读参考复核职责规则，只守写死的「复核者不得是
// 登记者」。这一格写在 `ReviewReferenceSeriesCommand` 上，带它的命令绕得过租户的规则——所以它只能由来源喂价在免复核
// 分支上置位，操作者面与受控登记口都不得置它。
//
// **字段不导出只挡得住包外。** 操作者面（adapters/http）与受控登记口（cmd/）在包外造命令，结构上给不出这一格；可
// `parcelpricing/application` 是一个包，所有编排同包而居，别的编排照着喂价那一行抄一个 `delegatedUnderExemption: true`，
// 编译器与领域都会收下，人工复核就此不再读规则。Go 没有 friend 可见性，**同包内的所有权只有扫源码守得住。**
//
// 守不住的那一格如实写在这里：本门禁只认两种写法——以这个字段名为键的复合字面量、以及左边是 `.delegatedUnderExemption`
// 的赋值。取它的地址再经指针写、或经反射写，绕得过去；它拦的是现实中会发生的那一种——另一个编排抄一行命令字面量。

// delegatedReviewField 是被守的那一格的字段名。
const delegatedReviewField = "delegatedUnderExemption"

// delegatedReviewOwningFile 是唯一许可置位的文件：来源喂价编排。换文件名要连同这里一起改——那正是这道门要你停一下的时刻。
const delegatedReviewOwningFile = "feed_reference_series.go"

// TestOnlyTheSourceFeedSetsTheExemptionDelegatedReview 守住：`parcelpricing/application` 包内（连同测试文件）写这一格的只有
// 来源喂价编排那一个文件，且确实写了——一次都没认出就是识别器失灵或字段改了名，不当通过。
func TestOnlyTheSourceFeedSetsTheExemptionDelegatedReview(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(repositoryRoot(t), "internal", "parcelpricing", "application")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("读取编排包目录：%v", err)
	}

	writers := map[string]int{}
	total := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		syntax, err := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, name), nil, 0)
		if err != nil {
			t.Fatalf("解析 %s：%v", name, err)
		}
		if writes := countDelegatedReviewWrites(syntax); writes > 0 {
			writers[name] = writes
			total += writes
		}
	}

	if total == 0 {
		t.Fatalf("编排包里一处置位 %s 的写法都没认出——要么识别器失灵，要么字段改了名；改 delegatedReviewField 之前先确认"+
			"代写那一格还在、还只由 %s 置位", delegatedReviewField, delegatedReviewOwningFile)
	}
	names := make([]string, 0, len(writers))
	for name := range writers {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) != 1 || names[0] != delegatedReviewOwningFile {
		t.Fatalf("置位 %s 的文件是 %v，只该是 [%s]——带这一格的复核不读租户的参考复核职责规则（ADR-0262 决定一），"+
			"只有来源喂价在免复核分支上置得；别的编排要复核，走人工复核那一路", delegatedReviewField, names, delegatedReviewOwningFile)
	}
}

// countDelegatedReviewWrites 数一份语法树里置位那一格的写法：以字段名为键的复合字面量元素，与左边选中该字段的赋值。
// 读它（`if command.delegatedUnderExemption`）与声明它（结构体字段）都不算。
func countDelegatedReviewWrites(syntax *ast.File) int {
	writes := 0
	ast.Inspect(syntax, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CompositeLit:
			for _, element := range typed.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := pair.Key.(*ast.Ident); ok && key.Name == delegatedReviewField {
					writes++
				}
			}
		case *ast.AssignStmt:
			for _, target := range typed.Lhs {
				if selector, ok := target.(*ast.SelectorExpr); ok && selector.Sel != nil && selector.Sel.Name == delegatedReviewField {
					writes++
				}
			}
		}
		return true
	})
	return writes
}

// TestTheExemptionDelegatedReviewGateCanActuallyCatchAViolation 证识别器两侧都认得：一个从来不会失败的门禁比没有门禁更坑人，
// 因为它还会取信。用合成源码逼出各种写法，不重复跑全仓。
func TestTheExemptionDelegatedReviewGateCanActuallyCatchAViolation(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		source string
		want   int
	}{
		"另一个编排在命令字面量里置它": {
			source: `package application
func leak() ReviewReferenceSeriesCommand {
	return ReviewReferenceSeriesCommand{SeriesID: "s", delegatedUnderExemption: true}
}`,
			want: 1,
		},
		"取地址的字面量同样算": {
			source: `package application
var leaked = &ReviewReferenceSeriesCommand{delegatedUnderExemption: true}`,
			want: 1,
		},
		"事后赋值算": {
			source: `package application
func leak(command ReviewReferenceSeriesCommand) ReviewReferenceSeriesCommand {
	command.delegatedUnderExemption = true
	return command
}`,
			want: 1,
		},
		"多值赋值里的那一格也算": {
			source: `package application
func leak(command *ReviewReferenceSeriesCommand) int {
	var n int
	command.delegatedUnderExemption, n = true, 1
	return n
}`,
			want: 1,
		},
		"只读它不算": {
			source: `package application
func read(command ReviewReferenceSeriesCommand) bool {
	if command.delegatedUnderExemption {
		return true
	}
	return false
}`,
			want: 0,
		},
		"声明它不算": {
			source: `package application
type ReviewReferenceSeriesCommand struct {
	delegatedUnderExemption bool
}`,
			want: 0,
		},
		"名字相近的别的格不算": {
			source: `package application
var command = ReviewReferenceSeriesCommand{ReviewerGrants: nil, delegatedUnderExemptions: true}`,
			want: 0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", test.source, 0)
			if err != nil {
				t.Fatalf("解析合成源码：%v", err)
			}
			if got := countDelegatedReviewWrites(syntax); got != test.want {
				t.Fatalf("认出 %d 处，want %d", got, test.want)
			}
		})
	}
}
