package application_test

import (
	"cmp"
	"context"
	"slices"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// pendingRegisterDouble 是派送待形成册的内存替身：写口照迁移 0045 那张表的改写规则（立或刷新、已形成的不改回、了结不改指、只作罢
// 待形成的、只给待形成的记问过），队列口照 ListPending 的排序与筛法。err 非空时各写口都答它，扮演册读写不动。
type pendingRegisterDouble struct {
	rows   map[ports.DeliveryDispatchPendingKey]ports.DeliveryDispatchPendingRecord
	err    error
	writes int
	// held 记下每一拍占过的那一件，次序照调用。
	held []ports.DeliveryDispatchPendingKey
}

func (double *pendingRegisterDouble) HoldFormation(_ context.Context, key ports.DeliveryDispatchPendingKey) error {
	if double.err != nil {
		return double.err
	}
	double.held = append(double.held, key)
	return nil
}

func newPendingRegister() *pendingRegisterDouble {
	return &pendingRegisterDouble{rows: map[ports.DeliveryDispatchPendingKey]ports.DeliveryDispatchPendingRecord{}}
}

func (double *pendingRegisterDouble) RecordMissing(
	_ context.Context, key ports.DeliveryDispatchPendingKey, basis domain.ParticipationBasisReference, missing []string, at time.Time,
) error {
	if double.err != nil {
		return double.err
	}
	double.writes++
	row, existed := double.rows[key]
	if existed && row.State == ports.DeliveryDispatchPendingFormed {
		return nil
	}
	if !existed {
		row = ports.DeliveryDispatchPendingRecord{Key: key, FirstAnsweredAt: at}
	}
	row.EntryBasis, row.Missing, row.LastAnsweredAt, row.LastAskedAt = basis, slices.Clone(missing), at, at
	row.State, row.Task, row.LapsedBy, row.SettledAt = ports.DeliveryDispatchPending, "", "", time.Time{}
	double.rows[key] = row
	return nil
}

func (double *pendingRegisterDouble) SettleFormed(
	_ context.Context, key ports.DeliveryDispatchPendingKey, task domain.DispatchTaskReference, at time.Time,
) error {
	if double.err != nil {
		return double.err
	}
	double.writes++
	if row, existed := double.rows[key]; existed && row.State != ports.DeliveryDispatchPendingFormed {
		row.State, row.Task, row.LapsedBy, row.SettledAt, row.LastAskedAt = ports.DeliveryDispatchPendingFormed, task.String(), "", at, at
		double.rows[key] = row
	}
	return nil
}

func (double *pendingRegisterDouble) SettleLapsed(
	_ context.Context, key ports.DeliveryDispatchPendingKey, lapsedBy string, at time.Time,
) error {
	if double.err != nil {
		return double.err
	}
	double.writes++
	if row, existed := double.rows[key]; existed && row.State == ports.DeliveryDispatchPending {
		row.State, row.LapsedBy, row.SettledAt, row.LastAskedAt = ports.DeliveryDispatchPendingLapsed, lapsedBy, at, at
		double.rows[key] = row
	}
	return nil
}

func (double *pendingRegisterDouble) MarkAsked(_ context.Context, key ports.DeliveryDispatchPendingKey, at time.Time) error {
	if double.err != nil {
		return double.err
	}
	double.writes++
	if row, existed := double.rows[key]; existed && row.State == ports.DeliveryDispatchPending {
		row.LastAskedAt = at
		double.rows[key] = row
	}
	return nil
}

func (double *pendingRegisterDouble) CountPending(ctx context.Context, tenant domain.TenantID, askedBy time.Time) (int, error) {
	list, err := double.ListPending(ctx, tenant, askedBy, len(double.rows)+1)
	return len(list), err
}

func (double *pendingRegisterDouble) ListPending(
	_ context.Context, tenant domain.TenantID, askedBy time.Time, limit int,
) ([]ports.DeliveryDispatchPendingRecord, error) {
	var list []ports.DeliveryDispatchPendingRecord
	for _, row := range double.rows {
		if row.Key.TenantID != tenant || row.State != ports.DeliveryDispatchPending {
			continue
		}
		if !askedBy.IsZero() && row.LastAskedAt.After(askedBy) {
			continue
		}
		list = append(list, row)
	}
	slices.SortFunc(list, func(a, b ports.DeliveryDispatchPendingRecord) int {
		return cmp.Or(a.LastAskedAt.Compare(b.LastAskedAt),
			cmp.Compare(a.Key.Segment.String(), b.Key.Segment.String()),
			cmp.Compare(a.Key.Object.String(), b.Key.Object.String()))
	})
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

// row 按（段，对象）取那一条；没有就答零值与 false。
func (double *pendingRegisterDouble) row(tenant, segment, object string) (ports.DeliveryDispatchPendingRecord, bool) {
	for key, row := range double.rows {
		if key.TenantID.String() == tenant && key.Segment.String() == segment && key.Object.String() == object {
			return row, true
		}
	}
	return ports.DeliveryDispatchPendingRecord{}, false
}
