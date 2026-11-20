package accessidentity

import (
	"context"
	"fmt"
)

// warehouseCustomerWriteAdmission 是仓配货主写口在铸造侧判的准入要求（ADR-0384 决定八）。能力词与事实类型词都是该口
// 自己的字面量，不用 CUSTOMER_COMMAND。不在这些口里的能力交回 nil，调用方不得把它当成「这一口不判准入」。
func warehouseCustomerWriteAdmission(capability CustomerCapability) *AdmissionRequirement {
	switch capability {
	case CustomerCapabilityGoodsMaintenance,
		CustomerCapabilityInboundPreAdviceSubmission,
		CustomerCapabilityInboundPreAdviceArrivalNotice,
		CustomerCapabilityInboundPreAdviceWithdrawal,
		CustomerCapabilityReceiptDiscrepancyConclusion,
		CustomerCapabilityProcessingInstructionSubmission,
		CustomerCapabilityProcessingInstructionWithdrawal,
		CustomerCapabilityRelabelInstructionSubmission,
		CustomerCapabilityRelabelInstructionWithdrawal,
		CustomerCapabilityOutboundRequestSubmission:
		requirement := AdmissionRequirement{Capability: capability.String(), FactKind: capability.String()}
		return &requirement
	default:
		return nil
	}
}

// InboundPreAdviceSubmissionEnvelope、InboundPreAdviceArrivalNoticeEnvelope 与 InboundPreAdviceWithdrawalEnvelope 是入库预报三个货主
// 写口的来源信封。按口分型，理由同送站预报两口：把提交的信封递给撤回口，编译期就走不通。
type InboundPreAdviceSubmissionEnvelope struct{ envelope SourceEnvelope }

func (submission InboundPreAdviceSubmissionEnvelope) Envelope() SourceEnvelope {
	return submission.envelope
}

type InboundPreAdviceArrivalNoticeEnvelope struct{ envelope SourceEnvelope }

func (notice InboundPreAdviceArrivalNoticeEnvelope) Envelope() SourceEnvelope {
	return notice.envelope
}

type InboundPreAdviceWithdrawalEnvelope struct{ envelope SourceEnvelope }

func (withdrawal InboundPreAdviceWithdrawalEnvelope) Envelope() SourceEnvelope {
	return withdrawal.envelope
}

// WarehouseReadEnvelope 是仓配查阅的身份：租户与货主客户账户，都取自客户渠道册上的绑定。它不是 SourceEnvelope：查阅不形成来源
// 请求，没有来源请求键可推。WAREHOUSE_READ 不判准入。
type WarehouseReadEnvelope struct {
	tenantID          string
	customerAccountID string
}

func (envelope WarehouseReadEnvelope) TenantID() string          { return envelope.tenantID }
func (envelope WarehouseReadEnvelope) CustomerAccountID() string { return envelope.customerAccountID }

// Minted 答这个信封是不是铸出来的。
func (envelope WarehouseReadEnvelope) Minted() bool {
	return envelope.tenantID != "" && envelope.customerAccountID != ""
}

// MintInboundPreAdviceSubmission 铸一次入库预报提交的来源信封。准入两格都是这一口的字面量，不走 mintCustomerCommand 那一对
// CUSTOMER_COMMAND。
func (minter *CustomerClientMinter) MintInboundPreAdviceSubmission(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (InboundPreAdviceSubmissionEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityInboundPreAdviceSubmission,
		customerAPIRequestKeys{}.DeriveInboundPreAdviceSubmissionRequestKey)
	if err != nil {
		return InboundPreAdviceSubmissionEnvelope{}, err
	}
	return InboundPreAdviceSubmissionEnvelope{envelope: envelope}, nil
}

// MintInboundPreAdviceArrivalNotice 铸一次告知更新的来源信封。被更新的那份预报由仓配按客户指名的预报标识去取，只动信封里那个货主的。
func (minter *CustomerClientMinter) MintInboundPreAdviceArrivalNotice(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (InboundPreAdviceArrivalNoticeEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityInboundPreAdviceArrivalNotice,
		customerAPIRequestKeys{}.DeriveInboundPreAdviceArrivalNoticeRequestKey)
	if err != nil {
		return InboundPreAdviceArrivalNoticeEnvelope{}, err
	}
	return InboundPreAdviceArrivalNoticeEnvelope{envelope: envelope}, nil
}

// MintInboundPreAdviceWithdrawal 铸一次撤回自身的来源信封，不是被撤那份预报的身份。
func (minter *CustomerClientMinter) MintInboundPreAdviceWithdrawal(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (InboundPreAdviceWithdrawalEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityInboundPreAdviceWithdrawal,
		customerAPIRequestKeys{}.DeriveInboundPreAdviceWithdrawalRequestKey)
	if err != nil {
		return InboundPreAdviceWithdrawalEnvelope{}, err
	}
	return InboundPreAdviceWithdrawalEnvelope{envelope: envelope}, nil
}

// MintWarehouseRead 铸一次仓配查阅的身份。不读请求，也不判准入：查阅不形成生产事实。
func (minter *CustomerClientMinter) MintWarehouseRead(
	ctx context.Context,
	credential IntegrationClientCredential,
) (WarehouseReadEnvelope, error) {
	registration, err := minter.admit(ctx, credential, customerClientRequest{capability: CustomerCapabilityWarehouseRead})
	if err != nil {
		return WarehouseReadEnvelope{}, err
	}
	return WarehouseReadEnvelope{
		tenantID:          registration.tenantID,
		customerAccountID: registration.customerAccountID,
	}, nil
}

// mintWarehouseCustomerWrite 是入库预报三个货主写口共用的一段。它不调用 mintCustomerCommand：那一个函数交回的准入词是
// CUSTOMER_COMMAND 加能力词，这三口两格都要是该口自己的字面量。
func (minter *CustomerClientMinter) mintWarehouseCustomerWrite(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
	capability CustomerCapability,
	derive func(context.Context, ChannelRequest) (string, error),
) (SourceEnvelope, error) {
	if readRequest == nil {
		return SourceEnvelope{}, ErrNilDependency
	}
	admission := warehouseCustomerWriteAdmission(capability)
	if admission == nil {
		return SourceEnvelope{}, fmt.Errorf("mint warehouse customer %s: capability is not a warehouse customer write", capability)
	}
	registration, err := minter.admit(ctx, credential, customerClientRequest{
		capability: capability,
		admission:  admission,
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
		return SourceEnvelope{}, fmt.Errorf("mint warehouse customer %s envelope: %w", capability, err)
	}
	return seal(registration, requestKey)
}

// DeriveInboundPreAdviceSubmissionRequestKey、DeriveInboundPreAdviceArrivalNoticeRequestKey 与
// DeriveInboundPreAdviceWithdrawalRequestKey 是入库预报三口的推导口：来源请求键 = 这一口的前缀 + 去两端空白的接入请求标识。
// 标识以任一保留前缀开头则推不出键，然后再加上自己的前缀。
func (customerAPIRequestKeys) DeriveInboundPreAdviceSubmissionRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixInboundPreAdvice, request)
}

func (customerAPIRequestKeys) DeriveInboundPreAdviceArrivalNoticeRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixInboundArrivalNotice, request)
}

func (customerAPIRequestKeys) DeriveInboundPreAdviceWithdrawalRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixInboundWithdrawal, request)
}

// ReceiptDiscrepancyConclusionEnvelope 是收货差异结论口的来源信封。来源请求键是 `receipt-discrepancy:` 加去两端空白的接入请求标识。
type ReceiptDiscrepancyConclusionEnvelope struct{ envelope SourceEnvelope }

func (conclusion ReceiptDiscrepancyConclusionEnvelope) Envelope() SourceEnvelope {
	return conclusion.envelope
}

// MintReceiptDiscrepancyConclusion 铸一次收货差异结论的来源信封。准入两格都是这一口的字面量。
func (minter *CustomerClientMinter) MintReceiptDiscrepancyConclusion(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (ReceiptDiscrepancyConclusionEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityReceiptDiscrepancyConclusion,
		customerAPIRequestKeys{}.DeriveReceiptDiscrepancyConclusionRequestKey)
	if err != nil {
		return ReceiptDiscrepancyConclusionEnvelope{}, err
	}
	return ReceiptDiscrepancyConclusionEnvelope{envelope: envelope}, nil
}

// DeriveReceiptDiscrepancyConclusionRequestKey 是收货差异结论口的推导：来源请求键 = `receipt-discrepancy:` + 去两端空白的接入请求标识。
func (customerAPIRequestKeys) DeriveReceiptDiscrepancyConclusionRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixReceiptDiscrepancy, request)
}

// GoodsMaintenanceEnvelope 是货主商品维护口（`/v1/warehouse/goods`）的来源信封。
// 来源请求键是 `goods:` 加去两端空白的接入请求标识。
type GoodsMaintenanceEnvelope struct{ envelope SourceEnvelope }

func (goods GoodsMaintenanceEnvelope) Envelope() SourceEnvelope {
	return goods.envelope
}

// MintGoodsMaintenance 铸一次商品维护的来源信封。准入两格都是 GOODS_MAINTENANCE，不走 CUSTOMER_COMMAND。
func (minter *CustomerClientMinter) MintGoodsMaintenance(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (GoodsMaintenanceEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityGoodsMaintenance,
		customerAPIRequestKeys{}.DeriveGoodsMaintenanceRequestKey)
	if err != nil {
		return GoodsMaintenanceEnvelope{}, err
	}
	return GoodsMaintenanceEnvelope{envelope: envelope}, nil
}

// DeriveGoodsMaintenanceRequestKey 是货主商品维护口的推导：来源请求键 = `goods:` + 去两端空白的接入请求标识。
func (customerAPIRequestKeys) DeriveGoodsMaintenanceRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixGoods, request)
}
