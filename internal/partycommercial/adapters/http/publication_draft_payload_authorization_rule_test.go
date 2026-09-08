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

// 本文件证授权规则册在运营操作者面载荷上的那一格（票 admin-write-faces/17）：同一份载荷过预览与过录入逐字节同摘要、
// 逐格问题按 authorizationRule.cancellationAuthority[i].* 路径收齐、跨行的判（同一请求方两行）在预览上答`未受理`带成因、
// 载荷里出现授予册的键按未知键拒。

const authorizationRulePayload = `{
  "kind": "AUTHORIZATION_RULE",
  "objectId": "authz-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "authorizationRule": {
    "cancellationAuthority": [
      { "party": "OPERATIONS", "rule": "CANCEL/operations-any-time" },
      { "party": "CUSTOMER", "rule": "CANCEL/customer-before-intake" }
    ]
  }
}`

// Covers: ADR-0126 Decision 四 — 授权规则载荷过预览与过录入从同一个 Publication 出发，摘要只在领域一处算、逐字节相同；
// 目录两行按原词到达领域，行序由规范化按请求方排（表单里换行序不换摘要）。
func TestAuthorizationRulePreviewAndSubmissionShareTheDigest(t *testing.T) {
	payload := decodePublication(t, authorizationRulePayload)
	tenant := pcNew(t, domain.NewTenantID, pcTenant)

	previewCommand, err := payload.PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	preview, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), previewCommand)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("preview = %q, %v (%v)", preview.Outcome(), err, preview.RefusalCause())
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
	if draft.Canonical().Digest() != previewed.Digest() || draft.Kind() != domain.AuthorizationRuleObject {
		t.Fatalf("preview digest %s ≠ submitted digest %s (kind %s)", previewed.Digest(), draft.Canonical().Digest(), draft.Kind())
	}
	rows := submitCommand.Content.AuthorizationRule
	if rows == nil || len(rows.CancellationAuthority) != 2 ||
		rows.CancellationAuthority[0].Party != domain.DeclaredOperationsCancellation ||
		rows.CancellationAuthority[1].Rule.String() != "CANCEL/customer-before-intake" {
		t.Fatalf("content = %#v", submitCommand.Content)
	}

	reordered := decodePublication(t, strings.Replace(strings.Replace(authorizationRulePayload,
		`{ "party": "OPERATIONS", "rule": "CANCEL/operations-any-time" },`, ``, 1),
		`{ "party": "CUSTOMER", "rule": "CANCEL/customer-before-intake" }`,
		`{ "party": "CUSTOMER", "rule": "CANCEL/customer-before-intake" }, { "party": "OPERATIONS", "rule": "CANCEL/operations-any-time" }`, 1))
	reorderedCommand, err := reordered.PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("reordered preview command: %v", err)
	}
	reorderedPreview, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), reorderedCommand)
	if err != nil {
		t.Fatalf("reordered preview: %v", err)
	}
	if canonical, _ := reorderedPreview.Canonical(); canonical.Digest() != previewed.Digest() {
		t.Fatalf("row order changed the digest: %s vs %s", canonical.Digest(), previewed.Digest())
	}
}

// Covers: ADR-0126 Decision 四「逐格问题」— 目录每一行的问题按 authorizationRule.cancellationAuthority[i].<键> 收齐再答：
// 集合外的请求方、空规则各点名自己那一格；跨行的判（同一请求方两行、零行）不在解码层，走领域门在预览上答`未受理`
// 带成因；授予册的键（authorizedActions）不在本册载荷里，出现即按未知键拒。
func TestAuthorizationRulePayloadProblemsAreCollectedPerRow(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)

	badRows := strings.Replace(authorizationRulePayload,
		`{ "party": "OPERATIONS", "rule": "CANCEL/operations-any-time" },
      { "party": "CUSTOMER", "rule": "CANCEL/customer-before-intake" }`,
		`{ "party": "anyone", "rule": "CANCEL/x" },
      { "party": "CUSTOMER", "rule": "  " }`, 1)
	_, err := decodePublication(t, badRows).PreviewCommand(tenant)
	var problems *commercialhttp.PublicationPayloadProblems
	if !errors.As(err, &problems) || len(problems.Problems) != 2 {
		t.Fatalf("err = %v, want two field problems", err)
	}
	fields := problems.Problems[0].Field + "," + problems.Problems[1].Field
	if fields != "authorizationRule.cancellationAuthority[0].party,authorizationRule.cancellationAuthority[1].rule" {
		t.Fatalf("fields = %q", fields)
	}

	duplicated := strings.Replace(authorizationRulePayload, `"party": "OPERATIONS"`, `"party": "CUSTOMER"`, 1)
	command, err := decodePublication(t, duplicated).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("同一请求方两行不是逐格问题，该到领域门：%v", err)
	}
	preview, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), command)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewNotAccepted ||
		!errors.Is(preview.RefusalCause(), domain.ErrConflictingCancellationAuthority) {
		t.Fatalf("preview = %q, cause = %v, err = %v; want NOT_ACCEPTED / ErrConflictingCancellationAuthority", preview.Outcome(), preview.RefusalCause(), err)
	}

	empty := strings.Replace(authorizationRulePayload, `[
      { "party": "OPERATIONS", "rule": "CANCEL/operations-any-time" },
      { "party": "CUSTOMER", "rule": "CANCEL/customer-before-intake" }
    ]`, `[]`, 1)
	command, err = decodePublication(t, empty).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("零行不是逐格问题，该到领域门：%v", err)
	}
	preview, err = application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), command)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewNotAccepted ||
		!errors.Is(preview.RefusalCause(), domain.ErrCancellationAuthorityNotConfigured) {
		t.Fatalf("preview = %q, cause = %v, err = %v; want NOT_ACCEPTED / ErrCancellationAuthorityNotConfigured", preview.Outcome(), preview.RefusalCause(), err)
	}

	withGrants := strings.Replace(authorizationRulePayload, `"cancellationAuthority": [`, `"authorizedActions": ["MANUAL_REVIEW"], "cancellationAuthority": [`, 1)
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(withGrants)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("授予册的键进了授权规则载荷却没被拒：%v", err)
	}

	foreign := strings.Replace(authorizationRulePayload, `"kind": "AUTHORIZATION_RULE"`, `"kind": "SETTLEMENT_POLICY"`, 1)
	command, err = decodePublication(t, foreign).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("kind 不符不是逐格问题，该到领域门：%v", err)
	}
	preview, err = application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), command)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewNotAccepted ||
		!errors.Is(preview.RefusalCause(), domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("preview = %q, cause = %v; want NOT_ACCEPTED / ErrPublicationContentKindMismatch", preview.Outcome(), preview.RefusalCause())
	}
}
