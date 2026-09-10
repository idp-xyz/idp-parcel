package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉票 sa-cc/02 的编排半边：采用成功交一封（载荷只带引用，由消费方按引用回查——SA CONTEXT
// 「银行、支付或财务系统拥有实际付款……」，SA 只采用、只发引用）；重放不翻倍；冲突不交；
// 交接失败不翻结果、留续办引用（SA 各编排的既有形：版本已落、意图未入队）。

// Covers: sa-cc/02 完成判据 1「采用成功交意图一封；重放不交；冲突不交」。
func TestAdoptingAFundsFactHandsOffOneEnvelopeAndReplayDoesNotDouble(t *testing.T) {
	fixture := newFundsFixture(t)
	command := adoptCommand(t, domain.FundsReceiptConfirmed)

	adopted, err := fixture.handler.AdoptFact(context.Background(), command)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if adopted.Outcome() != application.FundsFactAdopted {
		t.Fatalf("outcome = %q, want FUNDS_FACT_ADOPTED", adopted.Outcome())
	}
	if adopted.FundsHandoffReference() != "" {
		t.Fatalf("交接成功不该留续办引用，实得 %q", adopted.FundsHandoffReference())
	}
	if got := len(fixture.factHandoff.intents); got != 1 {
		t.Fatalf("采用成功后意图数 = %d, want 1", got)
	}
	for _, intent := range fixture.factHandoff.intents {
		if intent.Record.Key.Fact.String() != command.Fact {
			t.Fatalf("意图携带的事实引用 = %q, want %q", intent.Record.Key.Fact.String(), command.Fact)
		}
		if intent.Record.Fact.Version().String() != command.Version {
			t.Fatalf("意图携带的采用版本 = %q, want %q", intent.Record.Fact.Version().String(), command.Version)
		}
	}

	replay, err := fixture.handler.AdoptFact(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.FundsFactExisting {
		t.Fatalf("replay outcome = %q, want EXISTING_FUNDS_FACT", replay.Outcome())
	}
	if got := len(fixture.factHandoff.intents); got != 1 {
		t.Fatalf("重放后意图数 = %d, want 1——重放交的必须是同一份，由认领键吞掉", got)
	}

	flipped := command
	flipped.AmountMinor = 21000
	conflict, err := fixture.handler.AdoptFact(context.Background(), flipped)
	if err != nil {
		t.Fatalf("conflict adopt: %v", err)
	}
	if conflict.Outcome() != application.FundsFactConflict {
		t.Fatalf("conflict outcome = %q", conflict.Outcome())
	}
	if got := len(fixture.factHandoff.intents); got != 1 {
		t.Fatalf("冲突后意图数 = %d, want 1——冲突的那一份没有被采用，无物可交", got)
	}
}

// Covers: sa-cc/02 完成判据 1「交接失败按仓内既有形（版本已落、意图未入队 → 技术未形成 +
// 续办引用）」：事实已采用不翻成未决，续办引用非空；重放时同一份再交一次即补上那封。
func TestAFailedFundsFactHandoffLeavesAContinuationAndReplayResends(t *testing.T) {
	fixture := newFundsFixture(t)
	fixture.factHandoff.err = errors.New("outbox unavailable")
	command := adoptCommand(t, domain.FundsReceiptConfirmed)

	adopted, err := fixture.handler.AdoptFact(context.Background(), command)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if adopted.Outcome() != application.FundsFactAdopted {
		t.Fatalf("outcome = %q, want FUNDS_FACT_ADOPTED——事实已落库，交接失败不翻它", adopted.Outcome())
	}
	if adopted.FundsHandoffReference() == "" {
		t.Fatal("交接失败必须留续办引用")
	}
	factRef, err := domain.NewFundsFactReference(command.Fact)
	if err != nil {
		t.Fatalf("fact reference: %v", err)
	}
	if _, stored, _ := fixture.facts.FindByKey(context.Background(), ports.FundsFactKey{TenantID: command.TenantID, Fact: factRef}); !stored {
		t.Fatal("事实引用该已采用")
	}

	fixture.factHandoff.err = nil
	replay, err := fixture.handler.AdoptFact(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.FundsFactExisting {
		t.Fatalf("replay outcome = %q", replay.Outcome())
	}
	if replay.FundsHandoffReference() != "" {
		t.Fatalf("补交成功后不该再留续办引用，实得 %q", replay.FundsHandoffReference())
	}
	if got := len(fixture.factHandoff.intents); got != 1 {
		t.Fatalf("补交后意图数 = %d, want 1", got)
	}
}
