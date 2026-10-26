package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// 本文件证规则登记这一种的路由与退出码：文档译成规则（两格都必须写出来，不要求授予写 null），入册与重放答 0、冲突答 2、
// 依赖故障答 3；缺格、未知键、尾随内容、空授予名、没有租户在到达用例之前拒成 1。一租户一条的判定由真库用例证。

type dutyRuleRegistrarDouble struct {
	outcome application.RegisterReferenceReviewDutyRuleOutcome
	err     error
	last    application.RegisterReferenceReviewDutyRuleCommand
	calls   int
}

func (double *dutyRuleRegistrarDouble) Handle(_ context.Context, command application.RegisterReferenceReviewDutyRuleCommand) (application.RegisterReferenceReviewDutyRuleOutcome, error) {
	double.calls++
	double.last = command
	return double.outcome, double.err
}

const dutyRuleJSON = `{"tenant":"SYN-TENANT-01","distinctSubjects":true,"requiredGrant":null}`

func TestExecuteReviewDutyRuleTranslatesTheDocument(t *testing.T) {
	registrar := &dutyRuleRegistrarDouble{outcome: application.ReferenceReviewDutyRuleRecorded}
	message, code := executeReviewDutyRule(t.Context(), []byte(dutyRuleJSON), registrar, passthroughTransactor{})
	if code != exitRegistered || !strings.Contains(message, "RECORDED") {
		t.Fatalf("message=%q code=%d", message, code)
	}
	if _, required := registrar.last.Rule.RequiredGrant(); registrar.last.Rule.Tenant().String() != "SYN-TENANT-01" ||
		!registrar.last.Rule.RequiresDistinctSubjects() || required {
		t.Fatalf("规则翻译走样：%+v", registrar.last.Rule)
	}

	executeReviewDutyRule(t.Context(), []byte(`{"tenant":"SYN-TENANT-01","distinctSubjects":false,"requiredGrant":"SYN-GRANT-REFERENCE-REVIEW"}`), registrar, passthroughTransactor{})
	if grant, required := registrar.last.Rule.RequiredGrant(); registrar.last.Rule.RequiresDistinctSubjects() || !required || grant.String() != "SYN-GRANT-REFERENCE-REVIEW" {
		t.Fatalf("要求授予的规则翻译走样：%+v", registrar.last.Rule)
	}

	calls := registrar.calls
	for name, raw := range map[string]string{
		"缺 distinctSubjects": `{"tenant":"SYN-TENANT-01","requiredGrant":null}`,
		"缺 requiredGrant":    `{"tenant":"SYN-TENANT-01","distinctSubjects":true}`,
		"空授予名":               `{"tenant":"SYN-TENANT-01","distinctSubjects":true,"requiredGrant":" "}`,
		"授予不是串":              `{"tenant":"SYN-TENANT-01","distinctSubjects":true,"requiredGrant":7}`,
		"没有租户":               `{"distinctSubjects":true,"requiredGrant":null}`,
		"未知键":                `{"tenant":"SYN-TENANT-01","distinctSubjects":true,"requiredGrant":null,"appliesTo":"FUEL"}`,
		"尾随内容":               dutyRuleJSON + ` {}`,
		"尾随右括号":              dutyRuleJSON + `}`,
		"坏 JSON":             `not json`,
	} {
		if message, code := executeReviewDutyRule(t.Context(), []byte(raw), registrar, passthroughTransactor{}); code != exitUsage {
			t.Fatalf("%s：message=%q code=%d，want %d", name, message, code, exitUsage)
		}
	}
	if registrar.calls != calls {
		t.Fatal("坏文档到达了用例")
	}

	for outcome, want := range map[application.RegisterReferenceReviewDutyRuleOutcome]int{
		application.ReferenceReviewDutyRuleRecorded:          exitRegistered,
		application.ReferenceReviewDutyRuleAlreadyOnRegister: exitRegistered,
		application.ReferenceReviewDutyRuleConflict:          exitGovernance,
		application.ReferenceReviewDutyRuleNotAccepted:       exitUsage,
		application.ReferenceReviewDutyRuleUndecided:         exitUndecided,
	} {
		if code := reviewDutyRuleExitCode(outcome); code != want {
			t.Fatalf("%s：code = %d，want %d", outcome, code, want)
		}
	}
	if message, code := executeReviewDutyRule(t.Context(), []byte(dutyRuleJSON), &dutyRuleRegistrarDouble{err: errors.New("rule register down")}, passthroughTransactor{}); code != exitUndecided || !strings.Contains(message, "rule register down") {
		t.Fatalf("依赖故障：message=%q code=%d", message, code)
	}
}
