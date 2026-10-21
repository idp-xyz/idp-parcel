package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件是集运成员关系按时点的只读问法（ADR-0225）：别的上下文要知道「某一刻这一袋里直接装着什么」时问它。出发门禁读数
// 对段里在控的集运单元要按成员逐件问关务（transport-fulfillment），而成员是谁只有本上下文的成员关系册说得出。
//
// 另立文件、另立接口，不并进查阅页的 ReviewCatalogueRead：那一口按页列行、带 limit，答的是「册上有哪些行」；这里按（对象，时点）
// 答一件单元那一刻的直接成员，是另一种问法。也不扩 ConsolidationStore：写侧接口加一问，所有只为集运写入而写的替身都要跟着改。

// ConsolidationMembershipAt 是一件对象在某一时点作为集运单元的读数。
//
// Unit 说这件对象是不是本租户的集运单元实例；判据与权威运输交接认对象时同一条（ADR-0214 决定二）：与包裹号同串时也按单元判。
// Unit 为假时 Members 恒空——那是「不是单元」，不是「空单元」；调用方据此按包裹去问别处。
//
// Members 是时点那一刻的直接成员（CONTEXT「集运成员关系」：明确作业实物在适用时间内直接位于某一集运单元中），按作业实物标识定序、
// 去重。间接包含不在这里展开：嵌套成员带 NestedUnit，要它里面装着什么，按它自己的标识再问一次。实例在时点之前已经关闭的，那一刻
// 它不再装着任何东西（关闭要求成员已全部移出或已经处置转移），Members 为空。
type ConsolidationMembershipAt struct {
	Unit    bool
	Members []ConsolidationMemberAt
}

// ConsolidationMemberAt 是时点那一刻的一件直接成员。NestedUnit：这件成员本身是本租户的集运单元实例（袋套袋）。成员的正式包裹身份
// 不在这里给：那是收寄册上的版本化关联，问 IntakeIdentityView、由 domain.JudgeIdentityBasis 认，与方向依据记录的身份一格同一个
// 读法——这里再给一份，同一件实物「此刻是哪件包裹」就有了两处判据。
type ConsolidationMemberAt struct {
	Member     domain.HandlingUnitID
	NestedUnit bool
}

// ConsolidationMembershipAtView 按（租户，对象，时点）答 ConsolidationMembershipAt。成员按移入、移出事实的业务发生时间取（不按记录
// 时刻，也不按此刻的容纳索引）：迟到的事实按它发生那一刻算，问的人也是按某个已经发生的时刻问。不按封装成员快照取：快照只在封装
// 那一刻冻结，开放装入的单元没有它，封签异常时 CONTEXT 也不许依它推成员。
//
// object 是调用方手里的对象引用原文，本上下文只拿它对单元册；读不动或册上的值立不起来作为错误返回——「不是单元」与「没查到」是
// 两件事。只读，不取锁。
type ConsolidationMembershipAtView interface {
	ReadConsolidationMembershipAt(
		ctx context.Context,
		tenant domain.TenantID,
		object string,
		at time.Time,
	) (ConsolidationMembershipAt, error)
}
