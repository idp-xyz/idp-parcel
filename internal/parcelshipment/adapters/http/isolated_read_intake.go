package shipmenthttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// IsolatedOperationsReadIntake 是隔离读面准入（ADR-0078）的注入式放行：作用域与页大小
// 在构造时由装配点给定，进请求路径后不读任何授权输入。它只实现委托查阅的
// ShipmentRequestViewsIntake——提交、撤回、取消三个命令 Intake 一个不带，放行装不进
// 命令端点由编译期决定。
//
// 本上下文的查阅作用域带可见客户账户维（AuthorizedQueryScope）。那一维在这里是装配
// 注入的授权结果（运营侧可见集），不是调用方身份主张——与被排除的客户查阅面
// （/customer-tracking-view）的客户维不同类；分界记在 ADR-0078 Decision 一。
//
// 它与被禁的「开发用」采信实现分界在于：采信要求信任请求里的自报，本类型对请求不读
// 任何授权输入；运营查阅也不铸来源信封、零持久化，不存在事后与真实认证结果相混的
// 产物。启用与否、合成标识门禁与启动日志都在 cmd/parcel-api（ADR-0078 Decision 三）。
type IsolatedOperationsReadIntake struct {
	scope domain.AuthorizedQueryScope
	limit int
}

var (
	_ ShipmentRequestViewsIntake  = IsolatedOperationsReadIntake{}
	_ LabelTransactionQueryIntake = IsolatedOperationsReadIntake{}
)

// NewIsolatedOperationsReadIntake 由装配点以显式合成值构造。立不起来的作用域与非正
// 页大小在这里拒：装配错误要在启动时暴露，不该等到第一个请求。
func NewIsolatedOperationsReadIntake(
	scopeReference string,
	tenant string,
	customerAccounts []string,
	limit int,
) (IsolatedOperationsReadIntake, error) {
	reference, err := domain.NewQueryScopeReference(scopeReference)
	if err != nil {
		return IsolatedOperationsReadIntake{}, fmt.Errorf("parcel shipment http: isolated read intake: %w", err)
	}
	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		return IsolatedOperationsReadIntake{}, fmt.Errorf("parcel shipment http: isolated read intake: %w", err)
	}
	accounts := make([]domain.CustomerAccountID, 0, len(customerAccounts))
	for _, account := range customerAccounts {
		accountID, err := domain.NewCustomerAccountID(account)
		if err != nil {
			return IsolatedOperationsReadIntake{}, fmt.Errorf("parcel shipment http: isolated read intake: %w", err)
		}
		accounts = append(accounts, accountID)
	}
	scope, err := domain.NewAuthorizedQueryScope(reference, tenantID, accounts)
	if err != nil {
		return IsolatedOperationsReadIntake{}, fmt.Errorf("parcel shipment http: isolated read intake: %w", err)
	}
	if limit <= 0 {
		return IsolatedOperationsReadIntake{}, fmt.Errorf("parcel shipment http: isolated read intake: limit must be positive, got %d", limit)
	}
	return IsolatedOperationsReadIntake{scope: scope, limit: limit}, nil
}

// IntakeListQuery 不读请求（参数匿名）：作用域整组来自注入，答复与请求内容及自报
// 身份无关。
func (intake IsolatedOperationsReadIntake) IntakeListQuery(context.Context, *http.Request) (ShipmentRequestViewsQuery, error) {
	return ShipmentRequestViewsQuery{Scope: intake.scope, Limit: intake.limit}, nil
}

// IntakeLabelTransactionQuery 只交出注入作用域的租户维。同一个注入值同时服务两种查阅面
// 不是把两者混为一谈：面单交易没有账户维可分（ADR-0084 决定七），所以这里交出去的比委托
// 查阅少一维，而少的那一维是本册压根没有的那一维，不是被丢掉的过滤条件。
func (intake IsolatedOperationsReadIntake) IntakeLabelTransactionQuery(context.Context, *http.Request) (LabelTransactionQuery, error) {
	return LabelTransactionQuery{Tenant: intake.scope.TenantID(), Limit: intake.limit}, nil
}

// IntakeDetailQuery 解析定位标识 `shipmentRequestId`。标识只定位候选对象，不单独证明
// 查询权限——权限在注入的作用域上（与 ShipmentRequestViewQuery 的注释同界）；读定位
// 参数属传输形状，不是被禁的授权输入（ADR-0078 Decision 二）。
func (intake IsolatedOperationsReadIntake) IntakeDetailQuery(_ context.Context, request *http.Request) (ShipmentRequestViewQuery, error) {
	requestID, err := domain.NewShipmentRequestID(request.URL.Query().Get("shipmentRequestId"))
	if err != nil {
		return ShipmentRequestViewQuery{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return ShipmentRequestViewQuery{Scope: intake.scope, RequestID: requestID}, nil
}
