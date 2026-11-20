package accessidentity

import "context"

// OutboundRequestSubmissionEnvelope 是出库请求货主口的来源信封。
type OutboundRequestSubmissionEnvelope struct{ envelope SourceEnvelope }

func (submission OutboundRequestSubmissionEnvelope) Envelope() SourceEnvelope {
	return submission.envelope
}

// MintOutboundRequestSubmission 铸一次出库请求提交。准入两格都是 OUTBOUND_REQUEST_SUBMISSION。
func (minter *CustomerClientMinter) MintOutboundRequestSubmission(
	ctx context.Context, credential IntegrationClientCredential, readRequest func() (ChannelRequest, error),
) (OutboundRequestSubmissionEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityOutboundRequestSubmission,
		customerAPIRequestKeys{}.DeriveOutboundRequestSubmissionRequestKey)
	if err != nil {
		return OutboundRequestSubmissionEnvelope{}, err
	}
	return OutboundRequestSubmissionEnvelope{envelope: envelope}, nil
}

func (customerAPIRequestKeys) DeriveOutboundRequestSubmissionRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixOutboundRequest, request)
}

// MintOutboundRequestSplit 铸一次出库请求拆单。准入词与提交相同，来源请求键用拆单前缀。
func (minter *CustomerClientMinter) MintOutboundRequestSplit(
	ctx context.Context, credential IntegrationClientCredential, readRequest func() (ChannelRequest, error),
) (OutboundRequestSubmissionEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityOutboundRequestSubmission,
		customerAPIRequestKeys{}.DeriveOutboundRequestSplitRequestKey)
	if err != nil {
		return OutboundRequestSubmissionEnvelope{}, err
	}
	return OutboundRequestSubmissionEnvelope{envelope: envelope}, nil
}

func (customerAPIRequestKeys) DeriveOutboundRequestSplitRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixOutboundSplit, request)
}
