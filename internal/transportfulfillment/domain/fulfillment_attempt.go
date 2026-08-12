package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidFulfillmentAttempt  = errors.New("transport fulfillment: invalid fulfillment attempt")
	ErrInvalidAttemptObjectResult = errors.New("transport fulfillment: invalid attempt object result")
	// ErrObjectOutsideAttempt 与形状错误分格：恢复动作是改指对象或补对象范围，不是补字段。
	ErrObjectOutsideAttempt = errors.New("transport fulfillment: object is outside the attempt scope")
)

// DispatchTaskReference 指名要求执行场外揽收或末端派送的揽派任务。任务表达需要完成什么；
// 尝试才是实际发生的到场（CONTEXT「揽派任务与履约尝试分离」）。
type DispatchTaskReference struct{ requiredValue }

func NewDispatchTaskReference(value string) (DispatchTaskReference, error) {
	required, err := newRequiredValue("dispatch task reference", value)
	return DispatchTaskReference{required}, err
}

// AttemptPlaceReference 指名一次尝试实际到场的地点。揽收与派送共用一次到场的语言，
// 所以它不叫「揽收地点」。
type AttemptPlaceReference struct{ requiredValue }

func NewAttemptPlaceReference(value string) (AttemptPlaceReference, error) {
	required, err := newRequiredValue("attempt place reference", value)
	return AttemptPlaceReference{required}, err
}

// AttemptEvidenceReference 指名支持这次到场与执行过程的证据集合。
type AttemptEvidenceReference struct{ requiredValue }

func NewAttemptEvidenceReference(value string) (AttemptEvidenceReference, error) {
	required, err := newRequiredValue("attempt evidence reference", value)
	return AttemptEvidenceReference{required}, err
}

// AttemptResultBasisReference 指名一个对象结果的依据——失败原因证据（客户不在、货物
// 未备好、包装不合格……）由它承载，而不是一段自由文本状态。
type AttemptResultBasisReference struct{ requiredValue }

func NewAttemptResultBasisReference(value string) (AttemptResultBasisReference, error) {
	required, err := newRequiredValue("attempt result basis reference", value)
	return AttemptResultBasisReference{required}, err
}

// FulfillmentAttemptSpec 是形成一次履约尝试所需的全部输入。RescheduledFrom 只在改约或
// 重派时给出，指向被接续的旧尝试。
type FulfillmentAttemptSpec struct {
	TenantID        TenantID
	Attempt         AttemptReference
	Task            DispatchTaskReference
	ExecutedBy      ExecutingPartyReference
	Place           AttemptPlaceReference
	PlannedFrom     time.Time
	PlannedTo       time.Time
	ArrivedAt       time.Time
	Objects         []CarriedObjectReference
	Evidence        AttemptEvidenceReference
	RescheduledFrom AttemptReference
}

// FulfillmentAttempt 是针对揽派任务实际发生的一次到场与执行过程（CONTEXT「履约尝试」）。
// 它保存计划窗口、实际到场、执行方、地点、对象范围与证据；逐对象结果另立
// AttemptObjectResult，任务汇总只能由对象结果派生。
//
// 改约或重派形成**新**尝试：RescheduledFrom 指向旧尝试，新尝试必须有自己的身份——
// 顶替旧身份在构造期就被拒绝，旧尝试因此永久保留而不是被重开或覆盖。
type FulfillmentAttempt struct {
	tenantID        TenantID
	attempt         AttemptReference
	task            DispatchTaskReference
	executedBy      ExecutingPartyReference
	place           AttemptPlaceReference
	plannedFrom     time.Time
	plannedTo       time.Time
	arrivedAt       time.Time
	objects         []CarriedObjectReference
	evidence        AttemptEvidenceReference
	rescheduledFrom AttemptReference
}

func FormFulfillmentAttempt(spec FulfillmentAttemptSpec) (FulfillmentAttempt, error) {
	if !spec.TenantID.valid() ||
		!spec.Attempt.valid() ||
		!spec.Task.valid() ||
		!spec.ExecutedBy.valid() ||
		!spec.Place.valid() ||
		!spec.Evidence.valid() ||
		spec.PlannedFrom.IsZero() ||
		spec.PlannedTo.IsZero() ||
		!spec.PlannedTo.After(spec.PlannedFrom) ||
		spec.ArrivedAt.IsZero() ||
		len(spec.Objects) == 0 {
		return FulfillmentAttempt{}, ErrInvalidFulfillmentAttempt
	}
	seen := make(map[CarriedObjectReference]struct{}, len(spec.Objects))
	for _, object := range spec.Objects {
		if !object.valid() {
			return FulfillmentAttempt{}, ErrInvalidFulfillmentAttempt
		}
		if _, exists := seen[object]; exists {
			return FulfillmentAttempt{}, ErrInvalidFulfillmentAttempt
		}
		seen[object] = struct{}{}
	}
	// 改约/重派沿用旧身份，就是用新到场重写旧尝试——CONTEXT 禁止的正是这件事。
	if spec.RescheduledFrom.valid() && spec.RescheduledFrom == spec.Attempt {
		return FulfillmentAttempt{}, ErrInvalidFulfillmentAttempt
	}
	return FulfillmentAttempt{
		tenantID:        spec.TenantID,
		attempt:         spec.Attempt,
		task:            spec.Task,
		executedBy:      spec.ExecutedBy,
		place:           spec.Place,
		plannedFrom:     spec.PlannedFrom.UTC(),
		plannedTo:       spec.PlannedTo.UTC(),
		arrivedAt:       spec.ArrivedAt.UTC(),
		objects:         append([]CarriedObjectReference(nil), spec.Objects...),
		evidence:        spec.Evidence,
		rescheduledFrom: spec.RescheduledFrom,
	}, nil
}

func (attempt FulfillmentAttempt) TenantID() TenantID {
	return attempt.tenantID
}

func (attempt FulfillmentAttempt) Attempt() AttemptReference {
	return attempt.attempt
}

func (attempt FulfillmentAttempt) Task() DispatchTaskReference {
	return attempt.task
}

func (attempt FulfillmentAttempt) ExecutedBy() ExecutingPartyReference {
	return attempt.executedBy
}

func (attempt FulfillmentAttempt) Place() AttemptPlaceReference {
	return attempt.place
}

func (attempt FulfillmentAttempt) PlannedFrom() time.Time {
	return attempt.plannedFrom
}

func (attempt FulfillmentAttempt) PlannedTo() time.Time {
	return attempt.plannedTo
}

// ArrivedAt 是实际到场的业务时间；对象结果不得早于它。
func (attempt FulfillmentAttempt) ArrivedAt() time.Time {
	return attempt.arrivedAt
}

func (attempt FulfillmentAttempt) Objects() []CarriedObjectReference {
	return append([]CarriedObjectReference(nil), attempt.objects...)
}

func (attempt FulfillmentAttempt) Covers(object CarriedObjectReference) bool {
	for _, covered := range attempt.objects {
		if covered == object {
			return true
		}
	}
	return false
}

func (attempt FulfillmentAttempt) Evidence() AttemptEvidenceReference {
	return attempt.evidence
}

// RescheduledFrom 交回改约/重派所接续的旧尝试（若有）。
func (attempt FulfillmentAttempt) RescheduledFrom() (AttemptReference, bool) {
	if !attempt.rescheduledFrom.valid() {
		return AttemptReference{}, false
	}
	return attempt.rescheduledFrom, true
}

// AttemptObjectOutcome 是一次尝试中单个载运对象结果的封闭集合。取值随实际建模的揽派
// 场景增长；今天只有场外揽收一个生产者，所以成功一格叫「揽收到手」。
type AttemptObjectOutcome uint8

const (
	AttemptObjectOutcomeInvalid AttemptObjectOutcome = iota
	ObjectPickedUp
	CustomerAbsent
	GoodsNotReady
	PackagingUnacceptable
)

func (outcome AttemptObjectOutcome) valid() bool {
	return outcome >= ObjectPickedUp && outcome <= PackagingUnacceptable
}

// Succeeded 只在揽收到手时为真。失败家族刻意不给「哪一种失败」以外的语义：是否重试、
// 改约、终止或退运由适用产品、合同和授权处置规则决定，不由结果自己宣布。
func (outcome AttemptObjectOutcome) Succeeded() bool {
	return outcome == ObjectPickedUp
}

func (outcome AttemptObjectOutcome) Failed() bool {
	return outcome.valid() && !outcome.Succeeded()
}

func (outcome AttemptObjectOutcome) String() string {
	switch outcome {
	case ObjectPickedUp:
		return "PICKED_UP"
	case CustomerAbsent:
		return "CUSTOMER_ABSENT"
	case GoodsNotReady:
		return "GOODS_NOT_READY"
	case PackagingUnacceptable:
		return "PACKAGING_UNACCEPTABLE"
	default:
		return ""
	}
}

// AttemptObjectResult 是一次尝试中单个载运对象的结果。逐对象成立：共享一次到场的对象
// 分别记成功或失败，任务汇总只能由这些结果派生。
//
// 它结构上不携带任何运输控制或履约段引用——「失败结果不制造实际履约段」不靠纪律靠形状：
// 建立履约参与关系的控制依据只在 OffsitePickup（有效收寄）上，失败结果里没有字段可以
// 冒充它。一次失败也不自动结束任务、不自动创建退运：这里没有任务终局或退运字段，后续
// 处置由适用产品、合同和授权规则另行决定。
type AttemptObjectResult struct {
	attempt    AttemptReference
	object     CarriedObjectReference
	outcome    AttemptObjectOutcome
	basis      AttemptResultBasisReference
	occurredAt time.Time
}

// FormAttemptObjectResult 依附一次已形成的尝试记录单个对象的结果。
//
// 失败必须带依据——没有失败原因的失败与数据丢失无从分辨；成功不得携带失败依据——揽收
// 的证据与控制依据在 OffsitePickup 上，往成功结果里塞一份「依据」只会让两处口径打架。
// 结果时间不得早于实际到场：到场之前没有可记的执行结果。
func FormAttemptObjectResult(
	attempt FulfillmentAttempt,
	object CarriedObjectReference,
	outcome AttemptObjectOutcome,
	basis AttemptResultBasisReference,
	occurredAt time.Time,
) (AttemptObjectResult, error) {
	if !attempt.attempt.valid() || !object.valid() || !outcome.valid() || occurredAt.IsZero() {
		return AttemptObjectResult{}, ErrInvalidAttemptObjectResult
	}
	if occurredAt.Before(attempt.arrivedAt) {
		return AttemptObjectResult{}, ErrInvalidAttemptObjectResult
	}
	if !attempt.Covers(object) {
		return AttemptObjectResult{}, ErrObjectOutsideAttempt
	}
	if outcome.Failed() && !basis.valid() {
		return AttemptObjectResult{}, ErrInvalidAttemptObjectResult
	}
	if outcome.Succeeded() && basis.valid() {
		return AttemptObjectResult{}, ErrInvalidAttemptObjectResult
	}
	return AttemptObjectResult{
		attempt:    attempt.attempt,
		object:     object,
		outcome:    outcome,
		basis:      basis,
		occurredAt: occurredAt.UTC(),
	}, nil
}

func (result AttemptObjectResult) Attempt() AttemptReference {
	return result.attempt
}

func (result AttemptObjectResult) Object() CarriedObjectReference {
	return result.object
}

func (result AttemptObjectResult) Outcome() AttemptObjectOutcome {
	return result.outcome
}

// Basis 在失败时交回失败原因依据；成功没有它。
func (result AttemptObjectResult) Basis() (AttemptResultBasisReference, bool) {
	if !result.basis.valid() {
		return AttemptResultBasisReference{}, false
	}
	return result.basis, true
}

func (result AttemptObjectResult) OccurredAt() time.Time {
	return result.occurredAt
}
