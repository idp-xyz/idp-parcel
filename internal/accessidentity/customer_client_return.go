package accessidentity

import "context"

// ReturnPreAdviceEnvelope 是退货预报货主口的来源信封。
type ReturnPreAdviceEnvelope struct{ envelope SourceEnvelope }

func (submission ReturnPreAdviceEnvelope) Envelope() SourceEnvelope { return submission.envelope }

// MintReturnPreAdvice 铸一次退货预报提交。准入两格都是 RETURN_PRE_ADVICE_SUBMISSION。
func (minter *CustomerClientMinter) MintReturnPreAdvice(
	ctx context.Context, credential IntegrationClientCredential, readRequest func() (ChannelRequest, error),
) (ReturnPreAdviceEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityReturnPreAdviceSubmission,
		customerAPIRequestKeys{}.DeriveReturnPreAdviceRequestKey)
	if err != nil {
		return ReturnPreAdviceEnvelope{}, err
	}
	return ReturnPreAdviceEnvelope{envelope: envelope}, nil
}

func (customerAPIRequestKeys) DeriveReturnPreAdviceRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixReturnPreAdvice, request)
}

// ReturnDispositionEnvelope 是回库处置货主口的来源信封。
type ReturnDispositionEnvelope struct{ envelope SourceEnvelope }

func (submission ReturnDispositionEnvelope) Envelope() SourceEnvelope { return submission.envelope }

// MintReturnDisposition 铸一次回库处置。准入两格都是 RETURN_DISPOSITION。
func (minter *CustomerClientMinter) MintReturnDisposition(
	ctx context.Context, credential IntegrationClientCredential, readRequest func() (ChannelRequest, error),
) (ReturnDispositionEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityReturnDisposition,
		customerAPIRequestKeys{}.DeriveReturnDispositionRequestKey)
	if err != nil {
		return ReturnDispositionEnvelope{}, err
	}
	return ReturnDispositionEnvelope{envelope: envelope}, nil
}

func (customerAPIRequestKeys) DeriveReturnDispositionRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixReturnDisposition, request)
}
