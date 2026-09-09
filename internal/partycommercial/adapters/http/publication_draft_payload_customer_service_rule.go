package commercialhttp

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件是客户服务规则册在运营操作者面载荷里的那一格（票 admin-write-faces/18）。一格三层：父行的适用对象（服务产品 / 客户
// 合同恰一）、责任方、范围，加两张子表——索赔期限按种类成行、最低材料按索赔类型成行（0023，ADR-0104 Decision 二）；键名镜像
// 受控批文 customerServiceRuleBody 与规范化文档——同一册在三处（批文、载荷、文档）说同一套词。

// CustomerServiceRuleBodyPayload 是客户服务规则册的正文载荷。适用对象两键恰一在场由这里判成一格问题（落在 applicability 上，
// 判据同 CreditPolicyBodyPayload 的额度两键）——两格都填不是「更明确」，是两条适用声明挤在一条记录上。期限种类是本正文唯一的
// 封闭集，只按 String() 原词反查，集合外含空串都是那一格的问题——表单不内置枚举，下拉的码来自词表读口（票 20）。起算事件、
// 日历、索赔类型与材料都是开放引用（解释权在 visibility-exception），只查非空不校验存在性。两项合起来至少一行、每一种期限与
// 每一种索赔类型至多一行是跨行的门，由领域规范化在预览上答成`未受理`带成因，不在这里判。
type CustomerServiceRuleBodyPayload struct {
	ServiceProduct   string                        `json:"serviceProduct,omitempty"`
	CustomerContract string                        `json:"customerContract,omitempty"`
	Responsible      string                        `json:"responsible"`
	Scope            string                        `json:"scope"`
	ClaimDeadlines   []ClaimDeadlineRulePayload    `json:"claimDeadlines"`
	MinimumMaterials []MinimumMaterialsRulePayload `json:"minimumMaterials"`
}

// ClaimDeadlineRulePayload 是一行索赔期限：种类 × 起算事件引用 × 整数天 × 日历引用。days 用普通整数而不是指针：零与缺席在这里
// 同义——都不是一条算得出东西的期限（判据同批文 claimDeadlineDocument）。
type ClaimDeadlineRulePayload struct {
	Kind       string `json:"kind"`
	StartEvent string `json:"startEvent"`
	Days       int    `json:"days"`
	Calendar   string `json:"calendar"`
}

// MinimumMaterialsRulePayload 是一行最低材料：一种索赔类型与它必须齐备的材料清单。空清单由 NewMinimumMaterialsRule 拒——
// 「这一类不要材料」没有任何消费方读得出来，要说也该是不写这一行。
type MinimumMaterialsRulePayload struct {
	ClaimKind string   `json:"claimKind"`
	Materials []string `json:"materials"`
}

// body 把规则载荷逐格过领域构造门。两张子表逐行点名（claimDeadlines[i] / minimumMaterials[i]）；适用对象两键恰一在场在这里判成
// 一格问题，不由这里挑一格读。
func (payload CustomerServiceRuleBodyPayload) body(problems *PublicationPayloadProblems) domain.CustomerServiceRuleBody {
	const field = "customerServiceRule"
	body := domain.CustomerServiceRuleBody{
		Applicability: payload.applicability(problems, field),
		Responsible:   requireField(problems, field+".responsible", domain.NewPartyID, payload.Responsible),
		Scope:         requireField(problems, field+".scope", domain.NewCommercialScopeReference, payload.Scope),
		Deadlines:     make([]domain.ClaimDeadlineRule, 0, len(payload.ClaimDeadlines)),
		Materials:     make([]domain.MinimumMaterialsRule, 0, len(payload.MinimumMaterials)),
	}
	for index, row := range payload.ClaimDeadlines {
		body.Deadlines = append(body.Deadlines, row.rule(problems, fmt.Sprintf("%s.claimDeadlines[%d]", field, index)))
	}
	for index, row := range payload.MinimumMaterials {
		body.Materials = append(body.Materials, row.rule(problems, fmt.Sprintf("%s.minimumMaterials[%d]", field, index)))
	}
	return body
}

// applicability 把两键折成两格封闭的适用对象：恰一在场才立得住；两空与两满都是一格问题（落在 applicability 上）——同一个
// 标识串作产品与作合同是两件事，这里不替操作者挑一个。
func (payload CustomerServiceRuleBodyPayload) applicability(problems *PublicationPayloadProblems, field string) domain.CustomerServiceRuleApplicability {
	switch {
	case payload.ServiceProduct != "" && payload.CustomerContract == "":
		return domain.CustomerServiceRuleAppliesToServiceProduct(
			requireField(problems, field+".serviceProduct", domain.NewCommercialObjectID, payload.ServiceProduct))
	case payload.ServiceProduct == "" && payload.CustomerContract != "":
		return domain.CustomerServiceRuleAppliesToCustomerContract(
			requireField(problems, field+".customerContract", domain.NewCommercialObjectID, payload.CustomerContract))
	default:
		problems.add(field+".applicability", fmt.Errorf("适用对象须恰一格在场：serviceProduct 或 customerContract"))
		return domain.CustomerServiceRuleApplicability{}
	}
}

// rule 把一行索赔期限折成领域值对象。四格各自过门后再拼——上面某格已记过问题时不再拼，免得把同一格的零值再记一遍
// （判据同 PreAcceptanceControlItemPayload.item）。
func (payload ClaimDeadlineRulePayload) rule(problems *PublicationPayloadProblems, field string) domain.ClaimDeadlineRule {
	before := len(problems.Problems)
	kind, known := domain.ClaimDeadlineKindNamed(payload.Kind)
	if !known {
		problems.add(field+".kind", fmt.Errorf("集合外的期限种类 %q", payload.Kind))
	}
	startEvent := requireField(problems, field+".startEvent", domain.NewDeadlineStartEventReference, payload.StartEvent)
	if payload.Days <= 0 {
		problems.add(field+".days", fmt.Errorf("时长须为正整数天，收到 %d", payload.Days))
	}
	calendar := requireField(problems, field+".calendar", domain.NewBusinessCalendarReference, payload.Calendar)
	if len(problems.Problems) != before {
		return domain.ClaimDeadlineRule{}
	}
	rule, err := domain.NewClaimDeadlineRule(kind, startEvent, payload.Days, calendar)
	if err != nil {
		problems.add(field, err)
	}
	return rule
}

// rule 把一行最低材料折成领域值对象。清单逐项点名（materials[j]）；索赔类型与清单各自过门后再拼，判据同上。空清单与清单内
// 重复由 NewMinimumMaterialsRule 答、记在行上。
func (payload MinimumMaterialsRulePayload) rule(problems *PublicationPayloadProblems, field string) domain.MinimumMaterialsRule {
	before := len(problems.Problems)
	claimKind := requireField(problems, field+".claimKind", domain.NewClaimKindReference, payload.ClaimKind)
	materials := make([]domain.MaterialRequirementReference, 0, len(payload.Materials))
	for index, raw := range payload.Materials {
		materials = append(materials, requireField(problems, fmt.Sprintf("%s.materials[%d]", field, index), domain.NewMaterialRequirementReference, raw))
	}
	if len(problems.Problems) != before {
		return domain.MinimumMaterialsRule{}
	}
	rule, err := domain.NewMinimumMaterialsRule(claimKind, materials)
	if err != nil {
		problems.add(field, err)
	}
	return rule
}
