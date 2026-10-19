package domain

import (
	"fmt"
	"time"
)

// 本文件是供应商协议册在服务端规范化里的那一节（票 admin-write-faces/11）：正文输入面、PCC-1 文档节与
// 折回。接进的是 publication_canonicalization.go 里同一个版本号——加一册不改任何已算出的字节（ADR-0126
// Decision 一），所以不换号。
//
// 收费发生条件一节（ADR-0209）改的是本册既有的文档形状，照 ADR-0014 换号：带这一节的文档写成 PCC-2，不带的仍是
// PCC-1、逐字节不变。不换号会让只认 PCC-1 的构建把带条件的文档静默读成不带条件的正文——json 读不认识的键是
// 丢掉，不是报错——再算一遍摘要对不上，却报不出是形状变了。

// SupplierAgreementBody 是供应商商业协议版本的正文，以领域值对象给出：就是 NewSupplierAgreement 收的那几项，
// 只是不带拥有它的（已生效）版本——预览与录入发生在发布之前，那时没有已生效版本可挂。
//
// 方向不在正文上：领域把它钉死为 BUY（SupplierAgreement.Direction），正文里出现方向就是把一笔成本装扮成
// 可售价格。采购方案只是一条引用串——方案的方向与绑定换算是 PRICE_RULE 那册的事，这里不读方案内容。
type SupplierAgreementBody struct {
	Supplier     PartyID
	LegalEntity  LegalEntityReference
	Scope        CommercialScopeReference
	PurchasePlan PricingPlanReference
	Effective    EffectiveInterval
	// ChargeConditions 是可缺的收费发生条件一节（ADR-0209）；nil 即这一版没声明。
	ChargeConditions *ChargeOccurrenceConditions
}

func (body SupplierAgreementBody) valid() bool {
	return body.Supplier.valid() && body.LegalEntity.valid() && body.Scope.valid() &&
		body.PurchasePlan.valid() && body.Effective.valid() &&
		(body.ChargeConditions == nil || body.ChargeConditions.valid())
}

// canonicalizeSupplierAgreement 是 CanonicalizePublicationContent 里供应商协议那一支：正文缺席与零值正文各答
// 自己那一格（补正文 / 改正文），不折成一个「算不出」。
func canonicalizeSupplierAgreement(content PublicationContent) (CanonicalPublicationContent, error) {
	if content.SupplierAgreement == nil {
		return CanonicalPublicationContent{}, ErrPublicationContentAbsent
	}
	if !content.SupplierAgreement.valid() {
		return CanonicalPublicationContent{}, ErrInvalidSupplierAgreement
	}
	return canonicalDigestOf(canonicalPublicationDocument{
		Canonicalization:  canonicalizationOfSupplierAgreement(*content.SupplierAgreement),
		Kind:              content.Kind.String(),
		SupplierAgreement: canonicalSupplierAgreementBodyOf(*content.SupplierAgreement),
	})
}

// canonicalizationOfSupplierAgreement 答这份正文按哪一版形状写：带收费发生条件写 PCC-2，否则 PCC-1。按内容取最低
// 能表达它的那一版，不按构建取最新的一版——后者会让 v1 那类不带条件的正文换一个摘要，册上已存的串就再也对不上。
func canonicalizationOfSupplierAgreement(body SupplierAgreementBody) string {
	if body.ChargeConditions != nil {
		return supplierChargeConditionCanonicalizationVersion
	}
	return publicationCanonicalizationVersion
}

// canonicalSupplierAgreementBody 镜像批文 supplierAgreementBodyDocument 的键名：区间上界可缺，没有方向键。
// 时刻一律 UTC RFC 3339 纳秒，与信用政策那一节同一格式。
type canonicalSupplierAgreementBody struct {
	Supplier          string `json:"supplier"`
	LegalEntity       string `json:"legalEntity"`
	Scope             string `json:"scope"`
	PurchasePlan      string `json:"purchasePlan"`
	EffectiveStartsAt string `json:"effectiveStartsAt"`
	EffectiveEndsAt   string `json:"effectiveEndsAt,omitempty"`
	// ChargeConditions 是 PCC-2 多出的一节，按原因的声明顺序排；PCC-1 文档里没有这一键。
	ChargeConditions []canonicalChargeCondition `json:"chargeConditions,omitempty"`
}

// canonicalChargeCondition 镜像批文 chargeConditions 下的一条：charged 两值都写出——false 是一句声明（不计费），
// 不是缺席；quantityForm 只随计费在场。
type canonicalChargeCondition struct {
	Reason       string `json:"reason"`
	Charged      bool   `json:"charged"`
	QuantityForm string `json:"quantityForm,omitempty"`
}

func canonicalSupplierAgreementBodyOf(body SupplierAgreementBody) *canonicalSupplierAgreementBody {
	document := &canonicalSupplierAgreementBody{
		Supplier:          body.Supplier.String(),
		LegalEntity:       body.LegalEntity.String(),
		Scope:             body.Scope.String(),
		PurchasePlan:      body.PurchasePlan.String(),
		EffectiveStartsAt: canonicalTime(body.Effective.StartsAt()),
	}
	if endsAt, bounded := body.Effective.EndsAt(); bounded {
		document.EffectiveEndsAt = canonicalTime(endsAt)
	}
	if body.ChargeConditions != nil {
		for _, condition := range body.ChargeConditions.All() {
			entry := canonicalChargeCondition{Reason: condition.Reason().String(), Charged: condition.Charged()}
			if form, charged := condition.QuantityForm(); charged {
				entry.QuantityForm = form.String()
			}
			document.ChargeConditions = append(document.ChargeConditions, entry)
		}
	}
	return document
}

// body 把文档里的一节折回领域正文。每一格都过领域构造门：快照是数据，正文立不立得住仍由构造门说。
func (document canonicalSupplierAgreementBody) body() (SupplierAgreementBody, error) {
	supplier, err := NewPartyID(document.Supplier)
	if err != nil {
		return SupplierAgreementBody{}, err
	}
	legalEntity, err := NewLegalEntityReference(document.LegalEntity)
	if err != nil {
		return SupplierAgreementBody{}, err
	}
	scope, err := NewCommercialScopeReference(document.Scope)
	if err != nil {
		return SupplierAgreementBody{}, err
	}
	purchasePlan, err := NewPricingPlanReference(document.PurchasePlan)
	if err != nil {
		return SupplierAgreementBody{}, err
	}
	startsAt, err := time.Parse(time.RFC3339Nano, document.EffectiveStartsAt)
	if err != nil {
		return SupplierAgreementBody{}, fmt.Errorf("effectiveStartsAt: %w", err)
	}
	var endsAt time.Time
	if document.EffectiveEndsAt != "" {
		if endsAt, err = time.Parse(time.RFC3339Nano, document.EffectiveEndsAt); err != nil {
			return SupplierAgreementBody{}, fmt.Errorf("effectiveEndsAt: %w", err)
		}
	}
	effective, err := NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		return SupplierAgreementBody{}, err
	}
	body := SupplierAgreementBody{
		Supplier:     supplier,
		LegalEntity:  legalEntity,
		Scope:        scope,
		PurchasePlan: purchasePlan,
		Effective:    effective,
	}
	if document.ChargeConditions != nil {
		conditions, err := chargeConditionsOf(document.ChargeConditions)
		if err != nil {
			return SupplierAgreementBody{}, err
		}
		body.ChargeConditions = &conditions
	}
	return body, nil
}

// chargeConditionsOf 把文档里的条件一节逐条过构造门折回。条件在场却是空数组折不回：omitempty 从不写出空数组，
// 读到它就是这份文档不是本构建写的。
func chargeConditionsOf(entries []canonicalChargeCondition) (ChargeOccurrenceConditions, error) {
	conditions := make([]ChargeOccurrenceCondition, 0, len(entries))
	for _, entry := range entries {
		condition, err := ChargeOccurrenceConditionOf(entry.Reason, entry.Charged, entry.QuantityForm)
		if err != nil {
			return ChargeOccurrenceConditions{}, err
		}
		conditions = append(conditions, condition)
	}
	return NewChargeOccurrenceConditions(conditions...)
}

// ChargeOccurrenceConditionOf 按原词立一条声明：计费必带数量形，不计费必不带。凡把原词折成声明的入口都走这一处，
// 对「不计费却带了数量形」因此只有一种读法——拒，而不是丢掉那一格。
func ChargeOccurrenceConditionOf(reasonWord string, charged bool, formWord string) (ChargeOccurrenceCondition, error) {
	reason, known := TransportChargeReasonNamed(reasonWord)
	if !known {
		return ChargeOccurrenceCondition{}, fmt.Errorf("%w: reason %q", ErrInvalidChargeOccurrenceCondition, reasonWord)
	}
	if !charged {
		if formWord != "" {
			return ChargeOccurrenceCondition{}, fmt.Errorf("%w: %s is declared uncharged but carries quantity form %q",
				ErrInvalidChargeOccurrenceCondition, reasonWord, formWord)
		}
		return NewUnchargedOccurrenceCondition(reason)
	}
	form, known := ChargeQuantityFormNamed(formWord)
	if !known {
		return ChargeOccurrenceCondition{}, fmt.Errorf("%w: quantity form %q", ErrInvalidChargeOccurrenceCondition, formWord)
	}
	condition, err := NewChargedOccurrenceCondition(reason, form)
	if err != nil {
		return ChargeOccurrenceCondition{}, fmt.Errorf("%w: %s does not take %s", err, reasonWord, formWord)
	}
	return condition, nil
}
