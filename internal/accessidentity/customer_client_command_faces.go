package accessidentity

import "context"

// 本文件是首方客户命令五口（撤回、取消包裹、资料修订、补充、索赔）共用的铸造底座（票 first-party-api/06；ADR-0310）。
// 只到「封好的来源信封」为止：领域译命令归各口换上首方 Intake 的那一票（07 撤回、08 补充、09 索赔、10 资料修订、11 取消）。
// 写法照送站预报两口：一口一词、一个铸造方法、按命令分型的信封，生产写在铸造侧判准入范围。

// ParcelCancellationEnvelope、SourceDataAmendmentEnvelope、SupplementEnvelope 与 ClaimEnvelope 是余下四口的来源信封。撤回沿用已有的
// WithdrawalEnvelope。各口分型，理由同 SubmissionEnvelope：把取消的信封递给补充口，编译期就走不通。
type ParcelCancellationEnvelope struct{ envelope SourceEnvelope }

func (cancellation ParcelCancellationEnvelope) Envelope() SourceEnvelope {
	return cancellation.envelope
}

type SourceDataAmendmentEnvelope struct{ envelope SourceEnvelope }

func (amendment SourceDataAmendmentEnvelope) Envelope() SourceEnvelope { return amendment.envelope }

type SupplementEnvelope struct{ envelope SourceEnvelope }

func (supplement SupplementEnvelope) Envelope() SourceEnvelope { return supplement.envelope }

type ClaimEnvelope struct{ envelope SourceEnvelope }

func (claim ClaimEnvelope) Envelope() SourceEnvelope { return claim.envelope }

// MintWithdrawal 铸一次委托撤回**自身**的来源信封（不是被撤那份提交的身份：被撤的由 Intake 按客户指名的原提交接入请求标识，
// 带着信封里的账户去找）。次序与读请求的时机同 MintSubmission。
func (minter *CustomerClientMinter) MintWithdrawal(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (WithdrawalEnvelope, error) {
	envelope, err := minter.mintCustomerCommand(ctx, credential, readRequest, CustomerCapabilityShipmentRequestWithdrawal,
		customerAPIRequestKeys{}.DeriveWithdrawalRequestKey)
	if err != nil {
		return WithdrawalEnvelope{}, err
	}
	return WithdrawalEnvelope{envelope: envelope}, nil
}

// MintParcelCancellation 铸一次取消包裹的来源信封。取消授权规则是租户取值（ADR-0310 取舍 C）：铸造侧只判授予与准入范围，
// 谁能在哪个阶段取消归编排的授权缝，没登记就答未配置。
func (minter *CustomerClientMinter) MintParcelCancellation(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (ParcelCancellationEnvelope, error) {
	envelope, err := minter.mintCustomerCommand(ctx, credential, readRequest, CustomerCapabilityParcelCancellation,
		customerAPIRequestKeys{}.DeriveParcelCancellationRequestKey)
	if err != nil {
		return ParcelCancellationEnvelope{}, err
	}
	return ParcelCancellationEnvelope{envelope: envelope}, nil
}

// MintSourceDataAmendment 铸一次客户资料修订的来源信封。
func (minter *CustomerClientMinter) MintSourceDataAmendment(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (SourceDataAmendmentEnvelope, error) {
	envelope, err := minter.mintCustomerCommand(ctx, credential, readRequest, CustomerCapabilitySourceDataAmendment,
		customerAPIRequestKeys{}.DeriveSourceDataAmendmentRequestKey)
	if err != nil {
		return SourceDataAmendmentEnvelope{}, err
	}
	return SourceDataAmendmentEnvelope{envelope: envelope}, nil
}

// MintSupplement 铸一次委托补充的来源信封。
func (minter *CustomerClientMinter) MintSupplement(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (SupplementEnvelope, error) {
	envelope, err := minter.mintCustomerCommand(ctx, credential, readRequest, CustomerCapabilityShipmentRequestSupplement,
		customerAPIRequestKeys{}.DeriveSupplementRequestKey)
	if err != nil {
		return SupplementEnvelope{}, err
	}
	return SupplementEnvelope{envelope: envelope}, nil
}

// MintClaim 铸一次索赔提交的来源信封。索赔的唯一性以（账户、批次、项）为自然键（ADR-0310 取舍 F），信封里的来源请求键
// 只管传输重放。
func (minter *CustomerClientMinter) MintClaim(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (ClaimEnvelope, error) {
	envelope, err := minter.mintCustomerCommand(ctx, credential, readRequest, CustomerCapabilityClaimSubmission,
		customerAPIRequestKeys{}.DeriveClaimRequestKey)
	if err != nil {
		return ClaimEnvelope{}, err
	}
	return ClaimEnvelope{envelope: envelope}, nil
}

// 四个推导与 DeriveWithdrawalRequestKey 同一口径（ADR-0310 取舍 A），不进 RequestKeyDerivation 接口：那个接口是租户既有渠道适配
// 族也要实现的，把首方才有的口加进去，会让那一族凭空多出四个「未决」方法。

func (customerAPIRequestKeys) DeriveParcelCancellationRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKey(requestKeyPrefixParcelCancellation, request)
}

func (customerAPIRequestKeys) DeriveSourceDataAmendmentRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKey(requestKeyPrefixSourceDataAmend, request)
}

func (customerAPIRequestKeys) DeriveSupplementRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKey(requestKeyPrefixSupplement, request)
}

func (customerAPIRequestKeys) DeriveClaimRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKey(requestKeyPrefixClaim, request)
}
