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
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证实际履约补形成的两扇门（ADR-0231 决定一、五）：编排答出的三种处置各落成 inbox 的一种动作，键维缺一是毒丸
// 入账、不交处理方。处理方是真编排，前面几口用替身；逐件形成的各种答复由应用层与 cmd/parcel-dispatch 的真库用例证。夹具全部合成。

type adoptionLookupDouble struct {
	err   error
	calls int
}

func (double *adoptionLookupDouble) FindBySegment(context.Context, ports.SegmentCommissionAdoptionKey) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	double.calls++
	return ports.SegmentCommissionAdoptionRecord{}, false, double.err
}

type adoptionListDouble struct {
	err   error
	calls int
}

func (double *adoptionListDouble) ListByCommission(context.Context, ports.TransportCommissionKey) ([]ports.SegmentCommissionAdoptionRecord, error) {
	double.calls++
	return nil, double.err
}

func newCatchUpGates(t *testing.T, lookup *adoptionLookupDouble, list *adoptionListDouble) (
	*tfinbox.ActualFulfillmentCatchUpOnAdoptionConsumer, *tfinbox.ActualFulfillmentCatchUpOnRevisionConsumer,
) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	catchUp := application.NewFormActualFulfillmentOnParticipationEndHandler(application.FormActualFulfillmentOnParticipationEndDeps{
		Adoptions:             lookup,
		AdoptionsByCommission: list,
		Registration:          refusingRegistration{t: t},
	})
	onAdoption, err := tfinbox.NewActualFulfillmentCatchUpOnAdoptionConsumer(db.Transactor(), store, catchUp)
	if err != nil {
		t.Fatalf("构造采用门铃那一扇门：%v", err)
	}
	onRevision, err := tfinbox.NewActualFulfillmentCatchUpOnRevisionConsumer(db.Transactor(), store, catchUp)
	if err != nil {
		t.Fatalf("构造换版门铃那一扇门：%v", err)
	}
	return onAdoption, onRevision
}

func doorbellEnvelope(eventID string, eventType eventing.EventType, payload string) eventing.Envelope {
	envelope := routeEnvelope(eventID, eventType, payload)
	envelope.Source = "idp-parcel/transport-fulfillment"
	return envelope
}

// Covers: AT-TF-303（消费门，ADR-0231）——采用门铃那一扇：段已没有采用连线入账、同一封再到不再交处理方；采用连线读不动回滚、只带未决
// 哨兵；自家键过不了自家门回滚、只带缺陷哨兵；键维缺一是毒丸入账。
func TestTheAdoptionCatchUpGateSettlesRetriesOrFailsAsTheOrchestrationDisposes(t *testing.T) {
	const payload = `{"tenantId":"SYN-TENANT-01","segment":"SYN-SEG-01","commission":"SYN-COMMISSION-01"}`
	t.Run("入账", func(t *testing.T) {
		lookup := &adoptionLookupDouble{}
		onAdoption, _ := newCatchUpGates(t, lookup, &adoptionListDouble{})
		envelope := doorbellEnvelope("catch-up-adoption-settled", tfinbox.SegmentCommissionAdoptionRegisteredEventType, payload)
		for range 2 {
			if err := onAdoption.Consume(t.Context(), envelope); err != nil {
				t.Fatalf("段没有采用连线应入账：%v", err)
			}
		}
		if lookup.calls != 1 {
			t.Fatalf("采用连线被读 %d 次, want 1", lookup.calls)
		}
	})
	t.Run("重投", func(t *testing.T) {
		lookup := &adoptionLookupDouble{err: errors.New("adoptions down")}
		onAdoption, _ := newCatchUpGates(t, lookup, &adoptionListDouble{})
		err := onAdoption.Consume(t.Context(), doorbellEnvelope("catch-up-adoption-retry", tfinbox.SegmentCommissionAdoptionRegisteredEventType, payload))
		if !errors.Is(err, tfinbox.ErrActualFulfillmentFormationUndecided) || errors.Is(err, tfinbox.ErrActualFulfillmentFormationDefect) {
			t.Fatalf("采用连线读不动 = %v, want 只带未决哨兵", err)
		}
	})
	t.Run("缺陷与毒丸", func(t *testing.T) {
		lookup := &adoptionLookupDouble{}
		onAdoption, _ := newCatchUpGates(t, lookup, &adoptionListDouble{})
		err := onAdoption.Consume(t.Context(), doorbellEnvelope("catch-up-adoption-defect", tfinbox.SegmentCommissionAdoptionRegisteredEventType,
			`{"tenantId":"SYN-TENANT-01","segment":"   ","commission":"SYN-COMMISSION-01"}`))
		if !errors.Is(err, tfinbox.ErrActualFulfillmentFormationDefect) {
			t.Fatalf("空白段 = %v, want 缺陷哨兵", err)
		}
		if err := onAdoption.Consume(t.Context(), doorbellEnvelope("catch-up-adoption-poison", tfinbox.SegmentCommissionAdoptionRegisteredEventType,
			`{"tenantId":"SYN-TENANT-01","segment":"SYN-SEG-01"}`)); err != nil || lookup.calls != 0 {
			t.Fatalf("毒丸 = %v，交处理方 %d 次", err, lookup.calls)
		}
	})
}

// Covers: AT-TF-303（消费门，ADR-0231）——换版门铃那一扇：没有段采用这份委托入账；按委托列段读不动回滚、只带未决哨兵；号不是换版的号是毒丸。
func TestTheRevisionCatchUpGateSettlesRetriesOrFailsAsTheOrchestrationDisposes(t *testing.T) {
	const payload = `{"tenantId":"SYN-TENANT-01","commission":"SYN-COMMISSION-01","revision":2}`
	list := &adoptionListDouble{err: errors.New("adoptions down")}
	_, onRevision := newCatchUpGates(t, &adoptionLookupDouble{}, list)
	envelope := doorbellEnvelope("catch-up-revision", tfinbox.CommissionAgreementRevisedEventType, payload)
	if err := onRevision.Consume(t.Context(), envelope); !errors.Is(err, tfinbox.ErrActualFulfillmentFormationUndecided) {
		t.Fatalf("按委托列段读不动 = %v, want 未决哨兵", err)
	}
	list.err = nil
	if err := onRevision.Consume(t.Context(), envelope); err != nil || list.calls != 2 {
		t.Fatalf("恢复之后 = %v、列段 %d 次", err, list.calls)
	}
	if err := onRevision.Consume(t.Context(), doorbellEnvelope("catch-up-revision-poison", tfinbox.CommissionAgreementRevisedEventType,
		`{"tenantId":"SYN-TENANT-01","commission":"SYN-COMMISSION-01","revision":1}`)); err != nil || list.calls != 2 {
		t.Fatalf("号 1 的门铃 = %v、列段 %d 次, want 毒丸入账", err, list.calls)
	}
}
