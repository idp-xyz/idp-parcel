package accessidentity

import (
	"context"
	"errors"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

// CustomerDiscrepancyConclusionMinter 是本适配器对铸造收货差异结论口的最小依赖。
type CustomerDiscrepancyConclusionMinter interface {
	MintReceiptDiscrepancyConclusion(
		ctx context.Context,
		credential identity.IntegrationClientCredential,
		readRequest func() (identity.ChannelRequest, error),
	) (identity.ReceiptDiscrepancyConclusionEnvelope, error)
}

// CustomerDiscrepancyConclusionAuthenticator 把首方铸造译成仓配的客户身份。准入词由铸造方法自己定。
type CustomerDiscrepancyConclusionAuthenticator struct {
	minter CustomerDiscrepancyConclusionMinter
}

var _ warehousehttp.CustomerDiscrepancyConclusionAuthenticator = (*CustomerDiscrepancyConclusionAuthenticator)(nil)

func NewCustomerDiscrepancyConclusionAuthenticator(minter CustomerDiscrepancyConclusionMinter) (*CustomerDiscrepancyConclusionAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: receipt discrepancy conclusion customer authenticator needs a minter")
	}
	return &CustomerDiscrepancyConclusionAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerDiscrepancyConclusionAuthenticator) AuthenticateReceiptDiscrepancyConclusion(
	ctx context.Context,
	credential warehousehttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := authenticator.minter.MintReceiptDiscrepancyConclusion(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}
