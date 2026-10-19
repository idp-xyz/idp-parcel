package accessidentity

import "context"

// DeliveryHandoverConfirmationEnvelope 是送站交接确认口的来源信封（ADR-0212 决定三）。按命令分型，理由同预报两口的信封：把它递给别的
// 口，编译期就走不通。
type DeliveryHandoverConfirmationEnvelope struct{ envelope SourceEnvelope }

func (confirmation DeliveryHandoverConfirmationEnvelope) Envelope() SourceEnvelope {
	return confirmation.envelope
}

// MintDeliveryHandoverConfirmation 铸一次送站交接确认（客户账户对一张送站交接单作结论）的来源信封。结论落的是生产事实——全部确认之后
// 交接单名下的实物会经重核形成关联收寄——所以准入范围在铸造侧判（ADR-0139 决定八）；次序与读请求的时机同预报两口。
func (minter *CustomerClientMinter) MintDeliveryHandoverConfirmation(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (DeliveryHandoverConfirmationEnvelope, error) {
	envelope, err := minter.mintCustomerCommand(ctx, credential, readRequest, CustomerCapabilityDeliveryHandoverConfirmation,
		customerAPIRequestKeys{}.DeriveDeliveryHandoverConfirmationRequestKey)
	if err != nil {
		return DeliveryHandoverConfirmationEnvelope{}, err
	}
	return DeliveryHandoverConfirmationEnvelope{envelope: envelope}, nil
}

// DeriveDeliveryHandoverConfirmationRequestKey 是首方族在送站交接确认口的推导口：取载荷里的接入请求标识，口径同提交口。结论的重放
// 不按它认，按（交接单，客户账户）认（ADR-0212 决定三「同内容重投答已有结果」）；它随结论落册，是「绑定已否认」要存的来源请求键。
func (customerAPIRequestKeys) DeriveDeliveryHandoverConfirmationRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return accessRequestIdentifierOf(request)
}
