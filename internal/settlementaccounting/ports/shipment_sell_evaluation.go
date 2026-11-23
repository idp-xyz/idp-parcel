package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ShipmentBillingMomentAdoptionView 按（租户、结算账户）交回委托级计费时点那一行。册上没有这一户时第二个返回值为假，
// 调用方据此答未配置，不补一种默认时点。
type ShipmentBillingMomentAdoptionView interface {
	FindShipmentBillingMomentAdoption(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccountID,
	) (domain.ShipmentBillingMomentAdoption, bool, error)
}

// ShipmentBillingMomentAdoptionRegister 写下这一户采用的委托级时点。指向的结算账户还不在账户册上时，错误 errors.Is
// domain.ErrCatalogueTargetMissing。
type ShipmentBillingMomentAdoptionRegister interface {
	SaveShipmentBillingMomentAdoption(
		ctx context.Context,
		tenant domain.TenantID,
		adoption domain.ShipmentBillingMomentAdoption,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// ShipmentSellEvaluationRequestKey 是委托级请求册的身份键。
type ShipmentSellEvaluationRequestKey struct {
	TenantID domain.TenantID
	Request  domain.ShipmentSellEvaluationRequestID
}

// ShipmentSellEvaluationRequestRecord 是一次委托级销售评价请求越过提交边界留下的东西。
type ShipmentSellEvaluationRequestRecord struct {
	Key          ShipmentSellEvaluationRequestKey
	Request      domain.ShipmentSellEvaluationRequest
	RecordedAt   time.Time
	State        domain.SellEvaluationRequestState
	FormedCharge domain.CustomerChargeID
	StopCause    domain.EvaluationStopCause
}

// ShipmentSellEvaluationRequestRegistry 是委托级请求的登记面。写入代数与包裹那一路同一套。
type ShipmentSellEvaluationRequestRegistry interface {
	Save(ctx context.Context, record ShipmentSellEvaluationRequestRecord) (EvaluationRequestSaveOutcome, error)
	FindByNaturalKey(
		ctx context.Context,
		tenant domain.TenantID,
		key domain.ShipmentSellEvaluationRequestNaturalKey,
	) (ShipmentSellEvaluationRequestRecord, bool, error)
}

// ShipmentSellEvaluationRequestView 按铸造标识回读。计价与形成费用都只按信封上的标识来。
type ShipmentSellEvaluationRequestView interface {
	FindByID(ctx context.Context, key ShipmentSellEvaluationRequestKey) (ShipmentSellEvaluationRequestRecord, bool, error)
}

// ShipmentSellEvaluationRequestConclusionRecorder 在请求册上记一次结论。
type ShipmentSellEvaluationRequestConclusionRecorder interface {
	RecordConclusion(
		ctx context.Context,
		key ShipmentSellEvaluationRequestKey,
		conclusion domain.SellEvaluationRequestConclusion,
	) (SellEvaluationRequestConclusionEffect, error)
}

// ShipmentSellEvaluationRequestIdentityFactory 铸造委托级请求标识。与包裹那一路的铸造口分开。
type ShipmentSellEvaluationRequestIdentityFactory interface {
	MintShipmentSellEvaluationRequestID(ctx context.Context) (domain.ShipmentSellEvaluationRequestID, error)
}

// ShipmentSellEvaluationRequestHandoff 把委托级请求提交写入 Outbox，与请求行同一事务。
type ShipmentSellEvaluationRequestHandoff interface {
	HandOffShipmentSellEvaluationRequest(ctx context.Context, record ShipmentSellEvaluationRequestRecord) error
}

// ShipmentMemberIntake 是委托当前声明顺序里的一件：取消了的不再算有效；采用了有效网络收寄的带上责任起点与来源事实。
type ShipmentMemberIntake struct {
	Parcel              string
	Cancelled           bool
	Adopted             bool
	ResponsibilityStart time.Time
	Source              string
}

// ShipmentIntakePicture 是按一件包裹反查到的委托：当前声明顺序上的各件，以及这一版申报的服务产品（可空）。
type ShipmentIntakePicture struct {
	Shipment       string
	Members        []ShipmentMemberIntake
	ServiceProduct string
}

// ShipmentIntakePictureView 按包裹反查它此刻所属的已接受委托，并带上各件的收寄采用。读不到委托时第二个返回值为假，
// 调用方不得自己编一份件清单。
type ShipmentIntakePictureView interface {
	LoadByParcel(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.ParcelReference,
	) (ShipmentIntakePicture, bool, error)
}

// SellPriceCardPathOutcome 是这一刻这个范围上在用销售卡的聚合方式。冲突不挑一张。
type SellPriceCardPathOutcome uint8

const (
	SellPriceCardPathOutcomeInvalid SellPriceCardPathOutcome = iota
	SellPriceCardNotConfigured
	SellPriceCardInForce
	SellPriceCardConflict
)

func (outcome SellPriceCardPathOutcome) String() string {
	switch outcome {
	case SellPriceCardNotConfigured:
		return "NOT_CONFIGURED"
	case SellPriceCardInForce:
		return "IN_FORCE"
	case SellPriceCardConflict:
		return "CONFLICT"
	default:
		return ""
	}
}

// SellPriceCardPath 在恰好一张卡在用时带上它的聚合方式。聚合方式的词与计价的 AggregationMode 同字。
type SellPriceCardPath struct {
	Outcome     SellPriceCardPathOutcome
	Aggregation string
}

// SellPriceCardPathView 按（租户、范围、时点、申报的服务产品）看这一刻在用的销售·客户费用价卡是逐包裹还是逐委托。
type SellPriceCardPathView interface {
	ResolveSellPriceCardPath(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.EstimatePricingScope,
		at time.Time,
		serviceProduct string,
	) (SellPriceCardPath, error)
}
