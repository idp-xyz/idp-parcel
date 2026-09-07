package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: ADR-0114 决定一 — 段服务动作由控制事实登记方在段成立时声明：首个对象的`已交接`带声明 FINAL_DELIVERY
// 立起一个派送段；不声明的段照常成立且不是派送段；后续对象加入时矛盾的声明是领域拒绝（不留续办引用、交接照登）；
// 词不在封闭集合内同样拒而交接照登。
func TestHandoverRegistrationDeclaresTheSegmentServiceActionAtEstablishment(t *testing.T) {
	fixture := newHandoverSegmentFixture(t)
	command := registerHandoverCommand(t)
	command.Segment = "segment-1"
	command.SegmentServiceAction = "FINAL_DELIVERY"

	result, err := fixture.handler.Register(t.Context(), command)
	if err != nil || result.Outcome() != application.HandoverRegistered {
		t.Fatalf("登记：%v %q", err, result.Outcome())
	}
	if result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || result.SegmentContinuationReference() != "" {
		t.Fatalf("带声明立段不该被拒也不该欠账：%s %q", result.SegmentEntryRefusal(), result.SegmentContinuationReference())
	}
	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	action, declared := record.Segment.ServiceAction()
	if !declared || action != domain.SegmentServesFinalDelivery || !record.Segment.IsDeliverySegment() {
		t.Fatalf("段服务动作没有随成立固定：%v %s", declared, action)
	}
}

func TestAnUndeclaredSegmentEstablishesAndIsNotADeliverySegment(t *testing.T) {
	fixture := newHandoverSegmentFixture(t)
	command := registerHandoverCommand(t)
	command.Segment = "segment-1"

	if _, err := fixture.handler.Register(t.Context(), command); err != nil {
		t.Fatalf("登记：%v", err)
	}
	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	if _, declared := record.Segment.ServiceAction(); declared || record.Segment.IsDeliverySegment() {
		t.Fatal("没声明的段被填了服务动作——未声明是答案，不给默认")
	}
}

func TestAConflictingDeclarationOnJoinIsRefusedWhileTheHandoverStillRegisters(t *testing.T) {
	cases := map[string]struct {
		first, second string
	}{
		"声明与已固定的不一致":    {first: "FINAL_DELIVERY", second: "LINEHAUL"},
		"段成立时未声明而此刻要声明": {first: "", second: "FINAL_DELIVERY"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newHandoverSegmentFixture(t)
			first := registerHandoverCommand(t)
			first.Segment = "segment-1"
			first.SegmentServiceAction = testCase.first
			if _, err := fixture.handler.Register(t.Context(), first); err != nil {
				t.Fatalf("首个对象登记：%v", err)
			}

			second := registerHandoverCommand(t)
			second.Object = "parcel-2"
			second.Version = "handover-result/parcel-2/v1"
			second.JudgedAt = handoverJudgedTime.Add(time.Hour)
			second.Segment = "segment-1"
			second.SegmentServiceAction = testCase.second
			result, err := fixture.handler.Register(t.Context(), second)
			if err != nil || result.Outcome() != application.HandoverRegistered {
				t.Fatalf("交接本身该照登：%v %q", err, result.Outcome())
			}
			if result.SegmentEntryRefusal() != application.SegmentEntryRefusedServiceActionConflict {
				t.Fatalf("refusal = %s, want SEGMENT_SERVICE_ACTION_CONFLICT", result.SegmentEntryRefusal())
			}
			if result.SegmentContinuationReference() != "" {
				t.Fatal("领域拒绝不该留续办引用——重试不会变")
			}
			if fixture.segments.joins != 0 {
				t.Fatalf("被拒的对象不该加入段：joins=%d", fixture.segments.joins)
			}
			record := fixture.segments.saved(t, "tenant-1", "segment-1")
			if action, declared := record.Segment.ServiceAction(); declared != (testCase.first != "") || (declared && action.String() != testCase.first) {
				t.Fatalf("段成立时固定的服务动作被改写：%v %s", declared, action)
			}
		})
	}

	t.Run("一致的声明照常加入", func(t *testing.T) {
		fixture := newHandoverSegmentFixture(t)
		first := registerHandoverCommand(t)
		first.Segment = "segment-1"
		first.SegmentServiceAction = "FINAL_DELIVERY"
		fixture.handler.Register(t.Context(), first)

		second := registerHandoverCommand(t)
		second.Object = "parcel-2"
		second.Version = "handover-result/parcel-2/v1"
		second.JudgedAt = handoverJudgedTime.Add(time.Hour)
		second.Segment = "segment-1"
		second.SegmentServiceAction = "FINAL_DELIVERY"
		result, _ := fixture.handler.Register(t.Context(), second)
		if result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || fixture.segments.joins != 1 {
			t.Fatalf("一致的声明应照常加入：%s joins=%d", result.SegmentEntryRefusal(), fixture.segments.joins)
		}
	})
}

func TestAnUnknownServiceActionWordIsRefusedAndEstablishesNoSegment(t *testing.T) {
	fixture := newHandoverSegmentFixture(t)
	command := registerHandoverCommand(t)
	command.Segment = "segment-1"
	command.SegmentServiceAction = "LAST_MILE"

	result, err := fixture.handler.Register(t.Context(), command)
	if err != nil || result.Outcome() != application.HandoverRegistered {
		t.Fatalf("交接本身该照登：%v %q", err, result.Outcome())
	}
	if result.SegmentEntryRefusal() != application.SegmentEntryRefusedServiceActionUnknown {
		t.Fatalf("refusal = %s, want SEGMENT_SERVICE_ACTION_UNKNOWN", result.SegmentEntryRefusal())
	}
	if fixture.segments.saves != 0 {
		t.Fatal("词不在封闭集合内不该立段——不猜登记方想说哪一格")
	}
}
