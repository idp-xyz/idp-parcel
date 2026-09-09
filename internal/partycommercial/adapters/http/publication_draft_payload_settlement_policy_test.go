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

// 本文件证结算政策册在运营操作者面载荷上的那一格（票 admin-write-faces/15）：同一份载荷过预览与过录入逐字节同摘要、
// 合同两格进领域合成一个两段式指称串、逐格问题按 settlementPolicy.* 路径收齐（合同两格各自点名）、方式集外与
// 现成串形式的合同都拒、正文挂错类别在预览上答`未受理`。

const settlementPolicyPayload = `{
  "kind": "SETTLEMENT_POLICY",
  "objectId": "settlement-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "settlementPolicy": {
    "method": "TERMS",
    "legalEntity": "legal-1",
    "counterparty": "account-1",
    "contract": { "objectId": "contract-1", "version": "v3" },
    "chargeScope": "charge-terms",
    "currency": "CNY",
    "effectiveStartsAt": "2026-08-01T00:00:00Z",
    "effectiveEndsAt": "2026-12-31T16:00:00Z"
  }
}`

// Covers: ADR-0126 Decision 四 — 结算政策载荷过预览与过录入从同一个 Publication 出发，摘要只在领域一处算、逐字节相同；
// 合同的对象 + 版本两格在翻译那一步合成领域的两段式指称串（NewQualifiedVersionLabel 那一处），载体上记的就是它。
func TestSettlementPolicyPreviewAndSubmissionShareTheDigest(t *testing.T) {
	payload := decodePublication(t, settlementPolicyPayload)
	tenant := pcNew(t, domain.NewTenantID, pcTenant)

	previewCommand, err := payload.PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	preview, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), previewCommand)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("preview = %q, %v (cause %v)", preview.Outcome(), err, preview.RefusalCause())
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
	if draft.Kind() != domain.SettlementPolicyObject {
		t.Fatalf("draft kind = %s, want SETTLEMENT_POLICY", draft.Kind())
	}
	body := submitCommand.Content.SettlementPolicy
	if body == nil || body.Method != domain.TermsMethod {
		t.Fatalf("content = %#v", submitCommand.Content)
	}
	if body.Applicability.Contract().String() != "contract-1/v3" {
		t.Fatalf("contract label = %q, want the qualified label assembled from the two fields", body.Applicability.Contract())
	}
	if body.Applicability.Currency().String() != "CNY" || body.Applicability.ChargeScope().String() != "charge-terms" {
		t.Fatalf("applicability = %#v", body.Applicability)
	}
	if endsAt, bounded := body.Applicability.Effective().EndsAt(); !bounded || endsAt.IsZero() {
		t.Fatal("effectiveEndsAt did not travel into the interval")
	}
}

// Covers: ADR-0126 Decision 四「逐格问题」— 结算政策正文的每一格问题按 settlementPolicy.<键> 路径收齐再答，合同两格各自
// 点名（settlementPolicy.contract.objectId / .version），方式集外落在 settlementPolicy.method；立得住的格不被报。
func TestSettlementPolicyPayloadCollectsEveryFieldProblem(t *testing.T) {
	raw := `{
	  "kind": "SETTLEMENT_POLICY",
	  "objectId": "settlement-1",
	  "version": "v1",
	  "scope": "SYN-SCOPE-PC02C",
	  "effectiveStartsAt": "2026-08-01T00:00:00Z",
	  "settlementPolicy": {
	    "method": "CASH",
	    "legalEntity": "legal-1",
	    "counterparty": " ",
	    "contract": { "objectId": "", "version": "v3" },
	    "chargeScope": "charge-terms",
	    "currency": "",
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
	for _, want := range []string{"settlementPolicy.method", "settlementPolicy.counterparty",
		"settlementPolicy.contract.objectId", "settlementPolicy.currency", "settlementPolicy.effectiveEndsAt"} {
		if !fields[want] {
			t.Errorf("problem for %q missing; got %v", want, fields)
		}
	}
	for _, valid := range []string{"settlementPolicy.legalEntity", "settlementPolicy.contract.version",
		"settlementPolicy.chargeScope", "settlementPolicy.effectiveStartsAt", "settlementPolicy.contract", "scope"} {
		if fields[valid] {
			t.Fatalf("a valid field %q was reported: %v", valid, fields)
		}
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
		found = found || problem.Field == "settlementPolicy.effectiveStartsAt"
	}
	if !found {
		t.Fatalf("unparsable start not reported: %v", problems.Problems)
	}
}

// Covers: CONTEXT 结算方式那条规则末句（票 admin-write-faces/23 裁决二）——六维里的合同版本同时作壳上的指名引用交出。
// 载荷层核两处：壳 references.CUSTOMER_CONTRACT 与六维 contract.objectId 都在场且不同，问题落在 references.CUSTOMER_CONTRACT
// 一格（表单从六维镜像出壳引用，壳是派生的一侧），六维那两格不被连带点名；相同则壳引用照常进 PublicationDraftShell；壳上
// 缺席不补——旧载荷与受控批文照发，排序门那半由领域按壳上有没有引用答。
func TestSettlementPolicyShellReferenceMustAgreeWithTheSixDimensionContract(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	agreeing := strings.Replace(settlementPolicyPayload, `"effectiveStartsAt": "2026-08-01T00:00:00Z",
  "settlementPolicy"`, `"effectiveStartsAt": "2026-08-01T00:00:00Z",
  "references": { "CUSTOMER_CONTRACT": "contract-1" },
  "settlementPolicy"`, 1)
	if agreeing == settlementPolicyPayload {
		t.Fatal("fixture did not take the shell reference; the anchor text moved")
	}

	shell, _, err := decodePublication(t, agreeing).Publication(tenant)
	if err != nil {
		t.Fatalf("payload whose shell reference agrees with the six-dimension contract: %v", err)
	}
	if got, present := shell.References[domain.CustomerContractObject]; !present || got.String() != "contract-1" {
		t.Fatalf("shell reference = %q (present %v), want contract-1", got, present)
	}

	disagreeing := strings.Replace(agreeing, `"CUSTOMER_CONTRACT": "contract-1"`, `"CUSTOMER_CONTRACT": "contract-2"`, 1)
	_, _, err = decodePublication(t, disagreeing).Publication(tenant)
	var problems *commercialhttp.PublicationPayloadProblems
	if !errors.As(err, &problems) || !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v (%T), want *PublicationPayloadProblems", err, err)
	}
	fields := map[string]bool{}
	for _, problem := range problems.Problems {
		fields[problem.Field] = true
	}
	if !fields["references.CUSTOMER_CONTRACT"] {
		t.Fatalf("disagreement not reported on references.CUSTOMER_CONTRACT: %v", problems.Problems)
	}
	for _, valid := range []string{"settlementPolicy.contract.objectId", "settlementPolicy.contract.version", "settlementPolicy.contract"} {
		if fields[valid] {
			t.Fatalf("the six-dimension contract %q was blamed for the shell's disagreement: %v", valid, problems.Problems)
		}
	}

	shell, _, err = decodePublication(t, settlementPolicyPayload).Publication(tenant)
	if err != nil {
		t.Fatalf("payload without a shell reference must still translate: %v", err)
	}
	if fabricated, present := shell.References[domain.CustomerContractObject]; present {
		t.Fatalf("a shell reference %q was fabricated from the six-dimension contract", fabricated)
	}
}

// Covers: 合同维只收对象 + 版本两格，不收现成的「对象/版本」串——手拼的串在分隔符变化那天静静失配（QualifiedLabel 的
// 注释）；载荷里 contract 写成字符串按 JSON 形状拒，与受控批文 contractVersionDocument 同一条规矩。方式与类别不符时
// 预览口答`未受理`带成因。
func TestSettlementPolicyPayloadTakesTheContractAsTwoFieldsAndMustMatchItsKind(t *testing.T) {
	asString := strings.Replace(settlementPolicyPayload, `"contract": { "objectId": "contract-1", "version": "v3" }`, `"contract": "contract-1/v3"`, 1)
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(asString)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("payload with a pre-joined contract label: err = %v, want ErrMalformedRequest", err)
	}
	withLabelKey := strings.Replace(settlementPolicyPayload, `"contract": { "objectId": "contract-1", "version": "v3" }`, `"contractLabel": "contract-1/v3"`, 1)
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(withLabelKey)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("payload with contractLabel: err = %v, want ErrMalformedRequest (unknown key)", err)
	}

	underCreditKind := strings.Replace(settlementPolicyPayload, `"kind": "SETTLEMENT_POLICY"`, `"kind": "CREDIT_POLICY"`, 1)
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
