package domain

import (
	"errors"
	"time"
)

var (
	// ErrAttemptResultNotPending 说明被更正的那一格不是待确认：本更正只回指待确认那一版。确定结果不被更正成别的
	// 确定结果——那是另一种更正（对已确定判断的更正），不在这条路上。它与形状错误分格，恢复动作不是补字段，
	// 而是承认这一格已经是确定结果。
	ErrAttemptResultNotPending = errors.New("transport fulfillment: the attempt result is not pending confirmation")
	// ErrInvalidAttemptResultCorrection 是更正自身的形状错误：没有所凭的后补来源、更正成待确认、待确认属于另一次尝试，
	// 或时序不成立。确定结果自己的形状错误仍是各侧的 ErrInvalidAttemptObjectResult / ErrInvalidDeliveryAttemptResult。
	ErrInvalidAttemptResultCorrection = errors.New("transport fulfillment: invalid attempt result correction")
)

// AttemptObjectResultCorrection 是同一次上门的后补来源把一格待确认的揽收结果更正成确定结果的那一次判断（TF CONTEXT
// 「揽收、派送与交付」）：回指待确认那一版、带新的确定结果、所凭的后补来源与更正时刻。
//
// 待确认那一版原样留着（Pending），不被改写；确定结果（Effective）与首次提交的确定结果同一个类型、过同一道构造门，
// 下游各消费方（失败尝试费、任务汇总）读它就是读一格确定结果，不需要认识「更正」。后补来源只留在更正上：确定结果不带来源
// （AttemptObjectResult 头注），否则同一格事实在结果与更正上有两处口径。
//
// 更正成揽收到手并不在这里形成场外揽收：控制依据属 OffsitePickup，由编排经对象级揽收登记那一条路形成。
type AttemptObjectResultCorrection struct {
	pending     AttemptObjectResult
	effective   AttemptObjectResult
	source      AttemptResultSourceReference
	correctedAt time.Time
}

// CorrectPendingAttemptObjectResult 回指一格待确认的揽收结果，形成确定结果。
//
// 确定结果走既有的 FormAttemptObjectResult：失败与拒收必带依据、揽收到手不带依据、结果不早于到场、对象在尝试范围内——
// 更正不另立一套门，也就不会放进首次提交进不来的形状。所凭的后补来源必备；业务时间可以早于待确认那一版（伙伴迟到的
// 签收业务上先发生、后到达，UC-TF-002「按业务发生时间采用」），但更正这一判断本身不早于它所更正的待确认、也不早于它说的
// 那件事。
func CorrectPendingAttemptObjectResult(
	attempt FulfillmentAttempt,
	pending AttemptObjectResult,
	outcome AttemptObjectOutcome,
	basis AttemptResultBasisReference,
	source AttemptResultSourceReference,
	occurredAt time.Time,
	correctedAt time.Time,
) (AttemptObjectResultCorrection, error) {
	if !attempt.attempt.valid() || pending.attempt != attempt.attempt {
		return AttemptObjectResultCorrection{}, ErrInvalidAttemptResultCorrection
	}
	if !pending.outcome.PendingConfirmation() {
		return AttemptObjectResultCorrection{}, ErrAttemptResultNotPending
	}
	if !outcome.valid() || outcome.PendingConfirmation() || !source.valid() ||
		!correctionNotBefore(correctedAt, pending.occurredAt, occurredAt) {
		return AttemptObjectResultCorrection{}, ErrInvalidAttemptResultCorrection
	}
	effective, err := FormAttemptObjectResult(attempt, pending.object, outcome, basis, occurredAt)
	if err != nil {
		return AttemptObjectResultCorrection{}, err
	}
	return AttemptObjectResultCorrection{
		pending:     pending,
		effective:   effective,
		source:      source,
		correctedAt: correctedAt.UTC(),
	}, nil
}

// correctionNotBefore 判更正的判断时刻不早于它所更正的待确认、也不早于它说的那件事。时刻先折到登记册精度再比，
// 与库里读回的一样。
func correctionNotBefore(correctedAt time.Time, earlier ...time.Time) bool {
	if AtRegistryPrecision(correctedAt).IsZero() {
		return false
	}
	for _, moment := range earlier {
		if afterOnRegistry(moment, correctedAt) {
			return false
		}
	}
	return true
}

// Pending 交回被更正的待确认那一版，原样。
func (correction AttemptObjectResultCorrection) Pending() AttemptObjectResult {
	return correction.pending
}

// Effective 交回更正出来的确定结果：同一对象在同一次尝试里此后的当前有效结果。
func (correction AttemptObjectResultCorrection) Effective() AttemptObjectResult {
	return correction.effective
}

// Source 交回更正所凭的后补来源。
func (correction AttemptObjectResultCorrection) Source() AttemptResultSourceReference {
	return correction.source
}

func (correction AttemptObjectResultCorrection) CorrectedAt() time.Time {
	return correction.correctedAt
}

// DeliveryAttemptResultCorrection 是派送一侧的同一件事：待确认的派送结果凭同一次上门的后补来源（迟到的签收、复核后认定的
// 地址）更正成确定结果。纪律同 AttemptObjectResultCorrection。更正成妥投之后，同一次尝试里的那件对象才登得出有效交付；
// 妥投本身仍不转移控制（DeliveryAttemptResult 头注），有效交付自己的门不变。
type DeliveryAttemptResultCorrection struct {
	pending     DeliveryAttemptResult
	effective   DeliveryAttemptResult
	source      AttemptResultSourceReference
	correctedAt time.Time
}

// CorrectPendingDeliveryAttemptResult 回指一格待确认的派送结果，形成确定结果；门同 CorrectPendingAttemptObjectResult，
// 确定结果走 FormDeliveryAttemptResult。
func CorrectPendingDeliveryAttemptResult(
	attempt FulfillmentAttempt,
	pending DeliveryAttemptResult,
	outcome DeliveryObjectOutcome,
	basis AttemptResultBasisReference,
	source AttemptResultSourceReference,
	occurredAt time.Time,
	correctedAt time.Time,
) (DeliveryAttemptResultCorrection, error) {
	if !attempt.attempt.valid() || pending.attempt != attempt.attempt {
		return DeliveryAttemptResultCorrection{}, ErrInvalidAttemptResultCorrection
	}
	if !pending.outcome.PendingConfirmation() {
		return DeliveryAttemptResultCorrection{}, ErrAttemptResultNotPending
	}
	if !outcome.valid() || outcome.PendingConfirmation() || !source.valid() ||
		!correctionNotBefore(correctedAt, pending.occurredAt, occurredAt) {
		return DeliveryAttemptResultCorrection{}, ErrInvalidAttemptResultCorrection
	}
	effective, err := FormDeliveryAttemptResult(attempt, pending.object, outcome, basis, occurredAt)
	if err != nil {
		return DeliveryAttemptResultCorrection{}, err
	}
	return DeliveryAttemptResultCorrection{
		pending:     pending,
		effective:   effective,
		source:      source,
		correctedAt: correctedAt.UTC(),
	}, nil
}

func (correction DeliveryAttemptResultCorrection) Pending() DeliveryAttemptResult {
	return correction.pending
}

func (correction DeliveryAttemptResultCorrection) Effective() DeliveryAttemptResult {
	return correction.effective
}

func (correction DeliveryAttemptResultCorrection) Source() AttemptResultSourceReference {
	return correction.source
}

func (correction DeliveryAttemptResultCorrection) CorrectedAt() time.Time {
	return correction.correctedAt
}
