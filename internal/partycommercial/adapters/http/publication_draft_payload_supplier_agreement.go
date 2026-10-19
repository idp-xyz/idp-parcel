package commercialhttp

import (
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件是供应商协议册在运营操作者面载荷上的那一节（票 admin-write-faces/11）：线格式与逐格翻译。它挂在
// publication_draft_payload.go 的 CommercialPublicationPayload 上一格，解码、身份与逐格问题的规矩都在那一份。

// SupplierAgreementBodyPayload 镜像受控批文 supplierAgreementBodyDocument 与规范化文档的键名：供应商与责任法人是
// 主数据读面上的引用、采购方案是 parcel-pricing 方案版本的引用串（表单从价卡目录选，不读方案内容），区间上界可缺。
//
// 没有方向键：领域把供应商协议的方向钉死为 BUY，载荷里出现 direction 由严格解码按未知键拒——与批文口同一条规矩。
type SupplierAgreementBodyPayload struct {
	Supplier          string `json:"supplier"`
	LegalEntity       string `json:"legalEntity"`
	Scope             string `json:"scope"`
	PurchasePlan      string `json:"purchasePlan"`
	EffectiveStartsAt string `json:"effectiveStartsAt"`
	EffectiveEndsAt   string `json:"effectiveEndsAt,omitempty"`
	// ChargeConditions 镜像批文 chargeConditions（ADR-0209）：缺键即这一版没声明；空数组由领域拒。
	ChargeConditions []ChargeConditionPayload `json:"chargeConditions,omitempty"`
}

// ChargeConditionPayload 是协议对一个发生原因的声明，键名与批文同。charged 是指针，理由同批文：「不计费」要登记方
// 自己说，缺键不代说。
type ChargeConditionPayload struct {
	Reason       string `json:"reason"`
	Charged      *bool  `json:"charged"`
	QuantityForm string `json:"quantityForm,omitempty"`
}

// body 把供应商协议正文逐格过领域构造门；每一格的问题落在 supplierAgreement.<键> 上，收齐后由调用方一次交回。
func (payload SupplierAgreementBodyPayload) body(problems *PublicationPayloadProblems) domain.SupplierAgreementBody {
	body := domain.SupplierAgreementBody{
		Supplier:     requireField(problems, "supplierAgreement.supplier", domain.NewPartyID, payload.Supplier),
		LegalEntity:  requireField(problems, "supplierAgreement.legalEntity", domain.NewLegalEntityReference, payload.LegalEntity),
		Scope:        requireField(problems, "supplierAgreement.scope", domain.NewCommercialScopeReference, payload.Scope),
		PurchasePlan: requireField(problems, "supplierAgreement.purchasePlan", domain.NewPricingPlanReference, payload.PurchasePlan),
		Effective: intervalField(problems, "supplierAgreement.effectiveStartsAt", "supplierAgreement.effectiveEndsAt",
			payload.EffectiveStartsAt, payload.EffectiveEndsAt),
	}
	if payload.ChargeConditions != nil {
		body.ChargeConditions = chargeConditionsField(problems, payload.ChargeConditions)
	}
	return body
}

// chargeConditionsField 逐条过领域构造门；某一条的问题落在 supplierAgreement.chargeConditions[<序>] 上，整节立不住
// （空、同一原因两条）落在 supplierAgreement.chargeConditions 上。有问题时交回 nil，由调用方按问题清单整体作答。
func chargeConditionsField(problems *PublicationPayloadProblems, payloads []ChargeConditionPayload) *domain.ChargeOccurrenceConditions {
	conditions := make([]domain.ChargeOccurrenceCondition, 0, len(payloads))
	clean := true
	for index, payload := range payloads {
		field := fmt.Sprintf("supplierAgreement.chargeConditions[%d]", index)
		if payload.Charged == nil {
			problems.add(field+".charged", errors.New("计不计费要协议自己说，缺键不代说"))
			clean = false
			continue
		}
		condition, err := domain.ChargeOccurrenceConditionOf(payload.Reason, *payload.Charged, payload.QuantityForm)
		if err != nil {
			problems.add(field, err)
			clean = false
			continue
		}
		conditions = append(conditions, condition)
	}
	if !clean {
		return nil
	}
	conditionSet, err := domain.NewChargeOccurrenceConditions(conditions...)
	if err != nil {
		problems.add("supplierAgreement.chargeConditions", fmt.Errorf("空清单或同一原因两条：%w", err))
		return nil
	}
	return &conditionSet
}
