package commercialhttp_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件对服务产品册（票 admin-write-faces/09）证传输面：本册的载荷可以就是壳——正文格可缺（票 admin-write-faces/25
// 起唯一的正文是产品层交付条件一节，那一节的传输面见 publication_draft_payload_delivery_condition_test.go），管理台表单
// 不声明交付条件时组出来的就是这一份；同一份过预览与过录入逐字节同摘要；别册的正文夹进本册载荷由领域按 kind 不符拒成
// 一格答案；五步（预览 → 录入 → 批准 → 发布）在四口上走通，发布答案不带任何声明通道。

const serviceProductPayload = `{
  "kind": "SERVICE_PRODUCT",
  "objectId": "product-1",
  "version": "v2",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "effectiveEndsAt": "2027-08-01T00:00:00Z",
  "references": {"CUSTOMER_CONTRACT": "contract-1"}
}`

// Covers: 票 09「册与载荷」— 载荷只有壳：解出的正文面只有 kind，没有任何一册的正文；指名引用随壳走；壳的每一格
// 仍过领域构造门，立不住的格照旧收齐。
func TestServiceProductPayloadIsTheShellAlone(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	shell, content, err := decodePublication(t, serviceProductPayload).Publication(tenant)
	if err != nil {
		t.Fatalf("publication: %v", err)
	}
	if content.Kind != domain.ServiceProductObject || content.CreditPolicy != nil {
		t.Fatalf("content = %#v; want kind only", content)
	}
	if shell.Kind != domain.ServiceProductObject || shell.Version.String() != "v2" {
		t.Fatalf("shell = %#v", shell)
	}
	if reference, ok := shell.References[domain.CustomerContractObject]; !ok || reference.String() != "contract-1" {
		t.Fatalf("references = %#v", shell.References)
	}
	if endsAt, bounded := shell.Effective.EndsAt(); !bounded || !endsAt.Equal(time.Date(2027, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("effective ends at = %s, %v", endsAt, bounded)
	}

	broken := strings.Replace(strings.Replace(serviceProductPayload, `"objectId": "product-1"`, `"objectId": ""`, 1),
		`"CUSTOMER_CONTRACT": "contract-1"`, `"SERVICE_PRODUCT": "product-1", "NOPE": "x"`, 1)
	_, _, err = decodePublication(t, broken).Publication(tenant)
	var problems *commercialhttp.PublicationPayloadProblems
	if !errors.As(err, &problems) {
		t.Fatalf("err = %T %v, want *PublicationPayloadProblems", err, err)
	}
	fields := map[string]bool{}
	for _, problem := range problems.Problems {
		fields[problem.Field] = true
	}
	if !fields["objectId"] || !fields["references.NOPE"] || fields["kind"] || fields["version"] {
		t.Fatalf("problems = %v", fields)
	}
}

// Covers: ADR-0126 Decision 四对本册成立 — 同一份壳过预览与过录入逐字节同摘要，串带 PCC-1；本册的摘要不随壳变
// （换引用同串），分辨重放与修订靠壳。
func TestServiceProductPreviewAndSubmissionShareTheDigest(t *testing.T) {
	payload := decodePublication(t, serviceProductPayload)
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
	if previewed.Canonicalization() != "PCC-1" || !strings.HasPrefix(previewed.Digest().String(), "PCC-1:") {
		t.Fatalf("previewed = %s %s", previewed.Canonicalization(), previewed.Digest())
	}

	store := newDraftStore()
	submitCommand, err := payload.SubmitCommand(tenant, pcNew(t, domain.NewOperatorSubjectReference, "op-submitter"))
	if err != nil {
		t.Fatalf("submit command: %v", err)
	}
	submitted, err := application.NewSubmitPublicationDraftHandler(store, pcClock{at: pcNow}).Handle(context.Background(), submitCommand)
	if err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
		t.Fatalf("submit = %q, %v", submitted.Outcome(), err)
	}
	draft, _ := submitted.Draft()
	if draft.Canonical().Digest() != previewed.Digest() {
		t.Fatalf("preview digest %s ≠ submitted digest %s", previewed.Digest(), draft.Canonical().Digest())
	}

	// 换引用再录：摘要同串，但壳不同——登记册答修订，不是重放。
	rereferenced := decodePublication(t, strings.Replace(serviceProductPayload, `"contract-1"`, `"contract-2"`, 1))
	revisedCommand, err := rereferenced.SubmitCommand(tenant, pcNew(t, domain.NewOperatorSubjectReference, "op-submitter"))
	if err != nil {
		t.Fatalf("revised submit command: %v", err)
	}
	revised, err := application.NewSubmitPublicationDraftHandler(store, pcClock{at: pcNow}).Handle(context.Background(), revisedCommand)
	if err != nil || revised.Outcome() != application.PublicationDraftRevised {
		t.Fatalf("resubmit with another reference = %q, %v; want DRAFT_REVISED", revised.Outcome(), err)
	}
	revisedDraft, _ := revised.Draft()
	if revisedDraft.Canonical().Digest() != draft.Canonical().Digest() {
		t.Fatalf("a shell reference changed the digest: %s vs %s", revisedDraft.Canonical().Digest(), draft.Canonical().Digest())
	}
}

// Covers: 别册的正文夹进本册载荷——载荷解得开（键都认识），但预览口答 200 + NOT_ACCEPTED 带成因、不带摘要：
// 壳说服务产品、正文是信用政策，领域按 kind 不符拒（既有那一格对本册照旧）。
func TestServiceProductPayloadCarryingAForeignBodyIsNotAccepted(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	withBody := strings.Replace(serviceProductPayload, `"references": {"CUSTOMER_CONTRACT": "contract-1"}`,
		`"creditPolicy": {"legalEntity": "legal-1", "authorityLevel": "level-commercial", "chargeType": "charge-freight", "limitMinor": 1, "effectiveStartsAt": "2026-08-01T00:00:00Z"}`, 1)
	command, err := decodePublication(t, withBody).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	response := serve(commercialhttp.NewPreviewCommercialPublicationEndpoint(draftIntakeDouble{preview: command}, application.NewPreviewCommercialPublicationHandler()), http.MethodPost)
	body := decodeDraftAnswer(t, response)
	if response.Code != http.StatusOK || body.Outcome != "NOT_ACCEPTED" || body.Cause == "" || body.ContentDigest != "" {
		t.Fatalf("foreign body answer = %d %s", response.Code, response.Body.Bytes())
	}
}

// Covers: 票 09「完成判据」的服务端半边 — 五步在四口走通：预览 PREVIEWED；录入 201 DRAFT_SUBMITTED；批准 DRAFT_APPROVED
// 且摘要与录入时同串；发布 201 DRAFT_PUBLISHED，嵌的发布答案是 PUBLISHED_EFFECTIVE 且**没有任何声明通道**——本册
// 无正文，发布只登版本壳。
func TestServiceProductWalksTheFiveStepsWithoutDeclarations(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	store := newDraftStore()
	rule, err := domain.NewApprovalDutyRule(tenant, true, domain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	rules := dutyRuleStore{rule: rule, found: true}
	// 不指名任何对象：指名引用未发布会让发布用例答`发布未决`（AT-PC-005），那是另一条用例要演的。
	standalone := strings.Replace(serviceProductPayload, `,
  "references": {"CUSTOMER_CONTRACT": "contract-1"}`, "", 1)
	payload := decodePublication(t, standalone)

	previewCommand, err := payload.PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	response := serve(commercialhttp.NewPreviewCommercialPublicationEndpoint(draftIntakeDouble{preview: previewCommand}, application.NewPreviewCommercialPublicationHandler()), http.MethodPost)
	body := decodeDraftAnswer(t, response)
	if response.Code != http.StatusOK || body.Outcome != "PREVIEWED" || body.Canonicalization != "PCC-1" {
		t.Fatalf("preview = %d %s", response.Code, response.Body.Bytes())
	}
	digest := body.ContentDigest

	submitCommand, err := payload.SubmitCommand(tenant, pcNew(t, domain.NewOperatorSubjectReference, "op-submitter"))
	if err != nil {
		t.Fatalf("submit command: %v", err)
	}
	response = serve(commercialhttp.NewSubmitPublicationDraftEndpoint(draftIntakeDouble{submit: submitCommand}, newDraftOperator(store, rules, pcClock{at: pcNow})), http.MethodPost)
	body = decodeDraftAnswer(t, response)
	if response.Code != http.StatusCreated || body.Outcome != "DRAFT_SUBMITTED" || body.Status != "PENDING_APPROVAL" || body.ContentDigest != digest {
		t.Fatalf("submit = %d %s", response.Code, response.Body.Bytes())
	}

	reference, err := commercialhttp.DecodePublicationDraftReferencePayload(strings.NewReader(`{"kind":"SERVICE_PRODUCT","objectId":"product-1","version":"v2"}`))
	if err != nil {
		t.Fatalf("reference payload: %v", err)
	}
	approver, err := domain.NewOperatorSubject(pcNew(t, domain.NewOperatorSubjectReference, "op-approver"), nil)
	if err != nil {
		t.Fatalf("approver: %v", err)
	}
	approveCommand, err := reference.ApproveCommand(tenant, approver)
	if err != nil {
		t.Fatalf("approve command: %v", err)
	}
	response = serve(commercialhttp.NewApprovePublicationDraftEndpoint(draftIntakeDouble{approve: approveCommand}, newDraftOperator(store, rules, pcClock{at: pcNow.Add(time.Hour)})), http.MethodPost)
	if body = decodeDraftAnswer(t, response); response.Code != http.StatusOK || body.Outcome != "DRAFT_APPROVED" || body.ContentDigest != digest {
		t.Fatalf("approve = %d %s", response.Code, response.Body.Bytes())
	}

	publishCommand, err := reference.PublishCommand(tenant)
	if err != nil {
		t.Fatalf("publish command: %v", err)
	}
	response = serve(commercialhttp.NewPublishPublicationDraftEndpoint(draftIntakeDouble{publish: publishCommand}, newDraftOperator(store, rules, pcClock{at: pcNow.Add(2 * time.Hour)})), http.MethodPost)
	body = decodeDraftAnswer(t, response)
	if response.Code != http.StatusCreated || body.Outcome != "DRAFT_PUBLISHED" || body.Publication == nil || body.Publication.Outcome != "PUBLISHED_EFFECTIVE" {
		t.Fatalf("publish = %d %s", response.Code, response.Body.Bytes())
	}
	if len(body.Publication.Declarations) != 0 {
		t.Fatalf("a service product publication carried declarations: %#v", body.Publication.Declarations)
	}
}
