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

// 本文件证供应商协议册在运营操作者面载荷上的那一格（票 admin-write-faces/11）：同一份载荷过预览与过录入逐字节
// 同摘要、逐格问题按 supplierAgreement.* 路径收齐、正文里没有方向键（出现即按未知键拒）、正文挂错类别在预览上答`未受理`。

const supplierAgreementPayload = `{
  "kind": "SUPPLIER_AGREEMENT",
  "objectId": "agreement-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "supplierAgreement": {
    "supplier": "supplier-1",
    "legalEntity": "legal-1",
    "scope": "scope-procurement",
    "purchasePlan": "plan-buy-1",
    "effectiveStartsAt": "2026-08-01T00:00:00Z",
    "effectiveEndsAt": "2026-12-31T16:00:00Z"
  }
}`

// Covers: ADR-0126 Decision 四 — 供应商协议载荷过预览与过录入从同一个 Publication 出发，摘要只在领域一处算、逐字节
// 相同；载体的类别与正文是供应商协议，采购方案只作引用串到达领域。
func TestSupplierAgreementPreviewAndSubmissionShareTheDigest(t *testing.T) {
	payload := decodePublication(t, supplierAgreementPayload)
	tenant := pcNew(t, domain.NewTenantID, pcTenant)

	previewCommand, err := payload.PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	preview, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), previewCommand)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("preview = %q, %v", preview.Outcome(), err)
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
	if draft.Kind() != domain.SupplierAgreementObject {
		t.Fatalf("draft kind = %s, want SUPPLIER_AGREEMENT", draft.Kind())
	}
	if submitCommand.Content.SupplierAgreement == nil || submitCommand.Content.SupplierAgreement.PurchasePlan.String() != "plan-buy-1" {
		t.Fatalf("content = %#v", submitCommand.Content)
	}
	if endsAt, bounded := submitCommand.Content.SupplierAgreement.Effective.EndsAt(); !bounded || endsAt.IsZero() {
		t.Fatal("effectiveEndsAt did not travel into the interval")
	}
}

// Covers: ADR-0126 Decision 四「逐格问题」— 供应商协议正文的每一格问题按 supplierAgreement.<键> 路径收齐再答，
// 不撞第一格就停；立得住的格不被报。
func TestSupplierAgreementPayloadCollectsEveryFieldProblem(t *testing.T) {
	raw := `{
	  "kind": "SUPPLIER_AGREEMENT",
	  "objectId": "agreement-1",
	  "version": "v1",
	  "scope": "SYN-SCOPE-PC02C",
	  "effectiveStartsAt": "2026-08-01T00:00:00Z",
	  "supplierAgreement": {
	    "supplier": "",
	    "legalEntity": " ",
	    "scope": "scope-procurement",
	    "purchasePlan": "",
	    "effectiveStartsAt": "2026-08-01T00:00:00Z",
	    "effectiveEndsAt": "2026-07-01T00:00:00Z"
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
	for _, want := range []string{"supplierAgreement.supplier", "supplierAgreement.legalEntity",
		"supplierAgreement.purchasePlan", "supplierAgreement.effectiveEndsAt"} {
		if !fields[want] {
			t.Errorf("problem for %q missing; got %v", want, fields)
		}
	}
	if fields["supplierAgreement.scope"] || fields["supplierAgreement.effectiveStartsAt"] || fields["scope"] {
		t.Fatalf("a valid field was reported: %v", fields)
	}

	unparsable := strings.Replace(raw, `"effectiveStartsAt": "2026-08-01T00:00:00Z",
	    "effectiveEndsAt"`, `"effectiveStartsAt": "next monday",
	    "effectiveEndsAt"`, 1)
	_, _, err = decodePublication(t, unparsable).Publication(pcNew(t, domain.NewTenantID, pcTenant))
	if !errors.As(err, &problems) {
		t.Fatalf("err = %v", err)
	}
	found := false
	for _, problem := range problems.Problems {
		found = found || problem.Field == "supplierAgreement.effectiveStartsAt"
	}
	if !found {
		t.Fatalf("unparsable start not reported: %v", problems.Problems)
	}
}

// Covers: 供应商协议正文没有方向键——领域把方向钉死为 BUY，载荷里出现 direction 按未知键拒（判据同受控批文的
// supplierAgreementBodyDocument）；正文挂在别的类别下由领域答`正文与类别不符`，预览口把它答成`未受理`带成因。
func TestSupplierAgreementPayloadHasNoDirectionAndMustMatchItsKind(t *testing.T) {
	withDirection := strings.Replace(supplierAgreementPayload, `"supplier": "supplier-1",`, `"direction": "BUY", "supplier": "supplier-1",`, 1)
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(withDirection)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("payload with direction: err = %v, want ErrMalformedRequest", err)
	}

	underCreditKind := strings.Replace(supplierAgreementPayload, `"kind": "SUPPLIER_AGREEMENT"`, `"kind": "CREDIT_POLICY"`, 1)
	command, err := decodePublication(t, underCreditKind).PreviewCommand(pcNew(t, domain.NewTenantID, pcTenant))
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	preview, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Outcome() != application.CommercialPublicationPreviewNotAccepted || !errors.Is(preview.RefusalCause(), domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("preview = %q, cause = %v; want NOT_ACCEPTED / ErrPublicationContentKindMismatch", preview.Outcome(), preview.RefusalCause())
	}
}
