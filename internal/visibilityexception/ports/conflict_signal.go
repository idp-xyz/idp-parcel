package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是「冲突仍无法裁决时……形成适用异常信号」（CONTEXT）那一步的规则端口。「适用」
// 二字是这里的全部理由：信号必须保存类型、规则版本与可信度依据（CONTEXT「每个信号必须
// 保存对象、类型、规则版本、判断时间、事实依据、可信度」），而事实冲突该算哪一类异常、
// 依据哪一版识别规则、可信度记什么，属首发异常类型清单（`PAR-VIS-04`）的登记内容——
// 编排不替租户写死一种。

// ConflictSignalRule 是冲突信号规则的答复：无法裁决的事实冲突形成信号时用的类型、识别
// 规则版本与可信度依据。三样全部来自登记，编排一样都不补。
type ConflictSignalRule struct {
	Kind       domain.ExceptionSignalKindReference
	Rule       domain.SignalRuleVersionReference
	Confidence domain.ConfidenceReference
}

// ConflictSignalRuleView 回答「本租户的事实冲突按哪条已登记的规则形成信号」。第二个返回
// 值为 false 即「未配置」：那不是未决而是如实的空白——冲突照常留在投影里按信息待确认
// 表达，只是没有可形成的适用信号；编排把这一格记进结果，不虚构一个信号类型。依赖调不通
// 作为错误返回。租户在装配期固定，理由同本上下文其余只读视图。
type ConflictSignalRuleView interface {
	ConflictSignalRule(ctx context.Context) (ConflictSignalRule, bool, error)
}

// ConflictSignalRuleRegistration 登记本租户的冲突信号规则。
type ConflictSignalRuleRegistration struct {
	Kind       domain.ExceptionSignalKindReference
	Rule       domain.SignalRuleVersionReference
	Confidence domain.ConfidenceReference
	ApprovedBy string
}

// ConflictSignalRuleRegistry 是冲突信号规则的写入口。一租户一条、不可覆盖：撞既有行交回
// AlreadyRegistered——换规则版本是一次治理动作，登记口不替它静默换掉一条已据以形成过
// 信号的规则。单立端口而不并进 CatalogRegistry 的理由同 ExceptionDisclosureRuleRegistry。
// 事务纪律同 CatalogRegistry：在调用方的事务内执行。
type ConflictSignalRuleRegistry interface {
	RegisterConflictSignalRule(
		ctx context.Context,
		tenant domain.TenantID,
		registration ConflictSignalRuleRegistration,
	) (CatalogRegistrationOutcome, error)
}
