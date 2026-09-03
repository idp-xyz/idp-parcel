package tfhttp_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// segmentRegistryDouble 是 ports.ActualFulfillmentSegmentRegistry 的替身，供交接与揽收三个
// 端点的测试共用。
//
// 它按行存参与关系、读时走 RehydrateActualFulfillmentSegment 装回，理由与应用层同名替身
// 一字不差：真库那一侧每次读都要重过重建门，替身若存整个聚合原样交回就比真库宽容。端点
// 测试用它只为一件事——证明 Intake 交出的 `Segment`/`PlannedSegment` 真的让对象进了段，
// 而不是在传输层被丢掉；所以它不需要错误注入以外的任何花样。
type segmentRegistryDouble struct {
	rows    map[string]*segmentRowsDouble
	findErr error
}

type segmentRowsDouble struct {
	key            ports.FulfillmentSegmentKey
	participations []domain.RehydrateParticipationSpec
	recordedAt     time.Time
	closed         bool
	closedAt       time.Time
}

func newSegmentRegistry() *segmentRegistryDouble {
	return &segmentRegistryDouble{rows: map[string]*segmentRowsDouble{}}
}

func segmentRegistryKey(key ports.FulfillmentSegmentKey) string {
	return key.TenantID.String() + "|" + key.Segment.String()
}

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
		Closed:         rows.closed,
		ClosedAt:       rows.closedAt,
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
	key ports.FulfillmentSegmentKey,
	participation domain.FulfillmentParticipation,
) (ports.ParticipationEndOutcome, error) {
	rows, found := double.rows[segmentRegistryKey(key)]
	if !found {
		return ports.ParticipationEndOutcomeInvalid, nil
	}
	for index := range rows.participations {
		row := &rows.participations[index]
		if row.Object != participation.Object() {
			continue
		}
		if !row.EndedAt.IsZero() {
			return ports.ParticipationAlreadyEnded, nil
		}
		endKind, endBasis, endedAt, ended := participation.End()
		if !ended {
			return ports.ParticipationEndOutcomeInvalid, nil
		}
		row.EndKind, row.EndBasis, row.EndedAt = endKind, endBasis, endedAt
		return ports.ParticipationEnded, nil
	}
	return ports.ParticipationEndOutcomeInvalid, nil
}

func (double *segmentRegistryDouble) CloseSegment(
	_ context.Context,
	key ports.FulfillmentSegmentKey,
	closedAt time.Time,
) (ports.SegmentCloseOutcome, error) {
	rows, found := double.rows[segmentRegistryKey(key)]
	if !found {
		return ports.SegmentCloseOutcomeInvalid, nil
	}
	if rows.closed {
		return ports.SegmentAlreadyClosed, nil
	}
	rows.closed, rows.closedAt = true, closedAt.UTC()
	return ports.SegmentClosed, nil
}

// participationOf 找回某对象在某段里的参与关系；段或对象不在册就让测试失败。
func (double *segmentRegistryDouble) participationOf(
	t *testing.T,
	tenant, segment, object string,
) domain.FulfillmentParticipation {
	t.Helper()
	for _, rows := range double.rows {
		if rows.key.TenantID.String() != tenant || rows.key.Segment.String() != segment {
			continue
		}
		record, found, err := double.FindByKey(context.Background(), rows.key)
		if err != nil || !found {
			t.Fatalf("段 (%s, %s) 装不回来：err=%v found=%v", tenant, segment, err, found)
		}
		for _, participation := range record.Segment.Participations() {
			if participation.Object().String() == object {
				return participation
			}
		}
		t.Fatalf("段 (%s, %s) 里没有对象 %s", tenant, segment, object)
	}
	t.Fatalf("段登记册里没有 (%s, %s)", tenant, segment)
	return domain.FulfillmentParticipation{}
}

// closeSegment 把一个已成立的段按「全部参与已结束且不再接受新对象」的样子收口：逐条参与以明确
// 控制终止收尾，再落关闭两列。它直接改替身的行而不走编排，因为端点测试要的只是一个关着的段，
// 关段本身的规则由应用层与票 01 的测试守。
func (double *segmentRegistryDouble) closeSegment(t *testing.T, tenant, segment string, closedAt time.Time) {
	t.Helper()
	basis, err := domain.NewParticipationBasisReference("control-termination/test")
	if err != nil {
		t.Fatalf("basis: %v", err)
	}
	for _, rows := range double.rows {
		if rows.key.TenantID.String() != tenant || rows.key.Segment.String() != segment {
			continue
		}
		for index := range rows.participations {
			row := &rows.participations[index]
			if row.EndedAt.IsZero() {
				row.EndKind, row.EndBasis, row.EndedAt = domain.EndedByControlTermination, basis, closedAt.Add(-time.Minute)
			}
		}
		rows.closed, rows.closedAt = true, closedAt
		return
	}
	t.Fatalf("段登记册里没有 (%s, %s)，关不了", tenant, segment)
}

// endAllParticipations 让某段的每条在场参与以明确控制终止收尾，但**不关段**——关段那一步留给被测的
// 关段编排走，这样「全部参与已结束 → 才关得上」这条判据才是被端点真正触到的。
func (double *segmentRegistryDouble) endAllParticipations(t *testing.T, tenant, segment string, endedAt time.Time) {
	t.Helper()
	basis, err := domain.NewParticipationBasisReference("control-termination/test")
	if err != nil {
		t.Fatalf("basis: %v", err)
	}
	for _, rows := range double.rows {
		if rows.key.TenantID.String() != tenant || rows.key.Segment.String() != segment {
			continue
		}
		for index := range rows.participations {
			row := &rows.participations[index]
			if row.EndedAt.IsZero() {
				row.EndKind, row.EndBasis, row.EndedAt = domain.EndedByControlTermination, basis, endedAt
			}
		}
		return
	}
	t.Fatalf("段登记册里没有 (%s, %s)", tenant, segment)
}

func (double *segmentRegistryDouble) hasSegment(tenant, segment string) bool {
	for _, rows := range double.rows {
		if rows.key.TenantID.String() == tenant && rows.key.Segment.String() == segment {
			return true
		}
	}
	return false
}
