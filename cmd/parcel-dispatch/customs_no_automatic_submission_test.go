package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// 本文件证首发「不启用生产自动申报」在派发进程上的那一半（ADR-0227 决定二；票 product-strategy-boundary/18）：委托被接受、
// 接受信封经生产派发图扇出之后，关务那边既没有案件、申报单元，也没有提交授权、提交版本或提交尝试；派发进程的生产代码里
// 构造不出建案、授权确认与提交三只编排——它们只在 parcel-api 的运营决定口那一个装配文件里出现。证据只记 `S`。

// Covers: AT-CC-443（派发半边）——演示租户一份委托被接受，接受信封走两拍生产派发图，关务各册与两类关务意图仍是零。
func TestAnAcceptedRequestDoesNotEstablishACaseOrSubmitADeclaration(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.Outcome() != psapplication.AcceptanceDecided {
		t.Fatalf("outcome = %q, want DECIDED", result.Outcome())
	}
	if ids := fixture.outboxIDsOfType(t, string(nrinbox.AcceptedDecisionEventType)); len(ids) != 1 {
		t.Fatalf("接受信封 = %v, want 恰好一封", ids)
	}

	for beat := 1; beat <= 2; beat++ {
		if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
			t.Fatalf("第 %d 拍：%v", beat, err)
		}
	}
	if n := fixture.countInbox(t, synV0VERederiveConsumer, synV0DecisionID); n != 1 {
		t.Fatalf("接受信封没扇出到 VE 腿：inbox = %d——这条用例要在扇出确实发生之后看关务", n)
	}

	for _, table := range []string{
		"customs_compliance.customs_case",
		"customs_compliance.declaration_unit",
		"customs_compliance.submission_authority",
		"customs_compliance.declaration_submission",
		"customs_compliance.submission_attempt",
	} {
		if n := fixture.countSQL(t, `SELECT count(*) FROM `+table); n != 0 {
			t.Fatalf("接受扇出之后 %s 有 %d 行：生产派发图里有一路在替人建案或提交", table, n)
		}
	}
	for _, eventType := range []string{
		"customs-compliance.customs-case.established",
		"customs-compliance.declaration-submission.formed",
	} {
		if n := fixture.countOutboxOfType(t, eventType); n != 0 {
			t.Fatalf("接受扇出之后有 %d 封 %s", n, eventType)
		}
	}
}

// customsDecisionConstructors 是能形成关务案件、提交授权或提交版本的编排构造函数。
var customsDecisionConstructors = map[string]bool{
	"NewEstablishCaseHandler":                  true,
	"NewEstablishCaseForParcelsHandler":        true,
	"NewConfirmSubmissionAuthorizationHandler": true,
	"NewSubmitDeclarationHandler":              true,
	"NewCorrectDeclarationHandler":             true,
}

// customsDecisionAssembly 是这几只编排在 cmd/ 下唯一的生产构造处：parcel-api 运营决定三口的装配文件。
const customsDecisionAssembly = "parcel-api/assemble_customs_decisions.go"

// Covers: AT-CC-443（进程半边）——扫 cmd/ 下全部生产源码，上列构造函数只在 parcel-api 运营决定三口的装配文件里被调用：派发进程、
// 各登记 CLI 与其余装配文件都构造不出能建案或提交的编排，因此没有任何后台触发形成提交版本。新增一处调用要先回答它是不是自动
// 申报路径（首发不启用，PILOT-SCOPE），再改本用例。
func TestOnlyTheOperatorDecisionFacesConstructCustomsCaseAndSubmissionOrchestrations(t *testing.T) {
	root := filepath.Join("..")
	fileSet := token.NewFileSet()
	var callers []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		syntax, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(syntax, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && customsDecisionConstructors[selector.Sel.Name] {
				relative, _ := filepath.Rel(root, path)
				callers = append(callers, filepath.ToSlash(relative)+" "+selector.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("扫 cmd/：%v", err)
	}
	if len(callers) == 0 {
		t.Fatal("一处调用都没扫到：本用例自己扫错了地方，它就什么也证不了")
	}
	for _, caller := range callers {
		if !strings.HasPrefix(caller, customsDecisionAssembly+" ") {
			t.Fatalf("%s：建案或提交编排在运营决定口之外被构造了", caller)
		}
	}
}
