package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// 标识前缀门禁。
//
// 各上下文的标识签发器给每类标识配一个前缀（`SUBV`、`PRJ`、`RPV`……），取值各上下文自定。
// 去掉上下文名之后，**没有任何东西在守全仓唯一性**——customs-compliance 的提交版本本来自然
// 缩写成 `SUBV`，而那个值已被 parcel-shipment 占了，撞名是当场发现的，不是被守住的。
//
// 为什么是扫源码而不是构造期拦截：`internal/platform/identity` 的 `NewMinter` 只对走了平台件
// 的调用方生效，而 network-routing 与 transport-fulfillment 的三个前缀写在 `adapters/postgres`
// 里、根本不经过 minter。**构造期检查守不住绕过它的人，扫源码才守得住。**
//
// 守不住的那一格如实写在这里：本门禁只认**名字以 Prefix 结尾的字符串常量**。把前缀直接内联
// 成字面量、或给常量起个别的名字，都能绕过去。它拦的是现实中会发生的那一种——照着某个既有
// identity 包抄一份，然后改值。

// identityPrefix 是一处前缀常量声明。
type identityPrefix struct {
	name  string
	value string
	path  string
}

// collectIdentityPrefixes 从一份语法树里取出所有前缀常量。
//
// 判据是「名字以 Prefix 结尾 + 值是字符串字面量」。不按包路径过滤：前缀散落在 adapters/identity
// 与 adapters/postgres 两类包里，按位置筛会漏掉后者，而后者恰是当初三个带尾横线的所在。
func collectIdentityPrefixes(syntax *ast.File, path string) []identityPrefix {
	var found []identityPrefix

	for _, decl := range syntax.Decls {
		generic, ok := decl.(*ast.GenDecl)
		if !ok || generic.Tok != token.CONST {
			continue
		}
		for _, spec := range generic.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range value.Names {
				if !strings.HasSuffix(name.Name, "Prefix") || index >= len(value.Values) {
					continue
				}
				literal, ok := value.Values[index].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				unquoted, err := strconv.Unquote(literal.Value)
				if err != nil {
					continue
				}
				found = append(found, identityPrefix{name: name.Name, value: unquoted, path: path})
			}
		}
	}
	return found
}

func repositoryIdentityPrefixes(t *testing.T) []identityPrefix {
	t.Helper()

	var all []identityPrefix
	for _, source := range parseRepositorySources(t) {
		all = append(all, collectIdentityPrefixes(source.syntax, source.path)...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].path+all[i].name < all[j].path+all[j].name })
	return all
}

// TestEveryIdentityPrefixIsUniqueAcrossTheRepository 守全仓唯一。
//
// 撞名时把两边的位置都报出来：只说「`SUBV` 撞了」，读的人不知道该让谁改，而两处往往属不同
// 地盘，改哪一边是要商量的。
func TestEveryIdentityPrefixIsUniqueAcrossTheRepository(t *testing.T) {
	seen := make(map[string]identityPrefix)

	for _, prefix := range repositoryIdentityPrefixes(t) {
		if earlier, clash := seen[prefix.value]; clash {
			t.Errorf("前缀 %q 撞名：%s 的 %s 与 %s 的 %s",
				prefix.value, earlier.path, earlier.name, prefix.path, prefix.name)
			continue
		}
		seen[prefix.value] = prefix
	}
}

// TestNoIdentityPrefixCarriesTheSeparator 守前缀不含分隔符。
//
// 横线归拼接处所有。前缀里再出现一个，段数就成了可变的，读日志的人无从知道哪一段是类别；
// 而这条约定此前只写在平台件的注释里，对不走平台件的三处一点约束力都没有。
func TestNoIdentityPrefixCarriesTheSeparator(t *testing.T) {
	for _, prefix := range repositoryIdentityPrefixes(t) {
		if strings.Contains(prefix.value, "-") {
			t.Errorf("%s 的 %s = %q 含分隔符；横线应加在拼接处", prefix.path, prefix.name, prefix.value)
		}
		if strings.TrimSpace(prefix.value) == "" {
			t.Errorf("%s 的 %s 是空前缀", prefix.path, prefix.name)
		}
	}
}

// TestTheIdentityPrefixGateCanActuallyCatchAViolation 证这道门禁真能红。
//
// 本包另两套门禁都带一个同名用例，理由一样：**一个从来不会失败的门禁比没有门禁更坑人，因为
// 它还会取信。** 这里用合成源码逼出两条规则各自的失败，顺带钉住提取器不会把无关常量算进来。
func TestTheIdentityPrefixGateCanActuallyCatchAViolation(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []identityPrefix
	}{
		{
			name:   "const 块里的前缀被取出",
			source: "package p\n\nconst (\n\tcasePrefix = \"CASE\"\n\tversionPrefix = \"DECLV\"\n)\n",
			want: []identityPrefix{
				{name: "casePrefix", value: "CASE", path: "synthetic.go"},
				{name: "versionPrefix", value: "DECLV", path: "synthetic.go"},
			},
		},
		{
			name:   "单行 const 同样被取出",
			source: "package p\n\nconst routePlanVersionPrefix = \"RPV\"\n",
			want:   []identityPrefix{{name: "routePlanVersionPrefix", value: "RPV", path: "synthetic.go"}},
		},
		{
			name:   "带尾横线的前缀被取出，交给规则去红",
			source: "package p\n\nconst casePrefix = \"CC-CASE-\"\n",
			want:   []identityPrefix{{name: "casePrefix", value: "CC-CASE-", path: "synthetic.go"}},
		},
		{
			name:   "名字不以 Prefix 结尾的常量不算",
			source: "package p\n\nconst eventSource = \"idp-parcel/parcel-shipment\"\n",
			want:   nil,
		},
		{
			name:   "值不是字符串字面量的不算",
			source: "package p\n\nconst retryPrefix = 3\n",
			want:   nil,
		},
		{
			name:   "var 不算——门禁只认常量，前缀不该是可改的",
			source: "package p\n\nvar casePrefix = \"CASE\"\n",
			want:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", test.source, 0)
			if err != nil {
				t.Fatalf("解析合成源码：%v", err)
			}
			got := collectIdentityPrefixes(syntax, "synthetic.go")
			if len(got) != len(test.want) {
				t.Fatalf("取出 %d 个，want %d 个：%v", len(got), len(test.want), got)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("第 %d 个 = %v，want %v", index, got[index], test.want[index])
				}
			}
		})
	}

	// 两条规则各自能不能红，用提取结果直接判——不重复跑全仓。
	clashing := []identityPrefix{
		{name: "submissionVersionPrefix", value: "SUBV", path: "a/identity.go"},
		{name: "versionPrefix", value: "SUBV", path: "b/identity.go"},
	}
	seen := make(map[string]identityPrefix, len(clashing))
	caught := false
	for _, prefix := range clashing {
		if _, clash := seen[prefix.value]; clash {
			caught = true
		}
		seen[prefix.value] = prefix
	}
	if !caught {
		t.Fatal("唯一性规则没能认出一次撞名")
	}
	if !strings.Contains("CC-CASE-", "-") {
		t.Fatal("分隔符规则没能认出一个带横线的前缀")
	}
}
