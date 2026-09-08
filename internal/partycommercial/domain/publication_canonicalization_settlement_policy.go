package domain

import (
	"fmt"
	"time"
)

// 本文件是结算政策册在服务端规范化里的那一节（票 admin-write-faces/15）：正文输入面、PCC-1 文档节与折回。
// 接进的是 publication_canonicalization.go 里同一个版本号——加一册不改任何已算出的字节（ADR-0126 Decision 一），
// 所以不换号。

// SettlementPolicyBody 是结算政策版本的正文，以领域值对象给出：就是 NewSettlementPolicy 收的那两项，只是不带
// 拥有它的（已生效）版本——预览与录入发生在发布之前，那时没有已生效版本可挂。
//
// 六维不摊平、整体收 SettlementApplicability：「六维齐不齐」那条判据只能在 NewSettlementApplicability 一处
// （SettlementPolicyBodyDeclaration 处的注释说的是同一件事）——摊平之后这里就得再判一遍，少一维的范围写得进
// 文档、读回来却命不中任何查询，看起来像「这个范围没有结算政策」。
type SettlementPolicyBody struct {
	Method        SettlementMethod
	Applicability SettlementApplicability
}

// valid 在折成文档前再核一遍：方式在封闭集内、六维齐全。六维那一判不在这里另写，把六维原样送回
// NewSettlementApplicability 让它答——同一条规则只有一处，绕过构造门用零值拼出来的正文在这里折不出摘要。
func (body SettlementPolicyBody) valid() bool {
	if !body.Method.valid() {
		return false
	}
	applicability := body.Applicability
	_, err := NewSettlementApplicability(
		applicability.legalEntity, applicability.counterparty, applicability.contract,
		applicability.chargeScope, applicability.currency, applicability.effective)
	return err == nil
}

// SettlementMethodNamed 按 String() 的原词反查结算方式：规范化文档里的 method、运营操作者面载荷里的 method 都是
// 那一个词，名单只在 String() 一处，这里只是反查（判据同 CommercialObjectKindNamed）。集合外答 false——第三个取值
// （客户级默认）是本上下文明禁的，反查不替它开口。
func SettlementMethodNamed(name string) (SettlementMethod, bool) {
	for method := PrepaidMethod; method.valid(); method++ {
		if method.String() == name {
			return method, true
		}
	}
	return SettlementMethodInvalid, false
}

// canonicalizeSettlementPolicy 是 CanonicalizePublicationContent 里结算政策那一支：正文缺席与立不住的正文各答
// 自己那一格（补正文 / 改正文），不折成一个「算不出」。
func canonicalizeSettlementPolicy(content PublicationContent) (CanonicalPublicationContent, error) {
	if content.SettlementPolicy == nil {
		return CanonicalPublicationContent{}, ErrPublicationContentAbsent
	}
	if !content.SettlementPolicy.valid() {
		return CanonicalPublicationContent{}, ErrInvalidSettlementPolicy
	}
	return canonicalDigestOf(canonicalPublicationDocument{
		Canonicalization: publicationCanonicalizationVersion,
		Kind:             content.Kind.String(),
		SettlementPolicy: canonicalSettlementPolicyBodyOf(*content.SettlementPolicy),
	})
}

// canonicalSettlementPolicyBody 镜像批文 settlementPolicyBodyDocument 的键名：方式加六维，区间上界可缺。
//
// contract 一格写的是领域的两段式指称串「对象/版本」——NewQualifiedVersionLabel 那一处拼出、0011 的 contract_label
// 存的、闭包解出合同后拿去命中的同一个串——而不是批文里 objectId / version 两格。理由是拆回两格要另立一处知道
// 分隔符的代码，而 QualifiedLabel 的注释正是为「只许一处拼」写的；载荷与批文收两格，进领域那一刻就合成这一个串，
// 文档写它就是写领域此刻持有的东西。时刻一律 UTC RFC 3339 纳秒，与信用政策那一节同一格式。
type canonicalSettlementPolicyBody struct {
	Method            string `json:"method"`
	LegalEntity       string `json:"legalEntity"`
	Counterparty      string `json:"counterparty"`
	Contract          string `json:"contract"`
	ChargeScope       string `json:"chargeScope"`
	Currency          string `json:"currency"`
	EffectiveStartsAt string `json:"effectiveStartsAt"`
	EffectiveEndsAt   string `json:"effectiveEndsAt,omitempty"`
}

func canonicalSettlementPolicyBodyOf(body SettlementPolicyBody) *canonicalSettlementPolicyBody {
	applicability := body.Applicability
	document := &canonicalSettlementPolicyBody{
		Method:            body.Method.String(),
		LegalEntity:       applicability.legalEntity.String(),
		Counterparty:      applicability.counterparty.String(),
		Contract:          applicability.contract.String(),
		ChargeScope:       applicability.chargeScope.String(),
		Currency:          applicability.currency.String(),
		EffectiveStartsAt: canonicalTime(applicability.effective.StartsAt()),
	}
	if endsAt, bounded := applicability.effective.EndsAt(); bounded {
		document.EffectiveEndsAt = canonicalTime(endsAt)
	}
	return document
}

// body 把文档里的一节折回领域正文。每一格都过领域构造门，六维再过 NewSettlementApplicability：快照是数据，正文
// 立不立得住仍由构造门说。
func (document canonicalSettlementPolicyBody) body() (SettlementPolicyBody, error) {
	method, known := SettlementMethodNamed(document.Method)
	if !known {
		return SettlementPolicyBody{}, fmt.Errorf("method: %w: %q", ErrInvalidSettlementPolicy, document.Method)
	}
	legalEntity, err := NewLegalEntityReference(document.LegalEntity)
	if err != nil {
		return SettlementPolicyBody{}, err
	}
	counterparty, err := NewCounterpartyReference(document.Counterparty)
	if err != nil {
		return SettlementPolicyBody{}, err
	}
	contract, err := NewCommercialVersionLabel(document.Contract)
	if err != nil {
		return SettlementPolicyBody{}, fmt.Errorf("contract: %w", err)
	}
	chargeScope, err := NewChargeScopeReference(document.ChargeScope)
	if err != nil {
		return SettlementPolicyBody{}, err
	}
	currency, err := NewCurrencyCode(document.Currency)
	if err != nil {
		return SettlementPolicyBody{}, err
	}
	startsAt, err := time.Parse(time.RFC3339Nano, document.EffectiveStartsAt)
	if err != nil {
		return SettlementPolicyBody{}, fmt.Errorf("effectiveStartsAt: %w", err)
	}
	var endsAt time.Time
	if document.EffectiveEndsAt != "" {
		if endsAt, err = time.Parse(time.RFC3339Nano, document.EffectiveEndsAt); err != nil {
			return SettlementPolicyBody{}, fmt.Errorf("effectiveEndsAt: %w", err)
		}
	}
	effective, err := NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		return SettlementPolicyBody{}, err
	}
	applicability, err := NewSettlementApplicability(legalEntity, counterparty, contract, chargeScope, currency, effective)
	if err != nil {
		return SettlementPolicyBody{}, err
	}
	return SettlementPolicyBody{Method: method, Applicability: applicability}, nil
}
