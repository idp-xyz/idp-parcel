package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidClosure    = errors.New("customs compliance: invalid closure verification")
	ErrClosureBlocked    = errors.New("customs compliance: unresolved items block the closure")
	ErrInvalidReopening  = errors.New("customs compliance: invalid controlled reopening")
	ErrCaseAlreadyClosed = errors.New("customs compliance: the case is already closed")
	ErrCaseNotClosed     = errors.New("customs compliance: the case is not closed")
)

// ObligationItemState 是关闭依据项的封闭三值：已终结、已被有权接收方有效承接、未
// 解决。前两者可关，任一未解决阻止关闭（CONTEXT「任一未解决或冲突项都阻止关闭」）。
type ObligationItemState uint8

const (
	ObligationItemStateInvalid ObligationItemState = iota
	ObligationConcluded
	ObligationHandedOver
	ObligationUnresolved
)

func (state ObligationItemState) valid() bool {
	return state >= ObligationConcluded && state <= ObligationUnresolved
}

func (state ObligationItemState) String() string {
	switch state {
	case ObligationConcluded:
		return "CONCLUDED"
	case ObligationHandedOver:
		return "HANDED_OVER"
	case ObligationUnresolved:
		return "UNRESOLVED"
	default:
		return ""
	}
}

// ClosureObligationItem 是一项关闭依据：义务、范围、状态与依据。承接项必须指名接收
// 责任方——发送交接、技术送达、部分接受或默认超时接受都不能证明责任已经移交（219），
// 承接引用正是「有权接收方接受决定」的落点。
type ClosureObligationItem struct {
	Obligation string
	Scope      string
	State      ObligationItemState
	Basis      string
	HandedTo   string
}

func (item ClosureObligationItem) complete() bool {
	if item.Obligation == "" || item.Scope == "" || !item.State.valid() || item.Basis == "" {
		return false
	}
	if item.State == ObligationHandedOver && item.HandedTo == "" {
		return false
	}
	return true
}

// ClosureVerification 是在明确业务截点对案件内全部适用义务逐项形成关闭依据的版本化
// 评估（CONTEXT「关务案件关闭核对」）。它可以证明案件可关闭或指出未决项，但不等于
// 已经形成关闭决定——决定是另一步。
type ClosureVerification struct {
	caseRef    CustomsCaseID
	cutoffAt   time.Time
	items      []ClosureObligationItem
	verifiedAt time.Time
}

// VerifyClosure 形成关闭核对。义务清单非空且逐项完整；截点必备——没有业务截点的
// 盘点说不清「截至什么时候」。
func VerifyClosure(
	caseRef CustomsCaseID,
	cutoffAt time.Time,
	items []ClosureObligationItem,
	verifiedAt time.Time,
) (ClosureVerification, error) {
	if !caseRef.valid() || cutoffAt.IsZero() || len(items) == 0 || verifiedAt.IsZero() {
		return ClosureVerification{}, ErrInvalidClosure
	}
	for _, item := range items {
		if !item.complete() {
			return ClosureVerification{}, ErrInvalidClosure
		}
	}
	return ClosureVerification{
		caseRef:    caseRef,
		cutoffAt:   cutoffAt.UTC(),
		items:      append([]ClosureObligationItem(nil), items...),
		verifiedAt: verifiedAt.UTC(),
	}, nil
}

func (verification ClosureVerification) Items() []ClosureObligationItem {
	return append([]ClosureObligationItem(nil), verification.items...)
}

func (verification ClosureVerification) CaseRef() CustomsCaseID {
	return verification.caseRef
}

func (verification ClosureVerification) CutoffAt() time.Time {
	return verification.cutoffAt
}

func (verification ClosureVerification) VerifiedAt() time.Time {
	return verification.verifiedAt
}

// UnresolvedItems 给出全部未解决项——关闭被谁挡着一目了然。
func (verification ClosureVerification) UnresolvedItems() []ClosureObligationItem {
	unresolved := make([]ClosureObligationItem, 0)
	for _, item := range verification.items {
		if item.State == ObligationUnresolved {
			unresolved = append(unresolved, item)
		}
	}
	return unresolved
}

// Closable 报告全部依据项是否都已终结或已被有效承接。
func (verification ClosureVerification) Closable() bool {
	return len(verification.UnresolvedItems()) == 0
}

// CustomsCaseClosure 是关务案件的关闭决定与受控重开记录。单个案件不存在部分关闭
// （218）——关闭是整案一次决定；重开保留原关闭记录（233）。
type CustomsCaseClosure struct {
	caseRef      CustomsCaseID
	verification ClosureVerification
	decidedBy    string
	closedAt     time.Time
	reopenings   []ControlledReopening
}

// ControlledReopening 是一次受控重开：迟到监管事实仍属原固定案件身份和同一监管程序，
// 且依据使当前关闭期成立的关闭决定、受影响关闭依据项、原责任来源及当前授权形成
// （CONTEXT 生命周期「已关闭 → 重新打开」）。
type ControlledReopening struct {
	LateFact      string
	AffectedItems []string
	Authority     string
	ReopenedAt    time.Time
}

// CloseCase 依据可关闭的核对形成关闭决定。任一未解决项阻止关闭（独立哨兵带清单）；
// 关闭核对证明可关闭不等于已关闭——这一步才是有权责任角色的决定。
func CloseCase(
	verification ClosureVerification,
	decidedBy string,
	closedAt time.Time,
) (*CustomsCaseClosure, error) {
	if !verification.caseRef.valid() || decidedBy == "" || closedAt.IsZero() ||
		closedAt.Before(verification.verifiedAt) {
		return nil, ErrInvalidClosure
	}
	if !verification.Closable() {
		return nil, ErrClosureBlocked
	}
	return &CustomsCaseClosure{
		caseRef:      verification.caseRef,
		verification: verification,
		decidedBy:    decidedBy,
		closedAt:     closedAt.UTC(),
	}, nil
}

func (closure *CustomsCaseClosure) CaseRef() CustomsCaseID {
	return closure.caseRef
}

func (closure *CustomsCaseClosure) ClosedAt() time.Time {
	return closure.closedAt
}

func (closure *CustomsCaseClosure) DecidedBy() string {
	return closure.decidedBy
}

func (closure *CustomsCaseClosure) Verification() ClosureVerification {
	return closure.verification
}

// Reopenings 给出全部重开记录（副本）——原关闭记录和关闭期间事实继续保留，重开是
// 追加不是改写。
func (closure *CustomsCaseClosure) Reopenings() []ControlledReopening {
	return append([]ControlledReopening(nil), closure.reopenings...)
}

// Reopen 记录一次受控重开：迟到事实、受影响关闭依据项与当前授权缺一不可。受影响项
// 必须指向本次关闭核对里真实存在的义务——指不着的重开与凭空重开分不开。原关闭记录
// 不动（closedAt 与 verification 原样），重开只追加。
func (closure *CustomsCaseClosure) Reopen(reopening ControlledReopening) error {
	if reopening.LateFact == "" || len(reopening.AffectedItems) == 0 ||
		reopening.Authority == "" || reopening.ReopenedAt.IsZero() ||
		reopening.ReopenedAt.Before(closure.closedAt) {
		return ErrInvalidReopening
	}
	known := make(map[string]bool, len(closure.verification.items))
	for _, item := range closure.verification.items {
		known[item.Obligation] = true
	}
	for _, affected := range reopening.AffectedItems {
		if !known[affected] {
			return ErrInvalidReopening
		}
	}
	reopening.ReopenedAt = reopening.ReopenedAt.UTC()
	closure.reopenings = append(closure.reopenings, reopening)
	return nil
}
