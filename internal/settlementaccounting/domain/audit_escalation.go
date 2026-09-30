package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidAuditEscalation 说明上限或被审金额不是这套判断能用的输入。
var ErrInvalidAuditEscalation = errors.New("settlement accounting: invalid audit escalation")

// AuditEscalationJudgment 是越权升级的封闭判断。等于上限仍在权限内；超过上限必须升级。
// 没登记不是这里的一格。
type AuditEscalationJudgment uint8

const (
	AuditEscalationJudgmentInvalid AuditEscalationJudgment = iota
	AuditWithinAuthority
	AuditMustEscalate
)

func (judgment AuditEscalationJudgment) String() string {
	switch judgment {
	case AuditWithinAuthority:
		return "WITHIN_AUTHORITY"
	case AuditMustEscalate:
		return "MUST_ESCALATE"
	default:
		return ""
	}
}

// AuditEscalationCeiling 是某一供应商、责任法人、币种上的金额上限。零值不是一份登记：
// 上限 0 是租户可以登记的取值，任何正数金额都超过它，和「没登记」不是一回事。
type AuditEscalationCeiling struct {
	limitMinor int64
	ok         bool
}

func NewAuditEscalationCeiling(limitMinor int64) (AuditEscalationCeiling, error) {
	if limitMinor < 0 {
		return AuditEscalationCeiling{}, fmt.Errorf("%w: ceiling", ErrInvalidAuditEscalation)
	}
	return AuditEscalationCeiling{limitMinor: limitMinor, ok: true}, nil
}

func (ceiling AuditEscalationCeiling) LimitMinor() int64 { return ceiling.limitMinor }

func (ceiling AuditEscalationCeiling) Same(other AuditEscalationCeiling) bool {
	return ceiling.ok && other.ok && ceiling.limitMinor == other.limitMinor
}

// Judge 只做一次比较：被审金额小于或等于上限，在权限内；大于上限，必须升级。
// 不看角色，也不把超过上限改写成拒绝或放行。
func (ceiling AuditEscalationCeiling) Judge(amountMinor int64) (AuditEscalationJudgment, error) {
	if !ceiling.ok || amountMinor <= 0 {
		return AuditEscalationJudgmentInvalid, fmt.Errorf("%w: amount", ErrInvalidAuditEscalation)
	}
	if amountMinor > ceiling.limitMinor {
		return AuditMustEscalate, nil
	}
	return AuditWithinAuthority, nil
}

// AuditEscalationCeilingRegistration 把金额上限挂到供应商、责任法人和币种上。
// 审核人仍在审核授权册，不写进这一行。
type AuditEscalationCeilingRegistration struct {
	supplier    SupplierPartyReference
	legalEntity LegalEntityReference
	currency    CurrencyCode
	ceiling     AuditEscalationCeiling
}

func NewAuditEscalationCeilingRegistration(
	supplier SupplierPartyReference,
	legalEntity LegalEntityReference,
	currency CurrencyCode,
	ceiling AuditEscalationCeiling,
) (AuditEscalationCeilingRegistration, error) {
	if !supplier.valid() || !legalEntity.valid() || !currency.valid() || !ceiling.ok {
		return AuditEscalationCeilingRegistration{}, fmt.Errorf("%w: audit escalation ceiling", ErrBlankValue)
	}
	return AuditEscalationCeilingRegistration{
		supplier: supplier, legalEntity: legalEntity, currency: currency, ceiling: ceiling,
	}, nil
}

func (registration AuditEscalationCeilingRegistration) Supplier() SupplierPartyReference {
	return registration.supplier
}

func (registration AuditEscalationCeilingRegistration) LegalEntity() LegalEntityReference {
	return registration.legalEntity
}

func (registration AuditEscalationCeilingRegistration) Currency() CurrencyCode {
	return registration.currency
}

func (registration AuditEscalationCeilingRegistration) Ceiling() AuditEscalationCeiling {
	return registration.ceiling
}

func (registration AuditEscalationCeilingRegistration) SameRegistration(other AuditEscalationCeilingRegistration) bool {
	return registration.supplier.String() == other.supplier.String() &&
		registration.legalEntity.String() == other.legalEntity.String() &&
		registration.currency.String() == other.currency.String() &&
		registration.ceiling.Same(other.ceiling)
}
