package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ValidateCommercialBasisCommand 只回指原解析的标识，既不收调用方带回来的闭包，也不另收一份
// 解析键。用例步骤 8 校验的是「这一份解析是否仍然相容」，而键随结果走；让调用方另给一份，一次
// 「校验」就能拿另一个范围的视图去证明这份解析仍然成立。
//
// 键因此从头到尾没离开过本上下文——调用方给的只是一个标识，闭包由本上下文按标识取回
// （ADR-0027）。标识不是能力凭证，取回后仍要与调用方身份比对。
type ValidateCommercialBasisCommand struct {
	Caller     CallerScope
	Resolution domain.ResolutionID
}

type ValidateCommercialBasisHandler struct {
	resolutions ports.CommercialResolutionStore
	authority   ports.CommercialAuthorityView
	clock       ports.Clock
}

func NewValidateCommercialBasisHandler(
	resolutions ports.CommercialResolutionStore,
	authority ports.CommercialAuthorityView,
	clock ports.Clock,
) *ValidateCommercialBasisHandler {
	return &ValidateCommercialBasisHandler{resolutions: resolutions, authority: authority, clock: clock}
}

// Handle 执行用例步骤 8：在调用方提交业务决定之前，按原查询重解一次，看这份解析是否仍然成立。
//
// 它与第一阶段分成两个编排而不是一个带开关的方法：两者由不同时刻的不同事件触发——一次在消费方
// 开始判断时，一次在它即将提交决定时——而结果语义也不同，重校验能给出`已失效`，首次解析给不出。
//
// 交回的是解析结果本身，与第一阶段共用一个结果类型：重校验的产物就是一份解析结果，另立一个
// 形状相同的类型只会让调用方多一次转换。其中的判断时间说的是这一轮确认发生在何时，而不是原
// 解析何时作出。
func (handler *ValidateCommercialBasisHandler) Handle(
	ctx context.Context,
	command ValidateCommercialBasisCommand,
) (ResolveCommercialBasisResult, error) {
	prior, outcome, reason, ok := handler.loadPrior(ctx, command)
	if !ok {
		return handler.resultOf(domain.PriorResolutionUnavailable(outcome, reason)), nil
	}

	key := prior.ResolutionKey()
	view, err := handler.authority.LoadScope(ctx, key.TenantID, key.Scope)
	if err != nil {
		// 与第一阶段同一条分界：读不到权威是`解析未决`，不是技术错误，也不是`已失效`。空视图
		// 是领域已定的「权威不可读」表达。
		return handler.resultOf(domain.ValidateClosureBeforeDecision(nil, prior)), nil
	}

	return handler.resultOf(domain.ValidateClosureBeforeDecision(view, prior)), nil
}

// loadPrior 按标识取回原解析并核对归属。三种取不到分别落在不同的第一阶段结果取值上：读不回
// 是`解析未决`，查无此解析同样是`解析未决`（本上下文说不出它是不存在还是不属于你，说得出就
// 泄露了），范围不符是`输入未受理`。
//
// 「查无此解析」不报`无适用依据`：那是权威说了这个范围没有适用对象，与「我找不到你说的那次
// 解析」不是一回事，混起来会让调用方拿一次查不到当作拒单理由。
func (handler *ValidateCommercialBasisHandler) loadPrior(
	ctx context.Context,
	command ValidateCommercialBasisCommand,
) (domain.CommercialClosure, domain.ResolutionOutcome, domain.ResolutionReason, bool) {
	if command.Caller.TenantID.String() == "" ||
		command.Caller.CustomerAccountID.String() == "" ||
		command.Resolution.String() == "" {
		return domain.CommercialClosure{}, domain.InputNotAccepted, domain.ResolutionReasonNone, false
	}

	prior, found, err := handler.resolutions.LoadResolution(ctx, command.Caller.TenantID, command.Resolution)
	if err != nil || !found {
		return domain.CommercialClosure{}, domain.ResolutionPending, domain.AuthorityUnreadable, false
	}

	key := prior.ResolutionKey()
	if key.TenantID != command.Caller.TenantID || key.CustomerAccountID != command.Caller.CustomerAccountID {
		return domain.CommercialClosure{}, domain.InputNotAccepted, domain.ResolutionReasonNone, false
	}
	return prior, domain.ResolutionOutcomeInvalid, domain.ResolutionReasonNone, true
}

func (handler *ValidateCommercialBasisHandler) resultOf(closure domain.CommercialClosure) ResolveCommercialBasisResult {
	return ResolveCommercialBasisResult{closure: closure, judgedAt: handler.clock.Now()}
}
