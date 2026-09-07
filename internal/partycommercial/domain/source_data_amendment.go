package domain

import (
	"errors"
	"sort"
)

var (
	// ErrSourceDataAmendmentNotConfigured 是资料修订允许声明缺件：未封闭却零格，或某一格的资料组 /
	// 阶段 / 意图 / 允许性立不住（含把「未声明」当成一格的取值登进来）。恢复动作是把矩阵按 PAR-COM-13
	// 登齐（实例半边），不是本上下文代拟默认。
	ErrSourceDataAmendmentNotConfigured = errors.New("party commercial: source data amendment allowance content is not configured")
	// ErrConflictingSourceDataAmendment 是同一（资料组, 阶段, 意图）声明了两格。
	ErrConflictingSourceDataAmendment = errors.New("party commercial: conflicting source data amendment allowance declaration")
)

// SourceDataGroupReference 是矩阵一维的资料组引用（字段或字段组）。它是开放引用（ADR-0120 Decision 七）：
// 词表属 PAR-COM-13 的实例登记，本上下文只查非空、不校验存在；parcel-shipment 那边同名类型同样是开放串，
// 两侧只在「不得模糊指代」（UC-PS-002）一句上一致，而那是 PS 对请求的判断，不是 PC 对声明的校验。
type SourceDataGroupReference struct{ requiredValue }

func NewSourceDataGroupReference(value string) (SourceDataGroupReference, error) {
	required, err := newRequiredValue("source data group reference", value)
	return SourceDataGroupReference{required}, err
}

// DeclaredAmendmentStage 是矩阵一维的资料修订阶段，六格原词的权威在 parcel-shipment（PS CONTEXT 词条
// 「资料修订阶段」与 domain.AmendmentStage，ADR-0118）；本上下文只镜像不另定、一格不拆不加（ADR-0120
// Decision 五）。命名与本族既有的三个镜像（DeclaredIntakeSource / DeclaredResponsibilityOutcome /
// DeclaredCancellationParty）同形：这是「本上下文对那几格的引用枚举」。零值不合法——「判不出阶段」是 PS
// 的事，PS 判不出就不来问，本上下文不收一个判不出的阶段。
type DeclaredAmendmentStage uint8

const (
	DeclaredAmendmentStageInvalid DeclaredAmendmentStage = iota
	DeclaredAcceptedNotYetReceived
	DeclaredReceivedOrMeasured
	DeclaredLabelledOrBagged
	DeclaredCustomsDataFormingNotSubmitted
	DeclaredCustomsSubmitted
	DeclaredCaseClosedOrServiceCompleted
)

func (stage DeclaredAmendmentStage) valid() bool {
	return stage >= DeclaredAcceptedNotYetReceived && stage <= DeclaredCaseClosedOrServiceCompleted
}

func (stage DeclaredAmendmentStage) String() string {
	switch stage {
	case DeclaredAcceptedNotYetReceived:
		return "ACCEPTED_NOT_YET_RECEIVED"
	case DeclaredReceivedOrMeasured:
		return "RECEIVED_OR_MEASURED"
	case DeclaredLabelledOrBagged:
		return "LABELLED_OR_BAGGED"
	case DeclaredCustomsDataFormingNotSubmitted:
		return "CUSTOMS_DATA_FORMING_NOT_SUBMITTED"
	case DeclaredCustomsSubmitted:
		return "CUSTOMS_SUBMITTED"
	case DeclaredCaseClosedOrServiceCompleted:
		return "CASE_CLOSED_OR_SERVICE_COMPLETED"
	default:
		return ""
	}
}

// DeclaredAmendmentIntent 是矩阵一维的修订意图，三格原词的权威在 parcel-shipment（domain.AmendmentIntent）；
// 镜像纪律同 DeclaredAmendmentStage。意图进矩阵是 AT-PS-020 的机制半边：显式清空与改成新值在同一阶段的
// 允许性可以相反，矩阵收不到意图就登记不了那种规则。PS 那边加第四格（来源更正 / 撤销）那天，这里同步
// 加一格 + 一次迁移——那是镜像的已知代价。
type DeclaredAmendmentIntent uint8

const (
	DeclaredAmendmentIntentInvalid DeclaredAmendmentIntent = iota
	DeclaredSupplementIntent
	DeclaredCorrectionIntent
	DeclaredExplicitClearIntent
)

func (intent DeclaredAmendmentIntent) valid() bool {
	return intent >= DeclaredSupplementIntent && intent <= DeclaredExplicitClearIntent
}

func (intent DeclaredAmendmentIntent) String() string {
	switch intent {
	case DeclaredSupplementIntent:
		return "SUPPLEMENT"
	case DeclaredCorrectionIntent:
		return "CORRECTION"
	case DeclaredExplicitClearIntent:
		return "EXPLICIT_CLEAR"
	default:
		return ""
	}
}

// AmendmentAllowance 是「这一格能不能改」的三值。零值是「未声明」且刻意如此：它必须落在最保守的那一格
// （与 PS ports.SourceDataAmendmentAllowance 同一个理由）。三值里只有两值能登成一格（declarable）：
// 「未声明」是缺格的读法，不是一行的取值（ADR-0120 Decision 三）。
type AmendmentAllowance uint8

const (
	AmendmentAllowanceNotDeclared AmendmentAllowance = iota
	AmendmentAllowed
	AmendmentDisallowed
)

func (allowance AmendmentAllowance) declarable() bool {
	return allowance == AmendmentAllowed || allowance == AmendmentDisallowed
}

func (allowance AmendmentAllowance) String() string {
	switch allowance {
	case AmendmentAllowanceNotDeclared:
		return "NOT_DECLARED"
	case AmendmentAllowed:
		return "ALLOWED"
	case AmendmentDisallowed:
		return "DISALLOWED"
	default:
		return ""
	}
}

// SourceDataAmendmentRule 是矩阵的一格：某一资料组在某一阶段以某一意图，允许或不允许。Allowance 只能是
// 两值之一；零值（未声明）进构造门被拒。
type SourceDataAmendmentRule struct {
	DataGroup SourceDataGroupReference
	Stage     DeclaredAmendmentStage
	Intent    DeclaredAmendmentIntent
	Allowance AmendmentAllowance
}

// amendmentCell 是一格的键：三维。资料组按引用的字面比。
type amendmentCell struct {
	group  string
	stage  DeclaredAmendmentStage
	intent DeclaredAmendmentIntent
}

// SourceDataAmendmentAllowanceContent 是一个已生效接单规则包的资料修订允许声明（UC-PS-002「资料范围与
// 阶段边界」的提供方半边）：接受后客户原始资料在哪个阶段、以哪种意图、哪一组能不能改。产品与合同是采用方，
// 不拥有这份正文（ADR-0058 决定一的同一条纪律，ADR-0120 Decision 一）。
//
// closed 是登记方对缺格读法的显式选择（MCP-1 代裁 Q3）：未封闭时缺格读「未声明」（请求转复核），封闭时
// 缺格读「不允许」。封闭且零格是一句显式的话（这一版什么都不许改）；未封闭且零格什么都没说，不是声明。
// 「此刻在哪个阶段」由 parcel-shipment 判，本类型只对一个已判出的阶段答三值；本上下文不内置任何一格，
// 也不给任何资料组或阶段默认允许。
type SourceDataAmendmentAllowanceContent struct {
	owner  CommercialVersion
	closed bool
	rules  map[amendmentCell]SourceDataAmendmentRule
}

// NewSourceDataAmendmentAllowanceContent 组装声明。拥有对象必须是当前可用的接单规则包；未封闭至少一格；
// 每一格的资料组非空、阶段与意图在集内、允许性是两值之一（把「未声明」登成一格是缺件不是声明）；同一
// （资料组, 阶段, 意图）两格是冲突——即便两格取值相同：同键两行说明登记方自己没对齐，不替它挑一行。
func NewSourceDataAmendmentAllowanceContent(
	owner CommercialVersion,
	closed bool,
	rules []SourceDataAmendmentRule,
) (SourceDataAmendmentAllowanceContent, error) {
	if owner.kind != AcceptanceRulePackageObject ||
		owner.status != CommercialVersionEffective {
		return SourceDataAmendmentAllowanceContent{}, ErrUnusableRulePackage
	}
	if !closed && len(rules) == 0 {
		return SourceDataAmendmentAllowanceContent{}, ErrSourceDataAmendmentNotConfigured
	}
	byCell := make(map[amendmentCell]SourceDataAmendmentRule, len(rules))
	for _, rule := range rules {
		if !rule.DataGroup.valid() || !rule.Stage.valid() || !rule.Intent.valid() || !rule.Allowance.declarable() {
			return SourceDataAmendmentAllowanceContent{}, ErrSourceDataAmendmentNotConfigured
		}
		cell := amendmentCell{group: rule.DataGroup.String(), stage: rule.Stage, intent: rule.Intent}
		if _, exists := byCell[cell]; exists {
			return SourceDataAmendmentAllowanceContent{}, ErrConflictingSourceDataAmendment
		}
		byCell[cell] = rule
	}
	return SourceDataAmendmentAllowanceContent{owner: owner, closed: closed, rules: byCell}, nil
}

func (content SourceDataAmendmentAllowanceContent) Owner() CommercialVersion {
	return content.owner
}

// Closed 报出登记方对缺格的读法：true 即「没登的格一律不允许」。
func (content SourceDataAmendmentAllowanceContent) Closed() bool {
	return content.closed
}

// AllowanceFor 报告（资料组, 阶段, 意图）这一格能不能改（ADR-0120 Decision 四）：有格按格答；缺格未封闭
// 答「未声明」、封闭答「不允许」。三值在这里算出，消费方只做一对一翻译——「封闭意味着什么」是本上下文
// 的规则，不搬到消费方。
//
// 资料组为空、阶段或意图不在集内，一律答「未声明」而不看 closed：那不是一格，也不该被封闭标记折成
// 「不允许」——用一个判不出的阶段去拒绝，与用它去放行是同一个错的两面（BD-PS-010）。
func (content SourceDataAmendmentAllowanceContent) AllowanceFor(
	group SourceDataGroupReference,
	stage DeclaredAmendmentStage,
	intent DeclaredAmendmentIntent,
) AmendmentAllowance {
	if !group.valid() || !stage.valid() || !intent.valid() {
		return AmendmentAllowanceNotDeclared
	}
	if rule, declared := content.rules[amendmentCell{group: group.String(), stage: stage, intent: intent}]; declared {
		return rule.Allowance
	}
	if content.closed {
		return AmendmentDisallowed
	}
	return AmendmentAllowanceNotDeclared
}

// Rules 按（资料组, 阶段, 意图）的稳定顺序交回全部格（副本）。发布写入面按整份声明登记，需要枚举；
// 逐格取用仍走 AllowanceFor。
func (content SourceDataAmendmentAllowanceContent) Rules() []SourceDataAmendmentRule {
	rules := make([]SourceDataAmendmentRule, 0, len(content.rules))
	for _, rule := range content.rules {
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(left, right int) bool {
		if rules[left].DataGroup.String() != rules[right].DataGroup.String() {
			return rules[left].DataGroup.String() < rules[right].DataGroup.String()
		}
		if rules[left].Stage != rules[right].Stage {
			return rules[left].Stage < rules[right].Stage
		}
		return rules[left].Intent < rules[right].Intent
	})
	return rules
}
