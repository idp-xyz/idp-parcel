package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件是集运成员关系的反向只读问法：别的上下文要知道「这件实物曾经随过哪几件集运单元走」时问它。全程追踪在实物后来才识别成功时，
// 要把它当初随袋走过的移动事实补给它（ADR-0299），而移动事实声明的段只认袋、不认袋里的成员，所以第一步是由成员找袋。
//
// 与 ConsolidationMembershipAtView 互为正反：那一口按（单元，时点）答直接成员，这一口按成员答曾装它的单元。另立文件、另立接口，
// 理由同那一口——不并进 ConsolidationStore（写侧加一问，所有只为集运写入而写的替身都要跟着改）。

// ConsolidationUnitsHoldingMemberView 按（租户，成员）答这个成员曾被直接装入过哪几件集运单元。
//
// 答的是候选，不是某一刻的结论：只要来源事实层里有一条以它为成员的移入，那件单元就在其中，不问移入之后有没有移出、单元有没有
// 关闭、移入发生在哪一刻。「那一刻它在不在这件单元里」由 ConsolidationMembershipAtView 按事实发生那一刻回答，两口合起来才是
// 「它那一刻随哪件单元走」——候选一口放宽、时点一口收紧，正向展开与反向寻找因此永远用同一条成员口径，不会各算各的。
//
// member 是调用方手里的实物或嵌套单元标识原文，本上下文只拿它对成员事实；嵌套单元也是别的单元的成员，所以它同样可以作为 member
// 问「它曾被装进哪几件单元」，逐层向上就是反复问这一口。结果按单元标识定序、去重；没有任何一条移入即空，不是错。读不动作为错误
// 返回，不得读成「没有」——那等于让一件实物永远错过它随袋走过的移动。只读，不取锁。
type ConsolidationUnitsHoldingMemberView interface {
	ReadUnitsEverHoldingMember(
		ctx context.Context,
		tenant domain.TenantID,
		member string,
	) ([]domain.ConsolidationUnitID, error)
}
