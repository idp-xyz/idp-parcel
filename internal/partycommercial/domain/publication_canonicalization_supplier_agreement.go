package domain

import (
	"fmt"
	"time"
)

// 本文件是供应商协议册在服务端规范化里的那一节（票 admin-write-faces/11）：正文输入面、PCC-1 文档节与
// 折回。接进的是 publication_canonicalization.go 里同一个版本号——加一册不改任何已算出的字节（ADR-0126
// Decision 一），所以不换号。

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
}

func (body SupplierAgreementBody) valid() bool {
	return body.Supplier.valid() && body.LegalEntity.valid() && body.Scope.valid() &&
		body.PurchasePlan.valid() && body.Effective.valid()
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
		Canonicalization:  publicationCanonicalizationVersion,
		Kind:              content.Kind.String(),
		SupplierAgreement: canonicalSupplierAgreementBodyOf(*content.SupplierAgreement),
	})
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
	return SupplierAgreementBody{
		Supplier:     supplier,
		LegalEntity:  legalEntity,
		Scope:        scope,
		PurchasePlan: purchasePlan,
		Effective:    effective,
	}, nil
}
