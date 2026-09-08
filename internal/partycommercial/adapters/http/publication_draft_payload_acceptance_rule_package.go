package commercialhttp

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件是接单规则包册在运营操作者面载荷里的那一格（票 admin-write-faces/12）。一格分节：0014 的正文（rulePackageBody）
// 与挂在同一版本上的各条声明各占一节——asOfPolicies、acceptanceContent、intakeQualification、finalRules 与它父行上的
// finalRuleValidity、sourceDataAmendment；键名镜像受控批文 declarations 下的同名键与规范化文档——同一册在三处说同一套词。
//
// **某一节整节缺席 = 该通道未声明**（票 12「选形与理由」）：缺键不折成「无」，服务端照旧不为那一通道写任何东西。正文一节
// 不可缺——它是指针，是为了让「整节没给」与「给了但某格留白」在逐格问题里落在不同的格。封闭集的取值一律按 String()
// 原词反查，集合外含空串都是那一格的问题；词从哪来由词表读口（票 20）供，表单不内置。
//
// 待路由许可不在本册载荷里：它挂在服务产品版本上（domain.DeclarePendingRoutingPermission），写进来按未知键拒。

// AcceptanceRulePackageBodyPayload 是接单规则包册的正文载荷。
type AcceptanceRulePackageBodyPayload struct {
	RulePackageBody     *RulePackageBodyPayload     `json:"rulePackageBody"`
	AsOfPolicies        []AsOfPolicyPayload         `json:"asOfPolicies,omitempty"`
	AcceptanceContent   *AcceptanceContentPayload   `json:"acceptanceContent,omitempty"`
	IntakeQualification *IntakeQualificationPayload `json:"intakeQualification,omitempty"`
	FinalRules          []FinalRulePayload          `json:"finalRules,omitempty"`
	FinalRuleValidity   *FinalRuleValidityPayload   `json:"finalRuleValidity,omitempty"`
	SourceDataAmendment *SourceDataAmendmentPayload `json:"sourceDataAmendment,omitempty"`
}

// RulePackageBodyPayload 镜像批文 rulePackageBodyDocument：五维适用性（区间上界可缺）与按分类归档的规则引用表。
// 零行由领域构造门拒（空规则包等于无条件接受，规则包表达不了那种东西），这里不代判。
type RulePackageBodyPayload struct {
	ServiceProduct    string                 `json:"serviceProduct"`
	Contract          string                 `json:"contract"`
	LegalEntity       string                 `json:"legalEntity"`
	Scope             string                 `json:"scope"`
	EffectiveStartsAt string                 `json:"effectiveStartsAt"`
	EffectiveEndsAt   string                 `json:"effectiveEndsAt,omitempty"`
	Rules             []AssembledRulePayload `json:"rules,omitempty"`
}

// AssembledRulePayload 是一行规则：分类（封闭集原词）× 规则引用（开放引用，属执行它的那个上下文）。
type AssembledRulePayload struct {
	Category  string `json:"category"`
	Reference string `json:"reference"`
}

// AsOfPolicyPayload 是一条时点锚：判断类型（封闭集原词）× 时点语义引用 × 时点政策版本（两者都是 PAR-COM-14 的开放引用）。
type AsOfPolicyPayload struct {
	Judgment      string `json:"judgment"`
	Semantics     string `json:"semantics"`
	PolicyVersion string `json:"policyVersion"`
}

// AcceptanceContentPayload 镜像批文 acceptanceContentDocument：适用校验组（封闭集原词，至少一格由领域拒）与人工复核
// 指令（只收 REQUIRED / NOT_REQUIRED——「要不要人工复核」，不是「谁有权」）。
type AcceptanceContentPayload struct {
	ApplicableGroups []string `json:"applicableGroups,omitempty"`
	ManualReview     string   `json:"manualReview"`
}

// IntakeQualificationPayload 镜像批文 intakeQualificationDocument：允许来源（封闭集原词，至少一格由领域拒）与硬资格
// 清单（开放引用，可为显式空——真没有硬资格也要声明来源允许）。
type IntakeQualificationPayload struct {
	Sources        []string `json:"sources,omitempty"`
	Qualifications []string `json:"qualifications,omitempty"`
}

// FinalRulePayload 是一行终局规则：责任结果（封闭集原词）× 终局种类（开放引用，网络服务不统一规定跨产品终局集合）。
type FinalRulePayload struct {
	Outcome   string `json:"outcome"`
	FinalKind string `json:"finalKind"`
}

// FinalRuleValidityPayload 镜像批文 finalRuleValidityDocument（ADR-0119）：起算时刻种类（封闭集原词）× 时长（ISO-8601
// 子集 `P[nD][T[nH][nM][nS]]`，年 / 月 / 周不收，语法由领域 ParseLabelValidityDuration 定）。只给这一格不给 finalRules
// 由领域整项拒，这里不代判。
type FinalRuleValidityPayload struct {
	Anchor   string `json:"anchor"`
	Duration string `json:"duration"`
}

// SourceDataAmendmentPayload 镜像批文 sourceDataAmendmentDocument（ADR-0120）。closed 是 *bool：JSON 的 false 与缺席分不开，
// 而它是这一节正文的一部分——false 是「缺格转复核」、true 是「缺格即不允许」，两句话都要登记方自己说，表单不给默认、
// 这里不代填。rules 在 closed=true 时可省；closed=false 时零格由领域拒。
type SourceDataAmendmentPayload struct {
	Closed *bool                            `json:"closed"`
	Rules  []SourceDataAmendmentRulePayload `json:"rules,omitempty"`
}

// SourceDataAmendmentRulePayload 是矩阵的一格：资料组（开放引用）× 阶段 × 意图（parcel-shipment 原词）× 允许性
// （只收 ALLOWED / DISALLOWED，NOT_DECLARED 是缺格的读法不是一格的取值）。
type SourceDataAmendmentRulePayload struct {
	DataGroup string `json:"dataGroup"`
	Stage     string `json:"stage"`
	Intent    string `json:"intent"`
	Allowance string `json:"allowance"`
}

// body 把接单规则包载荷逐格过领域构造门或反查。行按下标点名（rules[i] / asOfPolicies[i] / …）；跨格的问题（同键两行、
// 空组、未封闭零格、只有有效期没有终局行）留给领域规范化，预览会把它们答成`未受理`带成因。
func (payload AcceptanceRulePackageBodyPayload) body(problems *PublicationPayloadProblems) domain.AcceptanceRulePackageBody {
	const root = "acceptanceRulePackage"
	var body domain.AcceptanceRulePackageBody
	if payload.RulePackageBody == nil {
		problems.add(root+".rulePackageBody", fmt.Errorf("规则包正文（五维适用性与规则表）须在场"))
	} else {
		body.Applicability, body.Rules = payload.RulePackageBody.body(problems, root+".rulePackageBody")
	}
	for index, row := range payload.AsOfPolicies {
		body.AsOfPolicies = append(body.AsOfPolicies, row.policy(problems, fmt.Sprintf("%s.asOfPolicies[%d]", root, index)))
	}
	if payload.AcceptanceContent != nil {
		content := payload.AcceptanceContent.body(problems, root+".acceptanceContent")
		body.AcceptanceContent = &content
	}
	if payload.IntakeQualification != nil {
		intake := payload.IntakeQualification.body(problems, root+".intakeQualification")
		body.IntakeQualification = &intake
	}
	for index, row := range payload.FinalRules {
		body.FinalRules = append(body.FinalRules, row.declaration(problems, fmt.Sprintf("%s.finalRules[%d]", root, index)))
	}
	if payload.FinalRuleValidity != nil {
		if validity, ok := payload.FinalRuleValidity.declaration(problems, root+".finalRuleValidity"); ok {
			body.FinalRuleValidity = &validity
		}
	}
	if payload.SourceDataAmendment != nil {
		amendment := payload.SourceDataAmendment.body(problems, root+".sourceDataAmendment")
		body.SourceDataAmendment = &amendment
	}
	return body
}

// body 把正文一节折成五维适用性与规则表。五维每格各自过构造门后再拼——某格已记过问题时不再拼，免得把零值再记一遍。
func (payload RulePackageBodyPayload) body(problems *PublicationPayloadProblems, field string) (domain.RulePackageApplicability, []domain.AssembledRule) {
	before := len(problems.Problems)
	serviceProduct := requireField(problems, field+".serviceProduct", domain.NewCommercialObjectID, payload.ServiceProduct)
	contract := requireField(problems, field+".contract", domain.NewCommercialObjectID, payload.Contract)
	legalEntity := requireField(problems, field+".legalEntity", domain.NewLegalEntityReference, payload.LegalEntity)
	scope := requireField(problems, field+".scope", domain.NewCommercialScopeReference, payload.Scope)
	effective := intervalField(problems, field+".effectiveStartsAt", field+".effectiveEndsAt", payload.EffectiveStartsAt, payload.EffectiveEndsAt)
	var applicability domain.RulePackageApplicability
	if len(problems.Problems) == before {
		built, err := domain.NewRulePackageApplicability(serviceProduct, contract, legalEntity, scope, effective)
		if err != nil {
			problems.add(field, err)
		}
		applicability = built
	}
	rules := make([]domain.AssembledRule, 0, len(payload.Rules))
	for index, row := range payload.Rules {
		rules = append(rules, row.rule(problems, fmt.Sprintf("%s.rules[%d]", field, index)))
	}
	return applicability, rules
}

func (payload AssembledRulePayload) rule(problems *PublicationPayloadProblems, field string) domain.AssembledRule {
	before := len(problems.Problems)
	category, known := domain.RuleCategoryNamed(payload.Category)
	if !known {
		problems.add(field+".category", fmt.Errorf("集合外的规则分类 %q", payload.Category))
	}
	reference := requireField(problems, field+".reference", domain.NewRuleReference, payload.Reference)
	if len(problems.Problems) != before {
		return domain.AssembledRule{}
	}
	rule, err := domain.NewAssembledRule(category, reference)
	if err != nil {
		problems.add(field, err)
	}
	return rule
}

func (payload AsOfPolicyPayload) policy(problems *PublicationPayloadProblems, field string) domain.AsOfPolicy {
	before := len(problems.Problems)
	judgment, known := domain.JudgmentTypeNamed(payload.Judgment)
	if !known {
		problems.add(field+".judgment", fmt.Errorf("集合外的判断类型 %q", payload.Judgment))
	}
	semantics := requireField(problems, field+".semantics", domain.NewAsOfSemanticsReference, payload.Semantics)
	version := requireField(problems, field+".policyVersion", domain.NewAsOfPolicyVersion, payload.PolicyVersion)
	if len(problems.Problems) != before {
		return domain.AsOfPolicy{}
	}
	policy, err := domain.NewAsOfPolicy(judgment, semantics, version)
	if err != nil {
		problems.add(field, err)
	}
	return policy
}

// body 把接受内容一节折成领域输入面：组逐格反查（applicableGroups[i]），人工复核按两值反查——集合外含空串都是那一格的问题；
// 空组由领域答（ErrAcceptanceContentNotConfigured），不在这里代判。
func (payload AcceptanceContentPayload) body(problems *PublicationPayloadProblems, field string) domain.AcceptanceContentBody {
	body := domain.AcceptanceContentBody{}
	for index, name := range payload.ApplicableGroups {
		group, known := domain.AcceptanceCheckGroupTypeNamed(name)
		if !known {
			problems.add(fmt.Sprintf("%s.applicableGroups[%d]", field, index), fmt.Errorf("集合外的校验组 %q", name))
		}
		body.ApplicableGroups = append(body.ApplicableGroups, group)
	}
	review, known := domain.ManualReviewDirectiveNamed(payload.ManualReview)
	if !known {
		problems.add(field+".manualReview", fmt.Errorf("集合外的人工复核指令 %q（只收 REQUIRED / NOT_REQUIRED）", payload.ManualReview))
	}
	body.ManualReview = review
	return body
}

func (payload IntakeQualificationPayload) body(problems *PublicationPayloadProblems, field string) domain.IntakeQualificationBody {
	body := domain.IntakeQualificationBody{}
	for index, name := range payload.Sources {
		source, known := domain.DeclaredIntakeSourceNamed(name)
		if !known {
			problems.add(fmt.Sprintf("%s.sources[%d]", field, index), fmt.Errorf("集合外的收寄来源 %q", name))
		}
		body.Sources = append(body.Sources, source)
	}
	for index, raw := range payload.Qualifications {
		body.Qualifications = append(body.Qualifications,
			requireField(problems, fmt.Sprintf("%s.qualifications[%d]", field, index), domain.NewRuleReference, raw))
	}
	return body
}

func (payload FinalRulePayload) declaration(problems *PublicationPayloadProblems, field string) domain.FinalizationDeclaration {
	outcome, known := domain.DeclaredResponsibilityOutcomeNamed(payload.Outcome)
	if !known {
		problems.add(field+".outcome", fmt.Errorf("集合外的责任结果 %q", payload.Outcome))
	}
	return domain.FinalizationDeclaration{
		Outcome:   outcome,
		FinalKind: requireField(problems, field+".finalKind", domain.NewRuleReference, payload.FinalKind),
	}
}

// declaration 把有效期一格折成领域声明。种类按原词反查、时长按子集语法解析各记各格；两格都立得住而声明立不住（零时长）
// 记在时长那一格——种类已经反查过，剩下能错的只有它。第二个返回值为假即这一格没折成，调用方不把零值挂上去。
func (payload FinalRuleValidityPayload) declaration(problems *PublicationPayloadProblems, field string) (domain.LabelValidityDeclaration, bool) {
	before := len(problems.Problems)
	anchor, known := domain.ValidityAnchorKindNamed(payload.Anchor)
	if !known {
		problems.add(field+".anchor", fmt.Errorf("集合外的起算时刻种类 %q", payload.Anchor))
	}
	duration, err := domain.ParseLabelValidityDuration(payload.Duration)
	if err != nil {
		problems.add(field+".duration", err)
	}
	if len(problems.Problems) != before {
		return domain.LabelValidityDeclaration{}, false
	}
	validity, err := domain.NewLabelValidityDeclaration(anchor, duration)
	if err != nil {
		problems.add(field+".duration", err)
		return domain.LabelValidityDeclaration{}, false
	}
	return validity, true
}

// body 把资料修订允许一节折成领域输入面。closed 缺席是这一格的问题：缺格读「未声明」还是「不允许」要登记方自己说，
// 不给默认（ADR-0120 Decision 六）。格逐行点名（rules[i].dataGroup / stage / intent / allowance）。
func (payload SourceDataAmendmentPayload) body(problems *PublicationPayloadProblems, field string) domain.SourceDataAmendmentBody {
	body := domain.SourceDataAmendmentBody{}
	if payload.Closed == nil {
		problems.add(field+".closed", fmt.Errorf("closed 须在场：缺格读「未声明」（false）还是「不允许」（true）要登记方自己说，不给默认"))
	} else {
		body.Closed = *payload.Closed
	}
	for index, row := range payload.Rules {
		body.Rules = append(body.Rules, row.rule(problems, fmt.Sprintf("%s.rules[%d]", field, index)))
	}
	return body
}

func (payload SourceDataAmendmentRulePayload) rule(problems *PublicationPayloadProblems, field string) domain.SourceDataAmendmentRule {
	rule := domain.SourceDataAmendmentRule{
		DataGroup: requireField(problems, field+".dataGroup", domain.NewSourceDataGroupReference, payload.DataGroup),
	}
	var known bool
	if rule.Stage, known = domain.DeclaredAmendmentStageNamed(payload.Stage); !known {
		problems.add(field+".stage", fmt.Errorf("集合外的资料修订阶段 %q", payload.Stage))
	}
	if rule.Intent, known = domain.DeclaredAmendmentIntentNamed(payload.Intent); !known {
		problems.add(field+".intent", fmt.Errorf("集合外的修订意图 %q", payload.Intent))
	}
	if rule.Allowance, known = domain.AmendmentAllowanceNamed(payload.Allowance); !known {
		problems.add(field+".allowance", fmt.Errorf("集合外的允许性 %q（只收 ALLOWED / DISALLOWED；缺格的读法由 closed 决定，不写进 rules）", payload.Allowance))
	}
	return rule
}
