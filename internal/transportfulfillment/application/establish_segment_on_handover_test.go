package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// segmentRegistryDouble 是 ports.ActualFulfillmentSegmentRegistry 的替身。
//
// 它按行存参与关系、读时走 RehydrateActualFulfillmentSegment 装回，与真库适配器同一道
// 重建门。存整个聚合再原样交回会让替身比真库宽容——真库那一侧每次读都要重过构造门，
// 替身若跳过，编排里「装不回的段」这一类就永远测不出来。
type segmentRegistryDouble struct {
	rows    map[string]*segmentRowsDouble
	findErr error
	saveErr error
	joinErr error
	saves   int
	joins   int
}

type segmentRowsDouble struct {
	key            ports.FulfillmentSegmentKey
	participations []domain.RehydrateParticipationSpec
	recordedAt     time.Time
}

func newSegmentRegistry() *segmentRegistryDouble {
	return &segmentRegistryDouble{rows: map[string]*segmentRowsDouble{}}
}

func segmentRegistryKey(key ports.FulfillmentSegmentKey) string {
	return key.TenantID.String() + "|" + key.Segment.String()
}

// participationSpecOf 把领域参与关系摊回库面的样子，逐格取自它自己的读面。
func participationSpecOf(participation domain.FulfillmentParticipation) domain.RehydrateParticipationSpec {
	spec := domain.RehydrateParticipationSpec{
		Object:     participation.Object(),
		EntryKind:  participation.EntryKind(),
		EntryBasis: participation.EntryBasis(),
		EnteredAt:  participation.EnteredAt(),
	}
	if planned, present := participation.PlannedSegment(); present {
		spec.Planned = planned
	}
	if endKind, endBasis, endedAt, ended := participation.End(); ended {
		spec.EndKind, spec.EndBasis, spec.EndedAt = endKind, endBasis, endedAt
	}
	return spec
}

func (double *segmentRegistryDouble) FindByKey(
	_ context.Context,
	key ports.FulfillmentSegmentKey,
) (ports.FulfillmentSegmentRecord, bool, error) {
	if double.findErr != nil {
		return ports.FulfillmentSegmentRecord{}, false, double.findErr
	}
	rows, found := double.rows[segmentRegistryKey(key)]
	if !found {
		return ports.FulfillmentSegmentRecord{}, false, nil
	}
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       key.TenantID,
		Segment:        key.Segment,
		Participations: rows.participations,
	})
	if err != nil {
		return ports.FulfillmentSegmentRecord{}, false, err
	}
	return ports.FulfillmentSegmentRecord{Key: key, Segment: segment, RecordedAt: rows.recordedAt}, true, nil
}

func (double *segmentRegistryDouble) Save(
	_ context.Context,
	record ports.FulfillmentSegmentRecord,
) (ports.SegmentSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.SegmentSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.rows[segmentRegistryKey(record.Key)]; exists {
		return ports.SegmentAlreadyRegistered, nil
	}
	rows := &segmentRowsDouble{key: record.Key, recordedAt: record.RecordedAt}
	for _, participation := range record.Segment.Participations() {
		rows.participations = append(rows.participations, participationSpecOf(participation))
	}
	double.rows[segmentRegistryKey(record.Key)] = rows
	return ports.SegmentSaved, nil
}

func (double *segmentRegistryDouble) Join(
	_ context.Context,
	key ports.FulfillmentSegmentKey,
	participation domain.FulfillmentParticipation,
	recordedAt time.Time,
) (ports.SegmentJoinOutcome, error) {
	double.joins++
	if double.joinErr != nil {
		return ports.SegmentJoinOutcomeInvalid, double.joinErr
	}
	rows, found := double.rows[segmentRegistryKey(key)]
	if !found {
		return ports.SegmentJoinOutcomeInvalid, nil
	}
	for _, existing := range rows.participations {
		if existing.Object == participation.Object() {
			return ports.ObjectAlreadyParticipating, nil
		}
	}
	rows.participations = append(rows.participations, participationSpecOf(participation))
	rows.recordedAt = recordedAt
	return ports.ObjectJoined, nil
}

func (double *segmentRegistryDouble) EndParticipation(
	_ context.Context,
	_ ports.FulfillmentSegmentKey,
	_ domain.FulfillmentParticipation,
) (ports.ParticipationEndOutcome, error) {
	return ports.ParticipationEndOutcomeInvalid, nil
}

func (double *segmentRegistryDouble) CloseSegment(
	_ context.Context,
	_ ports.FulfillmentSegmentKey,
	_ time.Time,
) (ports.SegmentCloseOutcome, error) {
	return ports.SegmentCloseOutcomeInvalid, nil
}

func (double *segmentRegistryDouble) saved(t *testing.T, tenant, segment string) ports.FulfillmentSegmentRecord {
	t.Helper()
	for _, rows := range double.rows {
		if rows.key.TenantID.String() != tenant || rows.key.Segment.String() != segment {
			continue
		}
		record, found, err := double.FindByKey(context.Background(), rows.key)
		if err != nil || !found {
			t.Fatalf("段 (%s, %s) 装不回来：err=%v found=%v", tenant, segment, err, found)
		}
		return record
	}
	t.Fatalf("段登记册里没有 (%s, %s)", tenant, segment)
	return ports.FulfillmentSegmentRecord{}
}

type handoverSegmentFixture struct {
	handovers *handoverRegistryDouble
	segments  *segmentRegistryDouble
	handler   *application.RegisterTransportHandoverHandler
}

func newHandoverSegmentFixture(t *testing.T) *handoverSegmentFixture {
	t.Helper()
	fixture := &handoverSegmentFixture{
		handovers: newHandoverRegistry(),
		segments:  newSegmentRegistry(),
	}
	fixture.handler = application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  fixture.handovers,
		Segments:   fixture.segments,
		Downstream: &handoverHandoffDouble{},
		Clock:      handoverClock{at: handoverRegisteredAt},
	})
	return fixture
}

// 首个对象凭`已交接`的权威交接立段：CONTEXT 生命周期①「有效收寄或权威交接确认首个
// 载运对象进入共同运输控制范围 → 实际履约段成立，该对象形成有效履约参与关系」。
func TestHandedOverFirstObjectEstablishesSegment(t *testing.T) {
	fixture := newHandoverSegmentFixture(t)
	command := registerHandoverCommand(t)
	command.Segment = "segment-1"
	command.PlannedSegment = "planned-segment-1"

	result, err := fixture.handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if result.Outcome() != application.HandoverRegistered {
		t.Fatalf("outcome = %q, want HANDOVER_REGISTERED", result.Outcome())
	}

	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	object, err := domain.NewCarriedObjectReference("parcel-1")
	if err != nil {
		t.Fatalf("对象引用：%v", err)
	}
	participation, joined := record.Segment.ParticipationFor(object)
	if !joined {
		t.Fatal("段成立了，首个对象却没有履约参与关系")
	}
	if participation.EntryKind() != domain.EnteredByTransportHandover {
		t.Fatalf("参与起点种类 = %q, want TRANSPORT_HANDOVER", participation.EntryKind())
	}
	// 起点时刻取裁决的业务时间而不是登记时刻——控制在判断成立时转移，不在写库时转移。
	if !participation.EnteredAt().Equal(handoverJudgedTime) {
		t.Fatalf("参与起点时刻 = %s, want %s", participation.EnteredAt(), handoverJudgedTime)
	}
}
