package governancehttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// OperatorOperationsReadAuthenticator 认证一次主数据与运营查阅读出示。它不交回租户：治理登记册是产品实例级的、没有
// 租户维（ADR-0083 决定二、三）。失败只交回本包的格；从共享接入身份能力译过来的那一层在 adapters/accessidentity。
type OperatorOperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) error
}

// OperatorOperationsReadIntake 是操作者渠道在治理登记册查阅页的 Intake（ADR-0100 决定四），受众是运营方的治理操作员
// （ADR-0083 决定四）。查阅不形成生产事实，不判准入范围。
type OperatorOperationsReadIntake struct {
	authenticator OperatorOperationsReadAuthenticator
}

var _ RegistryQueryIntake = (*OperatorOperationsReadIntake)(nil)

func NewOperatorOperationsReadIntake(authenticator OperatorOperationsReadAuthenticator) (*OperatorOperationsReadIntake, error) {
	if authenticator == nil {
		return nil, errors.New("pilot governance http: operator operations read intake needs an authenticator")
	}
	return &OperatorOperationsReadIntake{authenticator: authenticator}, nil
}

// 作用域只带引用一维（ADR-0083 决定二、三），引用是授权结果的审计引用、不参与行过滤，取能力面的名字。页大小是这一
// 渠道的产品策略，不采信请求。
const (
	operatorRegistryScopeReference = "MASTER_DATA_AND_OPERATIONS_READ"
	operatorRegistryReadLimit      = 200
)

// IntakeRegistryQuery 不读请求体。
func (intake *OperatorOperationsReadIntake) IntakeRegistryQuery(ctx context.Context, request *http.Request) (RegistryQuery, error) {
	if err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request)); err != nil {
		return RegistryQuery{}, err
	}
	reference, err := domain.NewOperationsScopeReference(operatorRegistryScopeReference)
	if err != nil {
		return RegistryQuery{}, err
	}
	scope, err := domain.NewOperationsQueryScope(reference)
	if err != nil {
		return RegistryQuery{}, err
	}
	return RegistryQuery{Scope: scope, Limit: operatorRegistryReadLimit}, nil
}

// 操作者渠道在本包查阅面的三格答复（ADR-0100 决定四），与 ErrAccessChannelNotConfigured 并列，各对一种恢复动作
// （ADR-0029）。查阅不形成生产事实，不判准入范围，所以没有「不在准入范围」那一格。
var (
	// ErrOperatorCredentialRejected：令牌缺失、过期或校验不过（401）——持令牌的人去登录或换令牌。
	ErrOperatorCredentialRejected = errors.New("pilot governance http: operator credential rejected")
	// ErrOperatorNotGranted：令牌有效，但不在册、不绑这个租户或没有主数据与运营查阅读的授予（403）——管授予的人去登记。
	ErrOperatorNotGranted = errors.New("pilot governance http: operator holds no master data and operations read grant")
	// ErrIdentityDependencyUnavailable：发行方公钥集或操作者册读不动（503）——运维去救依赖。
	ErrIdentityDependencyUnavailable = errors.New("pilot governance http: identity dependency unavailable")
)

const (
	codeOperatorCredentialRejected    = "OPERATOR_CREDENTIAL_REJECTED"
	codeOperatorNotGranted            = "OPERATOR_NOT_GRANTED"
	codeIdentityDependencyUnavailable = "IDENTITY_DEPENDENCY_UNAVAILABLE"
)
