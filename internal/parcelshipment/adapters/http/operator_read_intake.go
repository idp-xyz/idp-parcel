package shipmenthttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// OperatorOperationsReadAuthenticator 认证一次主数据与运营查阅读出示，交回操作者绑定的租户。失败只交回本包的格；
// 从共享接入身份能力译过来的那一层在 adapters/accessidentity。
type OperatorOperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) (domain.TenantID, error)
}

// OperatorOperationsReadIntake 是操作者渠道在面单交易与渠道择优决定两口的 Intake（ADR-0100 决定四）。两口的作用域
// 只有租户一维（ADR-0084 决定七），信封给得出。委托查阅及其子集视图刻意不实现：它们的读口按可见客户账户切，操作者信封只有
// 租户，拿一组账户去凑等于造一个作用域（票 operator-channel/53）。
type OperatorOperationsReadIntake struct {
	authenticator OperatorOperationsReadAuthenticator
}

var (
	_ LabelTransactionQueryIntake         = (*OperatorOperationsReadIntake)(nil)
	_ ChannelSelectionDecisionQueryIntake = (*OperatorOperationsReadIntake)(nil)
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
