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
		return handler.resultOf(domain.ValidateClosureBeforeDecision(nil, prior, nil)), nil
	}

	return handler.resultOf(domain.ValidateClosureBeforeDecision(view, prior, nil)), nil
}

// loadPrior 按标识取回原解析并核对归属。取不到时按调用方的**恢复动作**分格，不按本上下文
// 观察到的失败原因分格（ADR-0029）：
//
//   - 读不回 → `解析未决`，调用方重试同一次调用。
//   - 查无此解析、范围不符 → 同一个`依据未解析`，调用方回第一阶段重新解析。
//
// 后两者合并是本函数的要点。两者的恢复动作相同，分开就等于回答了「这份解析存不存在」——
// 拒绝一次越权重校验并不等于没泄露，被拒的那一次同样告诉了对方这个标识是真的，一串标识挨个
// 问即可枚举同租户下其他客户账户的解析（`AT-PC-028`）。
//
// `依据未解析`不报`无适用依据`：那是权威说了这个范围没有适用对象，可以拿去拒单；而这里说的是
// 找不到调用方指名的那次解析，混起来会让一次查不到变成一个客户的拒绝理由。
func (handler *ValidateCommercialBasisHandler) loadPrior(
	ctx context.Context,
	command ValidateCommercialBasisCommand,
) (domain.CommercialClosure, domain.ResolutionOutcome, domain.ResolutionReason, bool) {
	// 身份或标识缺失时不查询。这一支在任何查询发生之前短路，答案不依赖解析存不存在，因此
	// 不构成预言机，也就不并入上面那两格。
	if command.Caller.TenantID.String() == "" ||
		command.Caller.CustomerAccountID.String() == "" ||
		command.Resolution.String() == "" {
		return domain.CommercialClosure{}, domain.InputNotAccepted, domain.ResolutionReasonNone, false
	}

	prior, found, err := handler.resolutions.LoadResolution(ctx, command.Caller.TenantID, command.Resolution)
	if err != nil {
		return domain.CommercialClosure{}, domain.ResolutionPending, domain.AuthorityUnreadable, false
	}
	if !found {
		return domain.CommercialClosure{}, domain.BasisNotResolved, domain.ResolutionReasonNone, false
	}

	key := prior.ResolutionKey()
	if key.TenantID != command.Caller.TenantID || key.CustomerAccountID != command.Caller.CustomerAccountID {
		return domain.CommercialClosure{}, domain.BasisNotResolved, domain.ResolutionReasonNone, false
	}
	return prior, domain.ResolutionOutcomeInvalid, domain.ResolutionReasonNone, true
}

func (handler *ValidateCommercialBasisHandler) resultOf(closure domain.CommercialClosure) ResolveCommercialBasisResult {
	return ResolveCommercialBasisResult{closure: closure, judgedAt: handler.clock.Now()}
}
