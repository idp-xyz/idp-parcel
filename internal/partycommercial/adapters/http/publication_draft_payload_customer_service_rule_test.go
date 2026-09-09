package commercialhttp_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证客户服务规则册在运营操作者面载荷上的那一格（票 admin-write-faces/18）：同一份载荷过预览与过录入逐字节同摘要、
// 表单里两张表的行序都不是正文（文档按各自的键归一，换行序摘要不变）、逐格问题按 customerServiceRule.* 路径收齐且两张表
// 逐行点名、适用对象两键恰一在场是一格问题、跨行的门（两项都空、同一种期限两行）由领域在预览上答成`未受理`带成因而不是
// 解码问题、正文缺席与挂错类别同归`未受理`。

const (
	firstClaimRow       = `{"kind": "FIRST_CLAIM", "startEvent": "event-delivered", "days": 30, "calendar": "calendar-cn"}`
	conclusionReviewRow = `{"kind": "CONCLUSION_REVIEW", "startEvent": "event-conclusion-notified", "days": 15, "calendar": "calendar-cn"}`
	lossMaterialsRow    = `{"claimKind": "claim-loss", "materials": ["material-photo", "material-invoice"]}`
	damageMaterialsRow  = `{"claimKind": "claim-damage", "materials": ["material-photo"]}`
)

// serviceRulePayloadWith 把给定的期限行与材料行装进同一份壳与父行里；行之间用逗号接。
func serviceRulePayloadWith(deadlines []string, materials []string) string {
	return `{
  "kind": "CUSTOMER_SERVICE_RULE",
  "objectId": "csr-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "references": {"SERVICE_PRODUCT": "product-1"},
  "customerServiceRule": {
    "serviceProduct": "product-1",
    "responsible": "operator-1",
    "scope": "SYN-SCOPE-PC02C",
    "claimDeadlines": [` + strings.Join(deadlines, ", ") + `],
    "minimumMaterials": [` + strings.Join(materials, ", ") + `]
  }
}`
}

// 结论复核写在首次索赔前面、损毁写在灭失前面：文档按种类序与索赔类型序归一，载荷里的行序不是正文。
var serviceRulePayload = serviceRulePayloadWith([]string{conclusionReviewRow, firstClaimRow}, []string{lossMaterialsRow, damageMaterialsRow})

// 只有壳没有正文：规则版本壳单独发布是合法的（受控批文那一半今天就这么做），但运营主路径要的是正文，预览对它答正文缺席。
const serviceRuleShellOnly = `{
  "kind": "CUSTOMER_SERVICE_RULE",
  "objectId": "csr-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z"
}`

func previewServiceRule(t *testing.T, raw string) application.CommercialPublicationPreview {
	t.Helper()
	command, err := decodePublication(t, raw).PreviewCommand(pcNew(t, domain.NewTenantID, pcTenant))
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	answer, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	return answer
}

// Covers: ADR-0126 Decision 四 — 规则载荷过预览与过录入从同一个 Publication 出发，摘要只在领域一处算、逐字节相同；父行三格与两张表
// 逐行到达领域正文（适用对象折回两格封闭、期限四格与材料两格原词往返）；表单里换两张表任一的行序不是换正文——文档按各自的键写，
// 摘要不跟着行序变。
func TestServiceRulePreviewAndSubmissionShareTheDigest(t *testing.T) {
	payload := decodePublication(t, serviceRulePayload)
	tenant := pcNew(t, domain.NewTenantID, pcTenant)

	preview := previewServiceRule(t, serviceRulePayload)
	if preview.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("preview = %q (cause %v)", preview.Outcome(), preview.RefusalCause())
	}
	previewed, _ := preview.Canonical()
	if !strings.HasPrefix(previewed.Digest().String(), "PCC-1:") {
		t.Fatalf("digest %s does not carry PCC-1", previewed.Digest())
	}

	submitCommand, err := payload.SubmitCommand(tenant, pcNew(t, domain.NewOperatorSubjectReference, "op-submitter"))
	if err != nil {
		t.Fatalf("submit command: %v", err)
	}
	submitted, err := application.NewSubmitPublicationDraftHandler(newDraftStore(), pcClock{at: pcNow}).Handle(context.Background(), submitCommand)
	if err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
		t.Fatalf("submit = %q, %v", submitted.Outcome(), err)
	}
	draft, _ := submitted.Draft()
	if draft.Canonical().Digest() != previewed.Digest() {
		t.Fatalf("preview digest %s ≠ submitted digest %s", previewed.Digest(), draft.Canonical().Digest())
	}
	if draft.Kind() != domain.CustomerServiceRuleObject {
		t.Fatalf("draft kind = %s, want CUSTOMER_SERVICE_RULE", draft.Kind())
	}

	body := submitCommand.Content.CustomerServiceRule
	if body == nil || len(body.Deadlines) != 2 || len(body.Materials) != 2 {
		t.Fatalf("content = %#v", submitCommand.Content)
	}
	if product, applies := body.Applicability.ServiceProduct(); !applies || product.String() != "product-1" {
		t.Fatalf("applicability = %#v", body.Applicability)
	}
	if _, applies := body.Applicability.CustomerContract(); applies {
		t.Fatal("a rule hung on a product must not also read as hung on a contract")
	}
	if body.Responsible.String() != "operator-1" || body.Scope.String() != "SYN-SCOPE-PC02C" {
		t.Fatalf("parent row = %s / %s", body.Responsible, body.Scope)
	}
	// 翻译那一步只逐行过门、不排序：第一行仍是载荷里的第一行（结论复核）；归一是文档那一层的事。
	first, second := body.Deadlines[0], body.Deadlines[1]
	if first.Kind() != domain.ConclusionReviewDeadline || first.StartEvent().String() != "event-conclusion-notified" || first.DurationDays() != 15 ||
		first.Calendar().String() != "calendar-cn" {
		t.Fatalf("first deadline = %#v", first)
	}
	if second.Kind() != domain.FirstClaimDeadline || second.DurationDays() != 30 || second.StartEvent().String() != "event-delivered" {
		t.Fatalf("second deadline = %#v", second)
	}
	if body.Materials[0].ClaimKind().String() != "claim-loss" || len(body.Materials[0].Materials()) != 2 ||
		body.Materials[1].ClaimKind().String() != "claim-damage" || len(body.Materials[1].Materials()) != 1 {
		t.Fatalf("materials = %#v", body.Materials)
	}
	if references := submitCommand.Shell.References; len(references) != 1 || references[domain.ServiceProductObject].String() != "product-1" {
		t.Fatalf("shell references = %#v", references)
	}

	reordered := previewServiceRule(t, serviceRulePayloadWith([]string{firstClaimRow, conclusionReviewRow}, []string{damageMaterialsRow, lossMaterialsRow}))
	if reordered.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("swapped preview = %q (cause %v)", reordered.Outcome(), reordered.RefusalCause())
	}
	if canonical, _ := reordered.Canonical(); canonical.Digest() != previewed.Digest() {
		t.Fatalf("swapping rows changed the digest: %s ≠ %s", canonical.Digest(), previewed.Digest())
	}
	materialsSwapped := previewServiceRule(t, serviceRulePayloadWith([]string{conclusionReviewRow, firstClaimRow},
		[]string{`{"claimKind": "claim-loss", "materials": ["material-invoice", "material-photo"]}`, damageMaterialsRow}))
	if canonical, _ := materialsSwapped.Canonical(); canonical.Digest() != previewed.Digest() {
		t.Fatalf("swapping two materials inside a row changed the digest: %s ≠ %s", canonical.Digest(), previewed.Digest())
	}
}

// Covers: ADR-0126 Decision 四「逐格问题」— 适用对象两键皆无落在 customerServiceRule.applicability（两键皆有同一格）；责任方与范围
// 留白各记自己那一格；期限行逐格点名到 claimDeadlines[i].<键>：种类集外、起算事件留白、天数非正、日历留白；材料行点名到
// minimumMaterials[i].claimKind 与 materials[j]；立得住的那一行一格都不报；days 不是 JSON 整数按形状拒。
func TestServiceRulePayloadCollectsEveryFieldProblem(t *testing.T) {
	raw := `{
	  "kind": "CUSTOMER_SERVICE_RULE",
	  "objectId": "csr-1",
	  "version": "v1",
	  "scope": "SYN-SCOPE-PC02C",
	  "effectiveStartsAt": "2026-08-01T00:00:00Z",
	  "customerServiceRule": {
	    "responsible": " ",
	    "scope": "",
	    "claimDeadlines": [
	      {"kind": "APPEAL", "startEvent": "", "days": 0, "calendar": " "},
	      {"kind": "FIRST_CLAIM", "startEvent": "event-delivered", "days": 30, "calendar": "calendar-cn"}
	    ],
	    "minimumMaterials": [
	      {"claimKind": "", "materials": ["material-photo", ""]},
	      {"claimKind": "claim-damage", "materials": ["material-photo"]}
	    ]
	  }
	}`
	_, _, err := decodePublication(t, raw).Publication(pcNew(t, domain.NewTenantID, pcTenant))
	var problems *commercialhttp.PublicationPayloadProblems
	if !errors.As(err, &problems) || !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v (%T), want *PublicationPayloadProblems", err, err)
	}
	fields := map[string]bool{}
	for _, problem := range problems.Problems {
		fields[problem.Field] = true
	}
	for _, want := range []string{
		"customerServiceRule.applicability",
		"customerServiceRule.responsible",
		"customerServiceRule.scope",
		"customerServiceRule.claimDeadlines[0].kind",
		"customerServiceRule.claimDeadlines[0].startEvent",
		"customerServiceRule.claimDeadlines[0].days",
		"customerServiceRule.claimDeadlines[0].calendar",
		"customerServiceRule.minimumMaterials[0].claimKind",
		"customerServiceRule.minimumMaterials[0].materials[1]",
	} {
		if !fields[want] {
			t.Errorf("problem for %q missing; got %v", want, fields)
		}
	}
	for field := range fields {
		if strings.HasPrefix(field, "customerServiceRule.claimDeadlines[1]") || strings.HasPrefix(field, "customerServiceRule.minimumMaterials[1]") ||
			field == "customerServiceRule" || field == "customerServiceRule.claimDeadlines[0]" || field == "customerServiceRule.minimumMaterials[0]" ||
			field == "customerServiceRule.minimumMaterials[0].materials[0]" || field == "scope" {
			t.Fatalf("a valid field %q was reported: %v", field, fields)
		}
	}

	bothKeys := strings.Replace(serviceRulePayload, `"serviceProduct": "product-1",`, `"serviceProduct": "product-1", "customerContract": "contract-1",`, 1)
	_, _, err = decodePublication(t, bothKeys).Publication(pcNew(t, domain.NewTenantID, pcTenant))
	if !errors.As(err, &problems) {
		t.Fatalf("both applicability keys: err = %v, want *PublicationPayloadProblems", err)
	}
	if len(problems.Problems) != 1 || problems.Problems[0].Field != "customerServiceRule.applicability" {
		t.Fatalf("both applicability keys: problems = %#v, want exactly customerServiceRule.applicability", problems.Problems)
	}

	daysAsText := strings.Replace(serviceRulePayload, `"days": 30`, `"days": "30"`, 1)
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(daysAsText)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("days as text: err = %v, want ErrMalformedRequest", err)
	}
	daysAsFraction := strings.Replace(serviceRulePayload, `"days": 30`, `"days": 30.5`, 1)
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(daysAsFraction)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("days as fraction: err = %v, want ErrMalformedRequest", err)
	}
	// 另四项（追踪披露、异常响应、客户更新、通知义务）不进首发（ADR-0104）：载荷里出现就是未知键，按形状拒。
	extraItem := strings.Replace(serviceRulePayload, `"responsible": "operator-1",`, `"responsible": "operator-1", "trackingDisclosure": {},`, 1)
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(extraItem)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("an item outside the first release: err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: 票 18「适用对象恰一由服务端裁；两张子表至少一项有内容」与 ADR-0104 Decision 三 — 每一格各自立得住而跨行不成立的正文
// 不是解码问题，是领域的门：两项都空答 ErrInvalidCustomerServiceRuleVersion，同一种期限两行、同一索赔类型两行答
// ErrDuplicateCustomerServiceRuleItem，一行内材料重复答 ErrInvalidMinimumMaterialsRule（记在行上），都在预览上答`未受理`带成因、
// 不带摘要；正文缺席答 ErrPublicationContentAbsent；挂错类别答 ErrPublicationContentKindMismatch。
func TestServiceRulePreviewAnswersTheCrossRowGatesAndTheKind(t *testing.T) {
	refused := func(t *testing.T, raw string, want error) {
		t.Helper()
		answer := previewServiceRule(t, raw)
		if answer.Outcome() != application.CommercialPublicationPreviewNotAccepted || !errors.Is(answer.RefusalCause(), want) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / %v", answer.Outcome(), answer.RefusalCause(), want)
		}
		if _, ok := answer.Canonical(); ok {
			t.Fatal("a refused preview must not carry a digest")
		}
	}

	refused(t, serviceRulePayloadWith(nil, nil), domain.ErrInvalidCustomerServiceRuleVersion)

	sameKind := serviceRulePayloadWith([]string{firstClaimRow, strings.Replace(conclusionReviewRow, `"kind": "CONCLUSION_REVIEW"`, `"kind": "FIRST_CLAIM"`, 1)}, nil)
	refused(t, sameKind, domain.ErrDuplicateCustomerServiceRuleItem)

	sameClaimKind := serviceRulePayloadWith(nil, []string{lossMaterialsRow, strings.Replace(damageMaterialsRow, `"claimKind": "claim-damage"`, `"claimKind": "claim-loss"`, 1)})
	refused(t, sameClaimKind, domain.ErrDuplicateCustomerServiceRuleItem)

	// 一行内材料重复是行自己的门（NewMinimumMaterialsRule），在解码那一步就记成 minimumMaterials[0] 的问题，不到预览。
	duplicateMaterial := serviceRulePayloadWith(nil, []string{`{"claimKind": "claim-loss", "materials": ["material-photo", "material-photo"]}`})
	_, _, err := decodePublication(t, duplicateMaterial).Publication(pcNew(t, domain.NewTenantID, pcTenant))
	var problems *commercialhttp.PublicationPayloadProblems
	if !errors.As(err, &problems) || len(problems.Problems) != 1 || problems.Problems[0].Field != "customerServiceRule.minimumMaterials[0]" ||
		!strings.Contains(problems.Problems[0].Problem, domain.ErrInvalidMinimumMaterialsRule.Error()) {
		t.Fatalf("duplicate material: err = %v, want one problem on customerServiceRule.minimumMaterials[0]", err)
	}
	emptyMaterials := serviceRulePayloadWith(nil, []string{`{"claimKind": "claim-loss", "materials": []}`})
	if _, _, err := decodePublication(t, emptyMaterials).Publication(pcNew(t, domain.NewTenantID, pcTenant)); !errors.As(err, &problems) ||
		len(problems.Problems) != 1 || problems.Problems[0].Field != "customerServiceRule.minimumMaterials[0]" {
		t.Fatalf("empty materials: err = %v, want one problem on customerServiceRule.minimumMaterials[0]", err)
	}

	refused(t, serviceRuleShellOnly, domain.ErrPublicationContentAbsent)

	underCreditKind := strings.Replace(serviceRulePayload, `"kind": "CUSTOMER_SERVICE_RULE"`, `"kind": "CREDIT_POLICY"`, 1)
	refused(t, underCreditKind, domain.ErrPublicationContentKindMismatch)
}
