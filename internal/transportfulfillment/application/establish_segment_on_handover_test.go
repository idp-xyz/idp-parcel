package application_test

import (
	"context"
	"errors"
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
	// joinErrObject 限定 joinErr 只作用于某一个对象；空则所有加入都失败。多对象到访要
	// 证「只有没进去的那几个被报出来」，得让同一次到访里一部分成一部分败。
	joinErrObject string
	saves         int
	joins         int
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
	if double.joinErr != nil &&
		(double.joinErrObject == "" || double.joinErrObject == participation.Object().String()) {
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

// 后续对象加入既有段：CONTEXT 生命周期②「后续兼容载运对象取得同一运输控制 → 分别加入
// 实际履约段并形成自己的参与起点，不修改其他对象的起点」。
//
// 「这个段是否已成立」这一判落在段登记册的取回上，不在编排里记——并发两个对象同时到达时，
// 那个缓存就是一次竞态。
func TestSecondHandedOverObjectJoinsTheSameSegment(t *testing.T) {
	fixture := newHandoverSegmentFixture(t)

	first := registerHandoverCommand(t)
	first.Segment = "segment-1"
	if _, err := fixture.handler.Register(t.Context(), first); err != nil {
		t.Fatalf("首个对象登记：%v", err)
	}

	secondJudgedAt := handoverJudgedTime.Add(90 * time.Minute)
	second := registerHandoverCommand(t)
	second.Object = "parcel-2"
	second.Version = "handover-result/parcel-2/v1"
	second.JudgedAt = secondJudgedAt
	second.Segment = "segment-1"
	if _, err := fixture.handler.Register(t.Context(), second); err != nil {
		t.Fatalf("后续对象登记：%v", err)
	}

	if fixture.segments.saves != 1 {
		t.Fatalf("段首登 %d 次——后续对象该加入既有段而不是另立一个", fixture.segments.saves)
	}
	if fixture.segments.joins != 1 {
		t.Fatalf("加入既有段 %d 次, want 1", fixture.segments.joins)
	}

	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	firstEnteredAt := participationEnteredAt(t, record, "parcel-1")
	secondEnteredAt := participationEnteredAt(t, record, "parcel-2")
	if !secondEnteredAt.Equal(secondJudgedAt) {
		t.Fatalf("后续对象起点 = %s, want %s", secondEnteredAt, secondJudgedAt)
	}
	// 后一个对象加入不得改写前一个的起点——整段结果覆盖不了成员差异。
	if !firstEnteredAt.Equal(handoverJudgedTime) {
		t.Fatalf("首个对象起点被改写成 %s, want %s", firstEnteredAt, handoverJudgedTime)
	}
}

// 拒收与待确认立不起段，而登记本身照样成立：CONTEXT「已拒收或待确认不转出控制，此前
// 控制方在结果成立前继续保持控制」。**不立段不是失败**——那是正当的业务结果，把它做成
// 错误会让调用方以为这次交接没登上。
func TestHandoverWithoutControlTransferEstablishesNoSegment(t *testing.T) {
	for _, verdict := range []domain.HandoverVerdict{
		domain.HandoverRefused,
		domain.HandoverPendingConfirmation,
	} {
		t.Run(verdict.String(), func(t *testing.T) {
			fixture := newHandoverSegmentFixture(t)
			command := registerHandoverCommand(t)
			command.Verdict = verdict
			// 拒收与待确认必须带依据、且不带双方证据与适用规则（构造门的分格要求）。
			command.ReleasingEvidence = ""
			command.ReceivingEvidence = ""
			command.Rule = ""
			command.Basis = "handover-basis/refusal-1"
			command.Segment = "segment-1"

			result, err := fixture.handler.Register(t.Context(), command)
			if err != nil {
				t.Fatalf("登记：%v", err)
			}
			if result.Outcome() != application.HandoverRegistered {
				t.Fatalf("outcome = %q, want HANDOVER_REGISTERED——立不起段不该翻掉登记", result.Outcome())
			}
			if fixture.segments.saves != 0 || fixture.segments.joins != 0 {
				t.Fatalf("不转出控制的裁决动了段登记册：saves=%d joins=%d", fixture.segments.saves, fixture.segments.joins)
			}
		})
	}
}

// 立段失败不回滚交接登记，但要留下续办引用：接货时间是责任起点锚，一次段登记故障抹不掉
// 一条已经发生的物理事实；而派生的那一半没做成，调用方得有东西据以续办。
func TestSegmentRegistryFailureKeepsTheHandoverRegistered(t *testing.T) {
	fixture := newHandoverSegmentFixture(t)
	fixture.segments.saveErr = errors.New("段登记册不可用")
	command := registerHandoverCommand(t)
	command.Segment = "segment-1"

	result, err := fixture.handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("立段失败不该上抛技术错误：%v", err)
	}
	if result.Outcome() != application.HandoverRegistered {
		t.Fatalf("outcome = %q, want HANDOVER_REGISTERED——段没立起来抹不掉已经发生的交接", result.Outcome())
	}
	if _, recorded := result.Record(); !recorded {
		t.Fatal("交接记录没交回来")
	}
	if result.SegmentContinuationReference() == "" {
		t.Fatal("段没立起来却没有续办引用——调用方无从续办这一半")
	}
}

// 段立住时不留续办引用：那个引用非空的意思是「这一半还欠着」，成立时留着会让调用方
// 反复重试一件已经做完的事。
func TestEstablishedSegmentLeavesNoContinuation(t *testing.T) {
	fixture := newHandoverSegmentFixture(t)
	command := registerHandoverCommand(t)
	command.Segment = "segment-1"

	result, err := fixture.handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if reference := result.SegmentContinuationReference(); reference != "" {
		t.Fatalf("段已立起却留了续办引用 %q", reference)
	}
}

func participationEnteredAt(t *testing.T, record ports.FulfillmentSegmentRecord, object string) time.Time {
	t.Helper()
	reference, err := domain.NewCarriedObjectReference(object)
	if err != nil {
		t.Fatalf("对象引用：%v", err)
	}
	participation, joined := record.Segment.ParticipationFor(reference)
	if !joined {
		t.Fatalf("段里没有 %s 的履约参与关系", object)
	}
	return participation.EnteredAt()
}
