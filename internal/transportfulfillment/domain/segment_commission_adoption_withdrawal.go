package domain

import (
	"errors"
	"time"
)

// 解除履约段采用（ADR-0232 决定一至三）：登记方撤回「这一段在这份委托下履约」那条声明。采用那一行不改不删，解除另成一份事实；委托的
// 开始派生自已采用各段，撤掉一段，开始就按其余各段重新派生——还有别的已采用段时取其中最早的首个参与，一段也不剩时撤回到已提交。

var (
	// ErrInvalidAdoptionWithdrawal：解除的输入自相矛盾（连线与委托不是同一份、其余各段里混进了别的委托或这一段自己、委托此刻并未开始）。
	// 采用落下时委托随之开始，取消又挡在已开始上，所以一份仍有当前采用的委托不会未开始、不会已取消；走到这里是库里的数据违约。
	ErrInvalidAdoptionWithdrawal = errors.New("transport fulfillment: invalid segment commission adoption withdrawal")
)

// CommissionStartCorrection 说一次解除让委托的开始怎么变。这一转换只在解除里发生：采用只前移不后移（ADR-0204 决定二），解除撤掉的
// 那一段若正是开始所依的那一段，开始就要后移到其余各段里最早的那一条，或撤回到已提交——那次开始派生自后来证明错误的连线，不是已
// 发生的控制事实（ADR-0232 决定二）。
type CommissionStartCorrection uint8

const (
	CommissionStartCorrectionInvalid CommissionStartCorrection = iota
	// CommissionStartWithdrawn：委托再无别的已采用段，开始撤回，回到已提交。
	CommissionStartWithdrawn
	// CommissionStartRederived：还有别的已采用段，开始改按其中最早的首个参与；可能比原来晚。
	CommissionStartRederived
	// CommissionStartUnchanged：委托的开始本就依的是别的段，那一段还在，开始原样不动。
	CommissionStartUnchanged
)

func (correction CommissionStartCorrection) String() string {
	switch correction {
	case CommissionStartWithdrawn:
		return "WITHDRAWN"
	case CommissionStartRederived:
		return "REDERIVED"
	case CommissionStartUnchanged:
		return "KEPT"
	default:
		return ""
	}
}

// ParseCommissionStartCorrection 按字面认变法，大小写不折叠：解除册存的就是这三个字面量。
func ParseCommissionStartCorrection(value string) (CommissionStartCorrection, error) {
	for correction := CommissionStartWithdrawn; correction <= CommissionStartUnchanged; correction++ {
		if correction.String() == value {
			return correction, nil
		}
	}
	return CommissionStartCorrectionInvalid, ErrInvalidAdoptionWithdrawal
}

// SegmentCommissionAdoptionWithdrawal 是对一段的一代采用的一次解除：解的是哪一代、哪一份委托、谁解的，以及委托的开始在这一次之前
// 是什么、怎么变、之后是什么。之前与之后都随解除保存：委托那一行只是此刻的样子，开始换过什么、凭哪一次解除换的，只有这里说得出。
type SegmentCommissionAdoptionWithdrawal struct {
	tenantID         TenantID
	segment          FulfillmentSegmentReference
	generation       int
	commission       TransportCommissionReference
	withdrawnBy      RegistrantReference
	startBeforeBasis ParticipationBasisReference
	startBeforeAt    time.Time
	change           CommissionStartCorrection
	startAfterBasis  ParticipationBasisReference
	startAfterAt     time.Time
}

// WithdrawSegmentCommissionAdoption 判一次解除，交回解除这一份事实与委托此后的样子。
//
// remaining 是这份委托在别的段上的当前采用（不含这一段）。委托此后的开始取其中最早的首个参与——每一段在委托下的开始依据与时刻记在
// 它自己的采用上（ADR-0204 决定二），这里照它取，不回头按段上此刻的参与重算：首个参与被更正后要不要重新派生是另一问（票
// demo-chain-gaps/162）。同一刻的按依据字面序取第一条，与段上取首个参与同一个判法。
func WithdrawSegmentCommissionAdoption(
	adoption SegmentCommissionAdoption,
	generation int,
	commission TransportCommission,
	remaining []SegmentCommissionAdoption,
	by RegistrantReference,
) (SegmentCommissionAdoptionWithdrawal, TransportCommission, error) {
	if !adoption.tenantID.valid() || generation < 1 || !by.valid() ||
		adoption.tenantID != commission.tenantID || adoption.commission != commission.commission {
		return SegmentCommissionAdoptionWithdrawal{}, TransportCommission{}, ErrInvalidAdoptionWithdrawal
	}
	beforeBasis, beforeAt, started := commission.TransportStarted()
	if _, cancelled := commission.Cancelled(); !started || cancelled {
		return SegmentCommissionAdoptionWithdrawal{}, TransportCommission{}, ErrInvalidAdoptionWithdrawal
	}
	var earliest SegmentCommissionAdoption
	found, startStays := false, false
	for _, other := range remaining {
		if other.tenantID != adoption.tenantID || other.commission != adoption.commission || other.segment == adoption.segment {
			return SegmentCommissionAdoptionWithdrawal{}, TransportCommission{}, ErrInvalidAdoptionWithdrawal
		}
		// 开始所依的那一段还在，开始就不动——同刻并列时它不一定是字面序最小的那一条：采用遇同刻不前移，开始依的是先采用的那一段。
		if other.startBasis == beforeBasis && sameOnRegistry(other.startedAt, beforeAt) {
			startStays = true
		}
		if !found || other.startedAt.Before(earliest.startedAt) ||
			(other.startedAt.Equal(earliest.startedAt) && other.startBasis.String() < earliest.startBasis.String()) {
			earliest = other
			found = true
		}
	}
	withdrawal := SegmentCommissionAdoptionWithdrawal{
		tenantID:         adoption.tenantID,
		segment:          adoption.segment,
		generation:       generation,
		commission:       adoption.commission,
		withdrawnBy:      by,
		startBeforeBasis: beforeBasis,
		startBeforeAt:    beforeAt.UTC(),
	}
	corrected := commission
	corrected.members = append([]CarriedObjectReference(nil), commission.members...)
	switch {
	case !found:
		withdrawal.change = CommissionStartWithdrawn
		corrected.startedBasis = ParticipationBasisReference{}
		corrected.startedAt = time.Time{}
	case startStays:
		withdrawal.change = CommissionStartUnchanged
		withdrawal.startAfterBasis, withdrawal.startAfterAt = beforeBasis, beforeAt.UTC()
	default:
		if earliest.startedAt.Before(commission.submittedAt) {
			return SegmentCommissionAdoptionWithdrawal{}, TransportCommission{}, ErrInvalidAdoptionWithdrawal
		}
		withdrawal.change = CommissionStartRederived
		withdrawal.startAfterBasis, withdrawal.startAfterAt = earliest.startBasis, earliest.startedAt.UTC()
		corrected.startedBasis = earliest.startBasis
		corrected.startedAt = earliest.startedAt.UTC()
	}
	return withdrawal, corrected, nil
}

func (withdrawal SegmentCommissionAdoptionWithdrawal) TenantID() TenantID { return withdrawal.tenantID }

func (withdrawal SegmentCommissionAdoptionWithdrawal) Segment() FulfillmentSegmentReference {
	return withdrawal.segment
}

// Generation 是被解除的那一代采用：一段上第几次采用。
func (withdrawal SegmentCommissionAdoptionWithdrawal) Generation() int { return withdrawal.generation }

func (withdrawal SegmentCommissionAdoptionWithdrawal) Commission() TransportCommissionReference {
	return withdrawal.commission
}

// WithdrawnBy 是作出这一次解除决定的人，写成「发行方#sub」。
func (withdrawal SegmentCommissionAdoptionWithdrawal) WithdrawnBy() RegistrantReference {
	return withdrawal.withdrawnBy
}

// StartBefore 是这一次解除之前委托的开始依据与时刻。
func (withdrawal SegmentCommissionAdoptionWithdrawal) StartBefore() (ParticipationBasisReference, time.Time) {
	return withdrawal.startBeforeBasis, withdrawal.startBeforeAt
}

func (withdrawal SegmentCommissionAdoptionWithdrawal) StartChange() CommissionStartCorrection {
	return withdrawal.change
}

// StartAfter 是这一次解除之后委托的开始；撤回到已提交时 present=false。
func (withdrawal SegmentCommissionAdoptionWithdrawal) StartAfter() (ParticipationBasisReference, time.Time, bool) {
	if withdrawal.change == CommissionStartWithdrawn {
		return ParticipationBasisReference{}, time.Time{}, false
	}
	return withdrawal.startAfterBasis, withdrawal.startAfterAt, true
}

// RehydrateSegmentCommissionAdoptionWithdrawalSpec 是解除行在库里的样子。
type RehydrateSegmentCommissionAdoptionWithdrawalSpec struct {
	TenantID         TenantID
	Segment          FulfillmentSegmentReference
	Generation       int
	Commission       TransportCommissionReference
	WithdrawnBy      RegistrantReference
	StartBeforeBasis ParticipationBasisReference
	StartBeforeAt    time.Time
	StartChange      CommissionStartCorrection
	StartAfterBasis  ParticipationBasisReference
	StartAfterAt     time.Time
}

// RehydrateSegmentCommissionAdoptionWithdrawal 只验各格成形与变法和之后那一格成对，不重判：解除当时对着委托与其余各段判过，行里没有
// 它们可以再对一遍。
func RehydrateSegmentCommissionAdoptionWithdrawal(spec RehydrateSegmentCommissionAdoptionWithdrawalSpec) (SegmentCommissionAdoptionWithdrawal, error) {
	if !spec.TenantID.valid() || !spec.Segment.valid() || spec.Generation < 1 || !spec.Commission.valid() ||
		!spec.WithdrawnBy.valid() || !spec.StartBeforeBasis.valid() || spec.StartBeforeAt.IsZero() {
		return SegmentCommissionAdoptionWithdrawal{}, ErrInvalidAdoptionWithdrawal
	}
	after := spec.StartAfterBasis.valid() && !spec.StartAfterAt.IsZero()
	switch spec.StartChange {
	case CommissionStartWithdrawn:
		if spec.StartAfterBasis.valid() || !spec.StartAfterAt.IsZero() {
			return SegmentCommissionAdoptionWithdrawal{}, ErrInvalidAdoptionWithdrawal
		}
	case CommissionStartRederived, CommissionStartUnchanged:
		if !after {
			return SegmentCommissionAdoptionWithdrawal{}, ErrInvalidAdoptionWithdrawal
		}
	default:
		return SegmentCommissionAdoptionWithdrawal{}, ErrInvalidAdoptionWithdrawal
	}
	return SegmentCommissionAdoptionWithdrawal{
		tenantID:         spec.TenantID,
		segment:          spec.Segment,
		generation:       spec.Generation,
		commission:       spec.Commission,
		withdrawnBy:      spec.WithdrawnBy,
		startBeforeBasis: spec.StartBeforeBasis,
		startBeforeAt:    spec.StartBeforeAt.UTC(),
		change:           spec.StartChange,
		startAfterBasis:  spec.StartAfterBasis,
		startAfterAt:     spec.StartAfterAt.UTC(),
	}, nil
}
