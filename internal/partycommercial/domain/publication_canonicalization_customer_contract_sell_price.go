package domain

import "fmt"

// 本文件是客户合同正文里销售价格约定那一层的规范化（ADR-0380 决定一、七）。它是在已接规范化的客户合同一册里加键，
// 照 ADR-0267 决定一换号：带这一层的合同文档写成 PCC-3，不带的仍写 PCC-1、逐字节不变。受控批文把它写成
// declarations 下的 sellPriceArrangement，与 contractContent / preAcceptanceControl / deliveryConditions 并列；
// 文档里的键名镜像批文。

// contractSellPriceCanonicalizationVersion 是 PCC-1 加客户合同一节里的销售价格约定。只有带这一层的合同文档写成
// 这一版；本构建认 PCC-1、PCC-2 与它，都能折回。结算与计价按「合同版本带不带销售价格约定」分新旧两种读法
// （ADR-0380 决定七），分的就是这一版与 PCC-1。
const contractSellPriceCanonicalizationVersion = "PCC-3"

// SellPriceArrangementBody 是销售价格约定的输入面：来源列表与不计销售价的依据两格，恰一在场由 validate 判。
// Sources 的顺序是正文的一部分，折成文档时原样写出、不排序——换顺序就是换了取价的先后。
type SellPriceArrangementBody struct {
	Sources          []PricingScopeReference
	NoSellPriceBasis NoSellPriceBasis
}

func (body SellPriceArrangementBody) validate() error {
	return sellPriceArrangementDeclared(body.Sources, body.NoSellPriceBasis)
}

// canonicalSellPriceArrangement 只带在场的那一格：有来源的约定只有 sources，不计销售价的只有 noSellPriceBasis。
// 每个来源写成一个对象而不是裸串：来源是销售价格约定里的一项，今天只指名计价范围一格。
type canonicalSellPriceArrangement struct {
	Sources          []canonicalSellPriceSource `json:"sources,omitempty"`
	NoSellPriceBasis string                     `json:"noSellPriceBasis,omitempty"`
}

type canonicalSellPriceSource struct {
	PricingScope string `json:"pricingScope"`
}

func canonicalSellPriceArrangementOf(body SellPriceArrangementBody) *canonicalSellPriceArrangement {
	document := &canonicalSellPriceArrangement{}
	for _, source := range body.Sources {
		document.Sources = append(document.Sources, canonicalSellPriceSource{PricingScope: source.String()})
	}
	if body.NoSellPriceBasis.valid() {
		document.NoSellPriceBasis = body.NoSellPriceBasis.String()
	}
	return document
}

// body 把文档里的一节折回输入面。每一格过构造门；两格恰一与同一范围两次留给 validate。
func (document canonicalSellPriceArrangement) body() (SellPriceArrangementBody, error) {
	var body SellPriceArrangementBody
	for index, source := range document.Sources {
		scope, err := NewPricingScopeReference(source.PricingScope)
		if err != nil {
			return SellPriceArrangementBody{}, fmt.Errorf("sources[%d].pricingScope: %w", index, err)
		}
		body.Sources = append(body.Sources, scope)
	}
	if document.NoSellPriceBasis != "" {
		basis, err := NewNoSellPriceBasis(document.NoSellPriceBasis)
		if err != nil {
			return SellPriceArrangementBody{}, fmt.Errorf("noSellPriceBasis: %w", err)
		}
		body.NoSellPriceBasis = basis
	}
	return body, nil
}

// canonicalizationOfCustomerContract 答一份合同正文按哪一版形状写：带销售价格约定的写 PCC-3，其余仍是 PCC-1。
func canonicalizationOfCustomerContract(body CustomerContractBody) string {
	if body.SellPrice != nil {
		return contractSellPriceCanonicalizationVersion
	}
	return publicationCanonicalizationVersion
}
