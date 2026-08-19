// Package veconsume 拥有「投影派生结果 → inbox 消费两格」这一份判断。它放在
// visibility-exception 的适配器层而不是 platform：它认得 DeriveProjectionResult
// 的封闭集合，平台层对上下文类型一无所知这条不能破。
package veconsume

import (
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

var (
	// ErrProjectionUndecided 表示派生编排停在自己的未决上。生产装配把它登记进
	// WithUndecidedSentinels：运维据此去查消费方等的那个依赖，而不是查传输。
	ErrProjectionUndecided = errors.New(
		"visibility exception projection: tracking projection derivation is undecided")
	// ErrProjectionHandoffPending 表示投影已提交但发布意图还没交出去。它不是映射
	// 未配置：重投走已有结果路径会再交同一份意图。不要进 WithUndecidedSentinels——
	// 运维要查的是 outbox 下游，不是里程碑映射目录。
	ErrProjectionHandoffPending = errors.New(
		"visibility exception projection: tracking projection handoff is still pending")
	// ErrUnexpectedProjectionOutcome 表示应用层交回了封闭集合以外的结果。静默入账
	// 等于替编排作判断，因此不留 default 兜底。
	ErrUnexpectedProjectionOutcome = errors.New(
		"visibility exception projection: unexpected projection outcome")
)

// Consumption 把派生编排的封闭结果译成消费门的两格：nil 入账、error 回滚。
//
// 消费完成不等于形成标准里程碑。PROJECTION_DERIVED / EXISTING_RESULT /
// SOURCE_CONFLICT / NOT_ACCEPTED 是业务终局（含负向与未归类），重试不会让映射目录
// 或另一份源事实长出来，入账收工。映射未配置走的是 PROJECTION_DERIVED + 未归类，
// 禁止折成 MAPPING_VIEW_UNAVAILABLE。
//
// UNDECIDED 含事实库/映射视图/投影库调不通。应用层可能带着未交出去的
// HandoffReference——那是技术续办，必须先拦住，否则消费门一 MarkProcessed，投影
// 行在、下游 outbox 永久缺。
func Consumption(result application.DeriveProjectionResult) error {
	if result.HandoffReference() != "" {
		return fmt.Errorf("%w: %s", ErrProjectionHandoffPending, result.HandoffReference())
	}
	switch result.Outcome() {
	case application.ProjectionDerived,
		application.FactExistingResult,
		application.FactSourceConflict,
		application.FactNotAccepted:
		return nil
	case application.DeriveUndecided:
		return fmt.Errorf("%w: %s", ErrProjectionUndecided, result.UndecidedReason())
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedProjectionOutcome, result.Outcome())
	}
}
