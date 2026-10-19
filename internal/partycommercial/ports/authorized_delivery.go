package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// AuthorizedDeliveryJudgmentView 答授权交付判断（CONTEXT-MAP `party-commercial → node-operations`；ADR-0212 决定三）：
// 这个交付方此刻可不可以替这个客户账户在这个节点送站。答复只有 domain.AuthorizedDeliveryAnswer 的三格，判法见
// domain.JudgeAuthorizedDelivery。
//
// 另开一份文件、只交答复：问话的一方（节点作业确认送站交接单）要的是「准不准」，把参与方关系整段交出去，就等于让它
// 自己判生命周期、范围与相对方，判法会在两个上下文各长一份。
//
// 读不动与坏数据走 error，不折成任何一格：「否」是业务拒绝，「没问到」不是；问法缺格同样走 error
// （domain.ErrInvalidAuthorizedDeliveryQuery）。
type AuthorizedDeliveryJudgmentView interface {
	JudgeAuthorizedDelivery(
		ctx context.Context,
		query domain.AuthorizedDeliveryQuery,
	) (domain.AuthorizedDeliveryAnswer, error)
}
