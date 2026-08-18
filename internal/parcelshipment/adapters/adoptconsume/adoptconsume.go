// Package adoptconsume 拥有「来源采用结果 → inbox 消费两格」这一份判断。节点收寄与
// 场外揽收两条消费链共用它：两条链各抄一份等于对「未交出的 handoff 必须回滚」立第二
// 套口径，而这条判断改错一次就会留下 adoption 行在、下游 outbox 永久缺的账。
//
// 它放在 parcel-shipment 的适配器层而不是 platform：它认得 psapplication 的封闭结果
// 集合，平台层对上下文类型一无所知这条不能破。
package adoptconsume

import (
	"errors"
	"fmt"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

var (
	// ErrAdoptionUndecided 表示采用编排停在自己的未决上。生产装配把它登记进
	// WithUndecidedSentinels：运维据此去查消费方等的那个依赖，而不是查传输。
	ErrAdoptionUndecided = errors.New(
		"parcel shipment adoption: network intake adoption is undecided")
	// ErrAdoptionHandoffPending 表示采用记录已提交但发布意图还没交出去。它不是资格
	// 未决：重投走已有结果路径会再交同一份意图。不要进 WithUndecidedSentinels——运维
	// 要查的是 outbox 下游，不是商业资格目录。
	ErrAdoptionHandoffPending = errors.New(
		"parcel shipment adoption: network intake handoff is still pending")
	// ErrUnexpectedAdoptionOutcome 表示应用层交回了封闭集合以外的结果。静默入账等于
	// 替编排作判断，因此不留 default 兜底。
	ErrUnexpectedAdoptionOutcome = errors.New(
		"parcel shipment adoption: unexpected adoption outcome")
)

// Consumption 把采用编排的封闭结果译成消费门的两格：nil 入账、error 回滚。
//
// 消费完成不等于形成采用。REQUEST_NOT_ACCEPTED / SOURCE_CONFLICT / NOT_APPLICABLE
// 是业务负向终局，重试不会让另一份委托或另一份资格长出来，入账收工。COMMITMENT_FORMED、
// SOURCE_NOT_ADOPTED、EXISTING_RESULT 在应用层可能带着未交出去的 handoff 引用——那是
// 技术续办，必须先拦住，否则消费门一 MarkProcessed，adoption 行在、下游 outbox 永久缺。
func Consumption(result psapplication.AdoptNetworkIntakeResult) error {
	if result.IntakeHandoffReference().String() != "" {
		return fmt.Errorf("%w: %s", ErrAdoptionHandoffPending, result.IntakeHandoffReference())
	}
	switch result.Outcome() {
	case psapplication.IntakeCommitmentFormed,
		psapplication.IntakeExistingResult,
		psapplication.IntakeSourceNotAdopted,
		psapplication.IntakeSourceConflict,
		psapplication.IntakeNotApplicable,
		psapplication.IntakeRequestNotAccepted:
		return nil
	case psapplication.IntakeEligibilityUndecided:
		return fmt.Errorf("%w: %s", ErrAdoptionUndecided, result.UndecidedReason())
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedAdoptionOutcome, result.Outcome())
	}
}
