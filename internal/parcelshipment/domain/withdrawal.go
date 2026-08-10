package domain

import (
	"errors"
	"time"
)

var ErrInvalidWithdrawal = errors.New("parcel shipment: invalid shipment request withdrawal")

// 撤回的三项留痕分属两边：撤回授权规则属 party-commercial，实际请求方与原因属客户来源留痕。
// 本上下文只记引用——它不拥有客户代表的授权范围，也不判断某个原因码是否在目录里，那些是
// `PAR-COM-14` 尚未提供的实例参数。

type WithdrawalAuthorityReference struct{ requiredValue }

func NewWithdrawalAuthorityReference(value string) (WithdrawalAuthorityReference, error) {
	required, err := newRequiredValue("withdrawal authority reference", value)
	return WithdrawalAuthorityReference{required}, err
}

// WithdrawalRequesterReference 是实际提出撤回的主体：货主客户本人，或其当前有效授权代表。
// 它与授权引用分开保存，因为「谁提的」与「凭什么算数」在事后是两个问题。
type WithdrawalRequesterReference struct{ requiredValue }

func NewWithdrawalRequesterReference(value string) (WithdrawalRequesterReference, error) {
	required, err := newRequiredValue("withdrawal requester reference", value)
	return WithdrawalRequesterReference{required}, err
}

// WithdrawalReasonReference 是结构化原因，不是自由文本。用例要求撤回可按原因维度统计，
// 一句自由说明既统计不了，也无从判断它是否落在授权目录内。
type WithdrawalReasonReference struct{ requiredValue }

func NewWithdrawalReasonReference(value string) (WithdrawalReasonReference, error) {
	required, err := newRequiredValue("withdrawal reason reference", value)
	return WithdrawalReasonReference{required}, err
}

// Withdrawal 是客户终止整份待决服务请求的决定依据。它与拒绝分开保存：拒绝是运营企业不承担
// 这单，撤回是客户不要了。CONTEXT 明禁二者互相冒充，混成一种会让「我们不接」与「客户取消」
// 在事后分不开，而这两件事的客户回执、统计口径和后续路径都不同。
type Withdrawal struct {
	decisionID AcceptanceDecisionID
	authority  WithdrawalAuthorityReference
	requester  WithdrawalRequesterReference
	reason     WithdrawalReasonReference
	decidedAt  time.Time
}

func (withdrawal Withdrawal) DecisionID() AcceptanceDecisionID {
	return withdrawal.decisionID
}

func (withdrawal Withdrawal) Authority() WithdrawalAuthorityReference {
	return withdrawal.authority
}

func (withdrawal Withdrawal) Requester() WithdrawalRequesterReference {
	return withdrawal.requester
}

func (withdrawal Withdrawal) Reason() WithdrawalReasonReference {
	return withdrawal.reason
}

func (withdrawal Withdrawal) DecidedAt() time.Time {
	return withdrawal.decidedAt
}

func (withdrawal Withdrawal) formed() bool {
	return withdrawal.authority.valid() && withdrawal.requester.valid() &&
		withdrawal.reason.valid() && !withdrawal.decidedAt.IsZero()
}

type WithdrawalSpec struct {
	DecisionID AcceptanceDecisionID
	Authority  WithdrawalAuthorityReference
	Requester  WithdrawalRequesterReference
	Reason     WithdrawalReasonReference
	DecidedAt  time.Time
}

// Withdrawal 交回本委托上已经成立的撤回。未撤回时报告缺席而不是给零值——零值与「撤回了但
// 没带授权」在读的人眼里一样。
func (request ShipmentRequest) Withdrawal() (Withdrawal, bool) {
	return request.withdrawal, request.withdrawal.formed()
}

// WithdrawByCustomer 由客户或其当前有效授权代表终止整份仍为`已提交`的待决委托。
//
// 它与自动接受、主动拒绝竞争同一个不可覆盖决定边界，所以共用同一个 `decisionFormed` 闸门：
// 先到的决定获胜，后到的只能读取既有结果。分成两个闸门会让一份委托既被接受又被撤回。
//
// 但它不写进 `AcceptanceDecision`：撤回不是接受也不是拒绝，塞进那个槽位会让下游把一次客户
// 取消读成运营拒绝，而 CONTEXT 明禁两者互相冒充。因此撤回自带槽位，接受决定对已撤回委托
// 报告缺席——那是真话，这份委托确实没有接受决定。
//
// 任务到达`已停止`而不是`已完成`：完成只属于越过提交边界的接受或拒绝。已执行阶段和结果全部
// 保留，撤回不删除来源、提交版本或判断历史。
//
// 资金释放不在这里：那是编排按原业务关联向 settlement-accounting 发出的可补偿请求，释放失败
// 只续办原补偿，不回滚这个已经成立的撤回。
func (request ShipmentRequest) WithdrawByCustomer(spec WithdrawalSpec) (ShipmentRequest, error) {
	if request.decisionFormed {
		return ShipmentRequest{}, ErrDecisionAlreadyFormed
	}
	if request.state != ShipmentRequestSubmitted {
		return ShipmentRequest{}, ErrInvalidShipmentRequest
	}
	if !spec.DecisionID.valid() {
		return ShipmentRequest{}, ErrInvalidWithdrawal
	}

	withdrawal := Withdrawal{
		decisionID: spec.DecisionID,
		authority:  spec.Authority,
		requester:  spec.Requester,
		reason:     spec.Reason,
		decidedAt:  spec.DecidedAt.UTC(),
	}
	if !withdrawal.formed() {
		return ShipmentRequest{}, ErrInvalidWithdrawal
	}

	request.state = ShipmentRequestWithdrawn
	request.withdrawal = withdrawal
	request.decisionFormed = true
	request.acceptanceTask.state = AcceptanceTaskStopped
	request.acceptanceTask.waitingOn = ResumePathInvalid
	return request, nil
}
