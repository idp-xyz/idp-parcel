package ports

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是全程追踪在一件作业实物后来识别成功时要问的那两口（ADR-0299）：一封节点收寄形成信封指的那份收寄是不是「这件实物认出了
// 哪件包裹」，以及这件实物曾随过哪几件集运单元走。收寄身份、成员关系只有 node-operations 的收寄册与集运成员关系册说得出，本上下文
// 只按引用问，不重判（ADR-0025）。

// FormedIntakeIdentity 是一封节点收寄形成信封指的那份收寄说的事：Identified 为真时，Handling 这件作业实物在这份收寄上认出了 Parcel。
// Identified 为假是这份收寄没有认出任何包裹（待识别，或不是已形成的收寄），没有可补的对象，Handling 与 Parcel 不看。
type FormedIntakeIdentity struct {
	Identified bool
	Handling   string
	Parcel     domain.TrackedParcelReference
}

// IdentifiedMemberView 是 node-operations 收寄身份与集运成员关系在补派生这一路上的消费侧端口。
//
// FormedIntakeIdentity 按信封键（租户、收寄来源标识）回读收寄。读不回（册读不动、按键没有）作为普通错误交回，是续办；收寄记录与键不符、
// 时间缺席、形成格没有版本化关联这类仓储不变量已破，包住 ErrFormedIntakeUntranslatable 交回——重读还是同一个结果。
//
// UnitsEverHolding 答这个成员（作业实物或嵌套单元的标识原文）曾被直接装入过哪几件集运单元，不问哪一刻、不问现在还在不在：只是候选。
// 候选里哪一件在事实发生那一刻真的装着它，不由这一口回答，而由补派生自己用与正常路径同一处的展开去验证（trackedParcelsAt），正向
// 展开与反向寻找因此永远用同一条成员口径。读不动作为普通错误交回，不得读成空——那等于认定这件实物没随过任何袋；节点作业答了却立不成
// 单元标识的，包住 ErrConsolidationMembershipUntranslatable。
type IdentifiedMemberView interface {
	FormedIntakeIdentity(ctx context.Context, tenant domain.TenantID, source string) (FormedIntakeIdentity, error)
	UnitsEverHolding(ctx context.Context, tenant domain.TenantID, member string) ([]string, error)
}

// ErrFormedIntakeUntranslatable 见 IdentifiedMemberView。确定性：要查节点作业的收寄册，不是等那边恢复。
var ErrFormedIntakeUntranslatable = errors.New(
	"visibility exception: the formed node intake answer cannot be translated")
