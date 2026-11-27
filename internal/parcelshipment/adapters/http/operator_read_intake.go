package shipmenthttp

import (
	"context"
	"errors"
	"net/http"

	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// OperatorOperationsReadAuthenticator 认证一次主数据与运营查阅读出示，交回操作者绑定的租户。失败只交回本包的格；
// 从共享接入身份能力译过来的那一层在 adapters/accessidentity。
type OperatorOperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// OperatorOperationsReadIntake 是操作者渠道的查阅 Intake（ADR-0100 决定四）。面单交易、渠道择优、释放未受理册、
// 取消先后冲突册的作用域只有租户一维。委托查阅及其子集视图也走这里：运营侧看见本租户的全部账户
// （OperatorTenantScope），不把账户名单塞进客户侧的 AuthorizedQueryScope。
type OperatorOperationsReadIntake struct {
	authenticator OperatorOperationsReadAuthenticator
}

var _ ShipmentRequestViewsIntake = (*OperatorOperationsReadIntake)(nil)

var (
	_ LabelTransactionQueryIntake             = (*OperatorOperationsReadIntake)(nil)
	_ ChannelSelectionDecisionQueryIntake     = (*OperatorOperationsReadIntake)(nil)
	_ ControlReleaseRefusalQueryIntake        = (*OperatorOperationsReadIntake)(nil)
	_ CancellationOrderConflictQueryIntake    = (*OperatorOperationsReadIntake)(nil)
	_ PostIntakeDispositionRequestQueryIntake = (*OperatorOperationsReadIntake)(nil)
)

func NewOperatorOperationsReadIntake(authenticator OperatorOperationsReadAuthenticator) (*OperatorOperationsReadIntake, error) {
	if authenticator == nil {
		return nil, errors.New("parcel shipment http: operator operations read intake needs an authenticator")
	}
	return &OperatorOperationsReadIntake{authenticator: authenticator}, nil
}

// operatorOperationsReadLimit 是这一渠道的页大小：产品策略，不采信请求，也不随租户变。
const operatorOperationsReadLimit = 200

// IntakeLabelTransactionQuery 不读请求体。租户只取自信封。
func (intake *OperatorOperationsReadIntake) IntakeLabelTransactionQuery(ctx context.Context, request *http.Request) (LabelTransactionQuery, error) {
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request))
	if err != nil {
		return LabelTransactionQuery{}, err
	}
	return LabelTransactionQuery{Tenant: tenant, Limit: operatorOperationsReadLimit}, nil
}

// IntakeChannelSelectionDecisionQuery 同上；列表与单条两条分支的定位参数由端点自己读。
func (intake *OperatorOperationsReadIntake) IntakeChannelSelectionDecisionQuery(ctx context.Context, request *http.Request) (ChannelSelectionDecisionQuery, error) {
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request))
	if err != nil {
		return ChannelSelectionDecisionQuery{}, err
	}
	return ChannelSelectionDecisionQuery{Tenant: tenant, Limit: operatorOperationsReadLimit}, nil
}

// IntakeControlReleaseRefusalQuery 同上：租户只取自信封，页大小是这一渠道的产品策略。
func (intake *OperatorOperationsReadIntake) IntakeControlReleaseRefusalQuery(ctx context.Context, request *http.Request) (ControlReleaseRefusalQuery, error) {
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request))
	if err != nil {
		return ControlReleaseRefusalQuery{}, err
	}
	return ControlReleaseRefusalQuery{Tenant: tenant, Limit: operatorOperationsReadLimit}, nil
}

// IntakeCancellationOrderConflictQuery 同上：租户只取自信封，页大小是这一渠道的产品策略。
func (intake *OperatorOperationsReadIntake) IntakeCancellationOrderConflictQuery(ctx context.Context, request *http.Request) (CancellationOrderConflictQuery, error) {
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request))
	if err != nil {
		return CancellationOrderConflictQuery{}, err
	}
	return CancellationOrderConflictQuery{Tenant: tenant, Limit: operatorOperationsReadLimit}, nil
}

// operatorTenantScope 认证主数据与运营查阅读，作用域是整个租户。引用是固定的证据名，不从请求里取。
func (intake *OperatorOperationsReadIntake) operatorTenantScope(ctx context.Context, request *http.Request) (domain.OperatorTenantScope, error) {
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request))
	if err != nil {
		return domain.OperatorTenantScope{}, err
	}
	reference, err := domain.NewQueryScopeReference("operator-operations-read")
	if err != nil {
		return domain.OperatorTenantScope{}, err
	}
	scope, err := domain.NewOperatorTenantScope(reference, tenant)
	if err != nil {
		return domain.OperatorTenantScope{}, err
	}
	return scope, nil
}

// IntakeListQuery 不读请求体。租户只取自信封，页大小是这一渠道的产品策略。
func (intake *OperatorOperationsReadIntake) IntakeListQuery(ctx context.Context, request *http.Request) (ShipmentRequestViewsQuery, error) {
	scope, err := intake.operatorTenantScope(ctx, request)
	if err != nil {
		return ShipmentRequestViewsQuery{}, err
	}
	return ShipmentRequestViewsQuery{Scope: scope, Limit: operatorOperationsReadLimit}, nil
}

// IntakeDetailQuery 解析定位标识。标识只定位候选对象，权限在租户作用域上。
func (intake *OperatorOperationsReadIntake) IntakeDetailQuery(ctx context.Context, request *http.Request) (ShipmentRequestViewQuery, error) {
	scope, err := intake.operatorTenantScope(ctx, request)
	if err != nil {
		return ShipmentRequestViewQuery{}, err
	}
	requestID, err := domain.NewShipmentRequestID(request.URL.Query().Get("shipmentRequestId"))
	if err != nil {
		return ShipmentRequestViewQuery{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return ShipmentRequestViewQuery{Scope: scope, RequestID: requestID}, nil
}

// IntakePostIntakeDispositionRequestQuery 同上：租户只取自信封，页大小是这一渠道的产品策略（ADR-0332）。
func (intake *OperatorOperationsReadIntake) IntakePostIntakeDispositionRequestQuery(ctx context.Context, request *http.Request) (PostIntakeDispositionRequestQuery, error) {
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request))
	if err != nil {
		return PostIntakeDispositionRequestQuery{}, err
	}
	return PostIntakeDispositionRequestQuery{Tenant: tenant, Limit: operatorOperationsReadLimit}, nil
}
