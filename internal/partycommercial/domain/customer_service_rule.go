package domain

import (
	"errors"
	"sort"
)

var (
	// ErrInvalidCustomerServiceRuleVersion 拒绝立不住的客户服务规则版本。
	ErrInvalidCustomerServiceRuleVersion = errors.New("party commercial: invalid customer service rule version")
	// ErrDuplicateCustomerServiceRuleItem 拒绝同一键下的第二行正文：同一种期限两行、同一索赔类型
	// 两份材料清单。与「立不住」分格，因为恢复动作不同——前者去掉多出的那一行，后者去补内容。
	ErrDuplicateCustomerServiceRuleItem = errors.New("party commercial: duplicate customer service rule item")
	// ErrInvalidClaimDeadlineRule 拒绝立不住的索赔期限规则行。
	ErrInvalidClaimDeadlineRule = errors.New("party commercial: invalid claim deadline rule")
	// ErrInvalidMinimumMaterialsRule 拒绝立不住的最低材料规则行。
	ErrInvalidMinimumMaterialsRule = errors.New("party commercial: invalid minimum materials rule")
	// ErrCustomerServiceRuleApplicabilityMismatch 报出版本壳指名的产品 / 合同与正文适用声明不一致。
	ErrCustomerServiceRuleApplicabilityMismatch = errors.New("party commercial: customer service rule applicability disagrees with the version's named reference")
)

// CustomerServiceRuleApplicability 是规则版本挂在哪个商业对象上的封闭两格：服务产品版本或
// 客户合同版本（CONTEXT「按服务产品和客户合同明确适用范围」）。
//
// declared 让零值立不住。「没说挂在哪」与「挂在一个尚未指明的对象上」在裸的引用零值里长得
// 一样，而前者是输入缺件、后者是一句该被拒的声明——判据同 ProductChannelBinding 的两格封闭。
type CustomerServiceRuleApplicability struct {
	declared bool
	product  CommercialObjectID
	contract CommercialObjectID
}

// CustomerServiceRuleAppliesToServiceProduct 声明本规则版本随某个服务产品适用。
func CustomerServiceRuleAppliesToServiceProduct(product CommercialObjectID) CustomerServiceRuleApplicability {
	return CustomerServiceRuleApplicability{declared: true, product: product}
}

// CustomerServiceRuleAppliesToCustomerContract 声明本规则版本随某个客户合同适用。
//
// 与上一格分开而不合成「一个标识加一个类别字段」：同一个标识串作产品与作合同是两件事，而合成
// 之后它们在那一列里长得一模一样，类别设错时没有任何东西能分辨——判据同票 03 对额度取值形态
// 的裁断（并存两格，落在哪一格本身就是判别式）。
func CustomerServiceRuleAppliesToCustomerContract(contract CommercialObjectID) CustomerServiceRuleApplicability {
	return CustomerServiceRuleApplicability{declared: true, contract: contract}
}

func (applicability CustomerServiceRuleApplicability) ServiceProduct() (CommercialObjectID, bool) {
	return applicability.product, applicability.product.valid()
}

func (applicability CustomerServiceRuleApplicability) CustomerContract() (CommercialObjectID, bool) {
	return applicability.contract, applicability.contract.valid()
}

func (applicability CustomerServiceRuleApplicability) valid() bool {
	if !applicability.declared {
		return false
	}
	// 恰一格有值。两格都填不是「更明确」，是两条适用声明挤在一条记录上，续办时答不出该按哪条。
	return applicability.product.valid() != applicability.contract.valid()
}

// ConsistentCustomerServiceRuleApplicability 核版本壳与正文的适用声明：壳上指名了正文所挂那一类
// （产品或合同）的引用时，两处必须是同一个对象；壳上没指名、或只指名了另一类，无从比对而放行。
//
// 与 ConsistentAcceptanceRulePackage 同一道核对，也放在同样的两处：发布写入前与点读之后
// （ADR-0104 Decision 四）。解析按范围与锚点选版本壳、不看正文，所以「这一版挂的是不是我手上
// 这份产品 / 合同」只能在拿到正文之后核，核不过是坏数据而不是「没配规则」。
func ConsistentCustomerServiceRuleApplicability(
	version CommercialVersion,
	applicability CustomerServiceRuleApplicability,
) error {
	if product, applies := applicability.ServiceProduct(); applies {
		if named, present := version.ReferenceTo(ServiceProductObject); present && named != product {
			return ErrCustomerServiceRuleApplicabilityMismatch
		}
	}
	if contract, applies := applicability.CustomerContract(); applies {
		if named, present := version.ReferenceTo(CustomerContractObject); present && named != contract {
			return ErrCustomerServiceRuleApplicabilityMismatch
		}
	}
	return nil
}

// ClaimDeadlineKind 是索赔期限的种类，封闭三格，逐字对应 visibility-exception CONTEXT「客户首次
// 索赔期限、资料补充期限和结论复核期限是三个独立期限」。独立意味着一版规则对每一种至多说一次，
// 也不得拿一种顶替另一种——所以它是行的键而不是行上的一列标记。
type ClaimDeadlineKind uint8

const (
	ClaimDeadlineKindInvalid ClaimDeadlineKind = iota
	FirstClaimDeadline
	MaterialSupplementDeadline
	ConclusionReviewDeadline
)

func (kind ClaimDeadlineKind) valid() bool {
	return kind >= FirstClaimDeadline && kind <= ConclusionReviewDeadline
}

func (kind ClaimDeadlineKind) String() string {
	switch kind {
	case FirstClaimDeadline:
		return "FIRST_CLAIM"
	case MaterialSupplementDeadline:
		return "MATERIAL_SUPPLEMENT"
	case ConclusionReviewDeadline:
		return "CONCLUSION_REVIEW"
	default:
		return ""
	}
}

// DeadlineStartEventReference 指名一条期限从哪个事件起算。
//
// 是引用而不是封闭集：起算事件的解释权在 visibility-exception——它的期限记录按自己的硬句保存
// 「起算事件」，其读口今天把它当不透明串携带，CONTEXT 也只对结论复核点了名（结论通知、送达或
// 可获取事实）。本上下文替它造一套枚举就是发明 VE 的词；VE 定下封闭集那天走新迁移收紧列上 CHECK。
type DeadlineStartEventReference struct{ requiredValue }

func NewDeadlineStartEventReference(value string) (DeadlineStartEventReference, error) {
	required, err := newRequiredValue("deadline start event reference", value)
	return DeadlineStartEventReference{required}, err
}

// BusinessCalendarReference 指名一条期限按哪份业务日历或哪个时区计日。日历的内容（工作日、节假日、
// 时区）属实例半边，本上下文只携带引用。
type BusinessCalendarReference struct{ requiredValue }

func NewBusinessCalendarReference(value string) (BusinessCalendarReference, error) {
	required, err := newRequiredValue("business calendar reference", value)
	return BusinessCalendarReference{required}, err
}

// ClaimDeadlineRule 是索赔期限正文的一行：种类 × 起算事件 × 时长 × 日历引用（ADR-0104 Decision 二）。
//
// 时长以整数天计。与日历 / 时区引用配对的单位只能是日——按业务日历数的是工作日、按时区数的是
// 自然日，小时级时限不需要日历；截止时刻本身不在这里算，那是 VE 拿起算事实与这条规则派生出来
// 的事实，归 VE。
type ClaimDeadlineRule struct {
	kind       ClaimDeadlineKind
	startEvent DeadlineStartEventReference
	days       int
	calendar   BusinessCalendarReference
}

// NewClaimDeadlineRule 只校形状不校值：种类在集内、两处引用非空、时长为正。非正时长会把截止算到
// 起算之前或与之相等，那样的期限一形成就已届满——它不是一条严的规则，是一条算不出东西的规则。
func NewClaimDeadlineRule(
	kind ClaimDeadlineKind,
	startEvent DeadlineStartEventReference,
	days int,
	calendar BusinessCalendarReference,
) (ClaimDeadlineRule, error) {
	if !kind.valid() || !startEvent.valid() || days <= 0 || !calendar.valid() {
		return ClaimDeadlineRule{}, ErrInvalidClaimDeadlineRule
	}
	return ClaimDeadlineRule{kind: kind, startEvent: startEvent, days: days, calendar: calendar}, nil
}

func (rule ClaimDeadlineRule) Kind() ClaimDeadlineKind {
	return rule.kind
}

func (rule ClaimDeadlineRule) StartEvent() DeadlineStartEventReference {
	return rule.startEvent
}

func (rule ClaimDeadlineRule) DurationDays() int {
	return rule.days
}

func (rule ClaimDeadlineRule) Calendar() BusinessCalendarReference {
	return rule.calendar
}

// ClaimKindReference 指名一种索赔类型。类型目录归 visibility-exception，本上下文只携带引用。
type ClaimKindReference struct{ requiredValue }

func NewClaimKindReference(value string) (ClaimKindReference, error) {
	required, err := newRequiredValue("claim kind reference", value)
	return ClaimKindReference{required}, err
}

// MaterialRequirementReference 指名一件材料条目。材料目录归 visibility-exception（它的资格编排
// 拿目录签发的材料引用与已收到的作差集），本上下文只携带引用。
type MaterialRequirementReference struct{ requiredValue }

func NewMaterialRequirementReference(value string) (MaterialRequirementReference, error) {
	required, err := newRequiredValue("material requirement reference", value)
	return MaterialRequirementReference{required}, err
}

// MinimumMaterialsRule 是最低材料正文的一行：一种索赔类型在这一版规则下必须齐备的材料清单
// （ADR-0104 Decision 二）。
type MinimumMaterialsRule struct {
	claimKind ClaimKindReference
	materials []MaterialRequirementReference
}

// NewMinimumMaterialsRule 只校形状：索赔类型非空、清单至少一项且不重复。空清单不是「这一类
// 不要材料」——那一句没有任何消费方读得出来（差集为空与没有规则同形），要说也该是不登记这一行。
func NewMinimumMaterialsRule(
	claimKind ClaimKindReference,
	materials []MaterialRequirementReference,
) (MinimumMaterialsRule, error) {
	if !claimKind.valid() || len(materials) == 0 {
		return MinimumMaterialsRule{}, ErrInvalidMinimumMaterialsRule
	}
	seen := make(map[MaterialRequirementReference]struct{}, len(materials))
	for _, material := range materials {
		if !material.valid() {
			return MinimumMaterialsRule{}, ErrInvalidMinimumMaterialsRule
		}
		if _, duplicate := seen[material]; duplicate {
			return MinimumMaterialsRule{}, ErrInvalidMinimumMaterialsRule
		}
		seen[material] = struct{}{}
	}
	sorted := append([]MaterialRequirementReference(nil), materials...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left].String() < sorted[right].String()
	})
	return MinimumMaterialsRule{claimKind: claimKind, materials: sorted}, nil
}

func (rule MinimumMaterialsRule) ClaimKind() ClaimKindReference {
	return rule.claimKind
}

// Materials 按稳定顺序交回清单（副本）。
func (rule MinimumMaterialsRule) Materials() []MaterialRequirementReference {
	return append([]MaterialRequirementReference(nil), rule.materials...)
}

// CustomerServiceRuleVersion 是一个客户服务规则版本的正文：它挂在哪个商业对象上、责任方是谁、
// 适用范围是什么，以及首发的两项规则内容——索赔期限与最低材料（ADR-0104）。另四项（追踪披露、
// 异常响应、客户更新、通知义务）不在首发，重启条件记在该 ADR 的 Consequences。
//
// **本类型刻意不带内部异常检测阈值、事实有效性与最终赔付金额。** CONTEXT 明写这三样「不得写成
// 客户可以直接覆盖的商业配置」——不建字段是唯一守得住的办法：留一个字段再靠约定不填，下一个人
// 看到的是一个可填的口子，而那时没有任何东西会拦他。
type CustomerServiceRuleVersion struct {
	version       CommercialVersion
	applicability CustomerServiceRuleApplicability
	responsible   PartyID
	scope         CommercialScopeReference
	deadlines     map[ClaimDeadlineKind]ClaimDeadlineRule
	materials     map[ClaimKindReference]MinimumMaterialsRule
}

// NewCustomerServiceRuleVersion 在版本已生效且类别正确时形成一个规则版本。
//
// 类别必须是 CustomerServiceRuleObject。挂错类别的版本仍是一个合法的商业版本，入册与被解析
// 选中都不报错，只是解析按错的类别去找；那个错要到下游取不到规则依据时才显形，而那时它长得
// 像「这个客户没配规则」（ADR-0093 否决复用 AuthorizationRuleObject 时给的正是这条理由）。
//
// 两项正文合起来至少一行（ADR-0104 Decision 三）：一版客户服务规则存在的全部理由就是承载差异，
// 「对首发两项都无客户差异」不是一版规则，是不登记；允许显式空会造出「登记了但什么都没说」与
// 「没登记」两种在消费侧同形的空。每一种期限、每一种索赔类型至多一行——两行就答不出该按哪条。
func NewCustomerServiceRuleVersion(
	version CommercialVersion,
	applicability CustomerServiceRuleApplicability,
	responsible PartyID,
	scope CommercialScopeReference,
	deadlines []ClaimDeadlineRule,
	materials []MinimumMaterialsRule,
) (CustomerServiceRuleVersion, error) {
	if version.kind != CustomerServiceRuleObject ||
		version.status != CommercialVersionEffective ||
		!applicability.valid() || !responsible.valid() || !scope.valid() ||
		len(deadlines)+len(materials) == 0 {
		return CustomerServiceRuleVersion{}, ErrInvalidCustomerServiceRuleVersion
	}

	filedDeadlines := make(map[ClaimDeadlineKind]ClaimDeadlineRule, len(deadlines))
	for _, deadline := range deadlines {
		if !deadline.kind.valid() || !deadline.startEvent.valid() || deadline.days <= 0 || !deadline.calendar.valid() {
			return CustomerServiceRuleVersion{}, ErrInvalidClaimDeadlineRule
		}
		if _, duplicate := filedDeadlines[deadline.kind]; duplicate {
			return CustomerServiceRuleVersion{}, ErrDuplicateCustomerServiceRuleItem
		}
		filedDeadlines[deadline.kind] = deadline
	}

	filedMaterials := make(map[ClaimKindReference]MinimumMaterialsRule, len(materials))
	for _, rule := range materials {
		if !rule.claimKind.valid() || len(rule.materials) == 0 {
			return CustomerServiceRuleVersion{}, ErrInvalidMinimumMaterialsRule
		}
		if _, duplicate := filedMaterials[rule.claimKind]; duplicate {
			return CustomerServiceRuleVersion{}, ErrDuplicateCustomerServiceRuleItem
		}
		filedMaterials[rule.claimKind] = rule
	}

	return CustomerServiceRuleVersion{
		version:       version,
		applicability: applicability,
		responsible:   responsible,
		scope:         scope,
		deadlines:     filedDeadlines,
		materials:     filedMaterials,
	}, nil
}

func (rule CustomerServiceRuleVersion) Version() CommercialVersion {
	return rule.version
}

func (rule CustomerServiceRuleVersion) Applicability() CustomerServiceRuleApplicability {
	return rule.applicability
}

// ResponsibleParty 是本规则版本的责任方。CONTEXT 把它与适用范围、有效期间并列为必需项：
// 没有责任方的服务承诺在异常响应时答不出该找谁。
func (rule CustomerServiceRuleVersion) ResponsibleParty() PartyID {
	return rule.responsible
}

func (rule CustomerServiceRuleVersion) Scope() CommercialScopeReference {
	return rule.scope
}

// ClaimDeadline 交回某一种期限的规则。found=false 是「这一版对该种期限无客户差异」，不是零值期限。
func (rule CustomerServiceRuleVersion) ClaimDeadline(kind ClaimDeadlineKind) (ClaimDeadlineRule, bool) {
	deadline, found := rule.deadlines[kind]
	return deadline, found
}

// ClaimDeadlines 按种类顺序交回全部期限规则（副本）。
func (rule CustomerServiceRuleVersion) ClaimDeadlines() []ClaimDeadlineRule {
	deadlines := make([]ClaimDeadlineRule, 0, len(rule.deadlines))
	for _, deadline := range rule.deadlines {
		deadlines = append(deadlines, deadline)
	}
	sort.Slice(deadlines, func(left, right int) bool {
		return deadlines[left].kind < deadlines[right].kind
	})
	return deadlines
}

// MinimumMaterialsFor 交回某一索赔类型的最低材料规则。found=false 是「这一版对该类型无客户差异」。
func (rule CustomerServiceRuleVersion) MinimumMaterialsFor(claimKind ClaimKindReference) (MinimumMaterialsRule, bool) {
	materials, found := rule.materials[claimKind]
	return materials, found
}

// MinimumMaterials 按索赔类型的稳定顺序交回全部材料规则（副本）。
func (rule CustomerServiceRuleVersion) MinimumMaterials() []MinimumMaterialsRule {
	rules := make([]MinimumMaterialsRule, 0, len(rule.materials))
	for _, materials := range rule.materials {
		rules = append(rules, materials)
	}
	sort.Slice(rules, func(left, right int) bool {
		return rules[left].claimKind.String() < rules[right].claimKind.String()
	})
	return rules
}
