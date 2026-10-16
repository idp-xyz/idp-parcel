package parcelshipment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证 NO 消费 PS 服务结果的防腐适配器（票 operator-channel/25）：逐格翻译、PS 读口的错误原样交回、终局只认已定案的
// 那一份。夹具全为合成引用（S 级）。

var resultsAt = time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)

func must[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("%q：%v", raw, err)
	}
	return value
}

type cancellationsFake struct {
	cancellation psdomain.ParcelCancellation
	found        bool
	err          error
	asked        []string
}

func (fake *cancellationsFake) FindCancellation(_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID) (psdomain.ParcelCancellation, bool, error) {
	fake.asked = append(fake.asked, tenant.String()+"|"+parcel.String())
	return fake.cancellation, fake.found, fake.err
}

type finalsFake struct {
	record psports.FinalOutcomeRecord
	found  bool
	err    error
	asked  []string
}

func (fake *finalsFake) FindCurrentFinal(_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID) (psports.FinalOutcomeRecord, bool, error) {
	fake.asked = append(fake.asked, tenant.String()+"|"+parcel.String())
	return fake.record, fake.found, fake.err
}

func cancellationOf(t *testing.T) psdomain.ParcelCancellation {
	t.Helper()
	cancellation, err := psdomain.DecideParcelCancellation(psdomain.ParcelCancellationSpec{
		ID:          must(t, psdomain.NewParcelCancellationID, "SYN-CANCEL-25"),
		Parcel:      must(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-25"),
		Requester:   must(t, psdomain.NewCancellationRequesterReference, "SYN-CUSTOMER-25"),
		Authority:   must(t, psdomain.NewCancellationAuthorityReference, "SYN-CANCEL-RULE"),
		Reason:      must(t, psdomain.NewCancellationReasonReference, "SYN-CHANGED-MIND"),
		RequestedAt: resultsAt,
	}, psdomain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("取消决定：%v", err)
	}
	return cancellation
}

func finalRecordOf(t *testing.T, finalized bool) psports.FinalOutcomeRecord {
	t.Helper()
	source, err := psdomain.NewResponsibilityOutcome(psdomain.ResponsibilityOutcomeSpec{
		Kind:       psdomain.EffectiveDeliveryOutcome,
		Parcel:     must(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-25"),
		Decision:   must(t, psdomain.NewResponsibilityDecisionReference, "SYN-DELIVERY-JUDGMENT"),
		Execution:  must(t, psdomain.NewExecutionEvidenceReference, "SYN-POD"),
		Version:    must(t, psdomain.NewResponsibilityOutcomeVersion, "SYN-DELIVERY/v1"),
		OccurredAt: resultsAt,
	})
	if err != nil {
		t.Fatalf("责任结果：%v", err)
	}
	final, err := psdomain.FormParcelFinalOutcome(psdomain.ParcelFinalOutcomeSpec{
		Version:     must(t, psdomain.NewFinalOutcomeVersionID, "SYN-FINAL-25/v1"),
		Parcel:      must(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-25"),
		Kind:        must(t, psdomain.NewFinalKindReference, "NETWORK_SERVICE_DELIVERED"),
		Source:      source,
		RuleVersion: must(t, psdomain.NewFinalRuleVersionReference, "SYN-FINAL-RULES/v1"),
	})
	if err != nil {
		t.Fatalf("终局：%v", err)
	}
	return psports.FinalOutcomeRecord{Finalized: finalized, Final: final, AdoptedAt: resultsAt}
}

func ask(t *testing.T, view noports.ParcelServiceResultView) (noports.ParcelServiceResults, error) {
	t.Helper()
	return view.FindParcelServiceResults(context.Background(),
		must(t, nodomain.NewTenantID, "SYN-TENANT-01"), must(t, nodomain.NewParcelAssociationReference, "SYN-PARCEL-25"))
}

// Covers: 包裹按既有约定译（关联引用的串即 PS 声明包裹标识），租户逐字译；取消决定译成它的标识，已定案的当前终局译成
// 种类与版本。
func TestServiceResultsAreTranslatedFromTheParcelShipmentAnswers(t *testing.T) {
	cancellations := &cancellationsFake{cancellation: cancellationOf(t), found: true}
	finals := &finalsFake{record: finalRecordOf(t, true), found: true}
	view, err := noparcel.NewServiceResultView(cancellations, finals)
	if err != nil {
		t.Fatal(err)
	}
	results, err := ask(t, view)
	if err != nil {
		t.Fatalf("find：%v", err)
	}
	if results != (noports.ParcelServiceResults{Cancellation: "SYN-CANCEL-25", FinalKind: "NETWORK_SERVICE_DELIVERED", FinalVersion: "SYN-FINAL-25/v1"}) {
		t.Fatalf("results = %+v", results)
	}
	for name, asked := range map[string][]string{"取消读口": cancellations.asked, "终局读口": finals.asked} {
		if len(asked) != 1 || asked[0] != "SYN-TENANT-01|SYN-PARCEL-25" {
			t.Fatalf("%s被问 %v，want 恰一次、同一租户与包裹", name, asked)
		}
	}
}

// Covers: PS 没有取消决定、没有当前终局，或终局记录在册却没有定案（采用被拒的那一份不是终局）——答空，即「没有限制」。
func TestNoCancellationAndNoFinalizedOutcomeAnswerEmpty(t *testing.T) {
	for name, finals := range map[string]*finalsFake{
		"没有终局":    {},
		"终局记录未定案": {record: finalRecordOf(t, false), found: true},
	} {
		view, err := noparcel.NewServiceResultView(&cancellationsFake{}, finals)
		if err != nil {
			t.Fatal(err)
		}
		results, err := ask(t, view)
		if err != nil || results != (noports.ParcelServiceResults{}) {
			t.Fatalf("%s：results = %+v, err = %v, want 空", name, results, err)
		}
	}
}

// Covers: 两个读口任一读不动都原样交回错误——不把「没查到」译成「没有限制」；编排据此停在待确认。构造期拒缺件。
func TestReadFailuresAreReturnedAndMissingReadersAreRefused(t *testing.T) {
	down := errors.New("parcel shipment register unreachable")
	for name, view := range map[string]struct {
		cancellations *cancellationsFake
		finals        *finalsFake
	}{
		"取消读口读不动": {&cancellationsFake{err: down}, &finalsFake{}},
		"终局读口读不动": {&cancellationsFake{}, &finalsFake{err: down}},
	} {
		adapter, err := noparcel.NewServiceResultView(view.cancellations, view.finals)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ask(t, adapter); !errors.Is(err, down) {
			t.Fatalf("%s：err = %v, want 原样交回", name, err)
		}
	}
	if _, err := noparcel.NewServiceResultView(nil, &finalsFake{}); err == nil {
		t.Fatal("缺取消读口也装得起来")
	}
	if _, err := noparcel.NewServiceResultView(&cancellationsFake{}, nil); err == nil {
		t.Fatal("缺终局读口也装得起来")
	}
}
