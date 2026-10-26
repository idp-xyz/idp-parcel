package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidDeliveryAttemptResult = errors.New("transport fulfillment: invalid delivery attempt result")
	// ErrNotAnEffectiveDelivery 是 CONTEXT 交付节的硬句本体：「无人签收、地址错误、收件人
	// 拒收或证据不足不构成有效交付」（UC-TF-006 同句另列「包装不合格」——那是揽收侧失败
	// 家族的词，本类型的封闭集合不含它）。待确认落在「证据不足」那一种上。它与形状错误分格
	// ——恢复动作不是补字段，而是承认这个对象本次没有形成有效交付。
	ErrNotAnEffectiveDelivery   = errors.New("transport fulfillment: this outcome does not constitute an effective delivery")
	ErrInvalidEffectiveDelivery = errors.New("transport fulfillment: invalid effective delivery")
)

// DeliveryObjectOutcome 是一次派送尝试中单个载运对象结果的封闭集合（UC-TF-006：成功、
// 失败、拒收、待确认各自成立，任务汇总只能派生）。新取值只往末尾加，理由同 AttemptObjectOutcome。
type DeliveryObjectOutcome uint8

const (
	DeliveryObjectOutcomeInvalid DeliveryObjectOutcome = iota
	ObjectDelivered
	DeliveryRefused
	NoOneToReceive
	WrongAddress
	// DeliveryPendingConfirmation 是证据不足或来源冲突（UC-TF-006「尝试待确认」，AT-TF-070）：带全部来源与冲突或
	// 缺口，登不出有效交付、不转移控制，也不是失败。
	DeliveryPendingConfirmation
)

func (outcome DeliveryObjectOutcome) valid() bool {
	return outcome >= ObjectDelivered && outcome <= DeliveryPendingConfirmation
}

// Succeeded 只在妥投时为真。拒收刻意不带「已退回」语义：拒收只结束本次尝试的相应对象
// 结果，不自动等于已签收、已退回或服务终止（CONTEXT）。
func (outcome DeliveryObjectOutcome) Succeeded() bool {
	return outcome == ObjectDelivered
}

// Failed 是本次尝试对该对象的确定未成功：无人签收、地址错误、收件人拒收。待确认不在其内，理由同
// AttemptObjectOutcome.Failed。
func (outcome DeliveryObjectOutcome) Failed() bool {
	return outcome.valid() && !outcome.Succeeded() && !outcome.PendingConfirmation()
}

// PendingConfirmation 只在待确认时为真。
func (outcome DeliveryObjectOutcome) PendingConfirmation() bool {
	return outcome == DeliveryPendingConfirmation
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
	case DeliveryPendingConfirmation:
		return "PENDING_CONFIRMATION"
	default:
		return ""
	}
}

// DeliveryAttemptResult 是一次派送尝试中单个载运对象的结果，纪律与揽收侧的
// AttemptObjectResult 同一条：逐对象成立、失败必须带原因依据、妥投不得携带失败依据、待确认
// 带来源与冲突或缺口；结构上没有控制或履约段字段——妥投本身也不转移控制，控制转移只随有效
// 交付成立。
type DeliveryAttemptResult struct {
	attempt    AttemptReference
	object     CarriedObjectReference
	outcome    DeliveryObjectOutcome
	basis      AttemptResultBasisReference
	source     AttemptResultSourceReference
	occurredAt time.Time
}

// FormDeliveryAttemptResult 记单个对象的确定结果（妥投、无人签收、地址错误、收件人拒收）。待确认不从这里进，
// 理由同 FormAttemptObjectResult；它走 FormPendingDeliveryAttemptResult。
func FormDeliveryAttemptResult(
	attempt FulfillmentAttempt,
	object CarriedObjectReference,
	outcome DeliveryObjectOutcome,
	basis AttemptResultBasisReference,
	occurredAt time.Time,
) (DeliveryAttemptResult, error) {
	if !outcome.valid() || outcome.PendingConfirmation() {
		return DeliveryAttemptResult{}, ErrInvalidDeliveryAttemptResult
	}
	if err := deliveryAttemptResultInScope(attempt, object, occurredAt); err != nil {
		return DeliveryAttemptResult{}, err
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

// FormPendingDeliveryAttemptResult 记一个对象在这次派送尝试里的待确认：证据不足或来源冲突（UC-TF-006 步骤 4
// 「证据不足保持待确认」）。全部来源与冲突或缺口都必备；这一格以后不改写，理由同 FormPendingAttemptObjectResult。
// 冲突时不按外部最后状态码裁决：一份后到的 `DELIVERED` 只是来源之一，不把这一格改成妥投。
func FormPendingDeliveryAttemptResult(
	attempt FulfillmentAttempt,
	object CarriedObjectReference,
	source AttemptResultSourceReference,
	conflict AttemptResultBasisReference,
	occurredAt time.Time,
) (DeliveryAttemptResult, error) {
	if err := deliveryAttemptResultInScope(attempt, object, occurredAt); err != nil {
		return DeliveryAttemptResult{}, err
	}
	if !source.valid() || !conflict.valid() {
		return DeliveryAttemptResult{}, ErrInvalidDeliveryAttemptResult
	}
	return DeliveryAttemptResult{
		attempt:    attempt.attempt,
		object:     object,
		outcome:    DeliveryPendingConfirmation,
		basis:      conflict,
		source:     source,
		occurredAt: occurredAt.UTC(),
	}, nil
}

// deliveryAttemptResultInScope 走揽收那一侧同一道锚定门，形状错误换成派送一侧的错误值；范围外照旧单独一格。
func deliveryAttemptResultInScope(attempt FulfillmentAttempt, object CarriedObjectReference, occurredAt time.Time) error {
	err := attemptObjectResultInScope(attempt, object, occurredAt)
	if err == nil || errors.Is(err, ErrObjectOutsideAttempt) {
		return err
	}
	return ErrInvalidDeliveryAttemptResult
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

// Basis 在失败或拒收时交回原因依据，在待确认时交回冲突或缺口；妥投没有它。
func (result DeliveryAttemptResult) Basis() (AttemptResultBasisReference, bool) {
	if !result.basis.valid() {
		return AttemptResultBasisReference{}, false
	}
	return result.basis, true
}

// Source 只在待确认时交回全部来源。
func (result DeliveryAttemptResult) Source() (AttemptResultSourceReference, bool) {
	if !result.source.valid() {
		return AttemptResultSourceReference{}, false
	}
	return result.source, true
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
	// gateReading 是登记时随记的交付门禁读数（ADR-0213 决定五）。FormEffectiveDelivery 形成的那一份还没有它：读数在写
	// 事务之前问，由编排经 WithGateReading 带上；在册的每一版都有。
	gateReading DeliveryGateReading
}

// FormEffectiveDelivery 只接受妥投结果：无人签收、地址错误、收件人拒收与待确认都进不来——
// 前三格没有可交付的东西，待确认是证据不足或来源冲突，硬把它们造成交付正是 CONTEXT 明禁的
// （「证据不足不构成有效交付」，AT-TF-065/067/070）。
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
	if !proof.valid() || !version.valid() || AtRegistryPrecision(correctedAt).IsZero() {
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
