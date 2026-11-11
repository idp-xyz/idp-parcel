package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件钉客户合同正文的规则包一格在 PCC-1 上的同号改形（ADR-0376 决定三，票 declared-goods-lines/11）：指名了的旧文档
// 逐字节同前，没指名即不编码，折得回「没指名」。

// customerContractGolden 取证于改形之前的提交（22a710c40）：同一份正文在那里算出的就是这一串。
const customerContractGolden = "PCC-1:dd4783f31cd8af4fec84fe52945a0ccef795232779f2f20ff16430e0bd5cc226"

// Covers: 同号改形「指名了的旧文档逐字节同前」——加了可缺之后，指名规则包的合同摘要与改形前一字不差。
func TestAContractNamingItsRulePackageKeepsItsPCC1Digest(t *testing.T) {
	prepaid := appliedControl(t, "charge-prepaid")
	cod := inapplicableControl(t, "charge-cod", "COD-NA-01")
	canonical := canonicalCustomerContract(t, customerContractBody(t, requiredControl(), prepaid, cod))
	if got := canonical.Digest().String(); got != customerContractGolden {
		t.Fatalf("digest = %s, want %s（改形动了旧文档的摘要，已存的发布核对会失配）", got, customerContractGolden)
	}
}

// Covers: ADR-0376 决定三——不指名规则包的合同正文立得住：文档里没有 rulePackage 键，摘要与指名了的那份不同，折回去仍是
// 「没指名」，再算一遍摘要不变。
func TestAContractNamingNoRulePackageCanonicalizesWithoutTheKey(t *testing.T) {
	body := customerContractBody(t, requiredControl(), appliedControl(t, "charge-prepaid"))
	named := canonicalCustomerContract(t, body)
	body.RulePackage = domain.CommercialObjectID{}
	unnamed := canonicalCustomerContract(t, body)
	if unnamed.Digest() == named.Digest() {
		t.Fatal("指名与不指名规则包算出同一个摘要")
	}
	if strings.Contains(string(unnamed.Document()), `"rulePackage"`) {
		t.Fatalf("没指名却编进了 rulePackage 键：%s", unnamed.Document())
	}
	var raw map[string]any
	if err := json.Unmarshal(unnamed.Document(), &raw); err != nil {
		t.Fatalf("文档不是 JSON：%v", err)
	}
	rehydrated, err := domain.RehydratePublicationContent(unnamed.Canonicalization(), unnamed.Document())
	if err != nil {
		t.Fatalf("折回正文：%v", err)
	}
	if rehydrated.CustomerContract == nil || rehydrated.CustomerContract.RulePackage != (domain.CommercialObjectID{}) {
		t.Fatalf("折回的正文 = %#v, want 规则包仍是没指名", rehydrated.CustomerContract)
	}
	again, err := domain.CanonicalizePublicationContent(rehydrated)
	if err != nil {
		t.Fatalf("折回再算：%v", err)
	}
	if again.Digest() != unnamed.Digest() {
		t.Fatalf("折回再算摘要变了：%s / %s", again.Digest(), unnamed.Digest())
	}
}
