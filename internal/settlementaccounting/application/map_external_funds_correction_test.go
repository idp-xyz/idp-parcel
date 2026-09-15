package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉票 sa-cc/20 裁决 2：更正是采用的一种，走既有采用用例的「更正」格——新版本由 domain.CorrectAmount 从当前链头
// 形成、回指链头、落版本行、复用同一交接口再发一封（信封 ID 带新版本、载荷回指前版）；同（事实、版本）重放 / 冲突
// 照 AdoptFact 四格；回指非链头与更正未采用的事实都是`未受理`（本上下文是铸造方，与 CC 作为接收方容忍乱序不同）。

func correctCommand(t *testing.T) application.CorrectFundsFactCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.CorrectFundsFactCommand{
		TenantID:    tenant,
		Fact:        "bank-receipt-1",
		Corrects:    "bank-receipt-1/v1",
		Version:     "bank-receipt-1/v2",
		AmountMinor: 18000,
		CorrectedAt: fundsOccurredAt.Add(2 * time.Hour),
	}
}

// adoptFirstVersion 先把首版采用进去，更正格的每个用例都从「v1 已采用」出发。
func adoptFirstVersion(t *testing.T, fixture *fundsFixture) application.AdoptFundsFactCommand {
	t.Helper()
	command := adoptCommand(t, domain.FundsReceiptConfirmed)
	command.Payer = "payer-customer-7"
	adopted, err := fixture.handler.AdoptFact(context.Background(), command)
	if err != nil || adopted.Outcome() != application.FundsFactAdopted {
		t.Fatalf("首版采用：outcome = %q err = %v", adopted.Outcome(), err)
	}
	return command
}

// Covers: sa-cc/20 完成判据 (1)「v1 已采用 → 更正 v2（回指 v1）→ 版本行 +1、身份行不变、再发一封（信封 ID 带 v2、
// 载荷 corrects = v1）；FindByKey 交回 v2、FindVersion(v1) 仍原样」。内容列（来源 / 付款人 / 种类 / 币种 / 发生时刻）
// 从链头照抄，只有金额变。
func TestCorrectingTheChainHeadAdoptsANewVersionThatPointsBackAndHandsOffASecondEnvelope(t *testing.T) {
	fixture := newFundsFixture(t)
	first := adoptFirstVersion(t, fixture)
	command := correctCommand(t)

	corrected, err := fixture.handler.CorrectFact(context.Background(), command)
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if corrected.Outcome() != application.FundsFactAdopted {
		t.Fatalf("outcome = %q, want FUNDS_FACT_ADOPTED——更正版本被采用", corrected.Outcome())
	}
	if corrected.FundsHandoffReference() != "" {
		t.Fatalf("交接成功不该留续办引用，实得 %q", corrected.FundsHandoffReference())
	}
	record, ok := corrected.Fact()
	if !ok {
		t.Fatal("更正结果该带已采用的记录")
	}
	if record.Fact.Version().String() != command.Version {
		t.Fatalf("新版本 = %q, want %q", record.Fact.Version(), command.Version)
	}
	if predecessor, present := record.Fact.Corrects(); !present || predecessor.String() != command.Corrects {
		t.Fatalf("回指 = (%q, %v), want %q", predecessor, present, command.Corrects)
	}
	if correctedAt, present := record.Fact.CorrectedAt(); !present || !correctedAt.Equal(command.CorrectedAt) {
		t.Fatalf("更正时刻 = (%v, %v), want %v", correctedAt, present, command.CorrectedAt)
	}
	currency, amount := record.Fact.Amount()
	if amount != command.AmountMinor || currency.String() != first.Currency {
		t.Fatalf("金额 = %d %s, want %d %s", amount, currency, command.AmountMinor, first.Currency)
	}
	if record.Fact.Source().String() != first.Source || record.Fact.Kind() != first.Kind || !record.Fact.OccurredAt().Equal(first.OccurredAt) {
		t.Fatalf("来源 / 种类 / 发生时刻该从链头照抄：%#v", record.Fact)
	}
	if payer, provided := record.Fact.Payer(); !provided || payer.String() != first.Payer {
		t.Fatalf("付款人该同型带着走：(%q, %v)", payer, provided)
	}
	if record.RecordedAt != fundsNowAt {
		t.Fatalf("RecordedAt = %v, want 时钟 %v", record.RecordedAt, fundsNowAt)
	}

	if got := len(fixture.factHandoff.intents); got != 2 {
		t.Fatalf("两版之后意图数 = %d, want 2——更正再发一封，同一事件类型", got)
	}
	second, sent := fixture.factHandoff.intents[fundsFactKey(record.Key)+"|"+command.Version]
	if !sent {
		t.Fatalf("没有一封信封的 ID 带新版本 %q：%v", command.Version, fixture.factHandoff.intents)
	}
	if predecessor, present := second.Record.Fact.Corrects(); !present || predecessor.String() != command.Corrects {
		t.Fatalf("第二封的回指 = (%q, %v), want %q", predecessor, present, command.Corrects)
	}

	head, found, err := fixture.facts.FindByKey(context.Background(), record.Key)
	if err != nil || !found || head.Fact.Version().String() != command.Version {
		t.Fatalf("链头 = %q found=%v err=%v, want v2", head.Fact.Version(), found, err)
	}
	firstVersion, found, err := fixture.facts.FindVersion(context.Background(), record.Key, saVersion(t, command.Corrects))
	if err != nil || !found {
		t.Fatalf("v1 该仍在：found=%v err=%v", found, err)
	}
	if _, amount := firstVersion.Fact.Amount(); amount != first.AmountMinor {
		t.Fatalf("v1 金额 = %d, want 原样 %d——更正不改写前版", amount, first.AmountMinor)
	}
	if _, present := firstVersion.Fact.Corrects(); present {
		t.Fatal("v1 是首版，不该长出回指")
	}
}

// Covers: sa-cc/20 裁决 2「同（事实、版本）重放 → 同摘要`已采用` / 异摘要`冲突`，照 AdoptFact 四格」。重放再交同一封
// 由认领键吞掉；冲突的那一份没被采用，无物可交。
func TestReplayingACorrectionAnswersExistingAndADifferentAmountUnderTheSameVersionIsAConflict(t *testing.T) {
	fixture := newFundsFixture(t)
	adoptFirstVersion(t, fixture)
	command := correctCommand(t)
	if first, err := fixture.handler.CorrectFact(context.Background(), command); err != nil || first.Outcome() != application.FundsFactAdopted {
		t.Fatalf("首次更正：outcome = %q err = %v", first.Outcome(), err)
	}

	replay, err := fixture.handler.CorrectFact(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.FundsFactExisting {
		t.Fatalf("replay outcome = %q, want EXISTING_FUNDS_FACT", replay.Outcome())
	}
	if record, ok := replay.Fact(); !ok || record.Fact.Version().String() != command.Version {
		t.Fatalf("重放该交回已采用的 v2：%v %#v", ok, record.Fact)
	}
	if got := len(fixture.factHandoff.intents); got != 2 {
		t.Fatalf("重放后意图数 = %d, want 2——重放交的是同一封", got)
	}

	flipped := command
	flipped.AmountMinor = 17000
	conflict, err := fixture.handler.CorrectFact(context.Background(), flipped)
	if err != nil {
		t.Fatalf("conflict: %v", err)
	}
	if conflict.Outcome() != application.FundsFactConflict {
		t.Fatalf("conflict outcome = %q, want FUNDS_FACT_CONFLICT", conflict.Outcome())
	}
	if got := len(fixture.factHandoff.intents); got != 2 {
		t.Fatalf("冲突后意图数 = %d, want 2——冲突的那一份没被采用，无物可交", got)
	}
	if head, _, _ := fixture.facts.FindByKey(context.Background(), ports.FundsFactKey{TenantID: command.TenantID, Fact: saFact(t, command.Fact)}); func() int64 {
		_, amount := head.Fact.Amount()
		return amount
	}() != command.AmountMinor {
		t.Fatal("冲突顶替了先到的 v2")
	}
}

// Covers: 幂等的单位是（事实、版本）——更正 v2 落下之后，首版 v1 的采用命令重放仍答`已采用`并交回 v1 那一份
// （不是拿链头 v2 的摘要去比、答一个不存在的冲突）；同一事实换一个首版字面才是`冲突`。
func TestReplayingTheFirstVersionAfterACorrectionStillAnswersExisting(t *testing.T) {
	fixture := newFundsFixture(t)
	first := adoptFirstVersion(t, fixture)
	if corrected, err := fixture.handler.CorrectFact(context.Background(), correctCommand(t)); err != nil || corrected.Outcome() != application.FundsFactAdopted {
		t.Fatalf("更正：outcome = %q err = %v", corrected.Outcome(), err)
	}

	replay, err := fixture.handler.AdoptFact(context.Background(), first)
	if err != nil {
		t.Fatalf("replay v1: %v", err)
	}
	if replay.Outcome() != application.FundsFactExisting {
		t.Fatalf("v2 在链头时重放 v1 的采用：outcome = %q, want EXISTING_FUNDS_FACT", replay.Outcome())
	}
	if record, ok := replay.Fact(); !ok || record.Fact.Version().String() != first.Version {
		t.Fatalf("重放该交回 v1 那一份：%v %q", ok, record.Fact.Version())
	}
	if got := len(fixture.factHandoff.intents); got != 2 {
		t.Fatalf("重放后意图数 = %d, want 2（v1 那封由认领键吞掉）", got)
	}

	anotherFirst := first
	anotherFirst.Version = "bank-receipt-1/v1-bis"
	conflict, err := fixture.handler.AdoptFact(context.Background(), anotherFirst)
	if err != nil {
		t.Fatalf("another first version: %v", err)
	}
	if conflict.Outcome() != application.FundsFactConflict {
		t.Fatalf("同一事实换一个首版字面：outcome = %q, want FUNDS_FACT_CONFLICT", conflict.Outcome())
	}
}

// Covers: sa-cc/20 裁决 2「Corrects 等于当前链头版本（指向非链头 → `未受理`）」与「事实已采用（否则`未受理`）」。
// 本上下文是铸造方：纠正一个不是当前的版本是调用方编程错误，与 CC 作为接收方容忍乱序不同。
func TestCorrectingANonHeadVersionOrAnUnadoptedFactIsNotAccepted(t *testing.T) {
	fixture := newFundsFixture(t)
	adoptFirstVersion(t, fixture)
	command := correctCommand(t)
	if first, err := fixture.handler.CorrectFact(context.Background(), command); err != nil || first.Outcome() != application.FundsFactAdopted {
		t.Fatalf("首次更正：outcome = %q err = %v", first.Outcome(), err)
	}

	stale := command
	stale.Version = "bank-receipt-1/v3"
	// Corrects 仍指 v1，而链头已是 v2。
	result, err := fixture.handler.CorrectFact(context.Background(), stale)
	if err != nil {
		t.Fatalf("stale: %v", err)
	}
	if result.Outcome() != application.FundsNotAccepted {
		t.Fatalf("回指非链头 outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
	}
	if _, found, _ := fixture.facts.FindVersion(context.Background(),
		ports.FundsFactKey{TenantID: command.TenantID, Fact: saFact(t, command.Fact)}, saVersion(t, stale.Version)); found {
		t.Fatal("被拒的 v3 落进了库")
	}
	if got := len(fixture.factHandoff.intents); got != 2 {
		t.Fatalf("被拒后意图数 = %d, want 2", got)
	}

	unadopted := command
	unadopted.Fact = "bank-receipt-never"
	unadopted.Corrects = "bank-receipt-never/v1"
	unadopted.Version = "bank-receipt-never/v2"
	result, err = fixture.handler.CorrectFact(context.Background(), unadopted)
	if err != nil {
		t.Fatalf("unadopted: %v", err)
	}
	if result.Outcome() != application.FundsNotAccepted {
		t.Fatalf("更正未采用的事实 outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
	}
}

// Covers: 命令自身的形——金额非正、版本或回指空白、更正时刻早于业务发生时刻、新版本字面等于回指——都停在`未受理`，
// 不落库、不交。
func TestAMalformedCorrectionCommandIsNotAccepted(t *testing.T) {
	fixture := newFundsFixture(t)
	adoptFirstVersion(t, fixture)

	for name, mutate := range map[string]func(*application.CorrectFundsFactCommand){
		"金额为零":   func(command *application.CorrectFundsFactCommand) { command.AmountMinor = 0 },
		"金额为负":   func(command *application.CorrectFundsFactCommand) { command.AmountMinor = -1 },
		"新版本空白":  func(command *application.CorrectFundsFactCommand) { command.Version = "  " },
		"回指空白":   func(command *application.CorrectFundsFactCommand) { command.Corrects = "" },
		"事实引用空白": func(command *application.CorrectFundsFactCommand) { command.Fact = "" },
		"更正时刻缺席": func(command *application.CorrectFundsFactCommand) { command.CorrectedAt = time.Time{} },
		"更正时刻早于业务发生时刻": func(command *application.CorrectFundsFactCommand) {
			command.CorrectedAt = fundsOccurredAt.Add(-time.Minute)
		},
		"新版本等于回指": func(command *application.CorrectFundsFactCommand) { command.Version = command.Corrects },
	} {
		t.Run(name, func(t *testing.T) {
			command := correctCommand(t)
			mutate(&command)
			result, err := fixture.handler.CorrectFact(context.Background(), command)
			if err != nil {
				t.Fatalf("correct: %v", err)
			}
			if result.Outcome() != application.FundsNotAccepted {
				t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
			}
		})
	}
	if got := len(fixture.factHandoff.intents); got != 1 {
		t.Fatalf("坏命令之后意图数 = %d, want 1（只有首版那一封）", got)
	}
	if head, _, _ := fixture.facts.FindByKey(context.Background(), ports.FundsFactKey{
		TenantID: correctCommand(t).TenantID, Fact: saFact(t, "bank-receipt-1")}); head.Fact.Version().String() != "bank-receipt-1/v1" {
		t.Fatalf("链头 = %q, want 仍是 v1", head.Fact.Version())
	}
}

// Covers: 库不可达 → `未决` 指名资金事实库；交接失败 → 版本已落、结果不翻、留续办引用，重放补交同一封（与 AdoptFact 同形）。
func TestACorrectionStopsUndecidedOnStoreErrorsAndKeepsAContinuationWhenTheHandoffFails(t *testing.T) {
	fixture := newFundsFixture(t)
	adoptFirstVersion(t, fixture)
	command := correctCommand(t)

	fixture.facts.findErr = errors.New("store unavailable")
	undecided, err := fixture.handler.CorrectFact(context.Background(), command)
	if err != nil {
		t.Fatalf("undecided: %v", err)
	}
	if undecided.Outcome() != application.FundsUndecided || undecided.UndecidedReason() != application.FundsFactStoreUnavailable {
		t.Fatalf("outcome = %q reason = %q, want FUNDS_UNDECIDED / FUNDS_FACT_STORE_UNAVAILABLE", undecided.Outcome(), undecided.UndecidedReason())
	}
	if undecided.ContinuationReference() == "" {
		t.Fatal("未决必须留续办引用")
	}
	fixture.facts.findErr = nil

	fixture.factHandoff.err = errors.New("outbox unavailable")
	adopted, err := fixture.handler.CorrectFact(context.Background(), command)
	if err != nil {
		t.Fatalf("correct with failing handoff: %v", err)
	}
	if adopted.Outcome() != application.FundsFactAdopted {
		t.Fatalf("outcome = %q, want FUNDS_FACT_ADOPTED——版本已落库，交接失败不翻它", adopted.Outcome())
	}
	if adopted.FundsHandoffReference() == "" {
		t.Fatal("交接失败必须留续办引用")
	}
	if got := len(fixture.factHandoff.intents); got != 1 {
		t.Fatalf("交接失败时意图数 = %d, want 1（只有首版那一封）", got)
	}

	fixture.factHandoff.err = nil
	replay, err := fixture.handler.CorrectFact(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.FundsFactExisting || replay.FundsHandoffReference() != "" {
		t.Fatalf("补交：outcome = %q handoff = %q, want EXISTING 且无续办引用", replay.Outcome(), replay.FundsHandoffReference())
	}
	if got := len(fixture.factHandoff.intents); got != 2 {
		t.Fatalf("补交后意图数 = %d, want 2", got)
	}
}

func saVersion(t *testing.T, raw string) domain.FundsFactVersion {
	t.Helper()
	version, err := domain.NewFundsFactVersion(raw)
	if err != nil {
		t.Fatalf("version %q: %v", raw, err)
	}
	return version
}

func saFact(t *testing.T, raw string) domain.FundsFactReference {
	t.Helper()
	fact, err := domain.NewFundsFactReference(raw)
	if err != nil {
		t.Fatalf("fact %q: %v", raw, err)
	}
	return fact
}
