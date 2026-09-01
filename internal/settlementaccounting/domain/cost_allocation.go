package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCostAllocation = errors.New("settlement accounting: invalid cost allocation")
	// ErrAllocationNeedsARule：缺规则不使用默认比例——没有版本化分摊规则时不分摊、
	// 不均摊、不虚构对象（UC-SA-006 输入契约）。
	ErrAllocationNeedsARule = errors.New("settlement accounting: allocation needs a versioned rule")
	// ErrAllocationImbalance：分摊必须守恒——各份额之和不得超过来源金额，剩余以未分摊
	// 余额显式保留（AT-SA-125）。
	ErrAllocationImbalance    = errors.New("settlement accounting: portions exceed the source amount")
	ErrInvalidOperatingResult = errors.New("settlement accounting: invalid operating result")
	// ErrOperatingComponentRoles：组成项的角色在本口径下不成立，或审核应付与贷项那一对
	// 不合规（各至多一项、贷项在场应付必须在场）。与 ErrInvalidOperatingResult 分开是
	// 为了让「派生形状不对」与「口径采用物不对」在错误上就分得开——后者指向的是 CONTEXT
	// 的成对采用那句，恢复动作是重新选料，不是改数（ADR-0087 决定三）。
	ErrOperatingComponentRoles = errors.New("settlement accounting: operating component roles do not hold for the basis")
)

// AllocationRuleVersionReference 指名采用的分摊规则版本（版本生命周期属商业/结算
// 配置）。
type AllocationRuleVersionReference struct{ requiredValue }

func NewAllocationRuleVersionReference(value string) (AllocationRuleVersionReference, error) {
	required, err := newRequiredValue("allocation rule version reference", value)
	return AllocationRuleVersionReference{required}, err
}

// AllocationSourceReference 指名被分摊的来源金额身份（预期成本、审核应付、内部作业
// 成本、共享成本……）。分摊只引用来源——不改变供应商应付或内部债权债务（AT-SA-127）。
type AllocationSourceReference struct{ requiredValue }

func NewAllocationSourceReference(value string) (AllocationSourceReference, error) {
	required, err := newRequiredValue("allocation source reference", value)
	return AllocationSourceReference{required}, err
}

// AllocationTargetReference 指名分摊目标（包裹、客户、产品、线路或其他分析范围）。
type AllocationTargetReference struct{ requiredValue }

func NewAllocationTargetReference(value string) (AllocationTargetReference, error) {
	required, err := newRequiredValue("allocation target reference", value)
	return AllocationTargetReference{required}, err
}

// AllocationVersion 是分摊结果的版本：规则更正或重分摊换版本，原分摊保留（AT-SA-126）。
type AllocationVersion struct{ requiredValue }

func NewAllocationVersion(value string) (AllocationVersion, error) {
	required, err := newRequiredValue("allocation version", value)
	return AllocationVersion{required}, err
}

// AllocationID 是分摊结果的稳定身份。
type AllocationID struct{ requiredValue }

func NewAllocationID(value string) (AllocationID, error) {
	required, err := newRequiredValue("allocation ID", value)
	return AllocationID{required}, err
}

// AllocationPortion 是一个目标承接的分摊份额。份额由规则计算给出——本类型不提供
// 按对象数平均的入口。
type AllocationPortion struct {
	Target      AllocationTargetReference
	AmountMinor int64
}

// CostAllocationSpec 是形成一次成本分摊所需的全部输入。Portions 可以为空：无合格
// 对象或分母为零时全额保留为未分摊余额，不虚构对象（AT-SA-123/124）。
type CostAllocationSpec struct {
	ID          AllocationID
	Source      AllocationSourceReference
	SourceMinor int64
	Currency    CurrencyCode
	Rule        AllocationRuleVersionReference
	Portions    []AllocationPortion
	Version     AllocationVersion
	AllocatedAt time.Time
}

// CostAllocation 是来源金额按版本化规则向合格对象的守恒归因（UC-SA-006「分摊已形成」）。
// 来源金额只被引用不被修改；份额之和加未分摊余额恒等于来源金额。
type CostAllocation struct {
	id               AllocationID
	source           AllocationSourceReference
	sourceMinor      int64
	currency         CurrencyCode
	rule             AllocationRuleVersionReference
	portions         []AllocationPortion
	unallocatedMinor int64
	version          AllocationVersion
	allocatedAt      time.Time
	corrects         AllocationVersion
}

func FormCostAllocation(spec CostAllocationSpec) (CostAllocation, error) {
	if !spec.ID.valid() ||
		!spec.Source.valid() ||
		spec.SourceMinor <= 0 ||
		!spec.Currency.valid() ||
		!spec.Version.valid() ||
		spec.AllocatedAt.IsZero() {
		return CostAllocation{}, ErrInvalidCostAllocation
	}
	if !spec.Rule.valid() {
		return CostAllocation{}, ErrAllocationNeedsARule
	}
	allocated := int64(0)
	seen := make(map[AllocationTargetReference]struct{}, len(spec.Portions))
	for _, portion := range spec.Portions {
		if !portion.Target.valid() || portion.AmountMinor <= 0 {
			return CostAllocation{}, ErrInvalidCostAllocation
		}
		if _, exists := seen[portion.Target]; exists {
			return CostAllocation{}, ErrInvalidCostAllocation
		}
		seen[portion.Target] = struct{}{}
		allocated += portion.AmountMinor
	}
	if allocated > spec.SourceMinor {
		return CostAllocation{}, ErrAllocationImbalance
	}
	return CostAllocation{
		id:               spec.ID,
		source:           spec.Source,
		sourceMinor:      spec.SourceMinor,
		currency:         spec.Currency,
		rule:             spec.Rule,
		portions:         append([]AllocationPortion(nil), spec.Portions...),
		unallocatedMinor: spec.SourceMinor - allocated,
		version:          spec.Version,
		allocatedAt:      spec.AllocatedAt.UTC(),
	}, nil
}

func (allocation CostAllocation) ID() AllocationID {
	return allocation.id
}

func (allocation CostAllocation) Source() AllocationSourceReference {
	return allocation.source
}

// SourceAmount 是被分摊来源金额的快照引用值——分摊改不了来源本体，这里只是对账锚。
func (allocation CostAllocation) SourceAmount() (CurrencyCode, int64) {
	return allocation.currency, allocation.sourceMinor
}

func (allocation CostAllocation) Rule() AllocationRuleVersionReference {
	return allocation.rule
}

func (allocation CostAllocation) Portions() []AllocationPortion {
	return append([]AllocationPortion(nil), allocation.portions...)
}

// UnallocatedMinor 是未分摊余额：无合格对象、分母为零或尾差剩余都在这里显式可见，
// 不为了报表完整虚构对象。
func (allocation CostAllocation) UnallocatedMinor() int64 {
	return allocation.unallocatedMinor
}

func (allocation CostAllocation) Version() AllocationVersion {
	return allocation.version
}

func (allocation CostAllocation) AllocatedAt() time.Time {
	return allocation.allocatedAt
}

// Corrects 交回本版本重分摊所接续的前一版本（若有）。
func (allocation CostAllocation) Corrects() (AllocationVersion, bool) {
	if !allocation.corrects.valid() {
		return AllocationVersion{}, false
	}
	return allocation.corrects, true
}

// Reallocate 依据新规则版本或更正重新分摊：换版本、回指前身、原分摊保留（AT-SA-126
// 「原分摊保留，形成新版本和差额」）。来源金额与身份不变——重分摊改的是归因，不是钱。
func (allocation CostAllocation) Reallocate(
	rule AllocationRuleVersionReference,
	portions []AllocationPortion,
	version AllocationVersion,
	allocatedAt time.Time,
) (CostAllocation, error) {
	if !version.valid() || version == allocation.version {
		return CostAllocation{}, ErrInvalidCostAllocation
	}
	reallocated, err := FormCostAllocation(CostAllocationSpec{
		ID:          allocation.id,
		Source:      allocation.source,
		SourceMinor: allocation.sourceMinor,
		Currency:    allocation.currency,
		Rule:        rule,
		Portions:    portions,
		Version:     version,
		AllocatedAt: allocatedAt,
	})
	if err != nil {
		return CostAllocation{}, err
	}
	reallocated.corrects = allocation.version
	return reallocated, nil
}

// OperatingBasis 是经营指标口径的封闭三值：预估、已确认、已结算（UC-SA-006：口径
// 分别成对消费各自来源，不跨阶段混算）。索赔调整后口径必须声明所依附的基础口径，
// 属编排层组合，不是第四格。
type OperatingBasis uint8

const (
	OperatingBasisInvalid OperatingBasis = iota
	EstimatedBasis
	ConfirmedBasis
	SettledBasis
)

func (basis OperatingBasis) valid() bool {
	return basis >= EstimatedBasis && basis <= SettledBasis
}

func (basis OperatingBasis) String() string {
	switch basis {
	case EstimatedBasis:
		return "ESTIMATED"
	case ConfirmedBasis:
		return "CONFIRMED"
	case SettledBasis:
		return "SETTLED"
	default:
		return ""
	}
}

// ComponentEffect 是组成项对指标的封闭二向：增加或减少。审核应付与供应商贷项按各自
// 借贷方向分别计入一次，不静默净额（UC-SA-006 步骤 6）。
type ComponentEffect uint8

const (
	ComponentEffectInvalid ComponentEffect = iota
	IncreasesResult
	DecreasesResult
)

func (effect ComponentEffect) valid() bool {
	return effect == IncreasesResult || effect == DecreasesResult
}

func (effect ComponentEffect) String() string {
	switch effect {
	case IncreasesResult:
		return "INCREASES"
	case DecreasesResult:
		return "DECREASES"
	default:
		return ""
	}
}

// ComponentSourceReference 指名组成项的来源金额身份（客户费用、审核应付、贷项、
// 赔付、追偿认可、汇兑、分摊……）。
type ComponentSourceReference struct{ requiredValue }

func NewComponentSourceReference(value string) (ComponentSourceReference, error) {
	required, err := newRequiredValue("component source reference", value)
	return ComponentSourceReference{required}, err
}

// ComponentRole 是组成项在本口径下的采用物身份（ADR-0087 决定三）。来源引用说的是
// 「这笔钱是哪一条」，角色说的是「它在这个口径里充当什么」——CONTEXT 那句
// 「审核应付与贷项按各自借贷方向分别计入一次……不得把审核应付视为已净含贷项」要的是
// 后者，没有它写口连复验都无从谈起。
//
// **取值按口径分组，不是一个跨口径的大平集。** CONTEXT「经营毛利的客户与外部供应商
// 基础必须按阶段成对采用」逐口径点名了各自的采用物；平集会放进「这个角色在这个口径下
// 不该出现」的组成，那等于把一处空转换成另一处。
type ComponentRole uint8

const (
	ComponentRoleInvalid ComponentRole = iota
	// 预估口径：当前有效客户预估费用与当前有效供应商预期成本。
	CustomerEstimateRole
	SupplierExpectedCostRole
	// 已确认口径：当前有效客户运营应收，以及当前有效审核应付和与其关联的当前有效贷项。
	CustomerOperatingReceivableRole
	AuditedPayableRole
	SupplierCreditNoteRole
	// 已结算口径：只用 UC-SA-005 对上述三者分别形成的运营核销分配范围。
	SettledCustomerReceivableRole
	SettledAuditedPayableRole
	SettledSupplierCreditNoteRole
)

func (role ComponentRole) String() string {
	switch role {
	case CustomerEstimateRole:
		return "CUSTOMER_ESTIMATE"
	case SupplierExpectedCostRole:
		return "SUPPLIER_EXPECTED_COST"
	case CustomerOperatingReceivableRole:
		return "CUSTOMER_OPERATING_RECEIVABLE"
	case AuditedPayableRole:
		return "AUDITED_PAYABLE"
	case SupplierCreditNoteRole:
		return "SUPPLIER_CREDIT_NOTE"
	case SettledCustomerReceivableRole:
		return "SETTLED_CUSTOMER_RECEIVABLE"
	case SettledAuditedPayableRole:
		return "SETTLED_AUDITED_PAYABLE"
	case SettledSupplierCreditNoteRole:
		return "SETTLED_SUPPLIER_CREDIT_NOTE"
	default:
		return ""
	}
}

// admittedBy 判这个角色在该口径下成不成立。跨阶段替代由此拒：CONTEXT 明写「均不能把
// 缺失成本记为零、把审核应付视为已净含贷项或跨阶段替代」。
func (role ComponentRole) admittedBy(basis OperatingBasis) bool {
	switch basis {
	case EstimatedBasis:
		return role == CustomerEstimateRole || role == SupplierExpectedCostRole
	case ConfirmedBasis:
		return role == CustomerOperatingReceivableRole ||
			role == AuditedPayableRole ||
			role == SupplierCreditNoteRole
	case SettledBasis:
		return role == SettledCustomerReceivableRole ||
			role == SettledAuditedPayableRole ||
			role == SettledSupplierCreditNoteRole
	default:
		return false
	}
}

// payableAndCreditRolesOf 给出该口径下的「审核应付」与「与其关联的贷项」两个角色。
// 预估口径没有这一对——它采用的是预估费用与预期成本，账单与贷项都还不存在。
func payableAndCreditRolesOf(basis OperatingBasis) (payable, credit ComponentRole, paired bool) {
	switch basis {
	case ConfirmedBasis:
		return AuditedPayableRole, SupplierCreditNoteRole, true
	case SettledBasis:
		return SettledAuditedPayableRole, SettledSupplierCreditNoteRole, true
	default:
		return ComponentRoleInvalid, ComponentRoleInvalid, false
	}
}

// ResultComponent 是经营结果的一个组成项：来源、角色、方向与金额。
type ResultComponent struct {
	Source      ComponentSourceReference
	Role        ComponentRole
	Effect      ComponentEffect
	AmountMinor int64
}

// OperatingScopeReference 指名指标的分析范围（客户、产品、线路、法人……）。
type OperatingScopeReference struct{ requiredValue }

func NewOperatingScopeReference(value string) (OperatingScopeReference, error) {
	required, err := newRequiredValue("operating scope reference", value)
	return OperatingScopeReference{required}, err
}

// OperatingResultVersion 是指标快照的版本：迟到成本在截点后到达时原快照不变，新版本
// 关联原截点（AT-SA-137）。
type OperatingResultVersion struct{ requiredValue }

func NewOperatingResultVersion(value string) (OperatingResultVersion, error) {
	required, err := newRequiredValue("operating result version", value)
	return OperatingResultVersion{required}, err
}

// OperatingResult 是按口径、币种和截至时点派生的经营毛利/损失快照（UC-SA-006）。
// 指标是派生结果不可编辑：净额只由组成项算出，规格里没有可以直接写毛利的字段，类型
// 上也没有任何改数方法——改组成就换版本。
type OperatingResult struct {
	scope       OperatingScopeReference
	period      BillingPeriodReference
	basis       OperatingBasis
	currency    CurrencyCode
	components  []ResultComponent
	marginMinor int64
	version     OperatingResultVersion
	asOf        time.Time
	corrects    OperatingResultVersion
}

// DeriveOperatingResult 从组成项派生指标。组成不空、金额为正、方向封闭；毛利=增项
// 之和减去减项之和，只在这里算出（AT-SA-131/132 的口径成对消费由编排选料，这里守
// 派生纪律）。
func DeriveOperatingResult(
	scope OperatingScopeReference,
	period BillingPeriodReference,
	basis OperatingBasis,
	currency CurrencyCode,
	components []ResultComponent,
	version OperatingResultVersion,
	asOf time.Time,
) (OperatingResult, error) {
	if !scope.valid() || !period.valid() || !basis.valid() || !currency.valid() ||
		len(components) == 0 || !version.valid() || asOf.IsZero() {
		return OperatingResult{}, ErrInvalidOperatingResult
	}
	margin := int64(0)
	for _, component := range components {
		if !component.Source.valid() || !component.Effect.valid() || component.AmountMinor <= 0 {
			return OperatingResult{}, ErrInvalidOperatingResult
		}
		if !component.Role.admittedBy(basis) {
			return OperatingResult{}, ErrOperatingComponentRoles
		}
		if component.Effect == IncreasesResult {
			margin += component.AmountMinor
		} else {
			margin -= component.AmountMinor
		}
	}
	if err := verifyPayableAndCredit(basis, components); err != nil {
		return OperatingResult{}, err
	}
	return OperatingResult{
		scope:       scope,
		period:      period,
		basis:       basis,
		currency:    currency,
		components:  append([]ResultComponent(nil), components...),
		marginMinor: margin,
		version:     version,
		asOf:        asOf.UTC(),
	}, nil
}

// verifyPayableAndCredit 是 CONTEXT「审核应付与贷项按各自借贷方向分别计入一次……不得
// 把审核应付视为已净含贷项」那句在写侧的可核对形式（ADR-0087 决定三）：同一口径下两者
// 各至多一项，且贷项在场时应付必须在场。
//
// 各至多一项拦的是「分别计入一次」的反面；贷项不得独自在场拦的是净额化——一个只带贷项
// 而不带应付的口径，读起来就是应付已经把贷项吃进去了，而那正是那句禁的。
//
// 判据落在写侧而不是 SQL CHECK：逐元素校验 jsonb 在本模块无先例，而写口复验有（组成与
// 毛利是否相符就是在那里守的）。日后若出现绕过写口的写入路径，这一条要重裁。
func verifyPayableAndCredit(basis OperatingBasis, components []ResultComponent) error {
	payableRole, creditRole, paired := payableAndCreditRolesOf(basis)
	if !paired {
		return nil
	}
	payables, credits := 0, 0
	for _, component := range components {
		switch component.Role {
		case payableRole:
			payables++
		case creditRole:
			credits++
		}
	}
	if payables > 1 || credits > 1 {
		return ErrOperatingComponentRoles
	}
	if credits == 1 && payables == 0 {
		return ErrOperatingComponentRoles
	}
	return nil
}

func (result OperatingResult) Scope() OperatingScopeReference {
	return result.scope
}

func (result OperatingResult) Period() BillingPeriodReference {
	return result.period
}

func (result OperatingResult) Basis() OperatingBasis {
	return result.basis
}

func (result OperatingResult) Components() []ResultComponent {
	return append([]ResultComponent(nil), result.components...)
}

// Margin 是派生毛利/损失（负数即损失）。它没有对应的写入口——组成变了就派生新版本。
func (result OperatingResult) Margin() (CurrencyCode, int64) {
	return result.currency, result.marginMinor
}

func (result OperatingResult) Version() OperatingResultVersion {
	return result.version
}

// AsOf 是指标的截至时点——迟到成本不改写本快照，新版本关联原截点（AT-SA-137）。
func (result OperatingResult) AsOf() time.Time {
	return result.asOf
}

// Corrects 交回本版本重派生所接续的前一版本（若有）。
func (result OperatingResult) Corrects() (OperatingResultVersion, bool) {
	if !result.corrects.valid() {
		return OperatingResultVersion{}, false
	}
	return result.corrects, true
}

// Rederive 以新组成派生新指标版本：原快照原样保留（值语义），新版本回指前身并保留
// 原截点关联（AT-SA-137「原指标快照不变，新版本关联原截点」）。
func (result OperatingResult) Rederive(
	components []ResultComponent,
	version OperatingResultVersion,
	asOf time.Time,
) (OperatingResult, error) {
	if !version.valid() || version == result.version {
		return OperatingResult{}, ErrInvalidOperatingResult
	}
	rederived, err := DeriveOperatingResult(result.scope, result.period, result.basis, result.currency, components, version, asOf)
	if err != nil {
		return OperatingResult{}, err
	}
	rederived.corrects = result.version
	return rederived, nil
}
