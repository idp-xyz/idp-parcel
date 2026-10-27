package domain_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证本构建折不回正文的载体行（AT-PC-071 的领域那一半）：重建门把它与「行本身坏了」分成两格——前者交回带事实与成因的
// UnreadablePublicationDraft，后者是 ErrInvalidRehydratedPublicationDraft。

const volumetricPricePolicyDocument = `{"canonicalization":"PCC-1","kind":"PRICE_RULE","pricePolicy":{"direction":"SELL","pricingPlan":"plan-1@v1",` +
	`"planDirection":"SELL","conversion":"NONE","scope":"pricing-scope-1","effectiveStartsAt":"2026-01-03T00:00:00Z",` +
	`"caliber":{"taxDisposition":"TAX_EXCLUSIVE","taxClassification":"vat-standard","volumetricFactor":"sell-divisor-5000-cm"}}}`

// publishedCreditSpec 是一份已发布载体在库里的样子，折得回；各用例在它上面改一格。
func publishedCreditSpec(t *testing.T) (domain.RehydratePublicationDraftSpec, domain.PublicationDraft) {
	t.Helper()
	published, err := approvedCreditDraft(t).MarkPublished(draftPublishedAt)
	if err != nil {
		t.Fatalf("mark published: %v", err)
	}
	approver, _ := published.Approver()
	approvedAt, _ := published.ApprovedAt()
	publishedAt, _ := published.PublishedAt()
	return domain.RehydratePublicationDraftSpec{
		Shell: domain.PublicationDraftShell{
			TenantID: published.Tenant(), Kind: published.Kind(), ObjectID: published.ObjectID(),
			Version: published.Version(), Scope: published.Scope(), Effective: published.Effective(),
		},
		Canonicalization: published.Canonical().Canonicalization(),
		Document:         published.Canonical().Document(),
		Digest:           published.Canonical().Digest(),
		Submitter:        published.Submitter(),
		SubmittedAt:      published.SubmittedAt(),
		Status:           domain.PublicationDraftPublished,
		Approver:         approver,
		ApprovedAt:       approvedAt,
		PublishedAt:      publishedAt,
	}, published
}

func unreadableOf(t *testing.T, err error) *domain.UnreadablePublicationDraft {
	t.Helper()
	var unreadable *domain.UnreadablePublicationDraft
	if !errors.As(err, &unreadable) || unreadable == nil {
		t.Fatalf("err = %v, want an *UnreadablePublicationDraft", err)
	}
	return unreadable
}

// Covers: AT-PC-071——本构建不认得的规范化版本折不回来：重建门不答「行坏了」，答 ErrPublicationDraftNotReadable，错误链保住成因
// （ErrCanonicalizationUnsupported），并交回库上剩下的事实——身份、状态、各痕迹、列上的摘要与规范化版本、壳摘要——供上层不折正文就答业务格。
func TestADraftOfAnUnknownCanonicalizationIsNotReadableAndKeepsItsFacts(t *testing.T) {
	spec, published := publishedCreditSpec(t)
	spec.Canonicalization = "PCC-9"

	_, err := domain.RehydratePublicationDraft(spec)
	if !errors.Is(err, domain.ErrPublicationDraftNotReadable) || !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("err = %v, want ErrPublicationDraftNotReadable wrapping ErrCanonicalizationUnsupported", err)
	}
	if errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) {
		t.Fatal("a build-incompatible document is not a broken row")
	}
	unreadable := unreadableOf(t, err)
	if unreadable.Status() != domain.PublicationDraftPublished || unreadable.Kind() != domain.CreditPolicyObject ||
		unreadable.ObjectID().String() != "credit-1" || unreadable.Version().String() != "v1" || unreadable.Scope().String() != "scope-1" {
		t.Fatalf("identity/status facts lost: %#v", unreadable)
	}
	if unreadable.ContentDigest() != spec.Digest || unreadable.Canonicalization() != "PCC-9" {
		t.Fatalf("content facts = %s / %s, want the column values %s / PCC-9", unreadable.ContentDigest(), unreadable.Canonicalization(), spec.Digest)
	}
	if unreadable.ShellDigest() != published.ShellDigest() {
		t.Fatalf("shell digest = %s, want the readable draft's %s", unreadable.ShellDigest(), published.ShellDigest())
	}
	if approver, ok := unreadable.Approver(); !ok || approver != spec.Approver {
		t.Fatalf("approver = %v %v", approver, ok)
	}
	if at, ok := unreadable.PublishedAt(); !ok || !at.Equal(spec.PublishedAt) {
		t.Fatalf("published at = %v %v", at, ok)
	}
	if unreadable.Submitter() != spec.Submitter || !unreadable.SubmittedAt().Equal(spec.SubmittedAt) {
		t.Fatalf("submitter facts = %v at %v", unreadable.Submitter(), unreadable.SubmittedAt())
	}
	if !errors.Is(unreadable.Cause(), domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("cause = %v", unreadable.Cause())
	}
}

// Covers: AT-PC-071——0041 迁移之前录入的价格规则载体带着体积系数：折回时正文构造门点名拒，重建门把它算作本构建读不回的一格，
// 错误链认得 ErrVolumetricFactorOnPricePolicy（调用方不必解析文字）。
func TestAPricePolicyDraftCarryingAVolumetricFactorIsNotReadable(t *testing.T) {
	spec, _ := publishedCreditSpec(t)
	spec.Shell.Kind = domain.PriceRuleObject
	spec.Shell.ObjectID = commercialValue(t, domain.NewCommercialObjectID, "price-1")
	spec.Canonicalization = "PCC-1"
	spec.Document = []byte(volumetricPricePolicyDocument)
	spec.Digest = commercialValue(t, domain.NewCommercialContentDigest, "PCC-1:historical")
	spec.Status = domain.PublicationDraftPendingApproval
	spec.Approver, spec.ApprovedAt, spec.PublishedAt = domain.OperatorSubjectReference{}, time.Time{}, time.Time{}

	_, err := domain.RehydratePublicationDraft(spec)
	if !errors.Is(err, domain.ErrPublicationDraftNotReadable) || !errors.Is(err, domain.ErrVolumetricFactorOnPricePolicy) {
		t.Fatalf("err = %v, want ErrPublicationDraftNotReadable wrapping ErrVolumetricFactorOnPricePolicy", err)
	}
	if got := unreadableOf(t, err); got.Status() != domain.PublicationDraftPendingApproval || got.Kind() != domain.PriceRuleObject {
		t.Fatalf("facts = %#v", got)
	}
}

// Covers: AT-PC-071 的反面（一）——行本身坏了不被收进读不回：状态痕迹对不上、折得回而摘要对不上、快照里的册与壳声明的类别不符
// 仍是 ErrInvalidRehydratedPublicationDraft；并且痕迹与壳的检查先于正文折回，坏痕迹加读不回的正文答「行坏了」。
func TestABrokenRowIsStillRefusedAsBrokenNotAsNotReadable(t *testing.T) {
	spec, _ := publishedCreditSpec(t)

	brokenTraceAndUnknownVersion := spec
	brokenTraceAndUnknownVersion.Canonicalization = "PCC-9"
	brokenTraceAndUnknownVersion.PublishedAt = time.Time{}
	_, err := domain.RehydratePublicationDraft(brokenTraceAndUnknownVersion)
	if !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) || errors.Is(err, domain.ErrPublicationDraftNotReadable) {
		t.Fatalf("已发布缺发布时刻且版本不认得：err = %v, want the broken-row refusal", err)
	}

	kindMismatch := spec
	kindMismatch.Shell.Kind = domain.SettlementPolicyObject
	_, err = domain.RehydratePublicationDraft(kindMismatch)
	if !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) || errors.Is(err, domain.ErrPublicationDraftNotReadable) {
		t.Fatalf("壳与快照类别不符：err = %v, want the broken-row refusal", err)
	}

	wrongDigest := spec
	wrongDigest.Digest = commercialValue(t, domain.NewCommercialContentDigest, "PCC-1:deadbeef")
	_, err = domain.RehydratePublicationDraft(wrongDigest)
	if !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) || errors.Is(err, domain.ErrPublicationDraftNotReadable) {
		t.Fatalf("摘要对不上：err = %v, want the broken-row refusal", err)
	}
}

// Covers: AT-PC-071 的反面（二）——文档自身立不住的行在任何构建下都不合法，是行坏了而不是本构建读不回：非法 JSON、不是对象、正文缺席、
// 册名不认得、文档自称的规范化版本与列不一致、必填格为空、时刻格式不对、封闭集里的集外词。读不回只收两类（见下一条），
// 这几种若被收进去，真损坏就不再触发 5xx，而「同版本重录」也救不了它们的承诺会落空。
func TestADocumentFlawNoBuildAcceptsIsABrokenRowNotANotReadableOne(t *testing.T) {
	spec, _ := publishedCreditSpec(t)
	good := string(spec.Document)
	replaced := func(old, replacement string) string {
		t.Helper()
		if !strings.Contains(good, old) {
			t.Fatalf("夹具文档里没有 %q：%s", old, good)
		}
		return strings.Replace(good, old, replacement, 1)
	}
	legalEntity := regexp.MustCompile(`"legalEntity":"[^"]*"`).FindString(good)
	startsAt := regexp.MustCompile(`"effectiveStartsAt":"[^"]*"`).FindString(good)
	documents := map[string]string{
		"非法 JSON":      `{`,
		"不是对象":         `[]`,
		"是字符串":         `"x"`,
		"正文缺席":         `{"canonicalization":"PCC-1","kind":"CREDIT_POLICY"}`,
		"册名不认得":        replaced(`"kind":"CREDIT_POLICY"`, `"kind":"NOT_A_KIND"`),
		"文档自称的版本与列不一致": replaced(`"canonicalization":"PCC-1"`, `"canonicalization":"PCC-2"`),
		"必填格为空":        replaced(legalEntity, `"legalEntity":""`),
		"时刻格式不对":       replaced(startsAt, `"effectiveStartsAt":"yesterday"`),
	}
	for name, document := range documents {
		flawed := spec
		flawed.Document = []byte(document)
		_, err := domain.RehydratePublicationDraft(flawed)
		if !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) || errors.Is(err, domain.ErrPublicationDraftNotReadable) {
			t.Fatalf("%s：err = %v, want the broken-row refusal, not NotReadable", name, err)
		}
	}

	// 封闭集里的集外词：价格规则文档的方向不是 BUY / SELL。
	price := spec
	price.Shell.Kind = domain.PriceRuleObject
	price.Shell.ObjectID = commercialValue(t, domain.NewCommercialObjectID, "price-1")
	price.Document = []byte(strings.Replace(strings.Replace(volumetricPricePolicyDocument,
		`,"volumetricFactor":"sell-divisor-5000-cm"`, ``, 1), `"direction":"SELL"`, `"direction":"SIDEWAYS"`, 1))
	if _, err := domain.RehydratePublicationDraft(price); !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) || errors.Is(err, domain.ErrPublicationDraftNotReadable) {
		t.Fatalf("集外方向词：err = %v, want the broken-row refusal", err)
	}
}

// Covers: AT-PC-071——读不回只收两类成因：列上的规范化版本本构建不认得（不论文档长什么样：不认得的版本下文档无从判坏），
// 与文档带着构造门收紧后拒绝的内容（体积系数，见上一组用例）。
func TestOnlyAnUnknownColumnVersionOrATightenedGateMakesARowNotReadable(t *testing.T) {
	spec, _ := publishedCreditSpec(t)
	spec.Canonicalization = "PCC-9"
	spec.Document = []byte(`[]`)
	_, err := domain.RehydratePublicationDraft(spec)
	if !errors.Is(err, domain.ErrPublicationDraftNotReadable) || !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("不认得的列版本，文档无从判坏：err = %v, want NotReadable", err)
	}
}
