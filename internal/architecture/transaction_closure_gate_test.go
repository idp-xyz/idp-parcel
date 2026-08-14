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

// 事务闭包门禁：`WithinTransaction` 的回调字面量里不得出现会 Goexit 的断言。
//
// `t.Fatal` 系走 `runtime.Goexit`，回调因而**永不返回**——而 `WithinTransaction` 并不预期这
// 件事：提交与回滚两条分支都被跳过，只剩 defer 兜底。断言失败本该是「告诉我哪里不对」，在
// 这里却变成「把事务挂在半空」，而那恰恰是你最需要看清发生了什么的时刻。
//
// **为什么值得立成门禁：`t.Errorf` 不 Goexit，所以它恰好无害，而两个函数差一个字母、在代码
// 里长得一模一样。** 靠人分辨守不住——本轮 32 处违反里，写的人没有一个是想清楚了才选 Errorf
// 的，他们只是随手。统一成「断言不进闭包」就不必再分辨谁会 Goexit。
//
// 这与前缀、分区键那两道门禁是同一类病：**写的那一刻完全无害，只在断言真的失败时才显形。**
//
// 守不住的那一格，如实写在这里：本门禁只认**回调字面量里直接出现**的 `t.Fatal` 系调用。
// 闭包里调一个自己会 Fatal 的辅助函数（`mustXxx(t, ...)` 那种）它看不出来——那要跨函数分析。
// 它拦的是现实中会发生的那一种：顺手在闭包里写一句断言。

// goexitAssertions 是会走 runtime.Goexit 的 testing 方法。Skip 系同样 Goexit，一并拦下。
var goexitAssertions = map[string]bool{
	"Fatal":   true,
	"Fatalf":  true,
	"FailNow": true,
	"Skip":    true,
	"Skipf":   true,
	"SkipNow": true,
}

// transactionCallbackName 判定一个被调用的名字是不是事务入口。
//
// 取名字含 `WithinTransaction` 而不取精确相等，也不取包路径，两个理由：`application.Transactor`
// 与 `postgres.DB.Transactor()` 交回同一个接口而测试里拿到它的写法五花八门；更要紧的是**测试
// 里普遍套一层 `mustWithinTransaction(t, …)` 这样的包装**，只认精确名会把包装形式整批漏掉——
// 本门禁第一次跑就在同一个文件里漏了两处，全是包装调用。
func transactionCallbackName(name string) bool {
	return strings.Contains(name, "WithinTransaction")
}

type closureViolation struct {
	path      string
	assertion string
}

// collectGoexitInTransactionClosures 找出事务回调字面量里直接写的 Goexit 断言。
func collectGoexitInTransactionClosures(syntax *ast.File, path string) []closureViolation {
	var found []closureViolation

	ast.Inspect(syntax, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		var called string
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			called = fun.Sel.Name
		case *ast.Ident:
			called = fun.Name
		}
		if !transactionCallbackName(called) {
			return true
		}
		for _, arg := range call.Args {
			literal, ok := arg.(*ast.FuncLit)
			if !ok {
				continue
			}
			found = append(found, goexitCallsIn(literal, path)...)
		}
		return true
	})
	return found
}

func goexitCallsIn(literal *ast.FuncLit, path string) []closureViolation {
	var found []closureViolation

	ast.Inspect(literal.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !goexitAssertions[selector.Sel.Name] {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		found = append(found, closureViolation{
			path:      path,
			assertion: receiver.Name + "." + selector.Sel.Name,
		})
		return true
	})
	return found
}

// parseRepositoryTestSources 遍历 `_test.go`——parseRepositorySources 刻意跳过它们，而本门禁
// 要查的恰恰是测试文件。
func parseRepositoryTestSources(t *testing.T) []parsedSource {
	t.Helper()

	root := repositoryRoot(t)
	var files []parsedSource

	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "docs", "backup", ".scratch", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		syntax, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		files = append(files, parsedSource{
			pkg:    filepath.ToSlash(filepath.Join(modulePath, relative)),
			path:   filepath.ToSlash(relative) + "/" + filepath.Base(path),
			syntax: syntax,
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("遍历测试文件：%v", walkErr)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files
}

// TestNoTransactionClosureCarriesAGoexitAssertion 守那一条规则。
//
// 无例外清单：落地那一刻全仓已经是绿的（32 处此前由各地盘主人搬出闭包），所以不需要给任何
// 位置开口子。**清单是留给「立门禁时已经红着」的场合的**，这里不是。
func TestNoTransactionClosureCarriesAGoexitAssertion(t *testing.T) {
	for _, source := range parseRepositoryTestSources(t) {
		for _, violation := range collectGoexitInTransactionClosures(source.syntax, source.path) {
			t.Errorf("%s 在事务回调里调了 %s；%s 走 runtime.Goexit，回调永不返回，"+
				"提交与回滚两条分支都被跳过。闭包只做 IO 并回 error，断言搬到闭包外",
				violation.path, violation.assertion, violation.assertion)
		}
	}
}

// TestTheTransactionClosureGateCanActuallyCatchAViolation 证这道门禁真能红。
//
// 与本包另三套门禁同一约定：一个从来不会失败的门禁比没有门禁更坑人，因为它还会取信。
func TestTheTransactionClosureGateCanActuallyCatchAViolation(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   int
	}{
		{
			name: "闭包里直接 t.Fatalf",
			source: "package p\nfunc f() { transactor.WithinTransaction(ctx, " +
				"func(txCtx context.Context) error { t.Fatalf(\"x\"); return nil }) }\n",
			want: 1,
		},
		{
			name: "Skip 系同样 Goexit，一并拦",
			source: "package p\nfunc f() { db.Transactor().WithinTransaction(ctx, " +
				"func(txCtx context.Context) error { t.SkipNow(); return nil }) }\n",
			want: 1,
		},
		{
			name: "嵌套一层闭包里的 Fatal 也算——Goexit 不认层数",
			source: "package p\nfunc f() { transactor.WithinTransaction(ctx, " +
				"func(txCtx context.Context) error { g(func() { t.Fatal(\"x\") }); return nil }) }\n",
			want: 1,
		},
		{
			name: "t.Errorf 不算——它不 Goexit，闭包照常返回",
			source: "package p\nfunc f() { transactor.WithinTransaction(ctx, " +
				"func(txCtx context.Context) error { t.Errorf(\"x\"); return nil }) }\n",
			want: 0,
		},
		{
			name: "闭包外的 Fatal 不算——那正是我们要求的写法",
			source: "package p\nfunc f() { transactor.WithinTransaction(ctx, " +
				"func(txCtx context.Context) error { return store.Save(txCtx) }); t.Fatalf(\"x\") }\n",
			want: 0,
		},
		{
			name: "别的方法的回调不算",
			source: "package p\nfunc f() { pool.WithConnection(ctx, " +
				"func(c context.Context) error { t.Fatalf(\"x\"); return nil }) }\n",
			want: 0,
		},
		{
			// 第一次跑时漏掉的正是这一形：测试里普遍套一层 mustWithinTransaction。
			name: "包装函数形式也算——它照样把闭包送进事务",
			source: "package p\nfunc f() { mustWithinTransaction(t, transactor, ctx, " +
				"func(txCtx context.Context) error { t.Fatalf(\"x\"); return nil }) }\n",
			want: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", test.source, 0)
			if err != nil {
				t.Fatalf("解析合成源码：%v", err)
			}
			got := collectGoexitInTransactionClosures(syntax, "synthetic.go")
			if len(got) != test.want {
				t.Fatalf("认出 %d 处，want %d 处：%v", len(got), test.want, got)
			}
		})
	}
}
