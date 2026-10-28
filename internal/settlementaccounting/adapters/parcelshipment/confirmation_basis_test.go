package parcelshipment_test

import (
	"context"
	"errors"
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelshipment"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 钉 SA 读 PS 终局作确认依据的两只适配器（ADR-0336 决定二）：终局信封那一头按四维回读、只认已终局的有效交付，交给
// 「记依据并按时点确认」的是包裹与内置依据种类、引用与 SELL 来源事实同形；费用形成那一头按包裹读当前终局，同一判据。

type chargeConfirmerDouble struct {
	parcels []string
	bases   []saapplication.ConfirmationBasis
	err     error
}

func (double *chargeConfirmerDouble) OnParcel(
	_ context.Context,
	_ sadomain.TenantID,
	parcel sadomain.ParcelReference,
	basis saapplication.ConfirmationBasis,
) (saapplication.ConfirmOnBasisResult, error) {
	double.parcels = append(double.parcels, parcel.String())
	double.bases = append(double.bases, basis)
	return saapplication.ConfirmOnBasisResult{}, double.err
}

// Covers: AT-SA-218（ADR-0336 决定二）——已终局的有效交付交给确认那一口：包裹照信封，种类是内置的 EFFECTIVE_DELIVERY_FINALIZED，引用
// 是带前缀的终局采用键；不采用的终局与别的责任结果种类入账、不问确认。
func TestAFinalizedEffectiveDeliveryIsHandedToConfirmation(t *testing.T) {
	key := finalKey(t, psdomain.EffectiveDeliveryOutcome)
	confirmer := &chargeConfirmerDouble{}
	handler, err := adapter.NewConfirmOnFinalOutcomeAdapter(
		&finalOutcomesDouble{records: map[psports.FinalAdoptionKey]psports.FinalOutcomeRecord{key: finalizedRecord(t, key)}},
		confirmer)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := handler.HandleFormedFinalOutcome(t.Context(), formedFinalOf(key)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(confirmer.parcels) != 1 || confirmer.parcels[0] != "SYN-PARCEL-1" {
		t.Fatalf("交给确认的包裹 = %v", confirmer.parcels)
	}
	basis := confirmer.bases[0]
	if basis.Kind.String() != "EFFECTIVE_DELIVERY_FINALIZED" ||
		basis.Reference.String() != "final-outcome/SYN-PARCEL-1/EFFECTIVE_DELIVERY/SYN-OUTCOME-V1" {
		t.Fatalf("依据 = %s / %s", basis.Kind, basis.Reference)
	}

	returnedKey := finalKey(t, psdomain.ReturnCompletedOutcome)
	refused := finalKey(t, psdomain.EffectiveDeliveryOutcome)
	for name, records := range map[string]map[psports.FinalAdoptionKey]psports.FinalOutcomeRecord{
		"退运完成": {returnedKey: finalizedRecord(t, returnedKey)},
		"不采用":  {refused: {Key: refused, RefusalBasis: must(t, psdomain.NewCheckReason, "FINAL_RULE_NOT_SATISFIED/SYN"), AdoptedAt: adoptionRecordedAt}},
	} {
		quiet := &chargeConfirmerDouble{}
		handler, err := adapter.NewConfirmOnFinalOutcomeAdapter(&finalOutcomesDouble{records: records}, quiet)
		if err != nil {
			t.Fatal(err)
		}
		for key := range records {
			if err := handler.HandleFormedFinalOutcome(t.Context(), formedFinalOf(key)); err != nil {
				t.Fatalf("%s 应入账：%v", name, err)
			}
		}
		if len(quiet.parcels) != 0 {
			t.Fatalf("%s 问了确认", name)
		}
	}
}

// Covers: AT-SA-218（ADR-0336 决定二）——终局读不回是可见性滞后（续办哨兵）；确认那一口交回的错误原样上抛。
func TestConfirmationOnFinalOutcomeSortsLagFromBreakage(t *testing.T) {
	key := finalKey(t, psdomain.EffectiveDeliveryOutcome)
	handler, err := adapter.NewConfirmOnFinalOutcomeAdapter(&finalOutcomesDouble{}, &chargeConfirmerDouble{})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.HandleFormedFinalOutcome(t.Context(), formedFinalOf(key)); !errors.Is(err, adapter.ErrFinalOutcomeNotVisible) {
		t.Fatalf("读不回 err = %v", err)
	}
	broken := errors.New("synthetic confirm failure")
	handler, err = adapter.NewConfirmOnFinalOutcomeAdapter(
		&finalOutcomesDouble{records: map[psports.FinalAdoptionKey]psports.FinalOutcomeRecord{key: finalizedRecord(t, key)}},
		&chargeConfirmerDouble{err: broken})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.HandleFormedFinalOutcome(t.Context(), formedFinalOf(key)); !errors.Is(err, broken) {
		t.Fatalf("确认出错 err = %v", err)
	}
}

type currentFinalsDouble struct {
	record psports.FinalOutcomeRecord
	found  bool
	err    error
}

func (double currentFinalsDouble) FindCurrentFinal(
	context.Context, psdomain.TenantID, psdomain.DeclaredParcelID,
) (psports.FinalOutcomeRecord, bool, error) {
	return double.record, double.found, double.err
}

// Covers: AT-SA-219（ADR-0336 决定二）——费用形成那一头按包裹读当前终局：已终局的有效交付交回同形的依据引用，其余答没有。
func TestTheEffectiveDeliveryFinalViewOnlyAnswersAFinalizedEffectiveDelivery(t *testing.T) {
	tenant := must(t, sadomain.NewTenantID, "SYN-TENANT-1")
	parcel := must(t, sadomain.NewParcelReference, "SYN-PARCEL-1")
	key := finalKey(t, psdomain.EffectiveDeliveryOutcome)

	view, err := adapter.NewEffectiveDeliveryFinalAdapter(currentFinalsDouble{record: finalizedRecord(t, key), found: true})
	if err != nil {
		t.Fatal(err)
	}
	basis, found, err := view.FindEffectiveDeliveryFinal(t.Context(), tenant, parcel)
	if err != nil || !found || basis.String() != "final-outcome/SYN-PARCEL-1/EFFECTIVE_DELIVERY/SYN-OUTCOME-V1" {
		t.Fatalf("有效交付 = %s %v err %v", basis, found, err)
	}

	returnedKey := finalKey(t, psdomain.ReturnCompletedOutcome)
	for name, source := range map[string]currentFinalsDouble{
		"没有终局": {},
		"退运完成": {record: finalizedRecord(t, returnedKey), found: true},
	} {
		view, err := adapter.NewEffectiveDeliveryFinalAdapter(source)
		if err != nil {
			t.Fatal(err)
		}
		if _, found, err := view.FindEffectiveDeliveryFinal(t.Context(), tenant, parcel); err != nil || found {
			t.Fatalf("%s = %v err %v，want 没有", name, found, err)
		}
	}

	broken := errors.New("synthetic final store down")
	view, err = adapter.NewEffectiveDeliveryFinalAdapter(currentFinalsDouble{err: broken})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := view.FindEffectiveDeliveryFinal(t.Context(), tenant, parcel); !errors.Is(err, broken) {
		t.Fatalf("读口故障 err = %v", err)
	}
}
