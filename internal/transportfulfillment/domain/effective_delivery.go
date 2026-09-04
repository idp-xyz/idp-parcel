package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidDeliveryAttemptResult = errors.New("transport fulfillment: invalid delivery attempt result")
	// ErrNotAnEffectiveDelivery 是 CONTEXT 交付节的硬句本体：「无人签收、地址错误、收件人
	// 拒收或证据不足不构成有效交付」（UC-TF-006 同句另列「包装不合格」——那是揽收侧失败
	// 家族的词，本类型的封闭集合不含它）。它与形状错误分格——恢复动作不是补字段，而是
	// 承认这个对象本次没有交付。
	ErrNotAnEffectiveDelivery   = errors.New("transport fulfillment: this outcome does not constitute an effective delivery")
	ErrInvalidEffectiveDelivery = errors.New("transport fulfillment: invalid effective delivery")
)

// DeliveryObjectOutcome 是一次派送尝试中单个载运对象结果的封闭集合（UC-TF-006：成功、
// 失败、拒收各自成立，任务汇总只能派生）。
type DeliveryObjectOutcome uint8

const (
	DeliveryObjectOutcomeInvalid DeliveryObjectOutcome = iota
	ObjectDelivered
	DeliveryRefused
	NoOneToReceive
	WrongAddress
)

func (outcome DeliveryObjectOutcome) valid() bool {
	return outcome >= ObjectDelivered && outcome <= WrongAddress
}

// Succeeded 只在妥投时为真。拒收刻意不带「已退回」语义：拒收只结束本次尝试的相应对象
// 结果，不自动等于已签收、已退回或服务终止（CONTEXT）。
func (outcome DeliveryObjectOutcome) Succeeded() bool {
	return outcome == ObjectDelivered
}

func (outcome DeliveryObjectOutcome) Failed() bool {
	return outcome.valid() && !outcome.Succeeded()
}

func (outcome DeliveryObjectOutcome) String() string {
	switch outcome {
	case ObjectDelivered:
		return "DELIVERED"
	case DeliveryRefused:
		return "REFUSED"
	case NoOneToReceive:
		return "NO_ONE_TO_RECEIVE"
	case WrongAddress:
		return "WRONG_ADDRESS"
	default:
		return ""
	}
}

// DeliveryAttemptResult 是一次派送尝试中单个载运对象的结果，纪律与揽收侧的
// AttemptObjectResult 同一条：逐对象成立、失败必须带原因依据、妥投不得携带失败依据；
// 结构上没有控制或履约段字段——妥投本身也不转移控制，控制转移只随有效交付成立。
type DeliveryAttemptResult struct {
	attempt    AttemptReference
	object     CarriedObjectReference
	outcome    DeliveryObjectOutcome
	basis      AttemptResultBasisReference
	occurredAt time.Time
}

func FormDeliveryAttemptResult(
	attempt FulfillmentAttempt,
	object CarriedObjectReference,
	outcome DeliveryObjectOutcome,
	basis AttemptResultBasisReference,
	occurredAt time.Time,
) (DeliveryAttemptResult, error) {
	if !attempt.attempt.valid() || !object.valid() || !outcome.valid() || occurredAt.IsZero() {
		return DeliveryAttemptResult{}, ErrInvalidDeliveryAttemptResult
	}
	if occurredAt.Before(attempt.arrivedAt) {
		return DeliveryAttemptResult{}, ErrInvalidDeliveryAttemptResult
	}
	if !attempt.Covers(object) {
		return DeliveryAttemptResult{}, ErrObjectOutsideAttempt
	}
	if outcome.Failed() && !basis.valid() {
		return DeliveryAttemptResult{}, ErrInvalidDeliveryAttemptResult
	}
	if outcome.Succeeded() && basis.valid() {
		// 妥投的证据走 POD 进有效交付；这里塞依据只会与交付证明两处打架。
		return DeliveryAttemptResult{}, ErrInvalidDeliveryAttemptResult
	}
	return DeliveryAttemptResult{
		attempt:    attempt.attempt,
		object:     object,
		outcome:    outcome,
		basis:      basis,
		occurredAt: occurredAt.UTC(),
	}, nil
}

func (result DeliveryAttemptResult) Attempt() AttemptReference {
	return result.attempt
}

func (result DeliveryAttemptResult) Object() CarriedObjectReference {
	return result.object
}

func (result DeliveryAttemptResult) Outcome() DeliveryObjectOutcome {
	return result.outcome
}

// Basis 在失败或拒收时交回原因依据；妥投没有它。
func (result DeliveryAttemptResult) Basis() (AttemptResultBasisReference, bool) {
	if !result.basis.valid() {
		return AttemptResultBasisReference{}, false
	}
	return result.basis, true
}

func (result DeliveryAttemptResult) OccurredAt() time.Time {
	return result.occurredAt
}

// DeliveryMethodReference 指名本次交付采用的方式（本人签收、安全投放、智能柜……）。
// 方式是否被产品与合同允许由适用规则判断，这里只保存采用了哪种。
type DeliveryMethodReference struct{ requiredValue }

func NewDeliveryMethodReference(value string) (DeliveryMethodReference, error) {
	required, err := newRequiredValue("delivery method reference", value)
	return DeliveryMethodReference{required}, err
}

// ReceivingPartyReference 指名接收对象——控制转移的受方。
type ReceivingPartyReference struct{ requiredValue }

func NewReceivingPartyReference(value string) (ReceivingPartyReference, error) {
	required, err := newRequiredValue("receiving party reference", value)
	return ReceivingPartyReference{required}, err
}

// DeliveryProofReference 指名支持有效交付的证据集合（POD）。它是引用不是可改的
// 「已签收」状态。
type DeliveryProofReference struct{ requiredValue }

func NewDeliveryProofReference(value string) (DeliveryProofReference, error) {
	required, err := newRequiredValue("delivery proof reference", value)
	return DeliveryProofReference{required}, err
}

// DeliveryResultVersion 是交付结果的版本标识：POD 更正形成新版本，不覆盖本版。
type DeliveryResultVersion struct{ requiredValue }

func NewDeliveryResultVersion(value string) (DeliveryResultVersion, error) {
	required, err := newRequiredValue("delivery result version", value)
	return DeliveryResultVersion{required}, err
}

// EffectiveDeliverySpec 携带有效交付在尝试结果之外还需要的内容。地点与业务时间不在
// 其中——地点取自尝试、时间取自对象结果，两处各自只有一个权威来源。
type EffectiveDeliverySpec struct {
	Method    DeliveryMethodReference
	Recipient ReceivingPartyReference
	Proof     DeliveryProofReference
	Version   DeliveryResultVersion
}

// EffectiveDelivery 是收件方边界的专用权威运输交接结果（CONTEXT「有效交付结果」）：
// 它同时构成该对象向收件方的控制转移并结束相应履约参与关系——不存在一个独立的
// 「转移开关」字段，本类型成立即转移成立。
//
// 实际承运商轴（CONTEXT「有效交付结果必须关联……实际承运商或待确认依据」）不在本类型上：那是
// `ActualCarrierJudgment` 的当前版本，按（租户，实际履约段）读——交付所在的段由该对象的履约参与关系
// 给出。这里不复制一份承运方引用，理由与段、参与关系上不加承运商字段同一条（ADR-0103 决定二、八）：
// 判断是独立聚合，复制过来的那一格会在判断追加新版本时静默过期。
type EffectiveDelivery struct {
	tenantID    TenantID
	object      CarriedObjectReference
	attempt     AttemptReference
	place       AttemptPlaceReference
	method      DeliveryMethodReference
	recipient   ReceivingPartyReference
	proof       DeliveryProofReference
	version     DeliveryResultVersion
	occurredAt  time.Time
	corrects    DeliveryResultVersion
	correctedAt time.Time
}

// FormEffectiveDelivery 只接受妥投结果：无人签收、地址错误、收件人拒收都进不来——
// 那三格没有可交付的东西，硬把它们造成交付正是 CONTEXT 明禁的（AT-TF-065/067）。
// 结果必须锚在同一次尝试上：地点、租户从该尝试取，业务时间从对象结果取。
func FormEffectiveDelivery(
	attempt FulfillmentAttempt,
	result DeliveryAttemptResult,
	spec EffectiveDeliverySpec,
) (EffectiveDelivery, error) {
	if !attempt.attempt.valid() || result.attempt != attempt.attempt {
		return EffectiveDelivery{}, ErrInvalidEffectiveDelivery
	}
	if !result.outcome.valid() {
		return EffectiveDelivery{}, ErrInvalidEffectiveDelivery
	}
	if !result.outcome.Succeeded() {
		return EffectiveDelivery{}, ErrNotAnEffectiveDelivery
	}
	if !spec.Method.valid() || !spec.Recipient.valid() || !spec.Proof.valid() || !spec.Version.valid() {
		return EffectiveDelivery{}, ErrInvalidEffectiveDelivery
	}
	return EffectiveDelivery{
		tenantID:   attempt.tenantID,
		object:     result.object,
		attempt:    attempt.attempt,
		place:      attempt.place,
		method:     spec.Method,
		recipient:  spec.Recipient,
		proof:      spec.Proof,
		version:    spec.Version,
		occurredAt: result.occurredAt,
	}, nil
}

func (delivery EffectiveDelivery) TenantID() TenantID {
	return delivery.tenantID
}

func (delivery EffectiveDelivery) Object() CarriedObjectReference {
	return delivery.object
}

func (delivery EffectiveDelivery) Attempt() AttemptReference {
	return delivery.attempt
}

func (delivery EffectiveDelivery) Place() AttemptPlaceReference {
	return delivery.place
}

func (delivery EffectiveDelivery) Method() DeliveryMethodReference {
	return delivery.method
}

func (delivery EffectiveDelivery) Recipient() ReceivingPartyReference {
	return delivery.recipient
}

func (delivery EffectiveDelivery) Proof() DeliveryProofReference {
	return delivery.proof
}

func (delivery EffectiveDelivery) Version() DeliveryResultVersion {
	return delivery.version
}

// OccurredAt 是实际交付发生的业务时间——它取自对象结果，消息与处理时间不能替代。
func (delivery EffectiveDelivery) OccurredAt() time.Time {
	return delivery.occurredAt
}

// Corrects 交回本版本更正的前一版本（若本版本由更正产生）。
func (delivery EffectiveDelivery) Corrects() (DeliveryResultVersion, bool) {
	if !delivery.corrects.valid() {
		return DeliveryResultVersion{}, false
	}
	return delivery.corrects, true
}

func (delivery EffectiveDelivery) CorrectedAt() (time.Time, bool) {
	if delivery.correctedAt.IsZero() {
		return time.Time{}, false
	}
	return delivery.correctedAt, true
}

// CorrectProof 依据新证据形成更正版本：保留原版本与原判断（值语义，接收者不动），
// 新版本回指被更正版本；沿用原版本号就是覆盖，构造期拒绝（CONTEXT「POD 被更正或失效
// 时保留原证据和判断，形成新版本」，AT-TF-072）。
func (delivery EffectiveDelivery) CorrectProof(
	proof DeliveryProofReference,
	version DeliveryResultVersion,
	correctedAt time.Time,
) (EffectiveDelivery, error) {
	if !delivery.version.valid() {
		return EffectiveDelivery{}, ErrInvalidEffectiveDelivery
	}
	if !proof.valid() || !version.valid() || correctedAt.IsZero() {
		return EffectiveDelivery{}, ErrInvalidEffectiveDelivery
	}
	if version == delivery.version {
		return EffectiveDelivery{}, ErrInvalidEffectiveDelivery
	}
	if correctedAt.Before(delivery.occurredAt) {
		return EffectiveDelivery{}, ErrInvalidEffectiveDelivery
	}
	corrected := delivery
	corrected.proof = proof
	corrected.corrects = delivery.version
	corrected.version = version
	corrected.correctedAt = correctedAt.UTC()
	return corrected, nil
}
