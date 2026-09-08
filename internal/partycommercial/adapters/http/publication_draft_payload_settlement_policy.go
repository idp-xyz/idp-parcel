package commercialhttp

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件是结算政策册在运营操作者面载荷上的那一节（票 admin-write-faces/15）：线格式与逐格翻译。它挂在
// publication_draft_payload.go 的 CommercialPublicationPayload 上一格，解码、身份与逐格问题的规矩都在那一份。

// SettlementPolicyBodyPayload 镜像受控批文 settlementPolicyBodyDocument 的键名：方式加六维适用范围，区间上界可缺。
// 六维一维不少（ADR-0044：本上下文禁止借宽泛的客户关系跨维归集，载荷里省掉哪一维都会变成「这一维随便什么值都算命中」）。
//
// 方式只收 String() 原词（PREPAID / TERMS）；第三个取值——客户级默认——是本上下文明禁的，载荷层不替它开口。
// 合同维收对象 + 版本两格（ContractVersionReferencePayload），与批文的 contractVersionDocument 同形，不收现成的
// 「对象/版本」串——两段式指称串只许 NewQualifiedVersionLabel 一处拼，操作者手拼的串在分隔符变化那天会静静失配。
type SettlementPolicyBodyPayload struct {
	Method            string                          `json:"method"`
	LegalEntity       string                          `json:"legalEntity"`
	Counterparty      string                          `json:"counterparty"`
	Contract          ContractVersionReferencePayload `json:"contract"`
	ChargeScope       string                          `json:"chargeScope"`
	Currency          string                          `json:"currency"`
	EffectiveStartsAt string                          `json:"effectiveStartsAt"`
	EffectiveEndsAt   string                          `json:"effectiveEndsAt,omitempty"`
}

// ContractVersionReferencePayload 是客户合同版本的二维引用：哪个合同对象、哪一版。表单从客户与合同目录选，两格一起
// 落，不让操作者拼版本号。
type ContractVersionReferencePayload struct {
	ObjectID string `json:"objectId"`
	Version  string `json:"version"`
}

// body 把结算政策正文逐格过领域构造门；每一格的问题落在 settlementPolicy.<键> 上（合同两格各自点名为
// settlementPolicy.contract.objectId / .version），收齐后由调用方一次交回。
//
// 六维合成 SettlementApplicability 只在逐格都立得住之后做：某格已点名时合成必然失败，而那一失败只会把已报的格
// 再说一次，不是新的一格问题。
func (payload SettlementPolicyBodyPayload) body(problems *PublicationPayloadProblems) domain.SettlementPolicyBody {
	before := len(problems.Problems)
	method, known := domain.SettlementMethodNamed(payload.Method)
	if !known {
		problems.add("settlementPolicy.method", fmt.Errorf("集合外的结算方式 %q", payload.Method))
	}
	legalEntity := requireField(problems, "settlementPolicy.legalEntity", domain.NewLegalEntityReference, payload.LegalEntity)
	counterparty := requireField(problems, "settlementPolicy.counterparty", domain.NewCounterpartyReference, payload.Counterparty)
	contractObject := requireField(problems, "settlementPolicy.contract.objectId", domain.NewCommercialObjectID, payload.Contract.ObjectID)
	contractVersion := requireField(problems, "settlementPolicy.contract.version", domain.NewCommercialVersionLabel, payload.Contract.Version)
	chargeScope := requireField(problems, "settlementPolicy.chargeScope", domain.NewChargeScopeReference, payload.ChargeScope)
	currency := requireField(problems, "settlementPolicy.currency", domain.NewCurrencyCode, payload.Currency)
	effective := intervalField(problems, "settlementPolicy.effectiveStartsAt", "settlementPolicy.effectiveEndsAt",
		payload.EffectiveStartsAt, payload.EffectiveEndsAt)
	if len(problems.Problems) > before {
		return domain.SettlementPolicyBody{Method: method}
	}
	contract, err := domain.NewQualifiedVersionLabel(contractObject, contractVersion)
	if err != nil {
		problems.add("settlementPolicy.contract", err)
		return domain.SettlementPolicyBody{Method: method}
	}
	applicability, err := domain.NewSettlementApplicability(legalEntity, counterparty, contract, chargeScope, currency, effective)
	if err != nil {
		problems.add("settlementPolicy", err)
		return domain.SettlementPolicyBody{Method: method}
	}
	return domain.SettlementPolicyBody{Method: method, Applicability: applicability}
}
