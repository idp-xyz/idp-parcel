package veconsume

import (
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

var (
	// ErrCustomerViewUndecided 表示客户视图派生编排停在自己的未决上（披露策略/视图库/
	// 标识签发调不通）。生产装配把它登记进 WithUndecidedSentinels：运维据此去查消费方
	// 等的那个依赖，而不是查传输。
	ErrCustomerViewUndecided = errors.New(
		"visibility exception customer view: derivation is undecided")
	// ErrCustomerViewHandoffPending 表示视图已发布但发布意图还没交出去。重投走已有
	// 结果路径会再交同一份意图（ADR-0043）。不要进 WithUndecidedSentinels——运维要查
	// 的是 outbox 下游，不是披露策略。
	ErrCustomerViewHandoffPending = errors.New(
		"visibility exception customer view: handoff is still pending")
	// ErrUnexpectedCustomerViewOutcome 表示编排交回了封闭集合以外的结果——含
	// NOT_ACCEPTED：消费链把受理门要的维度全部填满才调编排，未受理只能是装配或编程
	// 错误。静默入账等于替编排作判断，因此不留 default 兜底。
	ErrUnexpectedCustomerViewOutcome = errors.New(
		"visibility exception customer view: unexpected outcome")
)

// ViewConsumption 把客户视图派生编排的封闭结果译成消费门的两格：nil 入账、error 回滚。
//
// PUBLISHED / EXISTING_RESULT 是业务终局，入账收工；UNDECIDED 回滚重投。未交出去的
// 发布意图必须先拦住（与 Consumption 同一次序），否则消费门一 MarkProcessed，视图在、
// 下游 outbox 永久缺。
func ViewConsumption(result application.DeriveCustomerViewResult) error {
	if result.HandoffReference() != "" {
		return fmt.Errorf("%w: %s", ErrCustomerViewHandoffPending, result.HandoffReference())
	}
	switch result.Outcome() {
	case application.CustomerViewPublished, application.CustomerViewExistingResult:
		return nil
	case application.CustomerViewUndecided:
		return fmt.Errorf("%w: %s", ErrCustomerViewUndecided, result.UndecidedReason())
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedCustomerViewOutcome, result.Outcome())
	}
}
