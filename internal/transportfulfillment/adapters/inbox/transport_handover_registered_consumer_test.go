package tfinbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证交接信封 → 派送发起那一扇门（ADR-0216）：编排答出的三种处置各落成 inbox 的一种动作——入账、
// 回滚等重投（带未决哨兵）、回滚并响亮失败（带缺陷哨兵）；四维缺一是毒丸入账，不交处理方。处理方是真编排，前面两口用
// 替身扮演交接登记册与段登记册；执行器那一格的各种答复由 cmd/parcel-dispatch 的真库用例证。夹具全部合成。

type handoverFinderDouble struct {
	record ports.TransportHandoverRecord
	found  bool
	calls  int
}

func (double *handoverFinderDouble) FindByKey(
	_ context.Context, _ ports.TransportHandoverKey,
) (ports.TransportHandoverRecord, bool, error) {
	double.calls++
	return double.record, double.found, nil
}

type activeSegmentsDouble struct{ keys []ports.FulfillmentSegmentKey }

func (double activeSegmentsDouble) FindActiveSegments(
	_ context.Context, _ domain.TenantID, _ domain.CarriedObjectReference,
) ([]ports.FulfillmentSegmentKey, error) {
	return double.keys, nil
}

// zeroTrigger 交回执行器结果的零值：封闭集合以外的那一格，编排该把它判成缺陷。
type zeroTrigger struct{}

func (zeroTrigger) Trigger(
	context.Context, application.TriggerDeliveryDispatchCommand,
) (application.TriggerDeliveryDispatchResult, error) {
	return application.TriggerDeliveryDispatchResult{}, nil
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC) }

func newHandoverDispatchConsumer(
	t *testing.T,
	handovers *handoverFinderDouble,
	segments activeSegmentsDouble,
) *tfinbox.TransportHandoverRegisteredConsumer {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := application.NewTriggerDispatchOnHandoverHandler(application.TriggerDispatchOnHandoverDeps{
		Handovers: handovers,
		Segments:  segments,
		Trigger:   zeroTrigger{},
		Clock:     fixedClock{},
	})
	consumer, err := tfinbox.NewTransportHandoverRegisteredConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer
}

func handedOverRecord(t *testing.T) ports.TransportHandoverRecord {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("合成交接：%v", err)
		}
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	must(err)
	object, err := domain.NewCarriedObjectReference("SYN-PARCEL-01")
	must(err)
	scope, err := domain.NewHandoverScopeReference("SYN-HANDOVER-SCOPE-01")
	must(err)
	releasedBy, err := domain.NewHandoverPartyReference("SYN-NODE-01")
	must(err)
	receivedBy, err := domain.NewHandoverPartyReference("SYN-CARRIER-01")
	must(err)
	releasing, err := domain.NewHandoverEvidenceReference("SYN-EVIDENCE-RELEASE-01")
	must(err)
	receiving, err := domain.NewHandoverEvidenceReference("SYN-EVIDENCE-RECEIVE-01")
	must(err)
	rule, err := domain.NewHandoverRuleReference("SYN-HANDOVER-RULE/v1")
	must(err)
	version, err := domain.NewHandoverResultVersion("SYN-HANDOVER-V1")
	must(err)
	handover, err := domain.FormTransportHandover(domain.TransportHandoverSpec{
		TenantID: tenant, Object: object, Scope: scope, ReleasedBy: releasedBy, ReceivedBy: receivedBy,
		Verdict: domain.ObjectHandedOver, ReleasingEvidence: releasing, ReceivingEvidence: receiving, Rule: rule,
		Version: version, JudgedAt: time.Date(2026, 10, 21, 2, 0, 0, 0, time.UTC),
	})
	must(err)
	return ports.TransportHandoverRecord{
		Key:      ports.TransportHandoverKey{TenantID: tenant, Object: object, Scope: scope, Version: version},
		Handover: handover,
	}
}

func handoverEnvelope(eventID string, payload string) eventing.Envelope {
	envelope := routeEnvelope(eventID, tfinbox.TransportHandoverRegisteredEventType, payload)
	envelope.Source = "idp-parcel/transport-fulfillment"
	return envelope
}

const registeredHandoverPayload = `{"tenantId":"SYN-TENANT-01","object":"SYN-PARCEL-01",` +
	`"scope":"SYN-HANDOVER-SCOPE-01","version":"SYN-HANDOVER-V1"}`

// Covers: AT-TF-194、AT-TF-196、AT-TF-198（消费门，ADR-0216 决定三）——入账的那一格提交 inbox，同一封再到不再交处理方；读不回的那一格回滚、
// 错误带未决哨兵，读回之后重投入账；缺陷的那一格回滚、错误带缺陷哨兵而不带未决哨兵。
func TestTheHandoverDispatchGateSettlesRetriesOrFailsAsTheOrchestrationDisposes(t *testing.T) {
	t.Run("入账：同一封只处理一次", func(t *testing.T) {
		handovers := &handoverFinderDouble{record: handedOverRecord(t), found: true}
		consumer := newHandoverDispatchConsumer(t, handovers, activeSegmentsDouble{})
		envelope := handoverEnvelope("handover-settled", registeredHandoverPayload)
		for range 2 {
			if err := consumer.Consume(t.Context(), envelope); err != nil {
				t.Fatalf("对象不在任何段里应入账：%v", err)
			}
		}
		if handovers.calls != 1 {
			t.Fatalf("交接被读 %d 次, want 1——入账后同一封不该再交处理方", handovers.calls)
		}
	})
	t.Run("重投：读不回时回滚，读回之后入账", func(t *testing.T) {
		handovers := &handoverFinderDouble{}
		consumer := newHandoverDispatchConsumer(t, handovers, activeSegmentsDouble{})
		envelope := handoverEnvelope("handover-retry", registeredHandoverPayload)
		err := consumer.Consume(t.Context(), envelope)
		if !errors.Is(err, tfinbox.ErrDeliveryDispatchOnHandoverUndecided) ||
			errors.Is(err, tfinbox.ErrDeliveryDispatchOnHandoverDefect) {
			t.Fatalf("交接读不回 = %v, want 只带未决哨兵", err)
		}
		handovers.record, handovers.found = handedOverRecord(t), true
		if err := consumer.Consume(t.Context(), envelope); err != nil || handovers.calls != 2 {
			t.Fatalf("读回之后重投 = %v、交接被读 %d 次, want 入账、2 次——上一次回滚了，这一封还没入账", err, handovers.calls)
		}
	})
	t.Run("缺陷：执行器答出封闭集合以外的格", func(t *testing.T) {
		tenant, _ := domain.NewTenantID("SYN-TENANT-01")
		segment, _ := domain.NewFulfillmentSegmentReference("SYN-SEGMENT-01")
		handovers := &handoverFinderDouble{record: handedOverRecord(t), found: true}
		consumer := newHandoverDispatchConsumer(t, handovers,
			activeSegmentsDouble{keys: []ports.FulfillmentSegmentKey{{TenantID: tenant, Segment: segment}}})
		envelope := handoverEnvelope("handover-defect", registeredHandoverPayload)
		for range 2 {
			err := consumer.Consume(t.Context(), envelope)
			if !errors.Is(err, tfinbox.ErrDeliveryDispatchOnHandoverDefect) ||
				errors.Is(err, tfinbox.ErrDeliveryDispatchOnHandoverUndecided) {
				t.Fatalf("执行器零值答复 = %v, want 只带缺陷哨兵", err)
			}
		}
		if handovers.calls != 2 {
			t.Fatalf("交接被读 %d 次, want 2——缺陷不入账，每次投递都重新走一遍", handovers.calls)
		}
	})
}

// Covers: AT-TF-198（译码）——四维缺一是毒丸，入账不交处理方；认不得的类型不入账、响亮报错。
func TestAHandoverEnvelopeMissingAKeyFieldIsPoison(t *testing.T) {
	handovers := &handoverFinderDouble{record: handedOverRecord(t), found: true}
	consumer := newHandoverDispatchConsumer(t, handovers, activeSegmentsDouble{})
	if err := consumer.Consume(t.Context(), handoverEnvelope("handover-poison",
		`{"tenantId":"SYN-TENANT-01","object":"SYN-PARCEL-01","scope":"SYN-HANDOVER-SCOPE-01"}`)); err != nil {
		t.Fatalf("毒丸应入账不报错：%v", err)
	}
	if handovers.calls != 0 {
		t.Fatal("毒丸交给了处理方")
	}
	foreign := handoverEnvelope("handover-foreign", registeredHandoverPayload)
	foreign.Type = "transport-fulfillment.effective-delivery.registered"
	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型没报错")
	}
}
