package domain

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 本文件是接单规则包册接进服务端规范化的那一格（加册不换号，仍是 PCC-1——ADR-0126 Decision 一；票
// admin-write-faces/12）。这一册是十类里声明通道最多的一类：0014 的正文（五维适用性 + 按分类归档的规则引用）
// 与挂在同一版本上的各条声明——时点锚（0005）、接受内容、收寄资格与终局规则（0013，终局规则父行上多一格面单
// 有效期，ADR-0119）、资料修订允许（0027，ADR-0120）。受控批文把它们写成 declarations 下并列的几键；这里把
// 正文与全部声明折进本册一格、同一个摘要下——「一版规则包的发布是一次提交」（票 12「选形与理由」），键名镜像批文。
//
// **某一节整节缺席 = 该通道未声明**，不折成「无」：文档里省掉那一键，消费方照旧读到`未声明`。正文一节不可缺——
// 没有正文的规则包版本在册上无从显示，读面「规则包 / 版本」那一行就是 0014 那一层。
//
// 待路由许可（pendingRoutingBasis）**不在**本册：DeclarePendingRoutingPermission 只收已生效的服务产品版本
// （acceptance_content.go「规则包声明不了它：UC 把这份许可判给服务产品」），本册的载荷里没有它的位置。

// AcceptanceRulePackageBody 是接单规则包版本的正文输入面，以领域值对象给出：正文两格是 NewAcceptanceRulePackage 收的
// 那几项，各声明节是各自构造门收的那几项——只是不带拥有它们的（已生效）版本，预览与录入发生在发布之前。
//
// 声明节各自可缺（nil / 空切片 = 未声明）。FinalRuleValidity 不是一节，是终局规则那一节父行上的一格（ADR-0119
// Decision 五）：只给它不给 FinalRules，整项拒——有效期没有独立的拥有对象。
type AcceptanceRulePackageBody struct {
	Applicability       RulePackageApplicability
	Rules               []AssembledRule
	AsOfPolicies        []AsOfPolicy
	AcceptanceContent   *AcceptanceContentBody
	IntakeQualification *IntakeQualificationBody
	FinalRules          []FinalizationDeclaration
	FinalRuleValidity   *LabelValidityDeclaration
	SourceDataAmendment *SourceDataAmendmentBody
}

// AcceptanceContentBody 是接受内容声明那一节的输入面：适用哪些下游校验组、要不要人工复核（ADR-0042）。
// ManualReview 答的是「要不要」，不是「谁有权」——那一问归授权治理（wiring-baseline-remainder/04）。
type AcceptanceContentBody struct {
	ApplicableGroups []AcceptanceCheckGroupType
	ManualReview     ManualReviewDirective
}

// IntakeQualificationBody 是收寄资格声明那一节的输入面：允许来源与硬资格清单（清单可为显式空）。
type IntakeQualificationBody struct {
	Sources        []DeclaredIntakeSource
	Qualifications []RuleReference
}

// SourceDataAmendmentBody 是资料修订允许声明那一节的输入面（ADR-0120）：Closed 说缺格怎么读，没有默认——
// 它是这一节正文的一部分；Rules 在 Closed 为真时可为空（这一版什么都不许改）。
type SourceDataAmendmentBody struct {
	Closed bool
	Rules  []SourceDataAmendmentRule
}

// acceptanceRulePackageDeclarations 是正文与各声明节过完构造门之后的领域对象。规范化文档从这些对象的访问器
// 取值而不是从输入面：Policies() / AllowedSources() / Declarations() / Rules() 各自给出稳定顺序，文档的字节序
// 因此与发布写入面读到的顺序同源，不另定一套排序。
type acceptanceRulePackageDeclarations struct {
	pack       AcceptanceRulePackage
	asOf       *AsOfDeclaration
	acceptance *AcceptanceRuleContent
	intake     *IntakeQualificationContent
	final      *FinalRuleContent
	amendment  *SourceDataAmendmentAllowanceContent
}

// canonicalizationOwner 是各声明构造门要的那份「已生效接单规则包版本」的占位。门对它只核两格——类别与状态
// （归属纪律，ADR-0042/0058）；预览与录入发生在发布之前，那时没有已生效版本可挂，所以给一个只带这两格的值。
// 用它而不是把每道门的规则再抄一遍进 validate：抄出来的那半会在构造门改动时无声变旧。
var canonicalizationOwner = CommercialVersion{kind: AcceptanceRulePackageObject, status: CommercialVersionEffective}

// declare 把正文与各声明节逐节过与发布时相同的构造门。规则与资格引用两处多一道「同一（分类, 引用）/ 同一引用
// 两行」的拒绝：构造门不查它们，但库上主键（0014 acceptance_rule_package_rule、0013 intake_qualification_ref）
// 不收第二行——不在这里拒，同一份正文预览能过、发布却以一条唯一约束错误收场。
func (body AcceptanceRulePackageBody) declare() (acceptanceRulePackageDeclarations, error) {
	none := acceptanceRulePackageDeclarations{}
	if !body.Applicability.valid() {
		return none, ErrInvalidRulePackageApplicability
	}
	seenRules := make(map[string]struct{}, len(body.Rules))
	for _, rule := range body.Rules {
		key := rule.category.String() + "\x1f" + rule.reference.String()
		if _, exists := seenRules[key]; exists && rule.category.valid() && rule.reference.valid() {
			return none, fmt.Errorf("%w: rule %s/%s is assembled twice", ErrInvalidAcceptanceRulePackage, rule.category, rule.reference)
		}
		seenRules[key] = struct{}{}
	}
	pack, err := NewAcceptanceRulePackage(canonicalizationOwner, body.Applicability, body.Rules)
	if err != nil {
		return none, err
	}
	declared := acceptanceRulePackageDeclarations{pack: pack}

	if len(body.AsOfPolicies) > 0 {
		asOf, err := DeclareAsOfPolicies(canonicalizationOwner, body.AsOfPolicies)
		if err != nil {
			return none, err
		}
		declared.asOf = &asOf
	}
	if body.AcceptanceContent != nil {
		acceptance, err := DeclareAcceptanceRuleContent(canonicalizationOwner,
			body.AcceptanceContent.ApplicableGroups, body.AcceptanceContent.ManualReview)
		if err != nil {
			return none, err
		}
		declared.acceptance = &acceptance
	}
	if body.IntakeQualification != nil {
		seenQualifications := make(map[string]struct{}, len(body.IntakeQualification.Qualifications))
		for _, qualification := range body.IntakeQualification.Qualifications {
			if _, exists := seenQualifications[qualification.String()]; exists && qualification.valid() {
				return none, fmt.Errorf("%w: qualification %s is declared twice", ErrIntakeContentNotConfigured, qualification)
			}
			seenQualifications[qualification.String()] = struct{}{}
		}
		intake, err := NewIntakeQualificationContent(canonicalizationOwner,
			body.IntakeQualification.Sources, body.IntakeQualification.Qualifications)
		if err != nil {
			return none, err
		}
		declared.intake = &intake
	}
	if body.FinalRuleValidity != nil && len(body.FinalRules) == 0 {
		return none, fmt.Errorf("%w: a label validity declaration needs the final rule declarations it belongs to",
			ErrFinalContentNotConfigured)
	}
	if len(body.FinalRules) > 0 {
		var final FinalRuleContent
		if body.FinalRuleValidity != nil {
			if _, whole := labelValidityDurationString(body.FinalRuleValidity.Duration()); !whole {
				return none, fmt.Errorf("%w: duration must be a whole number of seconds to be declared", ErrInvalidLabelValidity)
			}
			final, err = NewFinalRuleContentWithValidity(canonicalizationOwner, body.FinalRules, *body.FinalRuleValidity)
		} else {
			final, err = NewFinalRuleContent(canonicalizationOwner, body.FinalRules)
		}
		if err != nil {
			return none, err
		}
		declared.final = &final
	}
	if body.SourceDataAmendment != nil {
		amendment, err := NewSourceDataAmendmentAllowanceContent(canonicalizationOwner,
			body.SourceDataAmendment.Closed, body.SourceDataAmendment.Rules)
		if err != nil {
			return none, err
		}
		declared.amendment = &amendment
	}
	return declared, nil
}

func (applicability RulePackageApplicability) valid() bool {
	return applicability.serviceProduct.valid() && applicability.contract.valid() && applicability.legalEntity.valid() &&
		applicability.scope.valid() && applicability.effective.valid()
}

// canonicalizeAcceptanceRulePackage 是 CanonicalizePublicationContent 里接单规则包那一支：正文缺席与立不住的正文
// 各答自己那一格（补正文 / 改正文），不折成一个「算不出」。
func canonicalizeAcceptanceRulePackage(content PublicationContent) (CanonicalPublicationContent, error) {
	if content.AcceptanceRulePackage == nil {
		return CanonicalPublicationContent{}, ErrPublicationContentAbsent
	}
	declared, err := content.AcceptanceRulePackage.declare()
	if err != nil {
		return CanonicalPublicationContent{}, err
	}
	return canonicalDigestOf(canonicalPublicationDocument{
		Canonicalization:      publicationCanonicalizationVersion,
		Kind:                  content.Kind.String(),
		AcceptanceRulePackage: canonicalAcceptanceRulePackageBodyOf(declared),
	})
}

// canonicalAcceptanceRulePackageBody 镜像批文 declarations 下归本册的各键：rulePackageBody（0014）、asOfPolicies、
// acceptanceContent、intakeQualification、finalRules 与它的兄弟键 finalRuleValidity（ADR-0119 Decision 五）、
// sourceDataAmendment。缺席的节整键省略——文档的字节只由在场的节决定。
type canonicalAcceptanceRulePackageBody struct {
	RulePackageBody     canonicalRulePackageBody      `json:"rulePackageBody"`
	AsOfPolicies        []canonicalAsOfPolicy         `json:"asOfPolicies,omitempty"`
	AcceptanceContent   *canonicalAcceptanceContent   `json:"acceptanceContent,omitempty"`
	IntakeQualification *canonicalIntakeQualification `json:"intakeQualification,omitempty"`
	FinalRules          []canonicalFinalRule          `json:"finalRules,omitempty"`
	FinalRuleValidity   *canonicalFinalRuleValidity   `json:"finalRuleValidity,omitempty"`
	SourceDataAmendment *canonicalSourceDataAmendment `json:"sourceDataAmendment,omitempty"`
}

// canonicalRulePackageBody 镜像批文 rulePackageBodyDocument：五维适用性（区间上界可缺，时刻一律 UTC RFC 3339 纳秒）
// 与按（分类序, 引用）排好的规则表——表单里换行序不是换正文，摘要不该跟着变。
type canonicalRulePackageBody struct {
	ServiceProduct    string                   `json:"serviceProduct"`
	Contract          string                   `json:"contract"`
	LegalEntity       string                   `json:"legalEntity"`
	Scope             string                   `json:"scope"`
	EffectiveStartsAt string                   `json:"effectiveStartsAt"`
	EffectiveEndsAt   string                   `json:"effectiveEndsAt,omitempty"`
	Rules             []canonicalAssembledRule `json:"rules"`
}

type canonicalAssembledRule struct {
	Category  string `json:"category"`
	Reference string `json:"reference"`
}

type canonicalAsOfPolicy struct {
	Judgment      string `json:"judgment"`
	Semantics     string `json:"semantics"`
	PolicyVersion string `json:"policyVersion"`
}

type canonicalAcceptanceContent struct {
	ApplicableGroups []string `json:"applicableGroups"`
	ManualReview     string   `json:"manualReview"`
}

// canonicalIntakeQualification 的 qualifications 可缺：显式空清单（没有硬资格）与批文里缺键是同一句话，两者
// 折成同一份字节。
type canonicalIntakeQualification struct {
	Sources        []string `json:"sources"`
	Qualifications []string `json:"qualifications,omitempty"`
}

type canonicalFinalRule struct {
	Outcome   string `json:"outcome"`
	FinalKind string `json:"finalKind"`
}

// canonicalFinalRuleValidity 的 duration 写成 ISO-8601 子集的规范形（labelValidityDurationString）：同一时长只有一种
// 写法，`PT36H` 与 `P1DT12H` 折成同一份字节。
type canonicalFinalRuleValidity struct {
	Anchor   string `json:"anchor"`
	Duration string `json:"duration"`
}

type canonicalSourceDataAmendment struct {
	Closed bool                               `json:"closed"`
	Rules  []canonicalSourceDataAmendmentRule `json:"rules,omitempty"`
}

type canonicalSourceDataAmendmentRule struct {
	DataGroup string `json:"dataGroup"`
	Stage     string `json:"stage"`
	Intent    string `json:"intent"`
	Allowance string `json:"allowance"`
}

func canonicalAcceptanceRulePackageBodyOf(declared acceptanceRulePackageDeclarations) *canonicalAcceptanceRulePackageBody {
	applicability := declared.pack.Applicability()
	document := &canonicalAcceptanceRulePackageBody{
		RulePackageBody: canonicalRulePackageBody{
			ServiceProduct:    applicability.ServiceProduct().String(),
			Contract:          applicability.Contract().String(),
			LegalEntity:       applicability.LegalEntity().String(),
			Scope:             applicability.Scope().String(),
			EffectiveStartsAt: canonicalTime(applicability.Effective().StartsAt()),
			Rules:             make([]canonicalAssembledRule, 0),
		},
	}
	if endsAt, bounded := applicability.Effective().EndsAt(); bounded {
		document.RulePackageBody.EffectiveEndsAt = canonicalTime(endsAt)
	}
	for _, category := range closedCodesOf(RuleCategory.valid) {
		rules := declared.pack.RulesIn(category)
		sort.Slice(rules, func(left, right int) bool {
			return rules[left].Reference().String() < rules[right].Reference().String()
		})
		for _, rule := range rules {
			document.RulePackageBody.Rules = append(document.RulePackageBody.Rules,
				canonicalAssembledRule{Category: category.String(), Reference: rule.Reference().String()})
		}
	}
	if declared.asOf != nil {
		for _, policy := range declared.asOf.Policies() {
			document.AsOfPolicies = append(document.AsOfPolicies, canonicalAsOfPolicy{
				Judgment:      policy.Judgment().String(),
				Semantics:     policy.Semantics().String(),
				PolicyVersion: policy.PolicyVersion().String(),
			})
		}
	}
	if declared.acceptance != nil {
		groups := declared.acceptance.ApplicableGroups()
		sort.Slice(groups, func(left, right int) bool { return groups[left] < groups[right] })
		document.AcceptanceContent = &canonicalAcceptanceContent{
			ApplicableGroups: make([]string, 0, len(groups)),
			ManualReview:     declared.acceptance.ManualReview().String(),
		}
		for _, group := range groups {
			document.AcceptanceContent.ApplicableGroups = append(document.AcceptanceContent.ApplicableGroups, group.String())
		}
	}
	if declared.intake != nil {
		document.IntakeQualification = &canonicalIntakeQualification{Sources: make([]string, 0)}
		for _, source := range declared.intake.AllowedSources() {
			document.IntakeQualification.Sources = append(document.IntakeQualification.Sources, source.String())
		}
		for _, qualification := range declared.intake.Qualifications() {
			document.IntakeQualification.Qualifications = append(document.IntakeQualification.Qualifications, qualification.String())
		}
		sort.Strings(document.IntakeQualification.Qualifications)
	}
	if declared.final != nil {
		for _, declaration := range declared.final.Declarations() {
			document.FinalRules = append(document.FinalRules, canonicalFinalRule{
				Outcome:   declaration.Outcome.String(),
				FinalKind: declaration.FinalKind.String(),
			})
		}
		if validity, present := declared.final.Validity(); present {
			// declare 已把非整秒的时长拒在门外，这里的第二个返回值只可能为真。
			duration, _ := labelValidityDurationString(validity.Duration())
			document.FinalRuleValidity = &canonicalFinalRuleValidity{Anchor: validity.Anchor().String(), Duration: duration}
		}
	}
	if declared.amendment != nil {
		document.SourceDataAmendment = &canonicalSourceDataAmendment{Closed: declared.amendment.Closed()}
		for _, rule := range declared.amendment.Rules() {
			document.SourceDataAmendment.Rules = append(document.SourceDataAmendment.Rules, canonicalSourceDataAmendmentRule{
				DataGroup: rule.DataGroup.String(),
				Stage:     rule.Stage.String(),
				Intent:    rule.Intent.String(),
				Allowance: rule.Allowance.String(),
			})
		}
	}
	return document
}

// body 把文档里的一节折回领域正文。每一格过构造门或按 String() 原词反查；跨格的问题（同键两行、未封闭零格、
// 只有有效期没有终局行）留给 declare——快照是数据，正文立不立得住仍由构造门说。
func (document canonicalAcceptanceRulePackageBody) body() (AcceptanceRulePackageBody, error) {
	none := AcceptanceRulePackageBody{}
	applicability, err := document.RulePackageBody.applicability()
	if err != nil {
		return none, fmt.Errorf("rulePackageBody: %w", err)
	}
	body := AcceptanceRulePackageBody{Applicability: applicability}
	for _, row := range document.RulePackageBody.Rules {
		category, known := RuleCategoryNamed(row.Category)
		if !known {
			return none, fmt.Errorf("rulePackageBody.rules: %w: category %q", ErrInvalidAssembledRule, row.Category)
		}
		reference, err := NewRuleReference(row.Reference)
		if err != nil {
			return none, fmt.Errorf("rulePackageBody.rules: %w", err)
		}
		rule, err := NewAssembledRule(category, reference)
		if err != nil {
			return none, fmt.Errorf("rulePackageBody.rules: %w", err)
		}
		body.Rules = append(body.Rules, rule)
	}
	for _, row := range document.AsOfPolicies {
		judgment, known := JudgmentTypeNamed(row.Judgment)
		if !known {
			return none, fmt.Errorf("asOfPolicies: %w: judgment %q", ErrAsOfPolicyNotConfigured, row.Judgment)
		}
		semantics, err := NewAsOfSemanticsReference(row.Semantics)
		if err != nil {
			return none, fmt.Errorf("asOfPolicies: %w", err)
		}
		version, err := NewAsOfPolicyVersion(row.PolicyVersion)
		if err != nil {
			return none, fmt.Errorf("asOfPolicies: %w", err)
		}
		policy, err := NewAsOfPolicy(judgment, semantics, version)
		if err != nil {
			return none, fmt.Errorf("asOfPolicies: %w", err)
		}
		body.AsOfPolicies = append(body.AsOfPolicies, policy)
	}
	if document.AcceptanceContent != nil {
		content := &AcceptanceContentBody{}
		for _, name := range document.AcceptanceContent.ApplicableGroups {
			group, known := AcceptanceCheckGroupTypeNamed(name)
			if !known {
				return none, fmt.Errorf("acceptanceContent.applicableGroups: %w: %q", ErrAcceptanceContentNotConfigured, name)
			}
			content.ApplicableGroups = append(content.ApplicableGroups, group)
		}
		review, known := ManualReviewDirectiveNamed(document.AcceptanceContent.ManualReview)
		if !known {
			return none, fmt.Errorf("acceptanceContent.manualReview: %w: %q", ErrAcceptanceContentNotConfigured, document.AcceptanceContent.ManualReview)
		}
		content.ManualReview = review
		body.AcceptanceContent = content
	}
	if document.IntakeQualification != nil {
		intake := &IntakeQualificationBody{}
		for _, name := range document.IntakeQualification.Sources {
			source, known := DeclaredIntakeSourceNamed(name)
			if !known {
				return none, fmt.Errorf("intakeQualification.sources: %w: %q", ErrIntakeContentNotConfigured, name)
			}
			intake.Sources = append(intake.Sources, source)
		}
		for _, raw := range document.IntakeQualification.Qualifications {
			qualification, err := NewRuleReference(raw)
			if err != nil {
				return none, fmt.Errorf("intakeQualification.qualifications: %w", err)
			}
			intake.Qualifications = append(intake.Qualifications, qualification)
		}
		body.IntakeQualification = intake
	}
	for _, row := range document.FinalRules {
		outcome, known := DeclaredResponsibilityOutcomeNamed(row.Outcome)
		if !known {
			return none, fmt.Errorf("finalRules: %w: outcome %q", ErrFinalContentNotConfigured, row.Outcome)
		}
		finalKind, err := NewRuleReference(row.FinalKind)
		if err != nil {
			return none, fmt.Errorf("finalRules: %w", err)
		}
		body.FinalRules = append(body.FinalRules, FinalizationDeclaration{Outcome: outcome, FinalKind: finalKind})
	}
	if document.FinalRuleValidity != nil {
		anchor, known := ValidityAnchorKindNamed(document.FinalRuleValidity.Anchor)
		if !known {
			return none, fmt.Errorf("finalRuleValidity.anchor: %w: %q", ErrInvalidLabelValidity, document.FinalRuleValidity.Anchor)
		}
		duration, err := ParseLabelValidityDuration(document.FinalRuleValidity.Duration)
		if err != nil {
			return none, fmt.Errorf("finalRuleValidity.duration: %w", err)
		}
		validity, err := NewLabelValidityDeclaration(anchor, duration)
		if err != nil {
			return none, fmt.Errorf("finalRuleValidity: %w", err)
		}
		body.FinalRuleValidity = &validity
	}
	if document.SourceDataAmendment != nil {
		amendment := &SourceDataAmendmentBody{Closed: document.SourceDataAmendment.Closed}
		for _, row := range document.SourceDataAmendment.Rules {
			rule, err := sourceDataAmendmentRuleOf(row.DataGroup, row.Stage, row.Intent, row.Allowance)
			if err != nil {
				return none, fmt.Errorf("sourceDataAmendment.rules: %w", err)
			}
			amendment.Rules = append(amendment.Rules, rule)
		}
		body.SourceDataAmendment = amendment
	}
	if _, err := body.declare(); err != nil {
		return none, err
	}
	return body, nil
}

func (document canonicalRulePackageBody) applicability() (RulePackageApplicability, error) {
	none := RulePackageApplicability{}
	serviceProduct, err := NewCommercialObjectID(document.ServiceProduct)
	if err != nil {
		return none, fmt.Errorf("serviceProduct: %w", err)
	}
	contract, err := NewCommercialObjectID(document.Contract)
	if err != nil {
		return none, fmt.Errorf("contract: %w", err)
	}
	legalEntity, err := NewLegalEntityReference(document.LegalEntity)
	if err != nil {
		return none, err
	}
	scope, err := NewCommercialScopeReference(document.Scope)
	if err != nil {
		return none, err
	}
	startsAt, err := time.Parse(time.RFC3339Nano, document.EffectiveStartsAt)
	if err != nil {
		return none, fmt.Errorf("effectiveStartsAt: %w", err)
	}
	var endsAt time.Time
	if document.EffectiveEndsAt != "" {
		if endsAt, err = time.Parse(time.RFC3339Nano, document.EffectiveEndsAt); err != nil {
			return none, fmt.Errorf("effectiveEndsAt: %w", err)
		}
	}
	effective, err := NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		return none, err
	}
	return NewRulePackageApplicability(serviceProduct, contract, legalEntity, scope, effective)
}

// sourceDataAmendmentRuleOf 把一格的四个原词折回领域值对象：资料组过构造门（开放引用只查非空），阶段 / 意图 /
// 允许性按 String() 反查，集合外与「未声明」都是这一格立不住（ErrSourceDataAmendmentNotConfigured 那一格）。
func sourceDataAmendmentRuleOf(dataGroup, stage, intent, allowance string) (SourceDataAmendmentRule, error) {
	group, err := NewSourceDataGroupReference(dataGroup)
	if err != nil {
		return SourceDataAmendmentRule{}, err
	}
	declaredStage, known := DeclaredAmendmentStageNamed(stage)
	if !known {
		return SourceDataAmendmentRule{}, fmt.Errorf("%w: stage %q", ErrSourceDataAmendmentNotConfigured, stage)
	}
	declaredIntent, known := DeclaredAmendmentIntentNamed(intent)
	if !known {
		return SourceDataAmendmentRule{}, fmt.Errorf("%w: intent %q", ErrSourceDataAmendmentNotConfigured, intent)
	}
	declaredAllowance, known := AmendmentAllowanceNamed(allowance)
	if !known {
		return SourceDataAmendmentRule{}, fmt.Errorf("%w: allowance %q", ErrSourceDataAmendmentNotConfigured, allowance)
	}
	return SourceDataAmendmentRule{DataGroup: group, Stage: declaredStage, Intent: declaredIntent, Allowance: declaredAllowance}, nil
}

// ---- 封闭集反查：与 String() 同一份名单 ----
//
// 规范化文档、运营操作者面载荷与词表读口说的都是 String() 那一个词；这里只是反查，名单在各枚举的 String() 一处。
// 扫的是接受判据（valid / Declared / declarable）而不是首末常量：集合的边界就是判据的边界，不另抄一遍。

func closedCodeNamed[Code ~uint8](accepts func(Code) bool, name func(Code) string, raw string) (Code, bool) {
	if raw == "" {
		return Code(0), false
	}
	for candidate := 0; candidate <= math.MaxUint8; candidate++ {
		code := Code(candidate)
		if accepts(code) && name(code) == raw {
			return code, true
		}
	}
	return Code(0), false
}

// closedCodesOf 按枚举声明顺序列出被 accepts 接受的每个值。
func closedCodesOf[Code ~uint8](accepts func(Code) bool) []Code {
	codes := make([]Code, 0)
	for raw := 0; raw <= math.MaxUint8; raw++ {
		if code := Code(raw); accepts(code) {
			codes = append(codes, code)
		}
	}
	return codes
}

func RuleCategoryNamed(name string) (RuleCategory, bool) {
	return closedCodeNamed(RuleCategory.valid, RuleCategory.String, name)
}

func JudgmentTypeNamed(name string) (JudgmentType, bool) {
	return closedCodeNamed(JudgmentType.valid, JudgmentType.String, name)
}

func AcceptanceCheckGroupTypeNamed(name string) (AcceptanceCheckGroupType, bool) {
	return closedCodeNamed(AcceptanceCheckGroupType.valid, AcceptanceCheckGroupType.String, name)
}

// ManualReviewDirectiveNamed 只认两个已声明的值：「未声明」是零值的读法，不是一格能写的取值。
func ManualReviewDirectiveNamed(name string) (ManualReviewDirective, bool) {
	return closedCodeNamed(ManualReviewDirective.Declared, ManualReviewDirective.String, name)
}

func DeclaredIntakeSourceNamed(name string) (DeclaredIntakeSource, bool) {
	return closedCodeNamed(DeclaredIntakeSource.valid, DeclaredIntakeSource.String, name)
}

func DeclaredResponsibilityOutcomeNamed(name string) (DeclaredResponsibilityOutcome, bool) {
	return closedCodeNamed(DeclaredResponsibilityOutcome.valid, DeclaredResponsibilityOutcome.String, name)
}

func ValidityAnchorKindNamed(name string) (ValidityAnchorKind, bool) {
	return closedCodeNamed(ValidityAnchorKind.valid, ValidityAnchorKind.String, name)
}

func DeclaredAmendmentStageNamed(name string) (DeclaredAmendmentStage, bool) {
	return closedCodeNamed(DeclaredAmendmentStage.valid, DeclaredAmendmentStage.String, name)
}

func DeclaredAmendmentIntentNamed(name string) (DeclaredAmendmentIntent, bool) {
	return closedCodeNamed(DeclaredAmendmentIntent.valid, DeclaredAmendmentIntent.String, name)
}

// AmendmentAllowanceNamed 只认 ALLOWED / DISALLOWED：NOT_DECLARED 是缺格的读法，写成一格就是把「没说」登成了「说了」
// （ADR-0120 Decision 三）。
func AmendmentAllowanceNamed(name string) (AmendmentAllowance, bool) {
	return closedCodeNamed(AmendmentAllowance.declarable, AmendmentAllowance.String, name)
}

// ---- 面单有效期时长：ISO-8601 子集 `P[nD][T[nH][nM][nS]]` ----
//
// 年 / 月 / 周与小数段一律不收——年与月不是固定时长，周只是天的别写，小数在库上 interval 的微秒段之外还多一层
// 精度约定；子集最小（理由同受控批文 cmd/parcel-commercial 那一侧的 finalRuleValidityDocument）。运营操作者面
// 载荷与规范化文档共用这一份语法：表单填的、文档写的、快照折回的是同一种串。

const (
	labelValidityDay    = 24 * time.Hour
	labelValidityHour   = time.Hour
	labelValidityMinute = time.Minute
	labelValiditySecond = time.Second
)

// ParseLabelValidityDuration 解析子集里的一个时长：各段整数、按 D → T → H → M → S 的顺序至多一次、至少一段、
// `T` 之后必须有段。零时长在这里放行、由 NewLabelValidityDeclaration 拒：「P0D 立不住」是声明的话，这里只认形状。
func ParseLabelValidityDuration(raw string) (time.Duration, error) {
	if len(raw) < 2 || raw[0] != 'P' {
		return 0, fmt.Errorf("%w: duration %q is not a P-prefixed ISO-8601 duration", ErrInvalidLabelValidity, raw)
	}
	rest := raw[1:]
	var total time.Duration
	segments := 0
	inTime := false
	order := 0
	position := map[byte]int{'D': 1, 'H': 3, 'M': 4, 'S': 5}
	unit := map[byte]time.Duration{'D': labelValidityDay, 'H': labelValidityHour, 'M': labelValidityMinute, 'S': labelValiditySecond}
	for len(rest) > 0 {
		if rest[0] == 'T' {
			if inTime || order > 1 {
				return 0, fmt.Errorf("%w: duration %q places T twice or after a time segment", ErrInvalidLabelValidity, raw)
			}
			inTime = true
			order = 2
			rest = rest[1:]
			if len(rest) == 0 {
				return 0, fmt.Errorf("%w: duration %q has nothing after T", ErrInvalidLabelValidity, raw)
			}
			continue
		}
		digits := 0
		for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
			digits++
		}
		if digits == 0 || digits == len(rest) {
			return 0, fmt.Errorf("%w: duration %q has a segment that is not an integer followed by a unit", ErrInvalidLabelValidity, raw)
		}
		designator := rest[digits]
		segmentOrder, known := position[designator]
		if !known {
			return 0, fmt.Errorf("%w: duration %q uses unit %q (years, months, weeks and fractions are outside the subset)",
				ErrInvalidLabelValidity, raw, string(designator))
		}
		if (designator == 'D') == inTime {
			return 0, fmt.Errorf("%w: duration %q puts unit %q on the wrong side of T", ErrInvalidLabelValidity, raw, string(designator))
		}
		if segmentOrder <= order {
			return 0, fmt.Errorf("%w: duration %q repeats or reorders unit %q", ErrInvalidLabelValidity, raw, string(designator))
		}
		value, err := strconv.ParseInt(rest[:digits], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: duration %q: %v", ErrInvalidLabelValidity, raw, err)
		}
		total += time.Duration(value) * unit[designator]
		order = segmentOrder
		segments++
		rest = rest[digits+1:]
	}
	if segments == 0 {
		return 0, fmt.Errorf("%w: duration %q has no segment", ErrInvalidLabelValidity, raw)
	}
	return total, nil
}

// labelValidityDurationString 把时长写成子集的规范形：按天、时、分、秒贪心分解，零段省略，全零写 `PT0S`。不是整秒
// 的时长子集写不出来，第二个返回值为假——那一格由 declare 拒，不在这里四舍五入。
func labelValidityDurationString(duration time.Duration) (string, bool) {
	if duration < 0 || duration%labelValiditySecond != 0 {
		return "", false
	}
	var builder strings.Builder
	builder.WriteByte('P')
	if days := duration / labelValidityDay; days > 0 {
		builder.WriteString(strconv.FormatInt(int64(days), 10))
		builder.WriteByte('D')
		duration -= days * labelValidityDay
	}
	if duration > 0 || builder.Len() == 1 {
		builder.WriteByte('T')
		wrote := false
		for _, segment := range []struct {
			unit       time.Duration
			designator byte
		}{{labelValidityHour, 'H'}, {labelValidityMinute, 'M'}, {labelValiditySecond, 'S'}} {
			if count := duration / segment.unit; count > 0 {
				builder.WriteString(strconv.FormatInt(int64(count), 10))
				builder.WriteByte(segment.designator)
				duration -= count * segment.unit
				wrote = true
			}
		}
		if !wrote {
			builder.WriteString("0S")
		}
	}
	return builder.String(), true
}
