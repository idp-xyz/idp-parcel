package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是`面单继续尝试决定`写面（票 label-channel/30）要的三个口：授权、标识签发、写后交接。
// 登记册的读写口在 ports.go（ContinuedAttemptRegisterRepository，票 label-channel/10）。

// ContinuedAttemptDecisionAuthorizationQuery 说明谁要以什么原因、对哪件包裹形成哪一种`面单继续尝试决定`。
//
// 它与其余授权查询同样不带授权引用：调用方自带一个，就等于自己给自己签字。它也不带决定方——
// 决定方由 party-commercial 解出（ADR-0116 决定三 / 四），PS 不自判。Requester 是「请求方（如有）」：
// 货主、渠道服务方或代理商只能提出请求（CONTEXT Rules），它在这里只作请求事实随查询进提供方，
// 不带来任何授权；关闭这一格允许缺席，重开与关闭同型只为一口。
//
// Identity 是该包裹所属委托的来源身份：授权规则按责任法人与商业范围版本化，而范围要从委托上解
// ——那是实例半边的映射（`PAR-COM-14`），适配器只把身份交给映射，不代它挑范围。
type ContinuedAttemptDecisionAuthorizationQuery struct {
	Identity  domain.SourceIdentity
	Parcel    domain.DeclaredParcelID
	Kind      domain.ContinuedAttemptDecisionKind
	Requester domain.RequesterReference
	Reason    domain.ContinuedAttemptReasonReference
	At        time.Time
}

// ContinuedAttemptDecisionAuthorization 交回本次决定所采用的授权依据快照、授权角色与实际决定方。
//
// 三项一起由 party-commercial 给出，不由调用方声明：CONTEXT「登录操作人可以作为操作证据，但不能替代
// 实际决定方和授权角色」，而让请求方自报这三项正是那句话禁止的事（ADR-0116 决定四）。本上下文只保存
// 所采用的那一份，不判断它够不够格——授权规则属 party-commercial（`Append` 头注同一条分工）。
// 三项只在`已授权`时携带，其余取值一律不带。
type ContinuedAttemptDecisionAuthorization struct {
	Outcome       AuthorizationOutcome
	Authority     domain.ContinuedAttemptAuthoritySnapshot
	AuthorityRole domain.ContinuedAttemptAuthorityRoleReference
	Decider       domain.DeciderReference
}

// ContinuedAttemptDecisionAuthorizer 回答 party-commercial 是否授权这次关闭或重开。
//
// 关闭权与重开权在 PC 是互不蕴含的两格（`CONTROLLED_CLOSURE` / `REOPENING`，票 pc-gaps/13）；一口两问
// 由查询里的 Kind 分派，不合成一个动作——合成会让一份只授关闭权的规则被读成重开权。「同级或更高」
// 不由本上下文比较：谁可重开由 PC 登进 `REOPENING` 那一格的等级集合承担（pc-gaps/13 裁决 ①）。
//
// 答不出与答得出分属两回事：前者是错误，后者一律经 AuthorizationOutcome 交回。真实授权角色、等级与
// 范围仍是 `PAR-COM-13` / `PAR-COM-14` 待提供的实例参数，本上下文不内置任何默认——把没有规则答成
// `不允许`同样是一次默认，只是方向朝紧。
type ContinuedAttemptDecisionAuthorizer interface {
	AuthorizeContinuedAttemptDecision(
		ctx context.Context,
		query ContinuedAttemptDecisionAuthorizationQuery,
	) (ContinuedAttemptDecisionAuthorization, error)
}

// ContinuedAttemptDecisionIdentity 签发`面单继续尝试决定`的标识。
//
// 决定标识由本上下文铸而不从内容派生：关过—重开—再关是三条各自的决定，内容可以逐字相同。
// 它兼作`权威业务截断边界`的值（票 label-channel/30 裁决：`AuthoritativeCutoffBoundary` = 关闭决定标识）
// 与关闭路径终局的来源版本（`CONTINUED-ATTEMPT-CLOSURE/<决定标识>`）——一个事一个名。
type ContinuedAttemptDecisionIdentity interface {
	NextContinuedAttemptDecisionID(ctx context.Context) (domain.ContinuedAttemptDecisionID, error)
}

// ContinuedAttemptDecisionHandoffIntent 是「这件包裹值得判一次面单服务终局」的指针（ADR-0134 决定一），
// 由关闭 / 重开决定落册那一拍交出。它不是事实副本：判断读全册且读当下，消费者不按它读回决定；
// 决定标识进事件 ID 只为幂等与追溯——关过—重开—再关三封各自成封。登记册键就是租户 + 包裹，
// 一册一件，一封一决定即一封一包裹，不必展开。Kind 只作追溯，消费者不据它分支（两种决定都触发，
// 分格归判断；ADR-0134 决定五）。OccurredAt 是决定的生效时间——「生效 = 追加」（lc/27），不在这里铸。
type ContinuedAttemptDecisionHandoffIntent struct {
	Tenant     domain.TenantID
	Parcel     domain.DeclaredParcelID
	Decision   domain.ContinuedAttemptDecisionID
	Kind       domain.ContinuedAttemptDecisionKind
	OccurredAt time.Time
}

// ContinuedAttemptDecisionHandoff 把判断意图写入 Outbox（`OutboxContinuedAttemptDecisionHandoff`），
// 与写入方的 `Insert` / `Save` 同一事务——两者都从 ctx 取同一个事务执行器，事务由组合根的事务壳开
// （ADR-0134 决定三）。入队失败即整步失败：不留「决定已落、判断意图丢了」的中间态。
type ContinuedAttemptDecisionHandoff interface {
	HandOffContinuedAttemptDecision(ctx context.Context, intent ContinuedAttemptDecisionHandoffIntent) error
}
