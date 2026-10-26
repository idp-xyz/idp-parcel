package tfinbox_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证出发门禁补问那两扇门（ADR-0305）：问关务在消费门事务之外（关务的判断在调用方事务里即拒），追加读数与
// 交补问信封在消费门那一笔里；编排答出的三种处置各落成 inbox 的一种动作；揽收信封三维缺一是毒丸。处理方是真编排，各口用替身，替身
// 记下自己是在不在事务里被调的。夹具全部合成。

// txProbe 判一个 ctx 带不带环境事务。
type txProbe struct{ db *bentopg.DB }

func (probe txProbe) inTransaction(ctx context.Context) bool {
	_, err := probe.db.RequireExecutor(ctx)
	return !errors.Is(err, bentopg.ErrTransactionRequired)
}

type reaskSegmentsDouble struct {
	probe   txProbe
	segment domain.ActualFulfillmentSegment
	err     error
	findErr error
	inTx    []bool
}

func (double *reaskSegmentsDouble) FindSegmentsForObject(
	ctx context.Context, tenant domain.TenantID, _ domain.CarriedObjectReference,
) ([]ports.FulfillmentSegmentKey, error) {
	double.inTx = append(double.inTx, double.probe.inTransaction(ctx))
	if double.err != nil {
		return nil, double.err
	}
	return []ports.FulfillmentSegmentKey{{TenantID: tenant, Segment: double.segment.Segment()}}, nil
}

func (double *reaskSegmentsDouble) FindByKey(
	_ context.Context, key ports.FulfillmentSegmentKey,
) (ports.FulfillmentSegmentRecord, bool, error) {
	if double.findErr != nil {
		return ports.FulfillmentSegmentRecord{}, false, double.findErr
	}
	return ports.FulfillmentSegmentRecord{Key: key, Segment: double.segment}, true, nil
}

type reaskFactsDouble struct{ records []ports.MovementFactRecord }

func (double reaskFactsDouble) FindBySegment(context.Context, ports.FulfillmentSegmentKey) ([]ports.MovementFactRecord, error) {
	return double.records, nil
}

type reaskRegistryDouble struct {
	probe txProbe
	saved []ports.DepartureGateReaskRecord
	inTx  []bool
}

func (double *reaskRegistryDouble) FindByKey(context.Context, ports.DepartureGateReaskKey) (ports.DepartureGateReaskRecord, bool, error) {
	return ports.DepartureGateReaskRecord{}, false, nil
}

func (double *reaskRegistryDouble) FindByDeparture(context.Context, ports.MovementFactKey) ([]ports.DepartureGateReaskRecord, error) {
	return append([]ports.DepartureGateReaskRecord(nil), double.saved...), nil
}

func (double *reaskRegistryDouble) Save(ctx context.Context, record ports.DepartureGateReaskRecord) (ports.DepartureGateReaskSaveOutcome, error) {
	double.inTx = append(double.inTx, double.probe.inTransaction(ctx))
	double.saved = append(double.saved, record)
	return ports.DepartureGateReaskSaved, nil
}

type reaskHandoffDouble struct {
	probe txProbe
	inTx  []bool
}

func (double *reaskHandoffDouble) HandOffDepartureGateReask(ctx context.Context, _ ports.DepartureGateReaskRecord) error {
	double.inTx = append(double.inTx, double.probe.inTransaction(ctx))
	return nil
}

// reaskGateDouble 扮演按原出发时点问关务那一口：记下自己在不在事务里被问，答一份不受门禁的读数。
type reaskGateDouble struct {
	probe txProbe
	inTx  []bool
}

func (double *reaskGateDouble) Reask(
	ctx context.Context, fact domain.TransportMovementFact, object domain.CarriedObjectReference,
) (domain.DepartureGateReading, error) {
	double.inTx = append(double.inTx, double.probe.inTransaction(ctx))
	finding, err := domain.NewDepartureGateFinding(object, domain.DepartureGateNotGuarded, "AADJ-"+object.String(),
		[]domain.DepartureGateBasis{{Action: "LOADING_DEPARTURE", Cause: "OUTSIDE_CUSTOMS_BOOKS"}})
	if err != nil {
		return domain.DepartureGateReading{}, err
	}
	from, _ := domain.NewNetworkNodeReference("SYN-NODE-A")
	to, _ := domain.NewNetworkNodeReference("SYN-NODE-B")
	return domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{From: from, To: to, Findings: []domain.DepartureGateFinding{finding}})
}

type reaskPickupsDouble struct{}

func (reaskPickupsDouble) FindByKey(context.Context, ports.OffsitePickupKey) (ports.OffsitePickupRecord, bool, error) {
	return ports.OffsitePickupRecord{}, false, nil
}

type reaskGateFixture struct {
	consumer  *tfinbox.DepartureGateReaskOnHandoverConsumer
	pickup    *tfinbox.DepartureGateReaskOnPickupConsumer
	handovers *handoverFinderDouble
	segments  *reaskSegmentsDouble
	reasks    *reaskRegistryDouble
	handoff   *reaskHandoffDouble
	gate      *reaskGateDouble
}

// newReaskGateFixture 立一段：交接那一件 02:00 凭这一版交接进段；03:00 一条出发声明这一段、首版读数停在本侧「段不在册」。
func newReaskGateFixture(t *testing.T) *reaskGateFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	probe := txProbe{db: db}
	handover := handedOverRecord(t)
	segmentRef, err := domain.NewFulfillmentSegmentReference("SYN-SEGMENT-01")
	if err != nil {
		t.Fatalf("段号：%v", err)
	}
	segment, err := domain.EstablishSegmentWithHandover(segmentRef, handover.Handover, domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	departure := stoppedDeparture(t, segmentRef)

	fixture := &reaskGateFixture{
		handovers: &handoverFinderDouble{record: handover, found: true},
		segments:  &reaskSegmentsDouble{probe: probe, segment: segment},
		reasks:    &reaskRegistryDouble{probe: probe},
		handoff:   &reaskHandoffDouble{probe: probe},
		gate:      &reaskGateDouble{probe: probe},
	}
	handler, err := application.NewReaskDepartureGatesHandler(application.ReaskDepartureGatesDeps{
		Handovers: fixture.handovers,
		Pickups:   reaskPickupsDouble{},
		Segments:  fixture.segments,
		Facts:     reaskFactsDouble{records: []ports.MovementFactRecord{departure}},
		Reasks:    fixture.reasks,
		Gate:      fixture.gate,
		Handoff:   fixture.handoff,
		Clock:     fixedClock{},
	})
	if err != nil {
		t.Fatalf("装补问编排：%v", err)
	}
	if fixture.consumer, err = tfinbox.NewDepartureGateReaskOnHandoverConsumer(db.Transactor(), store, handler); err != nil {
		t.Fatalf("构造交接那一扇门：%v", err)
	}
	if fixture.pickup, err = tfinbox.NewDepartureGateReaskOnPickupConsumer(db.Transactor(), store, handler); err != nil {
		t.Fatalf("构造揽收那一扇门：%v", err)
	}
	return fixture
}

func stoppedDeparture(t *testing.T, segment domain.FulfillmentSegmentReference) ports.MovementFactRecord {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("合成出发：%v", err)
		}
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	must(err)
	fact, err := domain.NewMovementFactReference("SYN-MOVE-01")
	must(err)
	schedule, err := domain.NewScheduleReference("SYN-SCHED-01")
	must(err)
	location, err := domain.NewMovementLocationReference("SYN-NODE-A")
	must(err)
	source, err := domain.NewMovementSourceReference("SYN-FLEET/v1")
	must(err)
	version, err := domain.NewMovementFactVersion("v1")
	must(err)
	reading, err := domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{
		Stops: []domain.DepartureGateStop{domain.DepartureSegmentNotRegistered},
	})
	must(err)
	recorded, err := domain.RecordMovementFact(domain.MovementFactSpec{
		TenantID: tenant, Fact: fact, Schedule: schedule, Kind: domain.DepartureFact, Location: location, Source: source,
		Version: version, OccurredAt: time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC), Segment: segment, GateReading: reading,
	})
	must(err)
	return ports.MovementFactRecord{
		Key:        ports.MovementFactKey{TenantID: tenant, Fact: fact, Version: version},
		Fact:       recorded,
		RecordedAt: time.Date(2026, 10, 21, 3, 1, 0, 0, time.UTC),
	}
}

// Covers: AT-TF-343 消费门半边（ADR-0305）——问关务那一步在消费门事务之外（关务的判断在它自己的事务里记下），追加读数与交补问信封
// 在消费门那一笔里，与 inbox 的已处理同笔提交；同一封再到，消费门先认出已处理，不再问关务。
func TestTheReaskGateAsksCustomsOutsideTheInboxTransactionAndWritesInsideIt(t *testing.T) {
	fixture := newReaskGateFixture(t)
	envelope := handoverEnvelope("reask-settled", registeredHandoverPayload)
	for range 2 {
		if err := fixture.consumer.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("补问应入账：%v", err)
		}
	}
	if len(fixture.gate.inTx) != 1 || fixture.gate.inTx[0] {
		t.Fatalf("问关务 %v——要恰一次、且在任何事务之外", fixture.gate.inTx)
	}
	if len(fixture.segments.inTx) != 1 || fixture.segments.inTx[0] {
		t.Fatalf("扫段 %v——要恰一次、且在事务之外", fixture.segments.inTx)
	}
	if len(fixture.reasks.inTx) != 1 || !fixture.reasks.inTx[0] || len(fixture.handoff.inTx) != 1 || !fixture.handoff.inTx[0] {
		t.Fatalf("追加 %v、交信封 %v——要恰一次、且都在消费门那一笔里", fixture.reasks.inTx, fixture.handoff.inTx)
	}
	if saved := fixture.reasks.saved[0]; saved.Key.Object.String() != "SYN-PARCEL-01" || saved.Key.Sequence != 1 {
		t.Fatalf("补问那一行 = %+v", saved.Key)
	}
}

// Covers: AT-TF-346 消费门半边——编排答出的三种处置各落成 inbox 的一种动作：交接读不回时回滚、错误只带未决哨兵，读回之后重投入账；
// 段那一行装不回时回滚、错误只带缺陷哨兵，每次投递都重走一遍。
func TestTheReaskGateRetriesOrFailsAsTheOrchestrationDisposes(t *testing.T) {
	t.Run("重投：交接读不回", func(t *testing.T) {
		fixture := newReaskGateFixture(t)
		fixture.handovers.found = false
		envelope := handoverEnvelope("reask-retry", registeredHandoverPayload)
		err := fixture.consumer.Consume(t.Context(), envelope)
		if !errors.Is(err, tfinbox.ErrDepartureGateReaskUndecided) || errors.Is(err, tfinbox.ErrDepartureGateReaskDefect) {
			t.Fatalf("交接读不回 = %v, want 只带未决哨兵", err)
		}
		fixture.handovers.found = true
		if err := fixture.consumer.Consume(t.Context(), envelope); err != nil || len(fixture.reasks.saved) != 1 {
			t.Fatalf("读回之后重投 = %v，追加 %d 份", err, len(fixture.reasks.saved))
		}
	})
	t.Run("缺陷：段那一行装不回", func(t *testing.T) {
		fixture := newReaskGateFixture(t)
		fixture.segments.findErr = fmt.Errorf("adapter: %w: bad row", ports.ErrFulfillmentSegmentRecordUnreadable)
		envelope := handoverEnvelope("reask-defect", registeredHandoverPayload)
		for range 2 {
			err := fixture.consumer.Consume(t.Context(), envelope)
			if !errors.Is(err, tfinbox.ErrDepartureGateReaskDefect) || errors.Is(err, tfinbox.ErrDepartureGateReaskUndecided) {
				t.Fatalf("段那一行装不回 = %v, want 只带缺陷哨兵", err)
			}
		}
		if len(fixture.segments.inTx) != 2 || len(fixture.reasks.saved) != 0 {
			t.Fatalf("段被扫 %d 次、追加 %d 份, want 2、0——缺陷不入账，每次投递都重走一遍", len(fixture.segments.inTx), len(fixture.reasks.saved))
		}
	})
}

// Covers: AT-TF-346 消费门半边（译码）——揽收信封三维缺一是毒丸，入账、不交处理方。
func TestAPickupEnvelopeMissingAKeyFieldIsPoisonOnTheReaskGate(t *testing.T) {
	fixture := newReaskGateFixture(t)
	envelope := routeEnvelope("reask-pickup-poison", tfinbox.OffsitePickupRegisteredEventType,
		`{"tenantId":"SYN-TENANT-01","object":"SYN-PARCEL-01"}`)
	envelope.Source = "idp-parcel/transport-fulfillment"
	if err := fixture.pickup.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("毒丸应入账不报错：%v", err)
	}
	if len(fixture.segments.inTx) != 0 {
		t.Fatal("毒丸交给了处理方")
	}
}
