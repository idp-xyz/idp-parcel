package governancehttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// IsolatedOperationsReadIntake 是隔离读面准入（ADR-0078）在治理查阅行的注入式放行：
// 作用域与页大小在构造时由装配点给定，进请求路径后不读任何授权输入。
//
// 与各租户级上下文的同名类型差一维：构造**不收租户**（ADR-0083 Decision 三）——
// 治理册没有租户列，开关值里的合成租户不进治理作用域；注入的作用域只带登记册实有
// 的维度（作用域引用）。启用与否仍由同一开关（IDP_PARCEL_ISOLATED_READ_TENANT）、
// 同一装配点、同一 SYN- 前缀门禁决定（ADR-0078 Decision 三/四原文维持），本类型
// 不成第二种放行形态。
//
// 它与被禁的「开发用」采信实现分界在于：采信要求信任请求里的自报，本类型对请求零
// 读取；运营查阅也不铸来源信封、零持久化，不存在事后与真实认证结果相混的产物。
type IsolatedOperationsReadIntake struct {
	scope domain.OperationsQueryScope
	limit int
}

var _ RegistryQueryIntake = IsolatedOperationsReadIntake{}

// NewIsolatedOperationsReadIntake 由装配点以显式合成值构造。立不起来的作用域与非正
// 页大小在这里拒：装配错误要在启动时暴露，不该等到第一个请求。
func NewIsolatedOperationsReadIntake(
	scopeReference string,
	limit int,
) (IsolatedOperationsReadIntake, error) {
	reference, err := domain.NewOperationsScopeReference(scopeReference)
	if err != nil {
		return IsolatedOperationsReadIntake{}, fmt.Errorf("pilot governance http: isolated read intake: %w", err)
	}
	scope, err := domain.NewOperationsQueryScope(reference)
	if err != nil {
		return IsolatedOperationsReadIntake{}, fmt.Errorf("pilot governance http: isolated read intake: %w", err)
	}
	if limit <= 0 {
		return IsolatedOperationsReadIntake{}, fmt.Errorf("pilot governance http: isolated read intake: limit must be positive, got %d", limit)
	}
	return IsolatedOperationsReadIntake{scope: scope, limit: limit}, nil
}

// IntakeRegistryQuery 不读请求（参数匿名）：作用域整组来自注入，答复与请求内容及
// 自报身份无关。
func (intake IsolatedOperationsReadIntake) IntakeRegistryQuery(context.Context, *http.Request) (RegistryQuery, error) {
	return RegistryQuery{Scope: intake.scope, Limit: intake.limit}, nil
}
