package domain

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// 发生项有效性修订的两扇生产门（ADR-0232 决定四、五）：失效与失效之后凭同一件事实重新成立。两扇都只长出新版本、回指前一版，
// 原版一字不动（CONTEXT「来源更正、取消结果变化或发生范围被证明无效时，保留原发生项并形成失效、替代或范围更正关系」）。

var (
	// ErrChargeOccurrenceAlreadyInvalidated：链头已是失效版本。失效说的是「那个范围不成立」，同一个范围再失效一次说不出新东西。
	ErrChargeOccurrenceAlreadyInvalidated = errors.New("transport fulfillment: the charge occurrence is already invalidated")
	// ErrChargeOccurrenceNotInvalidated：要凭同一件事实重新成立，链头却不是失效版本。仍有效的那一版就是这件事的答案，同内容是重放、
	// 异内容是冲突（ADR-0207 决定四），不是长出新版本的理由。
	ErrChargeOccurrenceNotInvalidated = errors.New("transport fulfillment: the charge occurrence is not invalidated")
)

// NextOccurrenceValidity 交回链上紧接 current 的那一版有效性版本：首版是 V1（FirstOccurrenceValidity），此后逐版加一。
//
// 由前一版派生而不签发，理由与发生项身份同一条（ADR-0207 决定四）：两笔并发修订同一版要落在同一个主键上，后到者撞键读回，
// 才答得出「已失效」而不是在同一版后面长出两条分叉。不是 V 加正整数的版本说明这条链不是本处长出来的，不替它猜下一版。
func NextOccurrenceValidity(current OccurrenceValidityVersion) (OccurrenceValidityVersion, error) {
	digits, found := strings.CutPrefix(current.String(), "V")
	if !found {
		return OccurrenceValidityVersion{}, ErrInvalidChargeOccurrence
	}
	ordinal, err := strconv.Atoi(digits)
	if err != nil || ordinal < 1 || strconv.Itoa(ordinal) != digits {
		return OccurrenceValidityVersion{}, ErrInvalidChargeOccurrence
	}
	return NewOccurrenceValidityVersion("V" + strconv.Itoa(ordinal+1))
}

// Invalidated 说这一版是不是失效版本：链头失效，这一项此刻就不是任何外部成本的来源。
func (occurrence TransportChargeOccurrence) Invalidated() bool {
	return occurrence.revisionKind == OccurrenceInvalidated
}

// RevisedBy 交回作出这一次修订决定的人，只在经修订入口形成的修订版本上给出。
func (occurrence TransportChargeOccurrence) RevisedBy() (RegistrantReference, bool) {
	return occurrence.revisedBy, occurrence.revisedBy.valid()
}

// InvalidateChargeOccurrence 让链头那一版失效：下一版回指它、标失效，依据是作出决定的人交来的那一条，时刻是决定落册的时刻。
// 内容（旅程、范围、成员、采购上下文、数量）原样带过去——失效说的是那件事的范围不成立，不是另一件事（ReviseValidity 头注）。
func InvalidateChargeOccurrence(
	head TransportChargeOccurrence,
	basis OccurrenceBasisReference,
	at time.Time,
	by RegistrantReference,
) (TransportChargeOccurrence, error) {
	if head.Invalidated() {
		return TransportChargeOccurrence{}, ErrChargeOccurrenceAlreadyInvalidated
	}
	if !by.valid() {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	next, err := NextOccurrenceValidity(head.validity)
	if err != nil {
		return TransportChargeOccurrence{}, err
	}
	invalidated, err := head.ReviseValidity(OccurrenceInvalidated, next, basis, at)
	if err != nil {
		return TransportChargeOccurrence{}, err
	}
	invalidated.revisedBy = by
	return invalidated, nil
}

// ReestablishChargeOccurrence 让一项已失效的发生项凭同一件事实重新成立：formed 是这一次登记按事实与采购上下文新形成的那一份，
// 它从失效的链头长出替代版本（ADR-0232 决定五）。与参与关系同一个形状：链尾失效之后再成立，只能从失效版本长出替代版本，不回退到
// 更早的有效版本（CONTEXT「履约参与关系」）。
//
// 身份四维（租户、原因、事实依据、成员集合）同一，所以引用相同；范围由事实依据定，也同一。其余内容取这一次的判断——指错了委托的
// 那一项失效之后，新的采购上下文正是要换掉的东西。依据取这一次的事实依据：替代版本凭的是同一件事实的重新判断。修订者是这一次的
// 登记者。
func ReestablishChargeOccurrence(
	head TransportChargeOccurrence,
	formed TransportChargeOccurrence,
	at time.Time,
) (TransportChargeOccurrence, error) {
	if !head.Invalidated() {
		return TransportChargeOccurrence{}, ErrChargeOccurrenceNotInvalidated
	}
	if _, revised := formed.Corrects(); revised ||
		formed.tenantID != head.tenantID ||
		formed.occurrence != head.occurrence ||
		formed.reason != head.reason ||
		formed.factBasis != head.factBasis ||
		formed.scope != head.scope ||
		!sameMemberSet(formed.members, head.members) {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	next, err := NextOccurrenceValidity(head.validity)
	if err != nil {
		return TransportChargeOccurrence{}, err
	}
	shaped := formed
	shaped.validity = head.validity
	reestablished, err := shaped.ReviseValidity(OccurrenceSuperseded, next, formed.factBasis, at)
	if err != nil {
		return TransportChargeOccurrence{}, err
	}
	reestablished.revisedBy = formed.condition.registrant
	return reestablished, nil
}

// SameFactContentAs 判两份发生项是不是同一次判断：SameContentAs 去掉有效性版本与修订四件。重登时链头可能是一个替代版本，而这一次
// 新形成的是首版形状，两者说的是同一件事、同一份采购上下文时就是重放。
func (occurrence TransportChargeOccurrence) SameFactContentAs(other TransportChargeOccurrence) bool {
	left, right := occurrence, other
	left.validity, right.validity = OccurrenceValidityVersion{}, OccurrenceValidityVersion{}
	left.corrects, right.corrects = OccurrenceValidityVersion{}, OccurrenceValidityVersion{}
	left.revisionKind, right.revisionKind = OccurrenceRevisionKindInvalid, OccurrenceRevisionKindInvalid
	left.revisionBasis, right.revisionBasis = OccurrenceBasisReference{}, OccurrenceBasisReference{}
	left.revisedAt, right.revisedAt = time.Time{}, time.Time{}
	return left.SameContentAs(right)
}
