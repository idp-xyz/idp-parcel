package commercialhttp

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件是接受前财务控制策略册在运营操作者面载荷里的那一格（票 admin-write-faces/13）。一格两层：父行一格共同通过
// 条件加子表逐行的控制项（0024，ADR-0115 Decision 二），键名镜像受控批文 preAcceptanceFinancialControlPolicyBody 与
// 规范化文档——同一册在三处（批文、载荷、文档）说同一套词。

// PreAcceptanceFinancialControlPolicyBodyPayload 是接受前财务控制策略册的正文载荷。三个封闭集（共同通过条件、控制种类、
// 失败处置）只按 String() 原词反查，集合外含空串都是那一格的问题——表单不内置枚举，下拉的码来自词表读口（票 20），
// 这里认的就是规范化器接受的那些词。至少一项、顺序唯一、（种类 × 范围）唯一是跨行的门，由领域规范化在预览上答成`未受理`
// 带成因，不在这里判。
type PreAcceptanceFinancialControlPolicyBodyPayload struct {
	JointPassCondition string                            `json:"jointPassCondition"`
	Controls           []PreAcceptanceControlItemPayload `json:"controls"`
}

// PreAcceptanceControlItemPayload 是一行控制项：种类 × 费用范围引用 × 判断顺序 × 失败处置 × 责任引用。范围与责任是开放
// 引用（0024 头注），只查非空不校验存在性。order 用普通整数而不是指针：零与缺席在这里同义——都不是「排第几」的答案
// （判据同批文 preAcceptanceControlItemDocument）。控制种类里没有「无控制」：那一格属合同声明（ADR-0115 Decision 一）。
type PreAcceptanceControlItemPayload struct {
	Control        string `json:"control"`
	ChargeScope    string `json:"chargeScope"`
	Order          int    `json:"order"`
	OnFailure      string `json:"onFailure"`
	Responsibility string `json:"responsibility"`
}

// body 把策略载荷逐格过领域构造门。控制项逐行点名（controls[i]）；共同通过条件缺席不折成「全部通过」——CONTEXT 要求策略
// 明确它，没写就是没写（ADR-0115 Decision 三）。
func (payload PreAcceptanceFinancialControlPolicyBodyPayload) body(problems *PublicationPayloadProblems) domain.PreAcceptanceFinancialControlPolicyBody {
	const field = "preAcceptanceFinancialControlPolicy"
	jointPass, known := domain.JointPassConditionNamed(payload.JointPassCondition)
	if !known {
		problems.add(field+".jointPassCondition", fmt.Errorf("集合外的共同通过条件 %q", payload.JointPassCondition))
	}
	body := domain.PreAcceptanceFinancialControlPolicyBody{
		JointPass: jointPass,
		Items:     make([]domain.PreAcceptanceControlItem, 0, len(payload.Controls)),
	}
	for index, row := range payload.Controls {
		body.Items = append(body.Items, row.item(problems, fmt.Sprintf("%s.controls[%d]", field, index)))
	}
	return body
}

// item 把一行控制项折成领域值对象。五格各自过门后再拼——上面某格已记过问题时不再拼，免得把同一格的零值再记一遍
// （判据同 ControlBindingPayload.binding）。
func (payload PreAcceptanceControlItemPayload) item(problems *PublicationPayloadProblems, field string) domain.PreAcceptanceControlItem {
	before := len(problems.Problems)
	kind, known := domain.PreAcceptanceControlKindNamed(payload.Control)
	if !known {
		problems.add(field+".control", fmt.Errorf("集合外的控制种类 %q", payload.Control))
	}
	scope := requireField(problems, field+".chargeScope", domain.NewChargeScopeReference, payload.ChargeScope)
	if payload.Order <= 0 {
		problems.add(field+".order", fmt.Errorf("判断顺序须为从 1 起的正整数，收到 %d", payload.Order))
	}
	disposition, known := domain.ControlFailureDispositionNamed(payload.OnFailure)
	if !known {
		problems.add(field+".onFailure", fmt.Errorf("集合外的失败处置 %q", payload.OnFailure))
	}
	responsibility := requireField(problems, field+".responsibility", domain.NewControlResponsibilityReference, payload.Responsibility)
	if len(problems.Problems) != before {
		return domain.PreAcceptanceControlItem{}
	}
	item, err := domain.NewPreAcceptanceControlItem(kind, scope, payload.Order, disposition, responsibility)
	if err != nil {
		problems.add(field, err)
	}
	return item
}
