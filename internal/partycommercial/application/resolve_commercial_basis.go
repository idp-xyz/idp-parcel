// Package application 在 party-commercial 领域内核与本上下文自有端口之上编排用例。它不
// 含持久化、事务或事件机制，那些仍阻断在 Bento 闸门之后（ADR-0017）。
package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ResolveCommercialBasisCommand 直接携带领域的解析键，不在应用层把它的字段抄一遍。解析
// 键由哪些维度构成是领域的事；抄一份到这里就成了第二处定义，而维度漏一个正是同一范围的
// `BUY` 请求被 `SELL` 结果回答的成因。
type ResolveCommercialBasisCommand struct {
	Key domain.ClosureResolutionKey
}

// ResolveCommercialBasisResult 在领域结论之外补一个判断时间。用例步骤 5 要求固定解析
// 标识、判断时间、锚点、版本、有效区间与当前修订，六项里只有判断时间不属于领域——它是
// 本次处理发生的时刻，与选择锚点是两回事：锚点决定选哪个版本，判断时间只说明这次判断
// 何时作出。压成一个会让重放看起来像新判断。
type ResolveCommercialBasisResult struct {
	closure  domain.CommercialClosure
	judgedAt time.Time
	fixed    ports.ResolutionSaveOutcome
}

func (result ResolveCommercialBasisResult) Closure() domain.CommercialClosure {
	return result.closure
}

func (result ResolveCommercialBasisResult) JudgedAt() time.Time {
	return result.judgedAt
}

// Fixed 交回解析库写入的三格代数。没有解析标识因而未落库时是零值；`已记录`与`内容冲突`
// 都不是 error（ADR-0031）。
func (result ResolveCommercialBasisResult) Fixed() ports.ResolutionSaveOutcome {
	return result.fixed
}

type ResolveCommercialBasisHandler struct {
	authority   ports.CommercialAuthorityView
	resolutions ports.CommercialResolutionStore
	clock       ports.Clock
}

func NewResolveCommercialBasisHandler(
	authority ports.CommercialAuthorityView,
	resolutions ports.CommercialResolutionStore,
	clock ports.Clock,
) *ResolveCommercialBasisHandler {
	return &ResolveCommercialBasisHandler{authority: authority, resolutions: resolutions, clock: clock}
}

// Handle 执行第一阶段：在同一个权威视图下解析整个引用闭包。它不形成接受、计价、冻结或
// 任何下游 `asOf`——第二阶段由调用方按本次采用的规则包驱动。
func (handler *ResolveCommercialBasisHandler) Handle(
	ctx context.Context,
	command ResolveCommercialBasisCommand,
) (ResolveCommercialBasisResult, error) {
	// 先判身份再读权威。顺序不能反：最小身份不成立时用例禁止查询候选，而一次已经发出的
	// 查询无法收回，它本身就回答了「这个范围里有没有对象」。
	if !command.Key.MinimumIdentityEstablished() {
		return handler.resultOf(domain.ResolveCommercialClosure(nil, command.Key, nil)), nil
	}

	view, err := handler.authority.LoadScope(ctx, command.Key.TenantID, command.Key.Scope)
	if err != nil {
		// 读不到权威形成解析未决，不向上抛技术错误。用例明写依赖超时与权威确认「无适用
		// 依据」是不同结果：合并会让一次读取失败被下游读成这个客户没有合同，进而当作
		// 拒绝理由。空视图是领域已定的「权威不可读」表达，不是这里图省事的绕法。
		return handler.resultOf(domain.ResolveCommercialClosure(nil, command.Key, nil)), nil
	}

	return handler.fix(ctx, domain.ResolveCommercialClosure(view, command.Key, nil))
}

// fix 在结论形成之后才读时钟，因此判断时间落在权威读取之后而非之前。有解析标识的
// 唯一结果必须写入本上下文的解析库（ADR-0027）：第二阶段只回指标识，写漏了就会
// found=false。没有标识的结局（未受理、未决、冲突、无依据）不落库——它们不是可回指的固定解析。
func (handler *ResolveCommercialBasisHandler) fix(
	ctx context.Context,
	closure domain.CommercialClosure,
) (ResolveCommercialBasisResult, error) {
	result := handler.resultOf(closure)
	if closure.ResolutionID().String() == "" {
		return result, nil
	}
	outcome, err := handler.resolutions.Save(ctx, closure)
	if err != nil {
		return ResolveCommercialBasisResult{}, fmt.Errorf("fix commercial resolution: %w", err)
	}
	switch outcome {
	case ports.ResolutionSaved, ports.ResolutionAlreadyRecorded, ports.ResolutionContentConflict:
		result.fixed = outcome
		return result, nil
	default:
		return ResolveCommercialBasisResult{}, fmt.Errorf("fix commercial resolution: unexpected save outcome %q", outcome)
	}
}

func (handler *ResolveCommercialBasisHandler) resultOf(closure domain.CommercialClosure) ResolveCommercialBasisResult {
	return ResolveCommercialBasisResult{closure: closure, judgedAt: handler.clock.Now()}
}
