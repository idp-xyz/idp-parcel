package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// 本文件证登记口自己的表面：用法边界、译装先于事务、事务给不出来答未决、答案代数到退出码的归格（裁决 5）。
// 采用 / 更正的绿路径要真库与真 Outbox 才证得了「行与信封同生同灭」，在 vertical_test。

type passthroughTransactor struct{}

func (passthroughTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	return fn(ctx)
}

// failingTransactor 连笔都开不了；同时也是「译装拒绝不进事务」那一例的探针——它一被调用就答未决，
// 译装若先拒了它就永远收不到调用。
type failingTransactor struct{ err error }

func (transactor failingTransactor) WithinTransaction(context.Context, bentoapp.TxFunc) error {
	return transactor.err
}

const adoptInput = `{
	"tenantId": "SYN-T1", "factRef": "SYN-FACT-1", "sourceRef": "SYN-SOURCE-BANK-1",
	"kind": "RECEIPT_CONFIRMED", "currency": "EUR", "amountMinor": 8000,
	"version": "SYN-FACT-1/v1", "occurredAt": "2026-09-01T08:00:00Z"
}`

// TestRunRejectsUsageErrors 证进程口的用法边界（判据 4）：缺命令、集合外命令、缺 -input、缺 DSN 各自以用法格
// 退出，不碰数据库——`external-funds-fact` 之外的名字包括 CC 那本的 `duty-payment-verification`，本口不认。
func TestRunRejectsUsageErrors(t *testing.T) {
	ctx := context.Background()
	noEnv := func(string) string { return "" }

	if code := run(ctx, nil, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("缺命令退出码 = %d", code)
	}
	if code := run(ctx, []string{"duty-payment-verification"}, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("集合外命令退出码 = %d（税费核对归 parcel-customs-register）", code)
	}
	if code := run(ctx, []string{commandExternalFundsFact}, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("缺 -input 退出码 = %d", code)
	}

	input := filepath.Join(t.TempDir(), "funds-fact.json")
	if err := os.WriteFile(input, []byte(adoptInput), 0o600); err != nil {
		t.Fatalf("写输入文件：%v", err)
	}
	if code := run(ctx, []string{commandExternalFundsFact, "-input", input}, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("缺 DSN 退出码 = %d（登记口不猜连接串）", code)
	}
}

// TestExecuteTranslationRejectionIsUsageAndOpensNoTransaction 证译装拒绝在事务之前：用法错误与「登记与否未知」
// 是两个退出码，让坏载荷进了事务就分不开了。事务源是一碰即未决的探针，答 1 而不是 3 即证没碰。
func TestExecuteTranslationRejectionIsUsageAndOpensNoTransaction(t *testing.T) {
	registrar := registrar{transactor: failingTransactor{err: errors.New("must not be reached")}}

	message, code := execute(context.Background(), commandExternalFundsFact,
		[]byte(`{"tenantId": "SYN-T1", "typo": 1}`), registrar)
	if code != exitUsage || !strings.Contains(message, "译装被拒") {
		t.Fatalf("坏载荷 = %d（%s），要 %d 且含 译装被拒", code, message, exitUsage)
	}
}

// TestExecuteTransactorFailureIsUndecided 证环境事务给不出来也答未决：连笔都没开，登记与否同样未知。
func TestExecuteTransactorFailureIsUndecided(t *testing.T) {
	registrar := registrar{transactor: failingTransactor{err: errors.New("no transaction")}}

	message, code := execute(context.Background(), commandExternalFundsFact, []byte(adoptInput), registrar)
	if code != exitUndecided || !strings.Contains(message, "未决") {
		t.Fatalf("事务故障 = %d（%s），要 %d 且含 未决", code, message, exitUndecided)
	}
}

// TestFundsAnswerCoversEveryOutcome 证退出码翻译逐格对着 `application.FundsOutcome` 的封闭表成立（裁决 5 归格原则；
// 一族一张表，本口没有命令能交回的映射 / 核销格仍在表上，同一格不因来自哪条命令而换退出码），未知格折未决。
func TestFundsAnswerCoversEveryOutcome(t *testing.T) {
	cases := []struct {
		outcome application.FundsOutcome
		code    int
	}{
		{application.FundsFactAdopted, exitRegistered},
		{application.FundsFactExisting, exitRegistered},
		{application.FundsMapped, exitRegistered},
		{application.FundsMappingExisting, exitRegistered},
		{application.SettlementApplied, exitRegistered},
		{application.ApplicationExisting, exitRegistered},
		{application.ApplicationReversedOutcome, exitRegistered},
		{application.ApplicationAlreadyReversed, exitRegistered},
		{application.FundsNotAccepted, exitUsage},
		{application.UnfundableFactOutcome, exitUsage},
		{application.ApplicationImbalanceOutcome, exitUsage},
		{application.CrossCurrencyOutcome, exitUsage},
		{application.FundsFactConflict, exitConflict},
		{application.FundsMappingConflict, exitConflict},
		{application.ApplicationConflict, exitConflict},
		{application.FundsUndecided, exitUndecided},
		{application.FundsOutcomeInvalid, exitUndecided},
	}
	for _, spec := range cases {
		message, code := fundsAnswer(commandExternalFundsFact, spec.outcome, application.FundsUndecidedReasonNone, "", "")
		if code != spec.code {
			t.Fatalf("%s → %d（%s），要 %d", spec.outcome, code, message, spec.code)
		}
		if spec.outcome != application.FundsOutcomeInvalid && !strings.Contains(message, spec.outcome.String()) {
			t.Fatalf("%s 的答复 %q 没带用例原词", spec.outcome, message)
		}
	}
}

// TestFundsAnswerTurnsAPendingHandoffIntoUndecided 证裁决 5 单独点名的那一格：事实已采用（或已存在）而信封没出
// ——行已落、信封未出——不能与已登记同格，它需要人重跑同一命令补发；续办引用要打到答复里。
func TestFundsAnswerTurnsAPendingHandoffIntoUndecided(t *testing.T) {
	for _, outcome := range []application.FundsOutcome{application.FundsFactAdopted, application.FundsFactExisting} {
		message, code := fundsAnswer(commandExternalFundsFact, outcome, application.FundsUndecidedReasonNone,
			"CONT-0123456789abcdef", "SYN-FACT-1 版本 SYN-FACT-1/v1")
		if code != exitUndecided {
			t.Fatalf("%s 带续办引用 → %d（%s），要 %d", outcome, code, message, exitUndecided)
		}
		if !strings.Contains(message, "CONT-0123456789abcdef") || !strings.Contains(message, outcome.String()) {
			t.Fatalf("%s 的答复 %q 要同时带用例原词与续办引用", outcome, message)
		}
	}
}

// TestFundsAnswerNamesTheUndecidedReasonAndTheSubject 证未决带编排指名的原因、已登记带事实与版本字面（裁决 5
// 「stdout 打 FundsOutcome.String() 原名与版本字面」）。
func TestFundsAnswerNamesTheUndecidedReasonAndTheSubject(t *testing.T) {
	message, _ := fundsAnswer(commandExternalFundsFact, application.FundsUndecided, application.FundsFactStoreUnavailable, "", "")
	if !strings.Contains(message, application.FundsFactStoreUnavailable.String()) {
		t.Fatalf("未决答复 %q 没带原因", message)
	}
	message, _ = fundsAnswer(commandExternalFundsFact, application.FundsFactAdopted, application.FundsUndecidedReasonNone,
		"", "SYN-FACT-1 版本 SYN-FACT-1/v1")
	if !strings.Contains(message, "SYN-FACT-1/v1") {
		t.Fatalf("已采用答复 %q 没带版本字面", message)
	}
}
