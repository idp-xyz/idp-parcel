package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidFundsFact = errors.New("settlement accounting: invalid external funds fact")
	// ErrUnfundableFact：付款失败事实不形成收款或核销（AT-SA-111）——它是引用，不是钱。
	ErrUnfundableFact      = errors.New("settlement accounting: the funds fact cannot fund a mapping")
	ErrInvalidFundsMapping = errors.New("settlement accounting: invalid funds mapping")
	ErrInvalidApplication  = errors.New("settlement accounting: invalid settlement application")
	// ErrApplicationImbalance：分配合计必须等于本次核销使用的真实付款金额并不超过事实
	// 金额——金额严格守恒（AT-SA-106），差一分都不是「约等于」。
	ErrApplicationImbalance = errors.New("settlement accounting: allocations do not conserve the applied amount")
	// ErrCrossCurrencyApplication：付款币种与目标币种不同且无有效换算时保持待判断，
	// 不使用当前汇率（AT-SA-110）。
	ErrCrossCurrencyApplication = errors.New("settlement accounting: cross-currency application needs a conversion basis")
	ErrApplicationReversed      = errors.New("settlement accounting: the application is already reversed")
)

// FundsFactReference 指名一条银行/支付系统拥有的外部资金事实。本上下文只引用——
// 真实到账、支付清算和银行资金事实的所有权在外部（CONTEXT）。
type FundsFactReference struct{ requiredValue }

func NewFundsFactReference(value string) (FundsFactReference, error) {
	required, err := newRequiredValue("funds fact reference", value)
	return FundsFactReference{required}, err
}

// FundsSourceRegistrationReference 指名已登记的外部事实来源。来源未经登记的事实不
// 建立本地资金引用（AT-SA-102）。
type FundsSourceRegistrationReference struct{ requiredValue }

func NewFundsSourceRegistrationReference(value string) (FundsSourceRegistrationReference, error) {
	required, err := newRequiredValue("funds source registration reference", value)
	return FundsSourceRegistrationReference{required}, err
}

// FundsFactVersion 是外部事实引用的版本：金额被更正时保留原版本，按新有效版本重算
// （AT-SA-114）。
type FundsFactVersion struct{ requiredValue }

func NewFundsFactVersion(value string) (FundsFactVersion, error) {
	required, err := newRequiredValue("funds fact version", value)
	return FundsFactVersion{required}, err
}

// FundsFactKind 是外部资金事实的封闭三值：确认到账、付款失败、资金退回。通知不在
// 其中——运营方收到付款通知但财务未确认时保持待确认，不更新已入账（AT-SA-115）。
type FundsFactKind uint8

const (
	FundsFactKindInvalid FundsFactKind = iota
	FundsReceiptConfirmed
	FundsPaymentFailed
	FundsReturned
)

func (kind FundsFactKind) valid() bool {
	return kind >= FundsReceiptConfirmed && kind <= FundsReturned
}

func (kind FundsFactKind) String() string {
	switch kind {
	case FundsReceiptConfirmed:
		return "RECEIPT_CONFIRMED"
	case FundsPaymentFailed:
		return "PAYMENT_FAILED"
	case FundsReturned:
		return "FUNDS_RETURNED"
	default:
		return ""
	}
}

// ExternalFundsFactSpec 是采用一条外部资金事实引用所需的全部输入。
type ExternalFundsFactSpec struct {
	Fact        FundsFactReference
	Source      FundsSourceRegistrationReference
	Kind        FundsFactKind
	Currency    CurrencyCode
	AmountMinor int64
	Version     FundsFactVersion
	OccurredAt  time.Time
}

// ExternalFundsFact 是对外部真实收付事实的只读引用（AT-SA-101：采用只形成引用和
// 待匹配入口，不直接成为已核销）。类型上没有余额或已结清字段——引用变不成钱，核销
// 是另一个显式判断。
type ExternalFundsFact struct {
	fact        FundsFactReference
	source      FundsSourceRegistrationReference
	kind        FundsFactKind
	currency    CurrencyCode
	amountMinor int64
	version     FundsFactVersion
	occurredAt  time.Time
	corrects    FundsFactVersion
	correctedAt time.Time
}

func AdoptExternalFundsFact(spec ExternalFundsFactSpec) (ExternalFundsFact, error) {
	if !spec.Fact.valid() ||
		!spec.Source.valid() ||
		!spec.Kind.valid() ||
		!spec.Currency.valid() ||
		spec.AmountMinor <= 0 ||
		!spec.Version.valid() ||
		spec.OccurredAt.IsZero() {
		return ExternalFundsFact{}, ErrInvalidFundsFact
	}
	return ExternalFundsFact{
		fact:        spec.Fact,
		source:      spec.Source,
		kind:        spec.Kind,
		currency:    spec.Currency,
		amountMinor: spec.AmountMinor,
		version:     spec.Version,
		occurredAt:  spec.OccurredAt.UTC(),
	}, nil
}

func (fact ExternalFundsFact) Fact() FundsFactReference {
	return fact.fact
}

func (fact ExternalFundsFact) Source() FundsSourceRegistrationReference {
	return fact.source
}

func (fact ExternalFundsFact) Kind() FundsFactKind {
	return fact.kind
}

func (fact ExternalFundsFact) Amount() (CurrencyCode, int64) {
	return fact.currency, fact.amountMinor
}

func (fact ExternalFundsFact) Version() FundsFactVersion {
	return fact.version
}

func (fact ExternalFundsFact) OccurredAt() time.Time {
	return fact.occurredAt
}

// Corrects 交回本版本更正的前一版本（若本版本由更正产生）。
func (fact ExternalFundsFact) Corrects() (FundsFactVersion, bool) {
	if !fact.corrects.valid() {
		return FundsFactVersion{}, false
	}
	return fact.corrects, true
}

func (fact ExternalFundsFact) CorrectedAt() (time.Time, bool) {
	if fact.correctedAt.IsZero() {
		return time.Time{}, false
	}
	return fact.correctedAt, true
}

// CorrectAmount 依据外部更正形成新版本引用：保留原版本（值语义），新版本回指前身；
// 差额与核销的重算随新有效版本另行进行（AT-SA-114）。
func (fact ExternalFundsFact) CorrectAmount(
	amountMinor int64,
	version FundsFactVersion,
	correctedAt time.Time,
) (ExternalFundsFact, error) {
	if !fact.version.valid() {
		return ExternalFundsFact{}, ErrInvalidFundsFact
	}
	if amountMinor <= 0 || !version.valid() || correctedAt.IsZero() {
		return ExternalFundsFact{}, ErrInvalidFundsFact
	}
	if version == fact.version {
		return ExternalFundsFact{}, ErrInvalidFundsFact
	}
	corrected := fact
	corrected.amountMinor = amountMinor
	corrected.corrects = fact.version
	corrected.version = version
	corrected.correctedAt = correctedAt.UTC()
	return corrected, nil
}

// SettlementTargetKind 是资金可映射目标的封闭三值：客户对账单、供应商审核应付、
// 供应商费用贷项。费用、赔付金额与追偿认可本体不在其中——映射与核销改不了那些金额
// （AT-SA-116/121）。
type SettlementTargetKind uint8

const (
	SettlementTargetKindInvalid SettlementTargetKind = iota
	TargetStatement
	TargetPayable
	TargetCreditNote
)

func (kind SettlementTargetKind) valid() bool {
	return kind >= TargetStatement && kind <= TargetCreditNote
}

func (kind SettlementTargetKind) String() string {
	switch kind {
	case TargetStatement:
		return "STATEMENT"
	case TargetPayable:
		return "PAYABLE"
	case TargetCreditNote:
		return "CREDIT_NOTE"
	default:
		return ""
	}
}

// SettlementTargetReference 指名映射目标的身份（单号、应付、贷项）。
type SettlementTargetReference struct{ requiredValue }

func NewSettlementTargetReference(value string) (SettlementTargetReference, error) {
	required, err := newRequiredValue("settlement target reference", value)
	return SettlementTargetReference{required}, err
}

// MappingReference 指名一条运营映射。
type MappingReference struct{ requiredValue }

func NewMappingReference(value string) (MappingReference, error) {
	required, err := newRequiredValue("mapping reference", value)
	return MappingReference{required}, err
}

// MappingBasisReference 指名映射成立的显式依据：付款指示、唯一精确匹配证据或授权
// 人工决定。金额相同、同一客户或同一时间都不单独证明映射（UC-SA-005 匹配纪律）——
// 依据必填就是这句话的形状。
type MappingBasisReference struct{ requiredValue }

func NewMappingBasisReference(value string) (MappingBasisReference, error) {
	required, err := newRequiredValue("mapping basis reference", value)
	return MappingBasisReference{required}, err
}

// FundsMapping 是外部资金事实与结算目标之间的运营映射。映射不是核销：它回答「这笔
// 钱与哪个结算身份相关」，金额分配由核销另行显式判断。
type FundsMapping struct {
	mapping    MappingReference
	fact       FundsFactReference
	targetKind SettlementTargetKind
	target     SettlementTargetReference
	basis      MappingBasisReference
	mappedAt   time.Time
}

// MapFundsToTarget 建立映射。付款失败事实不形成收款或核销（AT-SA-111）；资金退回
// 可以映射（返款核销到贷项，AT-SA-170）。
func MapFundsToTarget(
	fact ExternalFundsFact,
	mapping MappingReference,
	targetKind SettlementTargetKind,
	target SettlementTargetReference,
	basis MappingBasisReference,
	mappedAt time.Time,
) (FundsMapping, error) {
	if !fact.fact.valid() || !mapping.valid() || !targetKind.valid() || !target.valid() || mappedAt.IsZero() {
		return FundsMapping{}, ErrInvalidFundsMapping
	}
	if fact.kind == FundsPaymentFailed {
		return FundsMapping{}, ErrUnfundableFact
	}
	if !basis.valid() {
		return FundsMapping{}, ErrInvalidFundsMapping
	}
	return FundsMapping{
		mapping:    mapping,
		fact:       fact.fact,
		targetKind: targetKind,
		target:     target,
		basis:      basis,
		mappedAt:   mappedAt.UTC(),
	}, nil
}

func (mapping FundsMapping) Mapping() MappingReference {
	return mapping.mapping
}

func (mapping FundsMapping) Fact() FundsFactReference {
	return mapping.fact
}

func (mapping FundsMapping) TargetKind() SettlementTargetKind {
	return mapping.targetKind
}

func (mapping FundsMapping) Target() SettlementTargetReference {
	return mapping.target
}

func (mapping FundsMapping) Basis() MappingBasisReference {
	return mapping.basis
}

func (mapping FundsMapping) MappedAt() time.Time {
	return mapping.mappedAt
}

// AllocationDirection 是核销分配的借贷方向：指向应付/对账单的正向消耗付款，指向贷项
// 的反向加回（AT-SA-167 的带方向分配）。
type AllocationDirection uint8

const (
	AllocationDirectionInvalid AllocationDirection = iota
	AllocationDebit
	AllocationCredit
)

func (direction AllocationDirection) valid() bool {
	return direction == AllocationDebit || direction == AllocationCredit
}

func (direction AllocationDirection) String() string {
	switch direction {
	case AllocationDebit:
		return "DEBIT"
	case AllocationCredit:
		return "CREDIT"
	default:
		return ""
	}
}

// SettlementAllocation 是核销内一条带方向的金额分配。每条分配必须指名它沿用的映射
// ——核销只沿既有映射走，不凭金额巧合直接分钱。
type SettlementAllocation struct {
	Mapping     MappingReference
	TargetKind  SettlementTargetKind
	Target      SettlementTargetReference
	Direction   AllocationDirection
	AmountMinor int64
}

// ApplicationReference 指名一次核销。
type ApplicationReference struct{ requiredValue }

func NewApplicationReference(value string) (ApplicationReference, error) {
	required, err := newRequiredValue("application reference", value)
	return ApplicationReference{required}, err
}

// ApplicationBasisReference 指名核销判断的依据（付款指示、唯一匹配证据、抵销依据或
// 授权决定），以及撤销时的撤销依据。
type ApplicationBasisReference struct{ requiredValue }

func NewApplicationBasisReference(value string) (ApplicationBasisReference, error) {
	required, err := newRequiredValue("application basis reference", value)
	return ApplicationBasisReference{required}, err
}

// SettlementApplication 是把一笔真实资金显式分配到结算目标的核销判断（UC-SA-005）。
// 核销不是自动推导：分配、方向与依据都在构造期给出并守恒校验；它不改写费用、审核
// 应付、贷项或原外部事实——类型上只有引用与分配金额。
type SettlementApplication struct {
	application   ApplicationReference
	fact          FundsFactReference
	currency      CurrencyCode
	factMinor     int64
	allocations   []SettlementAllocation
	appliedMinor  int64
	basis         ApplicationBasisReference
	appliedAt     time.Time
	reversalBasis ApplicationBasisReference
	reversedAt    time.Time
}

// ApplySettlement 形成核销。每条分配必须沿一条既有映射（映射在场、目标一致、同一
// 事实）；净分配（借减贷）必须为正、等于本次使用的付款金额且不超过事实金额——部分
// 核销留下剩余未结（AT-SA-107/108），超出事实金额的分配立不成（AT-SA-109 超额保持
// 未分配）；币种不一致停在待判断（AT-SA-110）。
func ApplySettlement(
	fact ExternalFundsFact,
	mappings []FundsMapping,
	allocations []SettlementAllocation,
	application ApplicationReference,
	basis ApplicationBasisReference,
	appliedAt time.Time,
) (SettlementApplication, error) {
	if !fact.fact.valid() || !application.valid() || !basis.valid() || appliedAt.IsZero() || len(allocations) == 0 {
		return SettlementApplication{}, ErrInvalidApplication
	}
	if fact.kind == FundsPaymentFailed {
		return SettlementApplication{}, ErrUnfundableFact
	}
	mappingsByRef := make(map[MappingReference]FundsMapping, len(mappings))
	for _, mapping := range mappings {
		if mapping.fact != fact.fact {
			return SettlementApplication{}, ErrInvalidApplication
		}
		mappingsByRef[mapping.mapping] = mapping
	}
	net := int64(0)
	for _, allocation := range allocations {
		if !allocation.Mapping.valid() || !allocation.TargetKind.valid() ||
			!allocation.Target.valid() || !allocation.Direction.valid() || allocation.AmountMinor <= 0 {
			return SettlementApplication{}, ErrInvalidApplication
		}
		mapping, backed := mappingsByRef[allocation.Mapping]
		if !backed || mapping.targetKind != allocation.TargetKind || mapping.target != allocation.Target {
			// 没有映射背书的分配就是凭巧合分钱——唯一可证明范围之外保持未分配
			// （AT-SA-168）。
			return SettlementApplication{}, ErrInvalidApplication
		}
		if allocation.Direction == AllocationDebit {
			net += allocation.AmountMinor
		} else {
			net -= allocation.AmountMinor
		}
	}
	if net <= 0 || net > fact.amountMinor {
		return SettlementApplication{}, ErrApplicationImbalance
	}
	return SettlementApplication{
		application:  application,
		fact:         fact.fact,
		currency:     fact.currency,
		factMinor:    fact.amountMinor,
		allocations:  append([]SettlementAllocation(nil), allocations...),
		appliedMinor: net,
		basis:        basis,
		appliedAt:    appliedAt.UTC(),
	}, nil
}

// ApplySettlementInCurrency 是跨币种入口的守门面：目标币种与事实币种不同且无换算
// 依据时保持待判断（AT-SA-110）。换算依据落地前只有同币种一条路。
func ApplySettlementInCurrency(
	fact ExternalFundsFact,
	targetCurrency CurrencyCode,
	mappings []FundsMapping,
	allocations []SettlementAllocation,
	application ApplicationReference,
	basis ApplicationBasisReference,
	appliedAt time.Time,
) (SettlementApplication, error) {
	if targetCurrency != fact.currency {
		return SettlementApplication{}, ErrCrossCurrencyApplication
	}
	return ApplySettlement(fact, mappings, allocations, application, basis, appliedAt)
}

func (application SettlementApplication) Application() ApplicationReference {
	return application.application
}

func (application SettlementApplication) Fact() FundsFactReference {
	return application.fact
}

func (application SettlementApplication) Allocations() []SettlementAllocation {
	return append([]SettlementAllocation(nil), application.allocations...)
}

// AppliedMinor 是本次核销使用的净金额；RemainderMinor 是事实金额中仍未分配的剩余
// ——部分到账/超额都以剩余表达，原事实与原费用不动（AT-SA-108/109）。
func (application SettlementApplication) AppliedMinor() int64 {
	return application.appliedMinor
}

func (application SettlementApplication) RemainderMinor() int64 {
	return application.factMinor - application.appliedMinor
}

func (application SettlementApplication) Currency() CurrencyCode {
	return application.currency
}

func (application SettlementApplication) Basis() ApplicationBasisReference {
	return application.basis
}

func (application SettlementApplication) AppliedAt() time.Time {
	return application.appliedAt
}

// Reversed 报告撤销依据与时刻（若已撤销）。
func (application SettlementApplication) Reversed() (ApplicationBasisReference, time.Time, bool) {
	if application.reversedAt.IsZero() {
		return ApplicationBasisReference{}, time.Time{}, false
	}
	return application.reversalBasis, application.reversedAt, true
}

// Reverse 追加核销撤销并恢复相应未结金额（AT-SA-112/113）：原收款引用与原分配不删
// （值语义，接收者不动），撤销一次、依据必备；退回或撤销的资金事实本身由外部拥有，
// 这里只登记核销关系的失效。
func (application SettlementApplication) Reverse(
	basis ApplicationBasisReference,
	reversedAt time.Time,
) (SettlementApplication, error) {
	if !basis.valid() || reversedAt.IsZero() || reversedAt.Before(application.appliedAt) {
		return SettlementApplication{}, ErrInvalidApplication
	}
	if _, _, reversed := application.Reversed(); reversed {
		return SettlementApplication{}, ErrApplicationReversed
	}
	reversed := application
	reversed.allocations = append([]SettlementAllocation(nil), application.allocations...)
	reversed.reversalBasis = basis
	reversed.reversedAt = reversedAt.UTC()
	return reversed, nil
}
