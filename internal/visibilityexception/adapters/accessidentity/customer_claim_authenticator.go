package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// CustomerClaimMinter 是本适配器对铸造首方索赔信封的最小依赖，identity.CustomerClientMinter 满足它。
type CustomerClaimMinter interface {
	MintClaim(
		ctx context.Context,
		credential identity.IntegrationClientCredential,
		readRequest func() (identity.ChannelRequest, error),
	) (identity.ClaimEnvelope, error)
}

// CustomerClaimAuthenticator 实现 visibilityhttp.CustomerClaimAuthenticator：以首方客户族铸索赔信封（ADR-0139 决定二、四、八，
// 铸造侧判准入范围），把（租户，客户账户）译成本上下文的类型。信封里的来源请求键只管传输重放，索赔的唯一性在自然键上
// （ADR-0310 取舍 F），所以不带进命令。
type CustomerClaimAuthenticator struct {
	minter CustomerClaimMinter
}

var _ visibilityhttp.CustomerClaimAuthenticator = (*CustomerClaimAuthenticator)(nil)

func NewCustomerClaimAuthenticator(minter CustomerClaimMinter) (*CustomerClaimAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("visibility accessidentity: customer claim authenticator needs a minter")
	}
	return &CustomerClaimAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerClaimAuthenticator) AuthenticateClaim(
	ctx context.Context,
	credential visibilityhttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
) (visibilityhttp.CustomerViewer, error) {
	envelope, err := authenticator.minter.MintClaim(ctx,
		identity.NewIntegrationClientCredential(credential.Token, credential.Certificate),
		func() (identity.ChannelRequest, error) {
			id, err := accessRequestID()
			if err != nil {
				return identity.ChannelRequest{}, err
			}
			return identity.NewChannelRequest(map[string]string{identity.AccessRequestIdentifier: id}), nil
		})
	if err != nil {
		return visibilityhttp.CustomerViewer{}, answerClaimCustomer(err)
	}
	minted := envelope.Envelope()
	tenant, err := domain.NewTenantID(minted.TenantID())
	if err != nil {
		return visibilityhttp.CustomerViewer{}, fmt.Errorf("visibility accessidentity: tenant: %w", err)
	}
	customer, err := domain.NewCustomerAccountReference(minted.CustomerAccountID())
	if err != nil {
		return visibilityhttp.CustomerViewer{}, fmt.Errorf("visibility accessidentity: customer account: %w", err)
	}
	return visibilityhttp.CustomerViewer{Tenant: tenant, Customer: customer}, nil
}

// answerClaimCustomer 在 answerCustomer 的格之上多译准入范围两格；推导不出来源请求键是客户系统没给接入请求标识，答形状错。
func answerClaimCustomer(err error) error {
	switch {
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrIdentityDependencyUnavailable, err)
	case errors.Is(err, identity.ErrRequestKeyNotDerived):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrMalformedClaim, err)
	}
	return answerCustomer(err)
}
