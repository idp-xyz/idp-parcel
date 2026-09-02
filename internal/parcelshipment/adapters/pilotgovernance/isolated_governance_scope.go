package pilotgovernance

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// IsolatedGovernanceScopeDirectory 是隔离形态（ADR-0091）的注入式范围目录：治理坐标
// 在构造时由装配点以合成值给定，进请求路径后不读拟受理范围。
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
type IsolatedGovernanceScopeDirectory struct {
	scope GovernanceScope
}

var _ GovernanceScopeDirectory = IsolatedGovernanceScopeDirectory{}

// NewIsolatedGovernanceScopeDirectory 由装配点以显式合成值构造。残缺坐标在这里拒：
// 装配错误要在启动时暴露，等到第一个请求才发现的表现是一个看不出原因的`权威未确定`。
func NewIsolatedGovernanceScopeDirectory(
	objectScope string,
	capability string,
	factKind string,
	pilotScope string,
) (IsolatedGovernanceScopeDirectory, error) {
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
	return IsolatedGovernanceScopeDirectory{scope: governance}, nil
}

// FindGovernanceScope 不读拟受理范围（参数匿名）：隔离环境里只有一份合成范围，坐标
// 整组来自注入。要按范围分叉的那天，分叉规则属 `PAR-GOV-03..07` 的实例半边语义，
// 不该由一个合成目录替它拟——匿名参数是不给「读一眼再决定」留位置。
func (directory IsolatedGovernanceScopeDirectory) FindGovernanceScope(
	context.Context, psdomain.AdmissionScope,
) (GovernanceScope, bool, error) {
	return directory.scope, true, nil
}
