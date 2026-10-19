package tfinbox_test

import (
	"context"
	"errors"
	"testing"

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

// 本文件对真实 PostgreSQL 证实际履约发生项自动形成的两扇门（ADR-0218）：编排答出的三种处置各落成 inbox 的一种动作——入账、
// 回滚等重投（带未决哨兵）、回滚并响亮失败（带缺陷哨兵）；键维缺一是毒丸入账，不交处理方。处理方是真编排，前面几口用替身；
// 登记用例那一格的各种答复由应用层与 cmd/parcel-dispatch 的真库用例证。夹具全部合成。

// objectSegmentsDouble 扮段登记册按对象列段的一口：列出零段，或读不动。
type objectSegmentsDouble struct {
	listErr error
	lists   int
}

func (double *objectSegmentsDouble) FindSegmentsForObject(
	_ context.Context, _ domain.TenantID, _ domain.CarriedObjectReference,
) ([]ports.FulfillmentSegmentKey, error) {
	double.lists++
	return nil, double.listErr
}

func (double *objectSegmentsDouble) FindByKey(
	_ context.Context, _ ports.FulfillmentSegmentKey,
) (ports.FulfillmentSegmentRecord, bool, error) {
	return ports.FulfillmentSegmentRecord{}, false, nil
}

type noAdoptionsDouble struct{}

func (noAdoptionsDouble) FindBySegment(
	context.Context, ports.SegmentCommissionAdoptionKey,
) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	return ports.SegmentCommissionAdoptionRecord{}, false, nil
}

type noJourneysDouble struct{}

func (noJourneysDouble) FindByMember(context.Context, domain.TenantID, domain.CarriedObjectReference) ([]ports.AlternateJourneyRecord, error) {
	return nil, nil
}

// refusingRegistration 一被调就让用例红：这两扇门的用例里没有一格该走到登记。
type refusingRegistration struct{ t *testing.T }

func (registration refusingRegistration) Register(
	context.Context, application.RegisterChargeOccurrenceCommand,
) (application.RegisterChargeOccurrenceResult, error) {
	registration.t.Error("没有参与被结束却交给了登记用例")
	return application.RegisterChargeOccurrenceResult{}, nil
}

func newFormationGates(
	t *testing.T,
	handovers *handoverFinderDouble,
	segments *objectSegmentsDouble,
) (*tfinbox.ActualFulfillmentOnHandoverConsumer, *tfinbox.ActualFulfillmentOnDeliveryConsumer) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	former := application.NewFormActualFulfillmentOnParticipationEndHandler(application.FormActualFulfillmentOnParticipationEndDeps{
		Handovers:    handovers,
		Segments:     segments,
		Adoptions:    noAdoptionsDouble{},
		Journeys:     noJourneysDouble{},
		Registration: refusingRegistration{t: t},
	})
	onHandover, err := tfinbox.NewActualFulfillmentOnHandoverConsumer(db.Transactor(), store, former)
	if err != nil {
		t.Fatalf("构造交接那一扇门：%v", err)
	}
	onDelivery, err := tfinbox.NewActualFulfillmentOnDeliveryConsumer(db.Transactor(), store, former)
	if err != nil {
		t.Fatalf("构造交付那一扇门：%v", err)
	}
	return onHandover, onDelivery
}

func deliveryEnvelope(eventID string, payload string) eventing.Envelope {
	envelope := routeEnvelope(eventID, tfinbox.EffectiveDeliveryRegisteredEventType, payload)
	envelope.Source = "idp-parcel/transport-fulfillment"
	return envelope
}

const registeredDeliveryPayload = `{"tenantId":"SYN-TENANT-01","object":"SYN-PARCEL-01",` +
	`"attempt":"SYN-ATTEMPT-01","version":"SYN-DRV-01"}`

// Covers: AT-TF-251、AT-TF-253、AT-TF-255（消费门，ADR-0218）——交接那一扇：没有参与被结束入账、同一封再到不再交处理方；交接
// 读不回回滚、错误只带未决哨兵，读回之后入账；自家键过不了自家门回滚、错误只带缺陷哨兵，每次投递都重走一遍。
func TestTheHandoverFormationGateSettlesRetriesOrFailsAsTheOrchestrationDisposes(t *testing.T) {
	t.Run("入账：同一封只处理一次", func(t *testing.T) {
		handovers := &handoverFinderDouble{record: handedOverRecord(t), found: true}
		segments := &objectSegmentsDouble{}
		onHandover, _ := newFormationGates(t, handovers, segments)
		envelope := handoverEnvelope("formation-handover-settled", registeredHandoverPayload)
		for range 2 {
			if err := onHandover.Consume(t.Context(), envelope); err != nil {
				t.Fatalf("没有参与被结束应入账：%v", err)
			}
		}
		if handovers.calls != 1 || segments.lists != 1 {
			t.Fatalf("交接被读 %d 次、段被列 %d 次, want 各 1——入账后同一封不该再交处理方", handovers.calls, segments.lists)
		}
	})
	t.Run("重投：交接读不回时回滚，读回之后入账", func(t *testing.T) {
		handovers := &handoverFinderDouble{}
		onHandover, _ := newFormationGates(t, handovers, &objectSegmentsDouble{})
		envelope := handoverEnvelope("formation-handover-retry", registeredHandoverPayload)
		err := onHandover.Consume(t.Context(), envelope)
		if !errors.Is(err, tfinbox.ErrActualFulfillmentFormationUndecided) || errors.Is(err, tfinbox.ErrActualFulfillmentFormationDefect) {
			t.Fatalf("交接读不回 = %v, want 只带未决哨兵", err)
		}
		handovers.record, handovers.found = handedOverRecord(t), true
		if err := onHandover.Consume(t.Context(), envelope); err != nil || handovers.calls != 2 {
			t.Fatalf("读回之后重投 = %v、交接被读 %d 次, want 入账、2 次", err, handovers.calls)
		}
	})
	t.Run("缺陷：自家键过不了自家门", func(t *testing.T) {
		handovers := &handoverFinderDouble{record: handedOverRecord(t), found: true}
		onHandover, _ := newFormationGates(t, handovers, &objectSegmentsDouble{})
		envelope := handoverEnvelope("formation-handover-defect",
			`{"tenantId":"SYN-TENANT-01","object":"SYN-PARCEL-01","scope":"SYN-HANDOVER-SCOPE-01","version":"   "}`)
		for range 2 {
			err := onHandover.Consume(t.Context(), envelope)
			if !errors.Is(err, tfinbox.ErrActualFulfillmentFormationDefect) || errors.Is(err, tfinbox.ErrActualFulfillmentFormationUndecided) {
				t.Fatalf("空白版本 = %v, want 只带缺陷哨兵", err)
			}
		}
	})
}

// Covers: AT-TF-247、AT-TF-253、AT-TF-255（消费门）——交付那一扇：没有参与被结束入账；段登记册读不动回滚、只带未决哨兵，恢复后
// 入账；版本缺席是毒丸入账、不交处理方（PS 那一路不读版本，本路要读）；空白版本是缺陷；认不得的类型响亮报错。
func TestTheDeliveryFormationGateSettlesRetriesOrFailsAsTheOrchestrationDisposes(t *testing.T) {
	t.Run("入账与重投", func(t *testing.T) {
		segments := &objectSegmentsDouble{listErr: errors.New("段登记册读不动")}
		_, onDelivery := newFormationGates(t, &handoverFinderDouble{}, segments)
		envelope := deliveryEnvelope("formation-delivery-retry", registeredDeliveryPayload)
		err := onDelivery.Consume(t.Context(), envelope)
		if !errors.Is(err, tfinbox.ErrActualFulfillmentFormationUndecided) || errors.Is(err, tfinbox.ErrActualFulfillmentFormationDefect) {
			t.Fatalf("段登记册读不动 = %v, want 只带未决哨兵", err)
		}
		segments.listErr = nil
		for range 2 {
			if err := onDelivery.Consume(t.Context(), envelope); err != nil {
				t.Fatalf("恢复之后应入账：%v", err)
			}
		}
		if segments.lists != 2 {
			t.Fatalf("段被列 %d 次, want 2——回滚的那一次加入账的那一次，入账之后不再交处理方", segments.lists)
		}
	})
	t.Run("版本缺席是毒丸", func(t *testing.T) {
		segments := &objectSegmentsDouble{}
		_, onDelivery := newFormationGates(t, &handoverFinderDouble{}, segments)
		if err := onDelivery.Consume(t.Context(), deliveryEnvelope("formation-delivery-poison",
			`{"tenantId":"SYN-TENANT-01","object":"SYN-PARCEL-01","attempt":"SYN-ATTEMPT-01"}`)); err != nil {
			t.Fatalf("毒丸应入账不报错：%v", err)
		}
		if segments.lists != 0 {
			t.Fatal("毒丸交给了处理方")
		}
	})
	t.Run("空白版本是缺陷", func(t *testing.T) {
		_, onDelivery := newFormationGates(t, &handoverFinderDouble{}, &objectSegmentsDouble{})
		err := onDelivery.Consume(t.Context(), deliveryEnvelope("formation-delivery-defect",
			`{"tenantId":"SYN-TENANT-01","object":"SYN-PARCEL-01","attempt":"SYN-ATTEMPT-01","version":"  "}`))
		if !errors.Is(err, tfinbox.ErrActualFulfillmentFormationDefect) {
			t.Fatalf("空白版本 = %v, want 缺陷哨兵", err)
		}
	})
	t.Run("认不得的类型", func(t *testing.T) {
		_, onDelivery := newFormationGates(t, &handoverFinderDouble{}, &objectSegmentsDouble{})
		foreign := deliveryEnvelope("formation-delivery-foreign", registeredDeliveryPayload)
		foreign.Type = tfinbox.TransportHandoverRegisteredEventType
		if err := onDelivery.Consume(t.Context(), foreign); err == nil {
			t.Fatal("认不得的类型没报错")
		}
	})
}
