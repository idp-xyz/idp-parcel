package pilotgovernance

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// IsolatedGovernanceScopeDirectory 是隔离形态（ADR-0091）的注入式范围目录：治理坐标
// 在构造时由装配点以合成值给定，不按拟受理范围分叉；它只答写开关那一个租户的范围。
//
// 它交出的只有坐标，归属仍由 ProductionOwnershipAdapter 拿这组坐标去读治理登记册后
// 形成——登记册为空时照旧答`权威未确定`。这条界线是本类型存在的全部前提：一个直接
// 交回「本产品即权威」的目录会把「诚实的未配置」换成「撒谎的已配置」，而后者在库里
// 与真实放行长着同一张脸（ADR-0091 决定二）。
//
// 它与红线所禁的「开发用」实现分界在于：那条禁的是从请求内容铸造本该由权威来源给出
// 的东西，而治理坐标本就不来自请求——真实形态下它来自租户随试点登记的目录，这里来自
// 装配点，两处都不是调用方。启用与否、`SYN-` 前缀门禁与启动日志都在 cmd/parcel-api
// （ADR-0091 决定四）。
//
// 租户那一格守的是 ADR-0091 决定六「也不因它出现而获得任何真实租户」：提交口挂的是真渠道，任何租户的委托都会走到
// 归属这一步；目录若对谁都交坐标，非开关租户的委托就会凭合成治理区间得到「本产品承接」、落成`已提交`。开关租户之外的
// 范围答「显式未配置」，归属如实答`权威未确定`，与生产形态同一个答法。
type IsolatedGovernanceScopeDirectory struct {
	tenant psdomain.TenantID
	scope  GovernanceScope
}

var _ GovernanceScopeDirectory = IsolatedGovernanceScopeDirectory{}

// NewIsolatedGovernanceScopeDirectory 由装配点以写开关的租户与显式合成坐标构造。残缺输入在这里拒：
// 装配错误要在启动时暴露，等到第一个请求才发现的表现是一个看不出原因的`权威未确定`。
func NewIsolatedGovernanceScopeDirectory(
	tenant string,
	objectScope string,
	capability string,
	factKind string,
	pilotScope string,
) (IsolatedGovernanceScopeDirectory, error) {
	tenantID, err := psdomain.NewTenantID(tenant)
	if err != nil {
		return IsolatedGovernanceScopeDirectory{}, fmt.Errorf(
			"parcel shipment pilotgovernance adapter: isolated governance scope: %w", err)
	}
	scopeVersion, err := pgdomain.NewScopeVersionReference(pilotScope)
	if err != nil {
		return IsolatedGovernanceScopeDirectory{}, fmt.Errorf(
			"parcel shipment pilotgovernance adapter: isolated governance scope: %w", err)
	}
	governance := GovernanceScope{
		ObjectScope: objectScope,
		Capability:  capability,
		FactKind:    factKind,
		PilotScope:  scopeVersion,
	}
	if !governance.complete() {
		return IsolatedGovernanceScopeDirectory{}, fmt.Errorf(
			"parcel shipment pilotgovernance adapter: isolated governance scope is incomplete")
	}
	return IsolatedGovernanceScopeDirectory{tenant: tenantID, scope: governance}, nil
}

// FindGovernanceScope 从拟受理范围只读租户一格，用来判这份范围在不在隔离形态之内；坐标不随范围变，也不从租户派生
// （ADR-0091 决定五）。要按范围分叉坐标的那天，分叉规则属 `PAR-GOV-03..07` 的实例半边语义，不该由一个合成目录替它拟。
//
// 认不出租户的范围（不是按客户账户立的那一形）同样答未配置：认不出就无从证明它属开关租户。
func (directory IsolatedGovernanceScopeDirectory) FindGovernanceScope(
	_ context.Context, scope psdomain.AdmissionScope,
) (GovernanceScope, bool, error) {
	tenant, _, recognised := scope.CustomerAccount()
	if !recognised || tenant != directory.tenant {
		return GovernanceScope{}, false, nil
	}
	return directory.scope, true, nil
}
