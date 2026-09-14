package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 本文件钉票 sa-cc/12 的领域半边：外部资金事实的付款人维度有「来源提供」与「来源未提供」两格、零值不是格；
// 「真实程序要不要付款人」是登记的规则、封闭二值、没有默认；规则对着付款人维度折出核对能不能进行——
// CC CONTEXT「来源提供或真实程序要求的付款人」与 Rules「未提供或不适用必须明确记录，规则要求但缺失时保持未决」。

// Covers: 做法 1 / 裁决 3——「来源未提供」是领域上的一格（显式值），不是空串默认也不是 (string, bool)。
func TestAFundsPayerIsEitherProvidedOrExplicitlyNotProvided(t *testing.T) {
	provided, err := domain.ProvidedFundsPayer("SYN-PAYER-01")
	if err != nil {
		t.Fatalf("来源提供的付款人：%v", err)
	}
	if !provided.Provided() || provided.Reference() != "SYN-PAYER-01" {
		t.Fatalf("provided = %#v", provided)
	}

	notProvided := domain.FundsPayerNotProvided()
	if notProvided.Provided() || notProvided.Reference() != "" {
		t.Fatalf("notProvided = %#v", notProvided)
	}

	if _, err := domain.ProvidedFundsPayer("   "); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("空白付款人引用被当成「提供了」：%v", err)
	}
}

// Covers: 裁决 1——规则封闭二值；零值不是格，String 为空，让登记面与库上的 CHECK 都能把它挡在外面。
func TestAPayerRequirementIsAClosedTwoValueRule(t *testing.T) {
	if domain.PayerRequired.String() != "REQUIRED" || domain.PayerNotRequired.String() != "NOT_REQUIRED" {
		t.Fatalf("规则词形：%q / %q", domain.PayerRequired, domain.PayerNotRequired)
	}
	if domain.PayerRequirementInvalid.String() != "" {
		t.Fatalf("零值规则有了词形：%q", domain.PayerRequirementInvalid)
	}
}

// Covers: 裁决 2 的三停格里前两格由规则折出：要求而未提供 → 未决哨兵（点名缺付款人，恢复动作是补事实）；
// 不要求 → 提供与否都放行，付款人「未提供」原样带着进核对；要求且提供 → 放行。第三格（规则未登记）不在
// 领域里——它是读口的 found=false，编排答「规则未配置」。
func TestAPayerRequirementAdmitsOrHoldsTheFundsFact(t *testing.T) {
	provided, err := domain.ProvidedFundsPayer("SYN-PAYER-01")
	if err != nil {
		t.Fatalf("来源提供的付款人：%v", err)
	}
	notProvided := domain.FundsPayerNotProvided()

	if err := domain.PayerRequired.Admit(provided); err != nil {
		t.Fatalf("要求且提供：%v", err)
	}
	if err := domain.PayerRequired.Admit(notProvided); !errors.Is(err, domain.ErrFundsPayerRequired) {
		t.Fatalf("要求而未提供：err = %v, want ErrFundsPayerRequired", err)
	}
	if err := domain.PayerNotRequired.Admit(provided); err != nil {
		t.Fatalf("不要求、来源却给了：%v", err)
	}
	if err := domain.PayerNotRequired.Admit(notProvided); err != nil {
		t.Fatalf("不要求且未提供：%v", err)
	}
}

// Covers: 零值付款人与零值规则都是调用方编程错误，不是任何一格——两边都以 ErrInvalidFundsPayer /
// ErrInvalidPayerRequirement 响亮拒，而不是悄悄当成「未提供」或「不要求」。
func TestZeroValuedPayerAndRuleAreRefusedLoudly(t *testing.T) {
	provided, err := domain.ProvidedFundsPayer("SYN-PAYER-01")
	if err != nil {
		t.Fatalf("来源提供的付款人：%v", err)
	}
	if err := domain.PayerRequired.Admit(domain.FundsPayer{}); !errors.Is(err, domain.ErrInvalidFundsPayer) {
		t.Fatalf("零值付款人：err = %v", err)
	}
	if err := domain.PayerRequirementInvalid.Admit(provided); !errors.Is(err, domain.ErrInvalidPayerRequirement) {
		t.Fatalf("零值规则：err = %v", err)
	}
}
