package application

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// DirectionalBasisDeps 是方向依据判断的三个来源与时钟（票 no-directional-basis/01「偏离标记半边：设计代决」）。
type DirectionalBasisDeps struct {
	// Intakes 是本上下文自己的收寄册：身份只问它。
	Intakes ports.IntakeIdentityView
	// ServiceResults 与收寄编排问的是同一个读口、同一只防腐适配器（票 operator-channel/25）。移入要的是此刻的结果，
	// 所以现问 PS，不读收寄记录上的服务结果标记——那是收寄那一刻的快照，之后才形成的取消与终局在标记上读不到。
	ServiceResults ports.ParcelServiceResultView
	// Routing 是路由依据端口；生产装配显式未配置。
	Routing ports.RoutingBasisView
	Clock   ports.Clock
}

// DirectionalBasisJudge 判一件作业实物此刻的方向依据，供 ConsolidateParcelsHandler.AddMember 与事实同一笔落册。
//
// 它由组合根在写事务之前、用外层 ctx 调：读在池上，不在写事务里。跨上下文读在写事务里出一次 SQL 级错误，整笔
// 事务就进了中止态，读不动于是挡住了落册——而已经发生的移入必须照记，读不动只是记录里的一格。
type DirectionalBasisJudge struct {
	deps DirectionalBasisDeps
}

// NewDirectionalBasisJudge 构造期拒缺件：漏装的来源到第一条移入才暴露，会被记成一格`读不动`，把装配缺陷演成
// 依赖故障。
func NewDirectionalBasisJudge(deps DirectionalBasisDeps) (*DirectionalBasisJudge, error) {
	if deps.Intakes == nil || deps.ServiceResults == nil || deps.Routing == nil || deps.Clock == nil {
		return nil, errors.New("node operations: directional basis judge needs the intake register, the service result view, the routing basis view and a clock")
	}
	return &DirectionalBasisJudge{deps: deps}, nil
}

// Judge 判 command 那件成员此刻的方向依据。任何来源答不出或答得读不懂，那一格记`读不动`，记录照样成立；只有记录
// 本身拼不出来（实现坏了）才交回错误。
func (judge *DirectionalBasisJudge) Judge(ctx context.Context, command AddMemberCommand) (domain.DirectionalBasisRecord, error) {
	judgedAt := judge.deps.Clock.Now()
	identity := judge.identity(ctx, command)
	parcel, established := identity.Association()
	if !established {
		return domain.RecordDirectionalBasisWithoutIdentity(identity, judgedAt)
	}
	return domain.RecordDirectionalBasis(identity,
		judge.serviceResult(ctx, command.TenantID, parcel),
		judge.routing(ctx, ports.RoutingBasisQuery{
			TenantID: command.TenantID,
			Parcel:   parcel,
			Member:   command.Member,
			Unit:     command.Unit,
		}),
		judgedAt)
}

func (judge *DirectionalBasisJudge) identity(ctx context.Context, command AddMemberCommand) domain.IdentityBasis {
	observations, err := judge.deps.Intakes.FindIntakeIdentities(ctx, command.TenantID, command.Member)
	if err != nil {
		return domain.UnreadableIdentityBasis()
	}
	identity, err := domain.JudgeIdentityBasis(observations)
	if err != nil {
		return domain.UnreadableIdentityBasis()
	}
	return identity
}

func (judge *DirectionalBasisJudge) serviceResult(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelAssociationReference,
) domain.ServiceResultBasis {
	results, err := judge.deps.ServiceResults.FindParcelServiceResults(ctx, tenant, parcel)
	if err != nil {
		return domain.UnreadableServiceResultBasis()
	}
	basis, err := domain.JudgeServiceResultBasis(results.Cancellation, results.FinalKind, results.FinalVersion)
	if err != nil {
		return domain.UnreadableServiceResultBasis()
	}
	return basis
}

// routing 把没经过任何构造门的答复（零值）也记成读不动：读不懂与读不出来，同样是此刻判断不了。
func (judge *DirectionalBasisJudge) routing(ctx context.Context, query ports.RoutingBasisQuery) domain.RoutingBasis {
	basis, err := judge.deps.Routing.FindRoutingBasis(ctx, query)
	if err != nil || basis.Cell().Answer() == domain.DirectionalBasisAnswerInvalid {
		return domain.UnreadableRoutingBasis()
	}
	return basis
}
