package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// participationEnderStub 让只关心来源登记本身的既有夹具照常构造处理器：它答一个空结果，不结束任何东西。
// 结束参与自己的规则由 end_fulfillment_participation_test 守，触发发生了没有由本文件守。
type participationEnderStub struct {
	calls []application.EndFulfillmentParticipationCommand
	err   error
}

func (stub *participationEnderStub) End(
	_ context.Context,
	command application.EndFulfillmentParticipationCommand,
) (application.EndFulfillmentParticipationResult, error) {
	stub.calls = append(stub.calls, command)
	return application.EndFulfillmentParticipationResult{}, stub.err
}

func (fixture *participationFixture) deliveryHandlerEndingParticipations(t *testing.T) *application.RegisterEffectiveDeliveryHandler {
	t.Helper()
	return application.NewRegisterEffectiveDeliveryHandler(application.RegisterEffectiveDeliveryDeps{
		Attempts:          &deliveryViewDouble{outcome: domain.ObjectDelivered, found: true},
		Deliveries:        fixture.deliveries,
		Versions:          &deliveryVersionFactory{},
		Downstream:        &deliveryHandoffDouble{},
		Clock:             deliveryClock{at: deliveryRecordedAt},
		ParticipationEnds: fixture.handler,
	})
}

// Covers: 票 tf-segment-lifecycle-closure/06 裁决 (i)——有效交付落库后**同一编排内**结束该对象的履约参与
// （UC-TF-005 步骤 7 的责任方是 TF 自己）：交付首登成立，对象在段里的参与随之结束，结果里透出
// PARTICIPATION_ENDED；对象不在任何段里则透出 NO_ACTIVE_PARTICIPATION 而不是静默。
func TestARegisteredDeliveryEndsTheObjectsParticipation(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)
	fixture.joinEarlyMember(t, "parcel-3")
	handler := fixture.deliveryHandlerEndingParticipations(t)

	result, err := handler.Register(t.Context(), deliveryCommandFor(t, "parcel-3", "attempt-1"))
	if err != nil {
		t.Fatalf("首登交付：%v", err)
	}
	if result.Outcome() != application.DeliveryRegistered {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if result.ParticipationEnd() != application.ParticipationEndedNow {
		t.Fatalf("participationEnd = %s, want PARTICIPATION_ENDED", result.ParticipationEnd())
	}
	if fixture.participation(t, "parcel-3").Active() {
		t.Fatal("交付落库后 parcel-3 的参与没有结束")
	}
	if !fixture.participation(t, "parcel-1").Active() {
		t.Fatal("邻居 parcel-1 的参与被一起结束了")
	}

	t.Run("an object in no segment is answered, not silenced", func(t *testing.T) {
		result, err := handler.Register(t.Context(), deliveryCommandFor(t, "parcel-9", "attempt-9"))
		if err != nil {
			t.Fatalf("首登交付：%v", err)
		}
		if result.Outcome() != application.DeliveryRegistered || result.ParticipationEnd() != application.ParticipationNoActiveParticipation {
			t.Fatalf("outcome = %s participationEnd = %s", result.Outcome(), result.ParticipationEnd())
		}
	})

	t.Run("a replay does not end anything twice", func(t *testing.T) {
		result, err := handler.Register(t.Context(), deliveryCommandFor(t, "parcel-3", "attempt-1"))
		if err != nil {
			t.Fatalf("重放：%v", err)
		}
		if result.Outcome() != application.DeliveryExistingVersion || result.ParticipationEnd() != application.ParticipationEndOutcomeInvalid {
			t.Fatalf("outcome = %s participationEnd = %s", result.Outcome(), result.ParticipationEnd())
		}
	})
}

// 结束参与失败则整笔不落（error → 5xx）：编排 error 与`未决`都作 error 交回，事务边界据以回滚交付。
func TestADeliveryWhoseParticipationEndFailsFormsNoAnswer(t *testing.T) {
	t.Run("the ender errors", func(t *testing.T) {
		fixture := newParticipationFixture(t)
		stub := &participationEnderStub{err: errors.New("segment registry inconsistent")}
		handler := application.NewRegisterEffectiveDeliveryHandler(application.RegisterEffectiveDeliveryDeps{
			Attempts:          &deliveryViewDouble{outcome: domain.ObjectDelivered, found: true},
			Deliveries:        fixture.deliveries,
			Versions:          &deliveryVersionFactory{},
			Downstream:        &deliveryHandoffDouble{},
			Clock:             deliveryClock{at: deliveryRecordedAt},
			ParticipationEnds: stub,
		})
		if _, err := handler.Register(t.Context(), deliveryCommandFor(t, "parcel-1", "attempt-1")); err == nil {
			t.Fatal("结束参与报错时交付却形成了答案")
		}
		if len(stub.calls) != 1 || stub.calls[0].Source != application.ParticipationEndedByDelivery || stub.calls[0].Segment != "" {
			t.Fatalf("calls = %+v，want 一次交付路、不带段", stub.calls)
		}
	})

	t.Run("the segment registry is down", func(t *testing.T) {
		fixture := newParticipationFixture(t)
		fixture.twoMemberSegment(t)
		handler := fixture.deliveryHandlerEndingParticipations(t)
		fixture.segments.findErr = errors.New("registry down")
		if _, err := handler.Register(t.Context(), deliveryCommandFor(t, "parcel-1", "attempt-1")); !errors.Is(err, application.ErrParticipationEndUnsettled) {
			t.Fatalf("err = %v, want ErrParticipationEndUnsettled", err)
		}
	})
}

// 漏接 ParticipationEnds 看得见但不崩：交付照登，结果答 PARTICIPATION_END_NOT_WIRED。不 panic 是因为按旧 Deps
// 构造处理器的测试遍布各处，一次 panic 会把整个测试进程连栈炸掉；不静默是因为那正是 ADR-0098 那一族的病。
func TestADeliveryWithoutAParticipationEnderSaysSo(t *testing.T) {
	fixture := newParticipationFixture(t)
	handler := application.NewRegisterEffectiveDeliveryHandler(application.RegisterEffectiveDeliveryDeps{
		Attempts:   &deliveryViewDouble{outcome: domain.ObjectDelivered, found: true},
		Deliveries: fixture.deliveries,
		Versions:   &deliveryVersionFactory{},
		Downstream: &deliveryHandoffDouble{},
		Clock:      deliveryClock{at: deliveryRecordedAt},
	})
	result, err := handler.Register(t.Context(), deliveryCommandFor(t, "parcel-1", "attempt-1"))
	if err != nil {
		t.Fatalf("首登交付：%v", err)
	}
	if result.Outcome() != application.DeliveryRegistered || result.ParticipationEnd() != application.ParticipationEndNotWired {
		t.Fatalf("outcome = %s participationEnd = %s, want PARTICIPATION_END_NOT_WIRED", result.Outcome(), result.ParticipationEnd())
	}
}

func deliveryCommandFor(t *testing.T, object, attempt string) application.RegisterEffectiveDeliveryCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.RegisterEffectiveDeliveryCommand{
		TenantID:  tenant,
		Attempt:   attempt,
		Object:    object,
		Method:    "signature",
		Recipient: "recipient-1",
		Proof:     "pod-" + object,
	}
}
