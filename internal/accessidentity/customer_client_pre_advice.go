package accessidentity

import (
	"context"
	"fmt"
	"slices"
)

// CustomerCommandAdmissionCapability 是首方客户族生产写口在治理登记册上的能力词，事实类型逐口取那一口的客户面能力词（ADR-0180
// 决定四末项「一个能力词，事实类型逐口取」；ADR-0139 决定八）。取法照集成客户端族：那一族的授予按事实类型登、没有能力面，于是另立
// EXTERNAL_FACT 一词、事实类型取授予的单位；首方族的授予单位是客户面能力，同样另立一词、事实类型取能力词。准入要求里不直接拿能力词
// 顶能力格：那样每一口在治理登记册上就是一个独立的能力，阶段治理无从按「客户业务命令面」一并登区间。
const CustomerCommandAdmissionCapability = "CUSTOMER_COMMAND"

// customerCapabilitiesAdmittedAtMint 是在铸造侧判准入范围的客户面能力（ADR-0139 决定八）。委托提交不在其中：它的生产归属由
// UC-PS-001 编排自己判。准入范围对照收哪几个词（checkAdmissionRequirement）、铸造方法判哪一对准入要求，都只从这里取：登得进对照的
// 词与铸造时问的词分成两份，改了一份，登在旧词下的区间就对另一边无声失效。
var customerCapabilitiesAdmittedAtMint = []CustomerCapability{
	CustomerCapabilityDeliveryPreAdviceSubmission,
	CustomerCapabilityDeliveryPreAdviceWithdrawal,
}

// customerCommandAdmission 交回这一口在铸造侧判的准入要求；不在铸造侧判的口交回 nil。
func customerCommandAdmission(capability CustomerCapability) *AdmissionRequirement {
	if !slices.Contains(customerCapabilitiesAdmittedAtMint, capability) {
		return nil
	}
	return &AdmissionRequirement{Capability: CustomerCommandAdmissionCapability, FactKind: capability.String()}
}

// DeliveryPreAdviceSubmissionEnvelope 与 DeliveryPreAdviceWithdrawalEnvelope 是送站预报两口的来源信封（ADR-0212 决定二）。按命令分型、
// 也与 SubmissionEnvelope、WithdrawalEnvelope 分型，理由同那两个类型的注释：把预报的信封递给委托提交口、把撤回的递给提交口，编译期
// 就走不通。
type DeliveryPreAdviceSubmissionEnvelope struct{ envelope SourceEnvelope }

func (submission DeliveryPreAdviceSubmissionEnvelope) Envelope() SourceEnvelope {
	return submission.envelope
}

type DeliveryPreAdviceWithdrawalEnvelope struct{ envelope SourceEnvelope }

func (withdrawal DeliveryPreAdviceWithdrawalEnvelope) Envelope() SourceEnvelope {
	return withdrawal.envelope
}

// MintDeliveryPreAdviceSubmission 铸一次送站预报提交的来源信封。预报是生产写，准入范围在铸造侧判（ADR-0139 决定八）；其余次序与
// 读请求的时机同 MintSubmission。
func (minter *CustomerClientMinter) MintDeliveryPreAdviceSubmission(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (DeliveryPreAdviceSubmissionEnvelope, error) {
	envelope, err := minter.mintCustomerCommand(ctx, credential, readRequest, CustomerCapabilityDeliveryPreAdviceSubmission,
		customerAPIRequestKeys{}.DeriveDeliveryPreAdviceSubmissionRequestKey)
	if err != nil {
		return DeliveryPreAdviceSubmissionEnvelope{}, err
	}
	return DeliveryPreAdviceSubmissionEnvelope{envelope: envelope}, nil
}

// MintDeliveryPreAdviceWithdrawal 铸一次送站预报撤回**自身**的来源信封，不是被撤那份预报的身份——那一份由节点作业按客户指名的预报
// 标识去取，只撤信封里那个客户账户的（UC-NO-006「权限、隔离与审计」）。
func (minter *CustomerClientMinter) MintDeliveryPreAdviceWithdrawal(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (DeliveryPreAdviceWithdrawalEnvelope, error) {
	envelope, err := minter.mintCustomerCommand(ctx, credential, readRequest, CustomerCapabilityDeliveryPreAdviceWithdrawal,
		customerAPIRequestKeys{}.DeriveDeliveryPreAdviceWithdrawalRequestKey)
	if err != nil {
		return DeliveryPreAdviceWithdrawalEnvelope{}, err
	}
	return DeliveryPreAdviceWithdrawalEnvelope{envelope: envelope}, nil
}

// mintCustomerCommand 是在铸造侧判准入的客户面写口共用的一段：过 admit（带这一口的准入要求）→ 读请求 → 推导来源请求键 → seal。
// 请求在认证与准入都过了之后才读，理由同 MintSubmission。
func (minter *CustomerClientMinter) mintCustomerCommand(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
	capability CustomerCapability,
	derive func(context.Context, ChannelRequest) (string, error),
) (SourceEnvelope, error) {
	if readRequest == nil {
		return SourceEnvelope{}, ErrNilDependency
	}
	registration, err := minter.admit(ctx, credential, customerClientRequest{
		capability: capability,
		admission:  customerCommandAdmission(capability),
	})
	if err != nil {
		return SourceEnvelope{}, err
	}
	request, err := readRequest()
	if err != nil {
		return SourceEnvelope{}, err
	}
	requestKey, err := derive(ctx, request)
	if err != nil {
		return SourceEnvelope{}, fmt.Errorf("mint customer %s envelope: %w", capability, err)
	}
	return seal(registration, requestKey)
}

// DeriveDeliveryPreAdviceSubmissionRequestKey 与 DeriveDeliveryPreAdviceWithdrawalRequestKey 是首方族在送站预报两口的推导口：都取载荷里
// 的接入请求标识，口径同提交口（去两端空白，此外一字不动）。
//
// 两口取同一格，不会让撤回被判成提交的重放——RequestKeyDerivation 头注担心的正是那件事：节点作业那一侧，预报与预报的撤回各在自己
// 那本册里按（租户，来源，来源请求键）认重放，同一接入请求标识先交预报、再撤回，落在两个判重空间里，是两次来源请求。仍分两个方法，
// 是让哪一口的推导日后要变时只动那一口。与委托撤回的推导（DeriveWithdrawalRequestKey）无关：那一口的判重空间与委托提交怎么分，
// 归委托撤回换上首方 Intake 的那一票。
func (customerAPIRequestKeys) DeriveDeliveryPreAdviceSubmissionRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return accessRequestIdentifierOf(request)
}

func (customerAPIRequestKeys) DeriveDeliveryPreAdviceWithdrawalRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return accessRequestIdentifierOf(request)
}
