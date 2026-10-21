package tfinbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
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

// 本文件对真实 PostgreSQL 证实际履约补形成的消费门（ADR-0231 决定一、五）：编排答出的三种处置各落成 inbox 的一种动作，键维缺一是毒丸
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
	return catchUpGatesOver(t, application.FormActualFulfillmentOnParticipationEndDeps{
		Adoptions:             lookup,
		AdoptionsByCommission: list,
		Registration:          refusingRegistration{t: t},
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func catchUpGatesOver(t *testing.T, deps application.FormActualFulfillmentOnParticipationEndDeps, logger *slog.Logger) (
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
	catchUp := application.NewFormActualFulfillmentOnParticipationEndHandler(deps)
	onAdoption, err := tfinbox.NewActualFulfillmentCatchUpOnAdoptionConsumer(db.Transactor(), store, catchUp, logger)
	if err != nil {
		t.Fatalf("构造采用门铃那一扇门：%v", err)
	}
	onRevision, err := tfinbox.NewActualFulfillmentCatchUpOnRevisionConsumer(db.Transactor(), store, catchUp, logger)
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

const (
	defectTenant     = "SYN-TENANT-01"
	defectSegment    = "SYN-SEG-01"
	defectCommission = "SYN-COMMISSION-01"
	defectParcel     = "SYN-PARCEL-DEFECT-01"
)

// adoptedSegmentDouble 扮段登记册与采用连线：那一段已采用委托，段上一件包裹的参与以下一次交接结束。
type adoptedSegmentDouble struct {
	segment  ports.FulfillmentSegmentRecord
	adoption ports.SegmentCommissionAdoptionRecord
}

func (double adoptedSegmentDouble) FindSegmentsForObject(context.Context, domain.TenantID, domain.CarriedObjectReference) ([]ports.FulfillmentSegmentKey, error) {
	return []ports.FulfillmentSegmentKey{double.segment.Key}, nil
}

func (double adoptedSegmentDouble) FindByKey(context.Context, ports.FulfillmentSegmentKey) (ports.FulfillmentSegmentRecord, bool, error) {
	return double.segment, true, nil
}

func (double adoptedSegmentDouble) FindBySegment(context.Context, ports.SegmentCommissionAdoptionKey) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	return double.adoption, true, nil
}

// vanishedCommissionRegistration 扮登记用例读不回采用连线指名的委托：上抛的是数据缺陷，补形成记进册。
type vanishedCommissionRegistration struct{}

func (vanishedCommissionRegistration) Register(context.Context, application.RegisterChargeOccurrenceCommand) (application.RegisterChargeOccurrenceResult, error) {
	return application.RegisterChargeOccurrenceResult{}, fmt.Errorf("register charge occurrence: %w: segment %s, commission %s",
		application.ErrAdoptionWithoutCommission, defectSegment, defectCommission)
}

type uncoveredDouble struct{}

func (uncoveredDouble) FindByKey(context.Context, ports.ChargeOccurrenceKey) (ports.ChargeOccurrenceRecord, bool, error) {
	return ports.ChargeOccurrenceRecord{}, false, nil
}

func (uncoveredDouble) FindCovering(context.Context, ports.ChargeOccurrenceCoverageQuery) (ports.ChargeOccurrenceRecord, bool, error) {
	return ports.ChargeOccurrenceRecord{}, false, nil
}

type unformedSink struct {
	entries []ports.ActualFulfillmentUnformedEntry
}

func (sink *unformedSink) RecordUnformed(_ context.Context, entry ports.ActualFulfillmentUnformedEntry, _ time.Time) error {
	sink.entries = append(sink.entries, entry)
	return nil
}

func (*unformedSink) SettleFormed(context.Context, ports.ActualFulfillmentUnformedKey, ports.ChargeOccurrenceKey, time.Time) error {
	return nil
}

func (*unformedSink) SettleNotCharged(context.Context, ports.ActualFulfillmentUnformedKey, domain.AgreementSnapshotReference, time.Time) error {
	return nil
}

func (*unformedSink) SettleInvalidatedByDecision(context.Context, ports.ActualFulfillmentUnformedKey, ports.ChargeOccurrenceKey, time.Time) error {
	return nil
}

type catchUpClock struct{}

func (catchUpClock) Now() time.Time { return time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC) }

func defectValue[T any](t *testing.T, build func(string) (T, error), value string) T {
	t.Helper()
	built, err := build(value)
	if err != nil {
		t.Fatalf("%q：%v", value, err)
	}
	return built
}

func adoptedSegmentWithAnEndedParcel(t *testing.T) adoptedSegmentDouble {
	t.Helper()
	tenant := defectValue(t, domain.NewTenantID, defectTenant)
	segmentRef := defectValue(t, domain.NewFulfillmentSegmentReference, defectSegment)
	enteredAt := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: tenant,
		Segment:  segmentRef,
		Participations: []domain.RehydrateParticipationSpec{{
			Object:     defectValue(t, domain.NewCarriedObjectReference, defectParcel),
			EntryKind:  domain.EnteredByTransportHandover,
			EntryBasis: defectValue(t, domain.NewParticipationBasisReference, "SYN-HO-SZX-"+defectParcel),
			EnteredAt:  enteredAt,
			EndKind:    domain.EndedByNextHandover,
			EndBasis:   defectValue(t, domain.NewParticipationBasisReference, "SYN-HO-SIN-"+defectParcel),
			EndedAt:    enteredAt.Add(6 * time.Hour),
		}},
	})
	if err != nil {
		t.Fatalf("段夹具：%v", err)
	}
	adoption, err := domain.RehydrateSegmentCommissionAdoption(domain.RehydrateSegmentCommissionAdoptionSpec{
		TenantID:   tenant,
		Segment:    segmentRef,
		Commission: defectValue(t, domain.NewTransportCommissionReference, defectCommission),
		Provider:   defectValue(t, domain.NewServiceProviderReference, "SYN-PARTY-CARRIER-01"),
		StartBasis: defectValue(t, domain.NewParticipationBasisReference, "SYN-HO-SZX-"+defectParcel),
		StartedAt:  enteredAt,
	})
	if err != nil {
		t.Fatalf("采用夹具：%v", err)
	}
	return adoptedSegmentDouble{
		segment:  ports.FulfillmentSegmentRecord{Key: ports.FulfillmentSegmentKey{TenantID: tenant, Segment: segmentRef}, Segment: segment},
		adoption: ports.SegmentCommissionAdoptionRecord{Key: ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: segmentRef}, Adoption: adoption},
	}
}

// Covers: AT-TF-303（消费门，ADR-0231 决定二）——记进未形成册的数据缺陷那一件入账、信封不失败，消费门为它打一条 error 级日志：带门铃的键、
// 段、对象、整封的处置与完整错误（根因在尾部，不截）。
func TestARecordedDataDefectIsLoggedWithItsWholeError(t *testing.T) {
	adopted := adoptedSegmentWithAnEndedParcel(t)
	unformed := &unformedSink{}
	var logged bytes.Buffer
	onAdoption, _ := catchUpGatesOver(t, application.FormActualFulfillmentOnParticipationEndDeps{
		Segments:     adopted,
		Adoptions:    adopted,
		Journeys:     noJourneysDouble{},
		Registration: vanishedCommissionRegistration{},
		Coverage:     uncoveredDouble{},
		Unformed:     unformed,
		Clock:        catchUpClock{},
	}, slog.New(slog.NewJSONHandler(&logged, nil)))

	payload := fmt.Sprintf(`{"tenantId":%q,"segment":%q,"commission":%q}`, defectTenant, defectSegment, defectCommission)
	if err := onAdoption.Consume(t.Context(), doorbellEnvelope("catch-up-adoption-data-defect", tfinbox.SegmentCommissionAdoptionRegisteredEventType, payload)); err != nil {
		t.Fatalf("记进册的数据缺陷应入账：%v", err)
	}
	if len(unformed.entries) != 1 || !strings.HasPrefix(unformed.entries[0].Defect, "ADOPTION_WITHOUT_COMMISSION: ") {
		t.Fatalf("册 = %+v", unformed.entries)
	}

	lines := strings.Split(strings.TrimSpace(logged.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("日志 %d 条, want 1：%s", len(lines), logged.String())
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &line); err != nil {
		t.Fatalf("日志不是 JSON：%v", err)
	}
	wantError := fmt.Sprintf("register charge occurrence: %s: segment %s, commission %s",
		application.ErrAdoptionWithoutCommission, defectSegment, defectCommission)
	if line["level"] != "ERROR" || line["segment"] != defectSegment || line["object"] != defectParcel ||
		line["disposition"] != application.FormationSettled.String() || line["error"] != wantError ||
		!strings.Contains(fmt.Sprint(line["doorbell"]), defectSegment) {
		t.Fatalf("日志 = %v", line)
	}
}
