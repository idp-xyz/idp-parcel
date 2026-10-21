package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: AT-TF-312 的自动形成那一格（ADR-0232 决定五）——段已采用委托，一项发生项经运营决定失效之后：自动形成（系统身份）对同一件
// 事实再交登记，答已存在、细格 OCCURRENCE_INVALIDATED_BY_DECISION、回显那一版失效版本，不长出替代版本；这一段的当前采用落册晚于那一次
// 失效（解除之后再采用）时，自动形成照常长出替代版本。操作者登记不受这一条管（TestReRegisteringAfterAnInvalidationReestablishesTheOccurrence）。
func TestAnAutomaticRegistrationDoesNotRegrowAnInvalidatedOccurrence(t *testing.T) {
	commission := trunkCommission(t, commissionShape{startedAt: occurrenceEnteredAt})
	rig := adoptedTrunkRig(t, commission)
	automatic := actualFulfillmentCommand(t)
	automatic.Provider, automatic.AgreementObject, automatic.AgreementVersion = "", "", ""
	automatic.Automatic = true
	original := registeredOccurrence(t, rig.register(t, automatic))
	if result := invalidate(t, invalidationHandler(rig, &scopeHoldDouble{}), original.Occurrence().String(), "V1", "SYN-EVIDENCE"); result.Outcome() != application.ChargeOccurrenceInvalidated {
		t.Fatalf("失效 = %q", result.Outcome())
	}

	kept := rig.register(t, automatic)
	echoed, _ := kept.Record()
	if kept.Outcome() != application.ChargeOccurrenceExisting || kept.Detail() != application.OccurrenceInvalidatedByDecision ||
		echoed.Key.Validity.String() != "V2" || !echoed.Occurrence.Invalidated() {
		t.Fatalf("失效之后自动形成 = %q/%q %v, want 已存在、回显失效版本", kept.Outcome(), kept.Detail(), echoed.Key)
	}
	if rig.registry.saves != 2 {
		t.Fatalf("自动形成长出了替代版本：落了 %d 条", rig.registry.saves)
	}

	segment := occurrenceValue(t, domain.NewFulfillmentSegmentReference, trunkSegment)
	readopted := rig.adoptions.records[segment]
	readopted.Generation, readopted.RecordedAt = 2, occurrenceNow.Add(2*time.Hour)
	rig.adoptions = adoptionViewDouble{records: map[domain.FulfillmentSegmentReference]ports.SegmentCommissionAdoptionRecord{segment: readopted}}
	regrown := rig.register(t, automatic)
	if record, _ := regrown.Record(); regrown.Outcome() != application.ChargeOccurrenceRegistered || record.Key.Validity.String() != "V3" {
		t.Fatalf("失效之后再采用，自动形成 = %q/%q, want 长出 V3", regrown.Outcome(), regrown.Detail())
	}
}
