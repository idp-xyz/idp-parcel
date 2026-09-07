package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证段服务动作（ADR-0114 决定一）：三格封闭词往返、空词不是动作；声明只在段成立那一刻开门（已声明再声明
// 拒、未成立的段拒）；未声明不是派送段；重建门带回声明并挡住越过封闭集合的值。

func TestSegmentServiceActionWordsRoundTripAndUnknownWordsAreRefused(t *testing.T) {
	for _, action := range []domain.SegmentServiceAction{domain.SegmentServesOffsitePickup, domain.SegmentServesLinehaul, domain.SegmentServesFinalDelivery} {
		parsed, err := domain.ParseSegmentServiceAction(action.String())
		if err != nil || parsed != action || !parsed.Declared() {
			t.Fatalf("%s 应能往返：%v %s", action, err, parsed)
		}
	}
	for _, word := range []string{"", "LAST_MILE", "final_delivery"} {
		if _, err := domain.ParseSegmentServiceAction(word); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
			t.Fatalf("%q 不在封闭集合内应拒：%v", word, err)
		}
	}
	if domain.SegmentServiceActionUndeclared.Declared() || domain.SegmentServiceActionUndeclared.String() != "" {
		t.Fatal("未声明不是一格动作，也没有词")
	}
}

func TestServiceActionIsDeclaredOnceAtEstablishment(t *testing.T) {
	established := establishedSegmentForServiceAction(t)
	if _, declared := established.ServiceAction(); declared || established.IsDeliverySegment() {
		t.Fatal("没声明的段却有服务动作")
	}

	delivery, err := established.DeclareServiceAction(domain.SegmentServesFinalDelivery)
	if err != nil {
		t.Fatalf("声明：%v", err)
	}
	if action, declared := delivery.ServiceAction(); !declared || action != domain.SegmentServesFinalDelivery || !delivery.IsDeliverySegment() {
		t.Fatalf("声明没有落到段上：%v %s", declared, action)
	}
	if _, declared := established.ServiceAction(); declared {
		t.Fatal("原值被改动——值语义被破了")
	}
	if _, err := delivery.DeclareServiceAction(domain.SegmentServesLinehaul); !errors.Is(err, domain.ErrServiceActionAlreadyDeclared) {
		t.Fatalf("已声明再声明应拒：%v", err)
	}
	if _, err := established.DeclareServiceAction(domain.SegmentServiceActionUndeclared); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
		t.Fatalf("用未声明去声明应拒：%v", err)
	}
	if _, err := (domain.ActualFulfillmentSegment{}).DeclareServiceAction(domain.SegmentServesLinehaul); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
		t.Fatalf("未成立的段没有「成立那一刻」可言，应拒：%v", err)
	}
	linehaul, _ := established.DeclareServiceAction(domain.SegmentServesLinehaul)
	if linehaul.IsDeliverySegment() {
		t.Fatal("节点间运输的段不是派送段")
	}
}

func TestRehydrationCarriesTheServiceActionAndRefusesValuesOutsideTheClosedSet(t *testing.T) {
	spec := rehydratedSegmentSpecForServiceAction(t)
	spec.ServiceAction = domain.SegmentServesFinalDelivery
	segment, err := domain.RehydrateActualFulfillmentSegment(spec)
	if err != nil || !segment.IsDeliverySegment() {
		t.Fatalf("重建没有带回服务动作：%v", err)
	}

	spec.ServiceAction = domain.SegmentServiceActionUndeclared
	segment, err = domain.RehydrateActualFulfillmentSegment(spec)
	if err != nil {
		t.Fatalf("未声明（NULL）应能重建：%v", err)
	}
	if _, declared := segment.ServiceAction(); declared {
		t.Fatal("NULL 装回来却成了已声明")
	}

	spec.ServiceAction = domain.SegmentServiceAction(99)
	if _, err := domain.RehydrateActualFulfillmentSegment(spec); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
		t.Fatalf("越过封闭集合的值应拒：%v", err)
	}
}

// establishedSegmentForServiceAction 凭一次`已交接`立一个只有首个对象的段。
func establishedSegmentForServiceAction(t *testing.T) domain.ActualFulfillmentSegment {
	t.Helper()
	segment, err := domain.RehydrateActualFulfillmentSegment(rehydratedSegmentSpecForServiceAction(t))
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	return segment
}

func rehydratedSegmentSpecForServiceAction(t *testing.T) domain.RehydrateActualFulfillmentSegmentSpec {
	t.Helper()
	return domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: mustRef(t, domain.NewTenantID, "tenant-1"),
		Segment:  mustRef(t, domain.NewFulfillmentSegmentReference, "segment-1"),
		Participations: []domain.RehydrateParticipationSpec{{
			Object:     mustRef(t, domain.NewCarriedObjectReference, "parcel-1"),
			EntryKind:  domain.EnteredByTransportHandover,
			EntryBasis: mustRef(t, domain.NewParticipationBasisReference, "handover-result/parcel-1/v1"),
			EnteredAt:  credentialAt,
		}},
	}
}
