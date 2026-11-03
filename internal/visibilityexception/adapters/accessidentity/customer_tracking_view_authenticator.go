package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// CustomerTrackingViewMinter 是本适配器对铸造查阅身份的最小依赖，identity.CustomerClientMinter 满足它。
type CustomerTrackingViewMinter interface {
	MintTrackingViewQuery(ctx context.Context, credential identity.IntegrationClientCredential) (identity.CustomerTrackingViewQueryEnvelope, error)
}

// CustomerTrackingViewAuthenticator 实现 visibilityhttp.CustomerTrackingViewAuthenticator：以首方客户族铸客户追踪视图查阅的身份
// （ADR-0139 决定二、四），把（租户，客户账户）译成本上下文的类型。查阅口不判准入范围（ADR-0139 决定八），铸造侧也不判。
type CustomerTrackingViewAuthenticator struct {
	minter CustomerTrackingViewMinter
}

var _ visibilityhttp.CustomerTrackingViewAuthenticator = (*CustomerTrackingViewAuthenticator)(nil)

func NewCustomerTrackingViewAuthenticator(minter CustomerTrackingViewMinter) (*CustomerTrackingViewAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("visibility accessidentity: customer tracking view authenticator needs a minter")
	}
	return &CustomerTrackingViewAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerTrackingViewAuthenticator) AuthenticateTrackingViewQuery(
	ctx context.Context,
	credential visibilityhttp.PresentedCustomerCredential,
) (visibilityhttp.CustomerViewer, error) {
	envelope, err := authenticator.minter.MintTrackingViewQuery(ctx, identity.NewIntegrationClientCredential(credential.Token, credential.Certificate))
	if err != nil {
		return visibilityhttp.CustomerViewer{}, answerCustomer(err)
	}
	return customerViewerOf(envelope)
}

// CustomerShipmentTrackingMinter 是按运单号查询那一口对铸造的最小依赖，identity.CustomerClientMinter 满足它。
type CustomerShipmentTrackingMinter interface {
	MintShipmentTrackingQuery(ctx context.Context, credential identity.IntegrationClientCredential) (identity.CustomerTrackingViewQueryEnvelope, error)
}

// CustomerShipmentTrackingAuthenticator 实现 visibilityhttp.CustomerShipmentTrackingAuthenticator：铸的是按运单号查询那一口的身份，
// 核那一口自己的授予（一口一词，ADR-0356 决定一）；译法与各格同 CustomerTrackingViewAuthenticator。
type CustomerShipmentTrackingAuthenticator struct {
	minter CustomerShipmentTrackingMinter
}

var _ visibilityhttp.CustomerShipmentTrackingAuthenticator = (*CustomerShipmentTrackingAuthenticator)(nil)

func NewCustomerShipmentTrackingAuthenticator(minter CustomerShipmentTrackingMinter) (*CustomerShipmentTrackingAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("visibility accessidentity: customer shipment tracking authenticator needs a minter")
	}
	return &CustomerShipmentTrackingAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerShipmentTrackingAuthenticator) AuthenticateShipmentTrackingQuery(
	ctx context.Context,
	credential visibilityhttp.PresentedCustomerCredential,
) (visibilityhttp.CustomerViewer, error) {
	envelope, err := authenticator.minter.MintShipmentTrackingQuery(ctx, identity.NewIntegrationClientCredential(credential.Token, credential.Certificate))
	if err != nil {
		return visibilityhttp.CustomerViewer{}, answerCustomer(err)
	}
	return customerViewerOf(envelope)
}

// customerViewerOf 把铸出的身份译成本上下文的类型。铸造方的约定是出错即零值、成功即已铸；这里仍问一句，是不让一个替身或日后的
// 改动把零值身份当成「空账户」递进查询键。
func customerViewerOf(envelope identity.CustomerTrackingViewQueryEnvelope) (visibilityhttp.CustomerViewer, error) {
	if !envelope.Minted() {
		return visibilityhttp.CustomerViewer{}, errors.New("visibility accessidentity: customer tracking query envelope was not minted")
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return visibilityhttp.CustomerViewer{}, fmt.Errorf("visibility accessidentity: tenant: %w", err)
	}
	customer, err := domain.NewCustomerAccountReference(envelope.CustomerAccountID())
	if err != nil {
		return visibilityhttp.CustomerViewer{}, fmt.Errorf("visibility accessidentity: customer account: %w", err)
	}
	return visibilityhttp.CustomerViewer{Tenant: tenant, Customer: customer}, nil
}

// answerCustomer 把共享接入身份能力的格译成 visibilityhttp 的格，首方族那几格各对其格。查阅口不判准入范围，准入那几格在这里不译：
// 真出现了就是铸造侧走错了口，落进端点的「Intake 失败」格，不冒充任何一种恢复动作。
func answerCustomer(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrCustomerClientCredentialRejected, err)
	case errors.Is(err, identity.ErrCustomerClientNotGranted):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrCustomerClientNotGranted, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrCustomerClientRegistryUnavailable):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
