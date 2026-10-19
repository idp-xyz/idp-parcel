package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是「阻断读数派生异常信号」（ADR-0205 决定五与文末补记）那一步的规则端口。出发门禁读数里关务判为阻断的那一件
// 已经随车走了——那是一次已经发生的违规出发，本上下文据它形成异常信号，交分诊。识别条件本身是产品的判断方法：「出发登记时
// 关务对这一件答阻断」不看任何租户的取值就答得出。信号该算哪一类、依据哪一版识别规则、可信度记什么，以及启不启用这一类，
// 属首发异常类型清单（`PAR-VIS-04`）的租户取值——与冲突信号规则（conflict_signal.go）同一纪律，编排不替租户写死一种。

// DepartureGateSignalRule 是出发门禁阻断信号规则的答复：阻断读数形成信号时用的类型、识别规则版本与可信度依据。三样全部来自
// 登记，编排一样都不补。
type DepartureGateSignalRule struct {
	Kind       domain.ExceptionSignalKindReference
	Rule       domain.SignalRuleVersionReference
	Confidence domain.ConfidenceReference
}

// DepartureGateSignalRuleView 回答「本租户的出发门禁阻断按哪条已登记的规则形成信号」。第二个返回值为 false 即「未配置」：
// 那不是未决而是如实的空白——投影照常派生，只是没有可形成的适用信号，消费照常入账；依赖调不通作为错误返回。租户在装配期
// 固定，理由同本上下文其余只读视图。
type DepartureGateSignalRuleView interface {
	DepartureGateSignalRule(ctx context.Context) (DepartureGateSignalRule, bool, error)
}

// DepartureGateSignalRuleRegistration 登记本租户的出发门禁阻断信号规则。
type DepartureGateSignalRuleRegistration struct {
	Kind       domain.ExceptionSignalKindReference
	Rule       domain.SignalRuleVersionReference
	Confidence domain.ConfidenceReference
	ApprovedBy string
}

// DepartureGateSignalRuleRegistry 是出发门禁阻断信号规则的写入口。一租户一条、不可覆盖：撞既有行交回 AlreadyRegistered，
// 理由同 ConflictSignalRuleRegistry——已据这条规则形成过的发作期指着它的规则版本，静默换掉会让历史发作期「按哪版判的」对不上。
// 事务纪律同 CatalogRegistry：在调用方的事务内执行。
type DepartureGateSignalRuleRegistry interface {
	RegisterDepartureGateSignalRule(
		ctx context.Context,
		tenant domain.TenantID,
		registration DepartureGateSignalRuleRegistration,
	) (CatalogRegistrationOutcome, error)
}
