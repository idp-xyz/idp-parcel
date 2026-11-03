package accessidentity

import "context"

// CustomerTrackingViewQueryEnvelope 是客户追踪两口（按包裹查视图、按运单号查委托下的包裹，UC-VE-008）经铸造的身份：租户与客户账户，
// 都取自客户渠道册上那一行绑定（ADR-0139 决定二；ADR-0003）。两口各要各的授予（一口一词），铸出的身份同形，所以共用一型。
//
// 它不是 SourceEnvelope：查阅不形成来源请求，没有来源请求键可推，借 seal 封一个就得给请求键编一个值；查阅也不该有一枚能递给
// 写口的信封。另立一型，把它递给任何写口编译期就走不通。字段不导出、包外没有构造函数，同 SourceEnvelope；包外仍写得出零值，
// 所以零值做成不可用，消费方先问 Minted，同 OperatorEnvelope。
type CustomerTrackingViewQueryEnvelope struct {
	tenantID          string
	customerAccountID string
}

func (envelope CustomerTrackingViewQueryEnvelope) TenantID() string { return envelope.tenantID }
func (envelope CustomerTrackingViewQueryEnvelope) CustomerAccountID() string {
	return envelope.customerAccountID
}

// Minted 答这个信封是不是铸出来的。
func (envelope CustomerTrackingViewQueryEnvelope) Minted() bool {
	return envelope.tenantID != "" && envelope.customerAccountID != ""
}

// MintTrackingViewQuery 铸一次客户追踪视图查阅的身份（UC-VE-008 主流程步骤 1）：校验令牌 → 查客户渠道册 → 核证书绑定 → 核这一口
// 的授予，次序与各格同 admit。不判准入范围：查阅不形成生产事实（ADR-0139 决定八「查阅口不判准入范围」）。
//
// 不读请求：查询键里请求能给的只有包裹定位，那由消费方在这一步过了之后自己读；客户账户只来自册上的绑定，请求里自报的账户
// 无处可递。
func (minter *CustomerClientMinter) MintTrackingViewQuery(
	ctx context.Context,
	credential IntegrationClientCredential,
) (CustomerTrackingViewQueryEnvelope, error) {
	registration, err := minter.admit(ctx, credential, customerClientRequest{capability: CustomerCapabilityTrackingViewQuery})
	if err != nil {
		return CustomerTrackingViewQueryEnvelope{}, err
	}
	return CustomerTrackingViewQueryEnvelope{
		tenantID:          registration.tenantID,
		customerAccountID: registration.customerAccountID,
	}, nil
}

// MintShipmentTrackingQuery 铸一次客户追踪按运单号查询的身份（ADR-0356 决定一）：次序与各格同 MintTrackingViewQuery，核的是这一口
// 自己的词 CustomerCapabilityShipmentTrackingQuery——只持按包裹查授予的客户端在这里答未授予。不判准入范围，理由同按包裹查。
func (minter *CustomerClientMinter) MintShipmentTrackingQuery(
	ctx context.Context,
	credential IntegrationClientCredential,
) (CustomerTrackingViewQueryEnvelope, error) {
	registration, err := minter.admit(ctx, credential, customerClientRequest{capability: CustomerCapabilityShipmentTrackingQuery})
	if err != nil {
		return CustomerTrackingViewQueryEnvelope{}, err
	}
	return CustomerTrackingViewQueryEnvelope{
		tenantID:          registration.tenantID,
		customerAccountID: registration.customerAccountID,
	}, nil
}
