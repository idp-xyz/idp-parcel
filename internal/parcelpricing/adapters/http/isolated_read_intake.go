package pricinghttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// IsolatedOperationsReadIntake 是隔离读面准入（ADR-0078）的注入式放行：作用域与页大小
// 在构造时由装配点给定，进请求路径后不读任何授权输入。它只实现目录查阅的
// PricingCatalogueIntake——命令 Intake 一个不带，放行装不进命令端点由编译期决定。
//
// 它与被禁的「开发用」采信实现分界在于：采信要求信任请求里的自报，本类型对请求零
// 读取；运营查阅也不铸来源信封、零持久化，不存在事后与真实认证结果相混的产物。启用
// 与否、合成标识门禁与启动日志都在 cmd/parcel-api（ADR-0078 Decision 三）——本类型
// 不知道也不判断自己被用在哪个环境。
type IsolatedOperationsReadIntake struct {
	scope domain.OperationsQueryScope
	limit int
}

var _ PricingCatalogueIntake = IsolatedOperationsReadIntake{}

// NewIsolatedOperationsReadIntake 由装配点以显式合成值构造。立不起来的作用域与非正
// 页大小在这里拒：装配错误要在启动时暴露，不该等到第一个请求。
func NewIsolatedOperationsReadIntake(
	scopeReference string,
	tenant string,
	limit int,
) (IsolatedOperationsReadIntake, error) {
	reference, err := domain.NewOperationsScopeReference(scopeReference)
	if err != nil {
		return IsolatedOperationsReadIntake{}, fmt.Errorf("parcel pricing http: isolated read intake: %w", err)
	}
	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		return IsolatedOperationsReadIntake{}, fmt.Errorf("parcel pricing http: isolated read intake: %w", err)
	}
	scope, err := domain.NewOperationsQueryScope(reference, tenantID)
	if err != nil {
		return IsolatedOperationsReadIntake{}, fmt.Errorf("parcel pricing http: isolated read intake: %w", err)
	}
	if limit <= 0 {
		return IsolatedOperationsReadIntake{}, fmt.Errorf("parcel pricing http: isolated read intake: limit must be positive, got %d", limit)
	}
	return IsolatedOperationsReadIntake{scope: scope, limit: limit}, nil
}

// IntakeCatalogueQuery 不读请求（参数匿名）：作用域整组来自注入，答复与请求内容及
// 自报身份无关。
func (intake IsolatedOperationsReadIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{Scope: intake.scope, Limit: intake.limit}, nil
}
