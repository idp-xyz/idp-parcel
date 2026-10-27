package tfinbox_test

import (
	"context"
	"errors"
	"fmt"
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

// 本文件对真实 PostgreSQL 证成员补问那一扇门（ADR-0331）：问关务与验证成员在消费门事务之外，追加读数与交补问信封在消费门那一笔里；编排
// 答出的三种处置各落成 inbox 的一种动作；信封两维缺一是毒丸。处理方是真编排，各口用替身，替身记下自己是在不在事务里被调的。夹具全部合成。

type identifiedViewDouble struct {
	probe       txProbe
	identity    ports.IdentifiedIntake
	identityErr error
	holders     map[string][]string
	inTx        []bool
}

func (double *identifiedViewDouble) FormedIntakeIdentity(ctx context.Context, _ domain.TenantID, _ string) (ports.IdentifiedIntake, error) {
	double.inTx = append(double.inTx, double.probe.inTransaction(ctx))
	return double.identity, double.identityErr
}

func (double *identifiedViewDouble) UnitsEverHolding(ctx context.Context, _ domain.TenantID, member string) ([]string, error) {
	double.inTx = append(double.inTx, double.probe.inTransaction(ctx))
	return double.holders[member], nil
}

// memberGateDouble 扮演「成员是不是随袋走」与「按原出发时点只问这一件」那两口：记下自己在不在事务里被问，答一份不受门禁的读数。
type memberGateDouble struct {
	probe    txProbe
	carried  bool
	carryTx  []bool
	reaskTx  []bool
	reasking []domain.CarriedObjectReference
}

func (double *memberGateDouble) CarriesMember(
	ctx context.Context, _ domain.TenantID, _, _ domain.CarriedObjectReference, _ time.Time,
) (bool, error) {
	double.carryTx = append(double.carryTx, double.probe.inTransaction(ctx))
	return double.carried, nil
}

func (double *memberGateDouble) ReaskMember(
	ctx context.Context, _ domain.TransportMovementFact, _, member domain.CarriedObjectReference,
) (domain.DepartureGateReading, error) {
	double.reaskTx = append(double.reaskTx, double.probe.inTransaction(ctx))
	double.reasking = append(double.reasking, member)
	finding, err := domain.NewDepartureGateFinding(member, domain.DepartureGateNotGuarded, "AADJ-"+member.String(),
		[]domain.DepartureGateBasis{{Action: "LOADING_DEPARTURE", Cause: "OUTSIDE_CUSTOMS_BOOKS"}})
	if err != nil {
		return domain.DepartureGateReading{}, err
	}
	from, _ := domain.NewNetworkNodeReference("SYN-NODE-A")
	to, _ := domain.NewNetworkNodeReference("SYN-NODE-B")
	return domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{From: from, To: to, Findings: []domain.DepartureGateFinding{finding}})
}

type memberReaskGateFixture struct {
	consumer   *tfinbox.DepartureGateReaskOnNodeIntakeConsumer
	identified *identifiedViewDouble
	segments   *reaskSegmentsDouble
	reasks     *reaskRegistryDouble
	handoff    *reaskHandoffDouble
	gate       *memberGateDouble
}

// newMemberReaskGateFixture 立一段：一袋 02:00 凭一版交接进段；03:00 一条出发声明这一段、首版读数停在本侧「段不在册」（可再问）。
// 作业实物 SYN-HU-01 已识别成 SYN-PARCEL-01，曾被装进这一袋。
func newMemberReaskGateFixture(t *testing.T) *memberReaskGateFixture {
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
	segmentRef, err := domain.NewFulfillmentSegmentReference("SYN-SEGMENT-01")
	if err != nil {
		t.Fatalf("段号：%v", err)
	}
	segment, err := domain.EstablishSegmentWithHandover(segmentRef, bagHandover(t), domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	parcel, err := domain.NewCarriedObjectReference("SYN-PARCEL-01")
	if err != nil {
		t.Fatalf("包裹：%v", err)
	}
	fixture := &memberReaskGateFixture{
		identified: &identifiedViewDouble{
			probe:    probe,
			identity: ports.IdentifiedIntake{Identified: true, Handling: "SYN-HU-01", Parcel: parcel},
			holders:  map[string][]string{"SYN-HU-01": {"SYN-BAG-01"}},
		},
		segments: &reaskSegmentsDouble{probe: probe, segment: segment},
		reasks:   &reaskRegistryDouble{probe: probe},
		handoff:  &reaskHandoffDouble{probe: probe},
		gate:     &memberGateDouble{probe: probe, carried: true},
	}
	handler, err := application.NewReaskDepartureGatesOnIdentifiedMemberHandler(application.ReaskDepartureGatesOnIdentifiedMemberDeps{
		Identified: fixture.identified,
		Segments:   fixture.segments,
		Facts:      reaskFactsDouble{records: []ports.MovementFactRecord{stoppedDeparture(t, segmentRef)}},
		Reasks:     fixture.reasks,
		Gate:       fixture.gate,
		Handoff:    fixture.handoff,
		Clock:      fixedClock{},
	})
	if err != nil {
		t.Fatalf("装成员补问编排：%v", err)
	}
	if fixture.consumer, err = tfinbox.NewDepartureGateReaskOnNodeIntakeConsumer(db.Transactor(), store, handler); err != nil {
		t.Fatalf("构造节点收寄那一扇门：%v", err)
	}
	return fixture
}

// bagHandover 是 SYN-BAG-01 在 02:00 凭这一版`已交接`进段。
func bagHandover(t *testing.T) domain.TransportHandover {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("合成交接：%v", err)
		}
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	must(err)
	object, err := domain.NewCarriedObjectReference("SYN-BAG-01")
	must(err)
	scope, err := domain.NewHandoverScopeReference("SYN-HANDOVER-SCOPE-BAG-01")
	must(err)
	releasedBy, err := domain.NewHandoverPartyReference("SYN-NODE-01")
	must(err)
	receivedBy, err := domain.NewHandoverPartyReference("SYN-CARRIER-01")
	must(err)
	releasing, err := domain.NewHandoverEvidenceReference("SYN-EVIDENCE-RELEASE-BAG-01")
	must(err)
	receiving, err := domain.NewHandoverEvidenceReference("SYN-EVIDENCE-RECEIVE-BAG-01")
	must(err)
	rule, err := domain.NewHandoverRuleReference("SYN-HANDOVER-RULE/v1")
	must(err)
	version, err := domain.NewHandoverResultVersion("SYN-HANDOVER-BAG-V1")
	must(err)
	handover, err := domain.FormTransportHandover(domain.TransportHandoverSpec{
		TenantID: tenant, Object: object, Scope: scope, ReleasedBy: releasedBy, ReceivedBy: receivedBy,
		Verdict: domain.ObjectHandedOver, ReleasingEvidence: releasing, ReceivingEvidence: receiving, Rule: rule,
		Version: version, JudgedAt: time.Date(2026, 10, 21, 2, 0, 0, 0, time.UTC),
	})
	must(err)
	return handover
}

func nodeIntakeEnvelope(eventID, payload string) eventing.Envelope {
	envelope := routeEnvelope(eventID, tfinbox.NodeIntakeFormedEventType, payload)
	envelope.Source = "idp-parcel/node-operations"
	return envelope
}

const formedNodeIntakePayload = `{"tenantId":"SYN-TENANT-01","sourceId":"SYN-INTAKE-01"}`

// Covers: AT-TF-364 消费门半边（ADR-0331）——问节点作业与验证成员、问关务都在消费门事务之外（关务的判断在它自己的事务里记下），追加读数与交补问
// 信封在消费门那一笔里，与 inbox 的已处理同笔提交；同一封再到，消费门先认出已处理，不问关务。
func TestTheMemberReaskGateAsksOutsideTheInboxTransactionAndWritesInsideIt(t *testing.T) {
	fixture := newMemberReaskGateFixture(t)
	envelope := nodeIntakeEnvelope("member-reask-settled", formedNodeIntakePayload)
	for range 2 {
		if err := fixture.consumer.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("补问应入账：%v", err)
		}
	}
	for name, calls := range map[string][]bool{
		"读收寄与曾装它的单元": fixture.identified.inTx, "扫段": fixture.segments.inTx, "验证成员": fixture.gate.carryTx, "问关务": fixture.gate.reaskTx,
	} {
		if len(calls) == 0 {
			t.Fatalf("%s 没有被调", name)
		}
		for _, inTx := range calls {
			if inTx {
				t.Fatalf("%s %v——要在任何事务之外", name, calls)
			}
		}
	}
	if len(fixture.gate.reaskTx) != 1 || len(fixture.segments.inTx) != 1 {
		t.Fatalf("问关务 %d 次、扫段 %d 次，要各恰一次——同一封再到不该重问", len(fixture.gate.reaskTx), len(fixture.segments.inTx))
	}
	if len(fixture.reasks.inTx) != 1 || !fixture.reasks.inTx[0] || len(fixture.handoff.inTx) != 1 || !fixture.handoff.inTx[0] {
		t.Fatalf("追加 %v、交信封 %v——要恰一次、且都在消费门那一笔里", fixture.reasks.inTx, fixture.handoff.inTx)
	}
	if saved := fixture.reasks.saved[0]; saved.Key.Object.String() != "SYN-PARCEL-01" || saved.Key.Sequence != 1 || saved.Trigger != "node-intake/SYN-INTAKE-01" {
		t.Fatalf("补问那一行 = %+v %q", saved.Key, saved.Trigger)
	}
}

// Covers: AT-TF-366 消费门半边——编排答出的三种处置各落成 inbox 的一种动作：收寄读不回时回滚、错误只带未决哨兵，读回之后重投入账；收寄立不成时
// 回滚、错误只带缺陷哨兵，每次投递都重走一遍；没认出包裹的收寄入账，不去找单元。
func TestTheMemberReaskGateRetriesFailsOrSettlesAsTheOrchestrationDisposes(t *testing.T) {
	t.Run("重投：收寄读不回", func(t *testing.T) {
		fixture := newMemberReaskGateFixture(t)
		fixture.identified.identityErr = errors.New("册不可用")
		envelope := nodeIntakeEnvelope("member-reask-retry", formedNodeIntakePayload)
		err := fixture.consumer.Consume(t.Context(), envelope)
		if !errors.Is(err, tfinbox.ErrDepartureGateReaskUndecided) || errors.Is(err, tfinbox.ErrDepartureGateReaskDefect) {
			t.Fatalf("收寄读不回 = %v, want 只带未决哨兵", err)
		}
		fixture.identified.identityErr = nil
		if err := fixture.consumer.Consume(t.Context(), envelope); err != nil || len(fixture.reasks.saved) != 1 {
			t.Fatalf("读回之后重投 = %v，追加 %d 份", err, len(fixture.reasks.saved))
		}
	})
	t.Run("缺陷：收寄立不成", func(t *testing.T) {
		fixture := newMemberReaskGateFixture(t)
		fixture.identified.identityErr = fmt.Errorf("adapter: %w", ports.ErrFormedIntakeUntranslatable)
		envelope := nodeIntakeEnvelope("member-reask-defect", formedNodeIntakePayload)
		for range 2 {
			err := fixture.consumer.Consume(t.Context(), envelope)
			if !errors.Is(err, tfinbox.ErrDepartureGateReaskDefect) || errors.Is(err, tfinbox.ErrDepartureGateReaskUndecided) {
				t.Fatalf("收寄立不成 = %v, want 只带缺陷哨兵", err)
			}
		}
		if len(fixture.identified.inTx) != 2 || len(fixture.reasks.saved) != 0 {
			t.Fatalf("收寄被读 %d 次、追加 %d 份, want 2、0——缺陷不入账，每次投递都重走一遍", len(fixture.identified.inTx), len(fixture.reasks.saved))
		}
	})
	t.Run("入账：没认出包裹", func(t *testing.T) {
		fixture := newMemberReaskGateFixture(t)
		fixture.identified.identity = ports.IdentifiedIntake{}
		if err := fixture.consumer.Consume(t.Context(), nodeIntakeEnvelope("member-reask-pending", formedNodeIntakePayload)); err != nil {
			t.Fatalf("没认出包裹应入账：%v", err)
		}
		if len(fixture.segments.inTx) != 0 || len(fixture.reasks.saved) != 0 {
			t.Fatal("没认出包裹却去读段、追加了补问")
		}
	})
}

// Covers: AT-TF-366 消费门半边（译码）——信封两维缺一是毒丸，入账、不交处理方。
func TestANodeIntakeEnvelopeMissingAKeyFieldIsPoisonOnTheMemberReaskGate(t *testing.T) {
	for name, payload := range map[string]string{
		"缺来源标识": `{"tenantId":"SYN-TENANT-01"}`,
		"缺租户":   `{"sourceId":"SYN-INTAKE-01"}`,
		"不是对象":  `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newMemberReaskGateFixture(t)
			if err := fixture.consumer.Consume(t.Context(), nodeIntakeEnvelope("member-reask-poison", payload)); err != nil {
				t.Fatalf("毒丸应入账不报错：%v", err)
			}
			if len(fixture.identified.inTx) != 0 {
				t.Fatal("毒丸交给了处理方")
			}
		})
	}
}

// Covers: AT-TF-364 消费门半边——构造期拒缺件。
func TestTheMemberReaskGateRefusesMissingParts(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	fixture := newMemberReaskGateFixture(t)
	handler, err := application.NewReaskDepartureGatesOnIdentifiedMemberHandler(application.ReaskDepartureGatesOnIdentifiedMemberDeps{
		Identified: fixture.identified, Segments: fixture.segments, Facts: reaskFactsDouble{}, Reasks: fixture.reasks,
		Gate: fixture.gate, Handoff: fixture.handoff, Clock: fixedClock{},
	})
	if err != nil {
		t.Fatalf("装编排：%v", err)
	}
	if _, err := tfinbox.NewDepartureGateReaskOnNodeIntakeConsumer(nil, store, handler); err == nil {
		t.Fatal("缺事务器也构造了")
	}
	if _, err := tfinbox.NewDepartureGateReaskOnNodeIntakeConsumer(db.Transactor(), nil, handler); err == nil {
		t.Fatal("缺 inbox 也构造了")
	}
	if _, err := tfinbox.NewDepartureGateReaskOnNodeIntakeConsumer(db.Transactor(), store, nil); err == nil {
		t.Fatal("缺编排也构造了")
	}
}
