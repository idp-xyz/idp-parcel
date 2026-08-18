// Package finalconsume 拥有「终局采用结果 → inbox 消费两格」这一份判断。它与
// adoptconsume 同形、不同类型：来源采用与终局采用各有封闭结果集合，合用一份映射等于
// 对「未交出的 handoff 必须回滚」立第二套口径。
//
// 它放在 parcel-shipment 的适配器层而不是 platform：它认得 psapplication 的封闭结果
// 集合，平台层对上下文类型一无所知这条不能破。
//
// 消费完成不等于形成终局。
package finalconsume

import (
	"errors"
	"fmt"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

var (
	// ErrFinalUndecided 表示终局编排停在自己的未决上。生产装配（CONS-FINAL-B）把它
	// 登记进 WithUndecidedSentinels：运维据此去查消费方等的那个依赖，而不是查传输。
	ErrFinalUndecided = errors.New(
		"parcel shipment final: parcel final adoption is undecided")
	// ErrFinalHandoffPending 表示终局记录已提交但发布意图还没交出去。它不是规则未决：
	// 重投走已有结果路径会再交同一份意图。不要进 WithUndecidedSentinels——运维要查的
	// 是 outbox 下游，不是终局规则目录。
	ErrFinalHandoffPending = errors.New(
		"parcel shipment final: parcel final handoff is still pending")
	// ErrUnexpectedFinalOutcome 表示应用层交回了封闭集合以外的结果。静默入账等于替
	// 编排作判断，因此不留 default 兜底。
	ErrUnexpectedFinalOutcome = errors.New(
		"parcel shipment final: unexpected final outcome")
)

// Consumption 把终局编排的封闭结果译成消费门的两格：nil 入账、error 回滚。
//
// FINAL_FORMED / FINAL_REDERIVED / EXISTING_RESULT / SOURCE_NOT_ADOPTED /
// SOURCE_CONFLICT / REQUEST_NOT_ACCEPTED 是业务终局（含负向），重试不会让另一份规则
// 或另一份委托长出来，入账收工。FINAL_UNDECIDED 是依赖缺口。应用层可能带着未交出去
// 的 FinalHandoffReference——那是技术续办，必须先拦住，否则消费门一 MarkProcessed，
// 终局行在、下游 outbox 永久缺。
func Consumption(result psapplication.FormParcelFinalResult) error {
	if result.FinalHandoffReference().String() != "" {
		return fmt.Errorf("%w: %s", ErrFinalHandoffPending, result.FinalHandoffReference())
	}
	switch result.Outcome() {
	case psapplication.ParcelFinalFormed,
		psapplication.ParcelFinalRederived,
		psapplication.ParcelFinalExistingResult,
		psapplication.FinalSourceNotAdopted,
		psapplication.FinalSourceConflict,
		psapplication.FinalRequestNotAccepted:
		return nil
	case psapplication.ParcelFinalUndecided:
		return fmt.Errorf("%w: %s", ErrFinalUndecided, result.UndecidedReason())
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedFinalOutcome, result.Outcome())
	}
}
