package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ValidateCommercialBasisCommand 只携带原解析结果，不另收一份解析键。用例步骤 8 校验的是
// 「这一份解析是否仍然相容」，而键随结果走；让调用方另给一份，一次「校验」就能拿另一个范围
// 的视图去证明这份解析仍然成立。
type ValidateCommercialBasisCommand struct {
	Prior domain.CommercialClosure
}

type ValidateCommercialBasisHandler struct {
	authority ports.CommercialAuthorityView
	clock     ports.Clock
}

func NewValidateCommercialBasisHandler(
	authority ports.CommercialAuthorityView,
	clock ports.Clock,
) *ValidateCommercialBasisHandler {
	return &ValidateCommercialBasisHandler{authority: authority, clock: clock}
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
	key := command.Prior.ResolutionKey()

	view, err := handler.authority.LoadScope(ctx, key.TenantID, key.Scope)
	if err != nil {
		// 与第一阶段同一条分界：读不到权威是`解析未决`，不是技术错误，也不是`已失效`。空视图
		// 是领域已定的「权威不可读」表达。
		return handler.resultOf(domain.ValidateClosureBeforeDecision(nil, command.Prior)), nil
	}

	return handler.resultOf(domain.ValidateClosureBeforeDecision(view, command.Prior)), nil
}

func (handler *ValidateCommercialBasisHandler) resultOf(closure domain.CommercialClosure) ResolveCommercialBasisResult {
	return ResolveCommercialBasisResult{closure: closure, judgedAt: handler.clock.Now()}
}
