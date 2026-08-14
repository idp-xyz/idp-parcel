package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidRehydratedCommission = errors.New("transport fulfillment: rehydrated transport commission violates its invariants")
	ErrInvalidRehydratedAcceptance = errors.New("transport fulfillment: rehydrated carrier acceptance violates its invariants")
)

// RehydrateTransportCommissionSpec 是委托行在库里的样子。开始与取消互斥；两者都缺
// 是刚提交。SubmitTransportCommission 不能兼职重建：它造不出已开始或已取消的委托。
type RehydrateTransportCommissionSpec struct {
	TenantID       TenantID
	Commission     TransportCommissionReference
	Provider       ServiceProviderReference
	Agreement      AgreementSnapshotReference
	Conditions     ConditionsSnapshotReference
	Role           RoleSnapshotReference
	Responsibility ResponsibilitySnapshotReference
	Members        []CarriedObjectReference
	SubmittedAt    time.Time
	StartedBasis   ParticipationBasisReference
	StartedAt      time.Time
	CancelledAt    time.Time
}

// RehydrateTransportCommission 验四快照、非空成员、提交时刻，以及开始/取消互斥与时刻
// 不早于提交。不重走 Cancel / MarkTransportStarted——那是状态转换门，不是读回。
func RehydrateTransportCommission(spec RehydrateTransportCommissionSpec) (TransportCommission, error) {
	if !spec.TenantID.valid() ||
		!spec.Commission.valid() ||
		!spec.Provider.valid() ||
		!spec.Agreement.valid() ||
		!spec.Conditions.valid() ||
		!spec.Role.valid() ||
		!spec.Responsibility.valid() ||
		len(spec.Members) == 0 ||
		spec.SubmittedAt.IsZero() {
		return TransportCommission{}, ErrInvalidRehydratedCommission
	}
	seen := make(map[CarriedObjectReference]struct{}, len(spec.Members))
	for _, member := range spec.Members {
		if !member.valid() {
			return TransportCommission{}, ErrInvalidRehydratedCommission
		}
		if _, exists := seen[member]; exists {
			return TransportCommission{}, ErrInvalidRehydratedCommission
		}
		seen[member] = struct{}{}
	}

	started := !spec.StartedAt.IsZero()
	cancelled := !spec.CancelledAt.IsZero()
	if started && cancelled {
		return TransportCommission{}, ErrInvalidRehydratedCommission
	}
	if started {
		if !spec.StartedBasis.valid() || spec.StartedAt.Before(spec.SubmittedAt) {
			return TransportCommission{}, ErrInvalidRehydratedCommission
		}
	} else if spec.StartedBasis.valid() {
		return TransportCommission{}, ErrInvalidRehydratedCommission
	}
	if cancelled && spec.CancelledAt.Before(spec.SubmittedAt) {
		return TransportCommission{}, ErrInvalidRehydratedCommission
	}

	commission := TransportCommission{
		tenantID:       spec.TenantID,
		commission:     spec.Commission,
		provider:       spec.Provider,
		agreement:      spec.Agreement,
		conditions:     spec.Conditions,
		role:           spec.Role,
		responsibility: spec.Responsibility,
		members:        append([]CarriedObjectReference(nil), spec.Members...),
		submittedAt:    spec.SubmittedAt.UTC(),
	}
	if started {
		commission.startedBasis = spec.StartedBasis
		commission.startedAt = spec.StartedAt.UTC()
	}
	if cancelled {
		commission.cancelledAt = spec.CancelledAt.UTC()
	}
	return commission, nil
}

// RehydrateCarrierAcceptanceSpec 是承运应答行在库里的样子。FormCarrierAcceptance
// 要一份订舱申请才能限量，而行里只有应答本身——申请量是写入时已经判过的。
type RehydrateCarrierAcceptanceSpec struct {
	TenantID   TenantID
	Acceptance CarrierAcceptanceReference
	Booking    BookingReference
	Outcome    CarrierAcceptanceOutcome
	Quantity   int64
	Basis      AcceptanceBasisReference
	DecidedAt  time.Time
}

// RehydrateCarrierAcceptance 验四值形状：接受带正数量无原因，拒绝/失效/撤回无数量
// 有原因。不重审申请量上限。
func RehydrateCarrierAcceptance(spec RehydrateCarrierAcceptanceSpec) (CarrierAcceptance, error) {
	if !spec.TenantID.valid() ||
		!spec.Acceptance.valid() ||
		!spec.Booking.valid() ||
		!spec.Outcome.valid() ||
		spec.DecidedAt.IsZero() {
		return CarrierAcceptance{}, ErrInvalidRehydratedAcceptance
	}
	if spec.Outcome == BookingAccepted {
		if spec.Quantity <= 0 || spec.Basis.valid() {
			return CarrierAcceptance{}, ErrInvalidRehydratedAcceptance
		}
	} else {
		if spec.Quantity != 0 || !spec.Basis.valid() {
			return CarrierAcceptance{}, ErrInvalidRehydratedAcceptance
		}
	}
	return CarrierAcceptance{
		tenantID:   spec.TenantID,
		acceptance: spec.Acceptance,
		booking:    spec.Booking,
		outcome:    spec.Outcome,
		quantity:   spec.Quantity,
		basis:      spec.Basis,
		decidedAt:  spec.DecidedAt.UTC(),
	}, nil
}
