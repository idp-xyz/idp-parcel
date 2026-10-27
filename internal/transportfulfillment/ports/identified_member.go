package ports

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 一件作业实物识别成功时，出发门禁补问要问 node-operations 的那两口（ADR-0331）。另立文件而不并进 consolidation_membership.go：
// 那一口问的是某一时点单元里装着什么，这里问的是一封节点收寄形成信封指的收寄认出了哪件包裹、这件实物曾被装进过哪几件单元。

// IdentifiedIntake 是一封节点收寄形成信封指的那份收寄在 node-operations 里的样子。Identified 为假即它没有认出包裹（待识别、未形成、
// 形成了却没有关联），此时 Handling 与 Parcel 为零值；为真时 Handling 是作业实物标识原文，Parcel 是认出的那件包裹。
type IdentifiedIntake struct {
	Identified bool
	Handling   string
	Parcel     domain.CarriedObjectReference
}

// IdentifiedMemberView 是 node-operations 收寄与集运成员关系的消费侧端口（ADR-0025）。实现只翻译：收寄认没认出包裹、这件实物曾被
// 装进哪几件单元，都是 node-operations 的判断，本上下文不重判。
//
// FormedIntakeIdentity 按信封键回读收寄。error 有两类：包住 ErrFormedIntakeUntranslatable 的是册上的确定性问题（记录与键不符、租户
// 对不上、关联立不成载运对象引用），重读还是同一个结果；其余是读不动或按键还没有（可见性滞后），重投可能就好。
//
// UnitsEverHolding 列这件实物曾被直接装入的单元标识原文，不问移入之后有没有移出、单元有没有关闭；读不动作为错误交回，不答空。
// 答复译不回来的一类包住 ErrConsolidationMembershipUntranslatable。
type IdentifiedMemberView interface {
	FormedIntakeIdentity(ctx context.Context, tenant domain.TenantID, source string) (IdentifiedIntake, error)
	UnitsEverHolding(ctx context.Context, tenant domain.TenantID, member string) ([]string, error)
}

// ErrFormedIntakeUntranslatable：node-operations 的收寄册答了，却立不成本上下文认得的东西，重读也是同一个结果。
var ErrFormedIntakeUntranslatable = errors.New("transport fulfillment: the formed node intake cannot be translated")
