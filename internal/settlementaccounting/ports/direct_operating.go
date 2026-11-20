package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 服务形态的线上词与 party-commercial 的 LabelChannelServiceForm / NetworkServiceForm 相同。
// 结算的领域与应用不 import 那个领域包，只认这两个字面；适配器把形态册上的词原样交回来。
const (
	ServiceFormLabelChannel   = "LABEL_CHANNEL_SERVICE"
	ServiceFormNetworkService = "NETWORK_SERVICE"
)

// OperatingBasisAdoptionStore 是一户一行的口径采用册。没找到就是没登记，不是两个布尔都为假。
type OperatingBasisAdoptionStore interface {
	FindOperatingBasisAdoption(ctx context.Context, tenant domain.TenantID) (domain.OperatingBasisAdoption, bool, error)
	SaveOperatingBasisAdoption(
		ctx context.Context,
		tenant domain.TenantID,
		adoption domain.OperatingBasisAdoption,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// DirectAmount 是一笔能进入成对的当前有效金额。PeriodStated 为假表示这笔金额自己没有账期
// （供应商预期成本行上没有账期列）。为真而 Period 空，表示它该有账期却对不齐。
type DirectAmount struct {
	Source       string
	Currency     string
	AmountMinor  int64
	Period       string
	PeriodStated bool
}

// ParcelDirectFacts 是一件包裹此刻能用来选料的事实，读口不在这里重算毛利。
// ServiceForm 空即形态不可观察。SharedCost 表示它正挂在集运单元或已接受订舱的班次上。
// NonUniqueSupplier 表示有一笔供应商金额的发生项成员不止这一件。
type ParcelDirectFacts struct {
	ServiceForm       string
	SharedCost        bool
	NonUniqueSupplier bool
	Estimates         []DirectAmount
	ExpectedCosts     []DirectAmount
	Receivables       []DirectAmount
	Payables          []DirectAmount
	Credits           []DirectAmount
}

// ParcelDirectFactsView 按包裹读直接成本选料要用的当前事实，并按费用、发生项、应付把触达的包裹找回来。
type ParcelDirectFactsView interface {
	LoadParcelDirectFacts(ctx context.Context, tenant domain.TenantID, parcel domain.ParcelReference) (ParcelDirectFacts, error)
	ParcelsOfCharge(ctx context.Context, tenant domain.TenantID, charge domain.CustomerChargeID) ([]domain.ParcelReference, error)
	ParcelsOfOccurrence(
		ctx context.Context,
		tenant domain.TenantID,
		occurrence domain.ChargeOccurrenceID,
		version domain.OccurrenceVersion,
	) ([]domain.ParcelReference, error)
	ParcelsOfPayable(ctx context.Context, tenant domain.TenantID, payable domain.PayableID) ([]domain.ParcelReference, error)
}

// DirectOperatingDispositionRecord 是一件包裹在某个已采用口径下答不出毛利的当前答案。
// 它不是经营结果快照：读口靠它把「判不出 / 成本不全 / 待判断」和「还没有结果」分开。
type DirectOperatingDispositionRecord struct {
	Scope       string
	Basis       string
	Disposition string
	RecordedAt  time.Time
}

// DirectOperatingDispositionStore 记下答不出毛利的当前答案。Save 会撤下同一范围同一口径上
// 已经写下的经营结果，避免一份只减了部分成本、或只剩客户侧的旧快照继续被读成完整毛利。
// Clear 只撤答案，不碰刚刚派生的快照。Forget 在两侧都没有时把答案和快照都拿掉。
type DirectOperatingDispositionStore interface {
	SaveDirectOperatingDisposition(
		ctx context.Context,
		tenant domain.TenantID,
		scope string,
		basis domain.OperatingBasis,
		disposition string,
		at time.Time,
	) error
	ClearDirectOperatingDisposition(ctx context.Context, tenant domain.TenantID, scope string, basis domain.OperatingBasis) error
	ForgetDirectOperatingAnswer(ctx context.Context, tenant domain.TenantID, scope string, basis domain.OperatingBasis) error
}

// OperatingBasisAdoptionAnswer 是读口对「这户采用了没有」的答案。Configured 为假即册上没有这行。
type OperatingBasisAdoptionAnswer struct {
	Configured bool
	Estimated  bool
	Confirmed  bool
}
