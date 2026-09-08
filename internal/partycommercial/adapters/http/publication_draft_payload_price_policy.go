package commercialhttp

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件是价格规则册在运营操作者面载荷上的那一节（票 admin-write-faces/14）：线格式与逐格翻译。它挂在
// publication_draft_payload.go 的 CommercialPublicationPayload 上一格，解码、身份与逐格问题的规矩都在那一份。

// PricePolicyBodyPayload 镜像受控批文 pricePolicyBodyDocument 与规范化文档的键名：0010 正文七格加可缺的口径节。
//
// planDirection 与 conversion 是发布当时 parcel-pricing 对方案方向的答复与当时声明的转换（ADR-0057）：表单如实收，
// 不从方案反推，这里也不代填——conversion 缺席或空串不折成 NONE，SELL 政策绑 BUY 方案而没写转换该由领域报出
// `适用冲突`（AT-PC-033），代填会把「没写」与「明说不转换」混在一起（判据同受控批文的 planBindingConversionFrom）。
type PricePolicyBodyPayload struct {
	Direction         string                     `json:"direction"`
	PricingPlan       string                     `json:"pricingPlan"`
	PlanDirection     string                     `json:"planDirection"`
	Conversion        string                     `json:"conversion"`
	Scope             string                     `json:"scope"`
	EffectiveStartsAt string                     `json:"effectiveStartsAt"`
	EffectiveEndsAt   string                     `json:"effectiveEndsAt,omitempty"`
	Caliber           *PricePolicyCaliberPayload `json:"caliber,omitempty"`
}

// PricePolicyCaliberPayload 镜像受控批文 pricePolicyCaliberDocument：没有方向键——体积口径的方向就是政策的方向，载荷里
// 出现 direction 由严格解码按未知键拒。taxClassification 只在含税 / 未税时在场、volumetricFactor 只在销售方向在场，
// 反过来给了或漏了都由领域构造门拒并点名那一格（表单按 taxDisposition / direction 显隐的就是这两格；显隐是呈现，
// 不是裁门）。fx 可缺：不涉及外币的政策没有汇率口径，缺席是「没声明」不是零口径；在场则三格缺一不可。
type PricePolicyCaliberPayload struct {
	TaxDisposition    string            `json:"taxDisposition"`
	TaxClassification string            `json:"taxClassification,omitempty"`
	VolumetricFactor  string            `json:"volumetricFactor,omitempty"`
	Fx                *FxCaliberPayload `json:"fx,omitempty"`
}

// FxCaliberPayload 镜像受控批文 fxCaliberDocument：牌价类型、取值时点语义与时点政策版本，三格缺一不可。
type FxCaliberPayload struct {
	QuoteType         string `json:"quoteType"`
	AsOfSemantics     string `json:"asOfSemantics"`
	AsOfPolicyVersion string `json:"asOfPolicyVersion"`
}

// body 把价格政策正文逐格过领域构造门；每一格的问题落在 pricePolicy.<键> 上，收齐后由调用方一次交回。封闭集三格按
// String() 原词反查，集合外（含空串）是那一格自己的问题。跨格的判（方向 × 方案方向 × 转换的绑定矩阵）不在这里——
// 那是领域规范化里 checkPlanBinding 的一句，预览答成`未受理`带成因。
func (payload PricePolicyBodyPayload) body(problems *PublicationPayloadProblems) domain.PricePolicyBody {
	body := domain.PricePolicyBody{
		Direction:     namedField(problems, "pricePolicy.direction", domain.PriceDirectionNamed, payload.Direction, "集合外的价格方向"),
		PricingPlan:   requireField(problems, "pricePolicy.pricingPlan", domain.NewPricingPlanReference, payload.PricingPlan),
		PlanDirection: namedField(problems, "pricePolicy.planDirection", domain.PriceDirectionNamed, payload.PlanDirection, "集合外的方案方向"),
		Conversion:    namedField(problems, "pricePolicy.conversion", domain.PlanBindingConversionNamed, payload.Conversion, "集合外的方案绑定转换"),
		Scope:         requireField(problems, "pricePolicy.scope", domain.NewCommercialScopeReference, payload.Scope),
		Effective: intervalField(problems, "pricePolicy.effectiveStartsAt", "pricePolicy.effectiveEndsAt",
			payload.EffectiveStartsAt, payload.EffectiveEndsAt),
	}
	if payload.Caliber != nil {
		caliber := payload.Caliber.body(problems, body.Direction)
		body.Caliber = &caliber
	}
	return body
}

// body 把口径节逐格过门。分类与系数两格的在场规则（含税 / 未税恰要求分类、不适用恰要求没有；销售方向恰要求系数、
// 采购与法人间恰要求没有）由 NewTaxCaliber / NewVolumetricCaliber 判，与库上 CHECK 同形；问题落在**条件格**上
// （taxClassification / volumetricFactor）——显了没填与隐了却传了都答在同一格。体积口径的方向取政策方向：载荷里没有
// 方向键，所以从这里造不出方向不一致的口径；方向自己立不住时这一格不再判，免得把 direction 的问题再记成系数的。
func (payload PricePolicyCaliberPayload) body(problems *PublicationPayloadProblems, direction domain.PriceDirection) domain.PricePolicyCaliberBody {
	var caliber domain.PricePolicyCaliberBody
	disposition := namedField(problems, "pricePolicy.caliber.taxDisposition", domain.TaxDispositionNamed, payload.TaxDisposition, "集合外的税务口径")
	var classification domain.TaxClassificationReference
	if payload.TaxClassification != "" {
		classification = requireField(problems, "pricePolicy.caliber.taxClassification", domain.NewTaxClassificationReference, payload.TaxClassification)
	}
	if disposition != domain.TaxDispositionInvalid {
		tax, err := domain.NewTaxCaliber(disposition, classification)
		if err != nil {
			problems.add("pricePolicy.caliber.taxClassification",
				fmt.Errorf("含税 / 未税恰要求一份税务分类，不适用恰要求没有（税务口径 %s）：%w", disposition, err))
		}
		caliber.Tax = tax
	}
	var factor domain.VolumetricFactorReference
	if payload.VolumetricFactor != "" {
		factor = requireField(problems, "pricePolicy.caliber.volumetricFactor", domain.NewVolumetricFactorReference, payload.VolumetricFactor)
	}
	if direction != domain.PriceDirectionInvalid {
		volumetric, err := domain.NewVolumetricCaliber(direction, factor)
		if err != nil {
			problems.add("pricePolicy.caliber.volumetricFactor",
				fmt.Errorf("销售方向恰要求一份体积系数，采购与法人间方向恰要求没有（政策方向 %s）：%w", direction, err))
		}
		caliber.Volumetric = volumetric
	}
	if payload.Fx != nil {
		before := len(problems.Problems)
		quoteType := requireField(problems, "pricePolicy.caliber.fx.quoteType", domain.NewFxQuoteTypeReference, payload.Fx.QuoteType)
		semantics := requireField(problems, "pricePolicy.caliber.fx.asOfSemantics", domain.NewAsOfSemanticsReference, payload.Fx.AsOfSemantics)
		policyVersion := requireField(problems, "pricePolicy.caliber.fx.asOfPolicyVersion", domain.NewAsOfPolicyVersion, payload.Fx.AsOfPolicyVersion)
		fx, err := domain.NewFxCaliber(quoteType, semantics, policyVersion)
		// 三格各自过门时已逐格点名；只在三格都立得住却仍构造不出时才记到节上，免得同一格记两遍。
		if err != nil && len(problems.Problems) == before {
			problems.add("pricePolicy.caliber.fx", err)
		}
		caliber.Fx = &fx
	}
	return caliber
}

// namedField 按 String() 原词反查一格封闭集；集合外含空串记进 problems 并交回零值——零值让后面的格照样能判，问题才收得齐
// （判据同 requireField）。
func namedField[T any](problems *PublicationPayloadProblems, field string, lookup func(string) (T, bool), raw string, what string) T {
	value, known := lookup(raw)
	if !known {
		problems.add(field, fmt.Errorf("%s %q", what, raw))
	}
	return value
}
