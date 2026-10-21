package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证发生项失效的生产入口与失效之后的重登（票 demo-chain-gaps/57，ADR-0232 决定四、五）。夹具复用登记用例那一套合成串，
// 证据只记 S。

const invalidatingOperator = "https://dex.syn.example#SYN-OPERATOR-CORRECTION-01"

// scopeHoldDouble 记下取过哪几个范围的锁；err 非空时取锁失败。
type scopeHoldDouble struct {
	held []domain.OccurrenceScopeReference
	err  error
}

func (double *scopeHoldDouble) HoldOccurrenceScope(
	_ context.Context, _ domain.TenantID, _ domain.ChargeOccurrenceReason, scope domain.OccurrenceScopeReference,
) error {
	if double.err != nil {
		return double.err
	}
	double.held = append(double.held, scope)
	return nil
}

type laterClock struct{ at time.Time }

func (clock laterClock) Now() time.Time { return clock.at }

func invalidationHandler(rig *occurrenceRig, scopes *scopeHoldDouble) *application.InvalidateChargeOccurrenceHandler {
	return application.NewInvalidateChargeOccurrenceHandler(application.InvalidateChargeOccurrenceDeps{
		Occurrences: rig.registry,
		Scopes:      scopes,
		Handoff:     rig.handoff,
		Clock:       laterClock{at: occurrenceNow.Add(time.Hour)},
	})
}

func invalidate(
	t *testing.T, handler *application.InvalidateChargeOccurrenceHandler, occurrence, version, basis string,
) application.ChargeOccurrenceInvalidationResult {
	t.Helper()
	result, err := handler.Invalidate(t.Context(), application.InvalidateChargeOccurrenceCommand{
		TenantID:   occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Operator:   invalidatingOperator,
		Occurrence: occurrence,
		Version:    version,
		Basis:      basis,
	})
	if err != nil {
		t.Fatalf("失效不该上抛：%v", err)
	}
	return result
}

// Covers: AT-TF-311——已登记的发生项经运营决定失效：长出 V2、回指 V1、标失效，依据与修订者是这一次决定的，先取它那个范围的锁；
// 失效版本与已形成信封同一次调用交出，V1 一字不动。同一次原样再交答重放、回显 V2 并重交同一份意图；点名 V1 而依据不同答点名的
// 不是链头；点名 V2 答已失效；不在册答不在册。
func TestAnOccurrenceIsInvalidatedThroughTheProductionEntry(t *testing.T) {
	rig := newOccurrenceRig(t)
	registered := rig.register(t, actualFulfillmentCommand(t))
	original, _ := registered.Record()
	scopes := &scopeHoldDouble{}
	handler := invalidationHandler(rig, scopes)

	result := invalidate(t, handler, original.Key.Occurrence.String(), "V1", "SYN-EVIDENCE-WRONG-ADOPTION")
	if result.Outcome() != application.ChargeOccurrenceInvalidated {
		t.Fatalf("outcome = %q, want OCCURRENCE_INVALIDATED", result.Outcome())
	}
	record, _ := result.Record()
	corrects, _ := record.Occurrence.Corrects()
	_, basis, _, _ := record.Occurrence.Revision()
	reviser, _ := record.Occurrence.RevisedBy()
	if record.Key.Validity.String() != "V2" || corrects.String() != "V1" || !record.Occurrence.Invalidated() ||
		basis.String() != "SYN-EVIDENCE-WRONG-ADOPTION" || reviser.String() != invalidatingOperator {
		t.Fatalf("失效版本 = %+v", record)
	}
	if len(scopes.held) == 0 || scopes.held[0] != original.Occurrence.Scope() {
		t.Fatalf("没先取范围锁：%v", scopes.held)
	}
	if kept := rig.registry.records[original.Key]; kept.Occurrence.Invalidated() || len(rig.handoff.intents) != 2 ||
		rig.handoff.intents[1].Record.Key != record.Key {
		t.Fatalf("V1 被改或意图没随失效版本交出：%d 份", len(rig.handoff.intents))
	}

	replay := invalidate(t, handler, original.Key.Occurrence.String(), "V1", "SYN-EVIDENCE-WRONG-ADOPTION")
	if echoed, _ := replay.Record(); replay.Outcome() != application.ChargeOccurrenceInvalidationExisting || echoed.Key != record.Key ||
		len(rig.handoff.intents) != 3 {
		t.Fatalf("重放 = %q %v，意图 %d 份", replay.Outcome(), echoed.Key, len(rig.handoff.intents))
	}
	stale := invalidate(t, handler, original.Key.Occurrence.String(), "V1", "SYN-ANOTHER-EVIDENCE")
	if echoed, _ := stale.Record(); stale.Outcome() != application.ChargeOccurrenceInvalidationVersionNotCurrent || echoed.Key != record.Key {
		t.Fatalf("点名旧版、依据不同 = %q %v", stale.Outcome(), echoed.Key)
	}
	again := invalidate(t, handler, original.Key.Occurrence.String(), "V2", "SYN-EVIDENCE-WRONG-ADOPTION")
	if again.Outcome() != application.ChargeOccurrenceInvalidationAlreadyInvalidated {
		t.Fatalf("链头已失效再失效 = %q", again.Outcome())
	}
	if missing := invalidate(t, handler, "TCO-0000", "V1", "SYN-EVIDENCE"); missing.Outcome() != application.ChargeOccurrenceInvalidationNotFound {
		t.Fatalf("不在册 = %q", missing.Outcome())
	}
	if rig.registry.saves != 2 {
		t.Fatalf("落了 %d 条，want 首版与失效版各一条", rig.registry.saves)
	}
}

func TestAnInvalidationThatCannotHoldItsScopeOrHandItOffIsUndecided(t *testing.T) {
	rig := newOccurrenceRig(t)
	registered := rig.register(t, actualFulfillmentCommand(t))
	original, _ := registered.Record()

	locked := invalidate(t, invalidationHandler(rig, &scopeHoldDouble{err: errors.New("lock down")}),
		original.Key.Occurrence.String(), "V1", "SYN-EVIDENCE")
	if locked.Outcome() != application.ChargeOccurrenceInvalidationUndecided ||
		locked.UndecidedReason() != application.ChargeOccurrenceInvalidationRegistryUnavailable || locked.ContinuationReference() == "" {
		t.Fatalf("取锁失败 = %q/%q", locked.Outcome(), locked.UndecidedReason())
	}
	rig.handoff.err = errors.New("outbox down")
	handedOff := invalidate(t, invalidationHandler(rig, &scopeHoldDouble{}), original.Key.Occurrence.String(), "V1", "SYN-EVIDENCE")
	if handedOff.Outcome() != application.ChargeOccurrenceInvalidationUndecided ||
		handedOff.UndecidedReason() != application.ChargeOccurrenceInvalidationHandoffUnavailable {
		t.Fatalf("交信封失败 = %q/%q", handedOff.Outcome(), handedOff.UndecidedReason())
	}
	for _, malformed := range []application.InvalidateChargeOccurrenceCommand{
		{TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant), Operator: invalidatingOperator, Occurrence: "", Version: "V1", Basis: "B"},
		{TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant), Operator: invalidatingOperator, Occurrence: "TCO-1", Version: "", Basis: "B"},
		{TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant), Operator: invalidatingOperator, Occurrence: "TCO-1", Version: "V1", Basis: ""},
		{TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant), Operator: "", Occurrence: "TCO-1", Version: "V1", Basis: "B"},
	} {
		result, err := invalidationHandler(rig, &scopeHoldDouble{}).Invalidate(t.Context(), malformed)
		if err != nil || result.Outcome() != application.ChargeOccurrenceInvalidationNotAccepted {
			t.Fatalf("%+v = %q, %v", malformed, result.Outcome(), err)
		}
	}
}

// Covers: AT-TF-312——失效之后同一件事实再登：不再答已存在、回显已失效的首版，而是从失效版本长出替代版本 V3，内容取这一次的判断
// （指错了委托时采购上下文正要换），信封随它交出；之后原样重登答已存在、回显 V3，异内容答冲突。声明自营也不再与已失效的那一项相抵。
func TestReRegisteringAfterAnInvalidationReestablishesTheOccurrence(t *testing.T) {
	rig := newOccurrenceRig(t)
	registered := rig.register(t, actualFulfillmentCommand(t))
	original, _ := registered.Record()
	if result := invalidate(t, invalidationHandler(rig, &scopeHoldDouble{}), original.Key.Occurrence.String(), "V1", "SYN-EVIDENCE"); result.Outcome() != application.ChargeOccurrenceInvalidated {
		t.Fatalf("失效 = %q", result.Outcome())
	}

	again := rig.register(t, actualFulfillmentCommand(t))
	if again.Outcome() != application.ChargeOccurrenceRegistered {
		t.Fatalf("失效之后重登 = %q/%q, want REGISTERED", again.Outcome(), again.Detail())
	}
	reestablished, _ := again.Record()
	corrects, _ := reestablished.Occurrence.Corrects()
	kind, _, _, _ := reestablished.Occurrence.Revision()
	if reestablished.Key.Occurrence != original.Key.Occurrence || reestablished.Key.Validity.String() != "V3" ||
		corrects.String() != "V2" || kind != domain.OccurrenceSuperseded || reestablished.Occurrence.Invalidated() {
		t.Fatalf("替代版本 = %+v", reestablished.Key)
	}
	if last := rig.handoff.intents[len(rig.handoff.intents)-1]; last.Record.Key != reestablished.Key {
		t.Fatalf("意图没随替代版本交出：%v", last.Record.Key)
	}

	replay := rig.register(t, actualFulfillmentCommand(t))
	if echoed, _ := replay.Record(); replay.Outcome() != application.ChargeOccurrenceExisting || echoed.Key != reestablished.Key {
		t.Fatalf("重登替代版本 = %q %v", replay.Outcome(), echoed.Key)
	}
	changed := actualFulfillmentCommand(t)
	changed.Journey = "SYN-JRN-ANOTHER"
	if conflict := rig.register(t, changed); conflict.Outcome() != application.ChargeOccurrenceConflict {
		t.Fatalf("异内容 = %q, want CONFLICT", conflict.Outcome())
	}
	if rig.registry.saves != 3 {
		t.Fatalf("落了 %d 条，want V1、V2、V3", rig.registry.saves)
	}
}

func TestADeclaredSelfOperationNoLongerCollidesWithAnInvalidatedOccurrence(t *testing.T) {
	rig := newOccurrenceRig(t)
	registered := rig.register(t, actualFulfillmentCommand(t))
	original, _ := registered.Record()
	declared := actualFulfillmentCommand(t)
	declared.SelfOperated, declared.Provider, declared.AgreementObject, declared.AgreementVersion = true, "", "", ""
	if before := rig.register(t, declared); before.Outcome() != application.ChargeOccurrenceConflict {
		t.Fatalf("失效之前声明自营 = %q, want CONFLICT", before.Outcome())
	}
	// 替身的覆盖一问不认修订版，这里把已失效那一项从覆盖里拿掉，只看按身份那一问。
	invalidate(t, invalidationHandler(rig, &scopeHoldDouble{}), original.Key.Occurrence.String(), "V1", "SYN-EVIDENCE")
	rig.registry.ignoreInvalidatedInCoverage = true
	if after := rig.register(t, declared); after.Outcome() != application.ChargeOccurrenceNotApplicable {
		t.Fatalf("失效之后声明自营 = %q, want NOT_APPLICABLE", after.Outcome())
	}
}
