package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidDutyPaymentVerificationReference = errors.New(
		"settlement accounting: invalid duty payment verification reference")
	ErrInvalidDutyPaymentVerificationAdoption = errors.New(
		"settlement accounting: invalid duty payment verification adoption")
)

// DeclarationScopeReference 指名 customs-compliance 拥有的申报范围——税费付款核对在那边按它逐范围
// 形成。本上下文只拿它当核对引用的一维，不解释范围里有什么。
type DeclarationScopeReference struct{ requiredValue }

func NewDeclarationScopeReference(value string) (DeclarationScopeReference, error) {
	required, err := newRequiredValue("declaration scope reference", value)
	return DeclarationScopeReference{required}, err
}

// DutyVerificationVersion 是一版税费付款核对的内容指纹。在 customs-compliance 那头，核对身份三维加
// 这个指纹就是「核对版本」：迟到事实换指纹换版、不按到达顺序覆盖。本上下文按它取信封所指的那一版，
// 不取 latest——信封先后与版本先后不同源。
type DutyVerificationVersion struct{ requiredValue }

func NewDutyVerificationVersion(value string) (DutyVerificationVersion, error) {
	required, err := newRequiredValue("duty verification version", value)
	return DutyVerificationVersion{required}, err
}

// DutyPaymentVerificationReference 是一版 customs-compliance 税费付款核对在本上下文的引用：核对身份
// 三维（申报范围、税费义务、资金事实）加版本指纹。四维合起来才指得到**一版**——少任一维只能指到
// 一族版本，而结算输入采用的是明确版本（UC-SA-001 步 2「采用明确版本的税费、付款核对……」）。
//
// 覆盖 / 差额 / 有效性三态不在引用里，也不在本上下文任何类型里：那是核对的结论，customs-compliance
// 是它的权威，本上下文按引用回读；复制一份就是第二处口径。
type DutyPaymentVerificationReference struct {
	scope   DeclarationScopeReference
	duty    TaxObligationReference
	funds   FundsFactReference
	version DutyVerificationVersion
}

func NewDutyPaymentVerificationReference(
	scope DeclarationScopeReference,
	duty TaxObligationReference,
	funds FundsFactReference,
	version DutyVerificationVersion,
) (DutyPaymentVerificationReference, error) {
	if !scope.valid() || !duty.valid() || !funds.valid() || !version.valid() {
		return DutyPaymentVerificationReference{}, ErrInvalidDutyPaymentVerificationReference
	}
	return DutyPaymentVerificationReference{scope: scope, duty: duty, funds: funds, version: version}, nil
}

func (reference DutyPaymentVerificationReference) Scope() DeclarationScopeReference {
	return reference.scope
}

func (reference DutyPaymentVerificationReference) Duty() TaxObligationReference {
	return reference.duty
}

func (reference DutyPaymentVerificationReference) Funds() FundsFactReference {
	return reference.funds
}

func (reference DutyPaymentVerificationReference) Version() DutyVerificationVersion {
	return reference.version
}

func (reference DutyPaymentVerificationReference) valid() bool {
	return reference.scope.valid() && reference.duty.valid() && reference.funds.valid() && reference.version.valid()
}

// DutyPaymentVerificationAdoption 是结算输入版本里「付款核对」那一格（CONTEXT 生命周期「结算输入已接收：
// 固定……付款核对……的采用版本」）：本上下文采用了 customs-compliance 的哪一版核对、何时采用。
//
// 它只有引用与时刻。采用一版核对**不是**实际代垫成立判断——那一格由 AssessActualAdvance 在全部输入
// 到齐后另行形成，且不由任一单项输入直接推导（CONTEXT「实际代垫成立判断」词条）；本类型上没有任何
// 裁决、金额或三态字段，正是让「输入已接收」与「判断已形成」在类型上就分得开。
type DutyPaymentVerificationAdoption struct {
	verification DutyPaymentVerificationReference
	adoptedAt    time.Time
}

// AdoptDutyPaymentVerification 形成一次采用：引用四维齐全、采用时刻在场。没有别的门——采用不判断
// 核对说了什么，也不判断其余输入到没到。
func AdoptDutyPaymentVerification(
	verification DutyPaymentVerificationReference,
	adoptedAt time.Time,
) (DutyPaymentVerificationAdoption, error) {
	if !verification.valid() || adoptedAt.IsZero() {
		return DutyPaymentVerificationAdoption{}, ErrInvalidDutyPaymentVerificationAdoption
	}
	return DutyPaymentVerificationAdoption{verification: verification, adoptedAt: adoptedAt.UTC()}, nil
}

func (adoption DutyPaymentVerificationAdoption) Verification() DutyPaymentVerificationReference {
	return adoption.verification
}

func (adoption DutyPaymentVerificationAdoption) AdoptedAt() time.Time {
	return adoption.adoptedAt
}
