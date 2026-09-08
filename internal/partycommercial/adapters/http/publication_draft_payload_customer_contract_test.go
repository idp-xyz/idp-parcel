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

// 本文件对客户合同册的运营操作者面载荷（票 admin-write-faces/10）证传输面：一格两层、键名镜像受控批文；逐格问题
// 收齐；恰一与「不适用必带依据」由服务端答——前者在解码那一格点名，后者由领域构造门在预览上答成`未受理`带成因。

const customerContractPayload = `{
  "kind": "CUSTOMER_CONTRACT",
  "objectId": "contract-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "references": {"ACCEPTANCE_RULE_PACKAGE": "rules-1"},
  "customerContract": {
    "contractContent": {
      "rulePackage": "rules-1",
      "bindings": [
        {"chargeScope": "charge-prepaid", "policy": "control-policy-1"},
        {"chargeScope": "charge-cod", "inapplicabilityBasis": "COD-NA-01"}
      ]
    },
    "preAcceptanceControl": {"requirement": "REQUIRED"}
  }
}`

// Covers: 载荷一格两层折成领域正文：规则包、两行约定（一指名、一不适用）、合同级声明逐格到达；预览算出 PCC-1 摘要，
// 且与直接用领域值对象算出的逐字节相同——载荷层没有第二处算摘要。
func TestCustomerContractPayloadTranslatesBothLayers(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	shell, content, err := decodePublication(t, customerContractPayload).Publication(tenant)
	if err != nil {
		t.Fatalf("publication: %v", err)
	}
	if shell.Kind != domain.CustomerContractObject || content.Kind != domain.CustomerContractObject || content.CustomerContract == nil || content.CreditPolicy != nil {
		t.Fatalf("shell kind = %s, content = %#v", shell.Kind, content)
	}
	body := content.CustomerContract
	if body.RulePackage.String() != "rules-1" || len(body.Bindings) != 2 {
		t.Fatalf("body = %#v", body)
	}
	if policy, applies := body.Bindings[0].Policy(); !applies || policy.String() != "control-policy-1" || body.Bindings[0].Scope().String() != "charge-prepaid" {
		t.Fatalf("first binding = %#v", body.Bindings[0])
	}
	if !body.Bindings[1].ExplicitlyInapplicable() || body.Bindings[1].InapplicabilityBasis().String() != "COD-NA-01" {
		t.Fatalf("second binding = %#v", body.Bindings[1])
	}
	if body.Control == nil || body.Control.Requirement != domain.PreAcceptanceControlRequired {
		t.Fatalf("control = %#v", body.Control)
	}

	previewCommand, err := decodePublication(t, customerContractPayload).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	preview, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), previewCommand)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("preview = %q, %v (%v)", preview.Outcome(), err, preview.RefusalCause())
	}
	previewed, _ := preview.Canonical()
	direct, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if previewed.Digest() != direct.Digest() || !strings.HasPrefix(previewed.Digest().String(), "PCC-1:") {
		t.Fatalf("preview digest %s ≠ domain digest %s", previewed.Digest(), direct.Digest())
	}
}

// Covers: 逐格问题收齐——正文缺席、规则包空、约定行两格都给 / 都不给、范围空、集合外的控制要求、依据留白各记在自己
// 那一格；成立的格不被点名。
func TestCustomerContractPayloadCollectsEveryFieldProblem(t *testing.T) {
	raw := `{
	  "kind": "CUSTOMER_CONTRACT",
	  "objectId": "contract-1",
	  "version": "v1",
	  "scope": "SYN-SCOPE-PC02C",
	  "effectiveStartsAt": "2026-08-01T00:00:00Z",
	  "customerContract": {
	    "contractContent": {
	      "rulePackage": " ",
	      "bindings": [
	        {"chargeScope": "charge-both", "policy": "control-policy-1", "inapplicabilityBasis": "NA"},
	        {"chargeScope": "charge-neither"},
	        {"chargeScope": "", "policy": "control-policy-1"},
	        {"chargeScope": "charge-ok", "policy": "control-policy-1"}
	      ]
	    },
	    "preAcceptanceControl": {"requirement": "NO_CONTROL", "notApplicableBasis": " "}
	  }
	}`
	_, _, err := decodePublication(t, raw).Publication(pcNew(t, domain.NewTenantID, pcTenant))
	var problems *commercialhttp.PublicationPayloadProblems
	if !errors.As(err, &problems) || !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v, want *PublicationPayloadProblems", err)
	}
	fields := map[string]bool{}
	for _, problem := range problems.Problems {
		fields[problem.Field] = true
	}
	for _, want := range []string{
		"customerContract.contractContent.rulePackage",
		"customerContract.contractContent.bindings[0]",
		"customerContract.contractContent.bindings[1]",
		"customerContract.contractContent.bindings[2].chargeScope",
		"customerContract.preAcceptanceControl.requirement",
		"customerContract.preAcceptanceControl.notApplicableBasis",
	} {
		if !fields[want] {
			t.Errorf("problem for %q missing; got %v", want, fields)
		}
	}
	for _, unwanted := range []string{"customerContract.contractContent.bindings[3]", "customerContract.contractContent.bindings[3].chargeScope", "customerContract.contractContent.bindings[3].policy"} {
		if fields[unwanted] {
			t.Errorf("a valid binding was reported: %v", fields)
		}
	}

	absent := strings.Replace(customerContractPayload, `"contractContent": {
      "rulePackage": "rules-1",
      "bindings": [
        {"chargeScope": "charge-prepaid", "policy": "control-policy-1"},
        {"chargeScope": "charge-cod", "inapplicabilityBasis": "COD-NA-01"}
      ]
    },
    `, "", 1)
	_, _, err = decodePublication(t, absent).Publication(pcNew(t, domain.NewTenantID, pcTenant))
	if !errors.As(err, &problems) {
		t.Fatalf("payload without contractContent: err = %v", err)
	}
	if len(problems.Problems) != 1 || problems.Problems[0].Field != "customerContract.contractContent" {
		t.Fatalf("problems = %#v, want contractContent alone", problems.Problems)
	}
}

// Covers: 票 10「『不适用必带依据』由服务端裁，表单不代判」——两格各自立得住而配对不成立的声明（`不适用`无依据、
// `要求控制`带依据）不是解码问题，是领域构造门的拒绝：预览答`未受理`带 ErrPreAcceptanceControlNotDeclared，不带摘要；
// 同一范围约定两次同理答 ErrConflictingFinancialControlBinding。合同级声明整节缺席则照常算——那是「本版未声明」。
func TestCustomerContractPreviewAnswersTheDomainGateForPairing(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	previewer := application.NewPreviewCommercialPublicationHandler()
	preview := func(t *testing.T, raw string) application.CommercialPublicationPreview {
		t.Helper()
		command, err := decodePublication(t, raw).PreviewCommand(tenant)
		if err != nil {
			t.Fatalf("preview command: %v", err)
		}
		answer, err := previewer.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("preview: %v", err)
		}
		return answer
	}

	notApplicableWithoutBasis := strings.Replace(customerContractPayload, `{"requirement": "REQUIRED"}`, `{"requirement": "NOT_APPLICABLE"}`, 1)
	if answer := preview(t, notApplicableWithoutBasis); answer.Outcome() != application.CommercialPublicationPreviewNotAccepted ||
		!errors.Is(answer.RefusalCause(), domain.ErrPreAcceptanceControlNotDeclared) {
		t.Fatalf("NOT_APPLICABLE without basis: outcome = %q, cause = %v", answer.Outcome(), answer.RefusalCause())
	}
	requiredWithBasis := strings.Replace(customerContractPayload, `{"requirement": "REQUIRED"}`, `{"requirement": "REQUIRED", "notApplicableBasis": "CLAUSE-9"}`, 1)
	if answer := preview(t, requiredWithBasis); answer.Outcome() != application.CommercialPublicationPreviewNotAccepted ||
		!errors.Is(answer.RefusalCause(), domain.ErrPreAcceptanceControlNotDeclared) {
		t.Fatalf("REQUIRED with basis: outcome = %q, cause = %v", answer.Outcome(), answer.RefusalCause())
	}
	duplicateScope := strings.Replace(customerContractPayload, `"chargeScope": "charge-cod"`, `"chargeScope": "charge-prepaid"`, 1)
	if answer := preview(t, duplicateScope); answer.Outcome() != application.CommercialPublicationPreviewNotAccepted ||
		!errors.Is(answer.RefusalCause(), domain.ErrConflictingFinancialControlBinding) {
		t.Fatalf("duplicate scope: outcome = %q, cause = %v", answer.Outcome(), answer.RefusalCause())
	}

	undeclared := strings.Replace(customerContractPayload, `,
    "preAcceptanceControl": {"requirement": "REQUIRED"}`, "", 1)
	answer := preview(t, undeclared)
	if answer.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("undeclared control: outcome = %q, cause = %v", answer.Outcome(), answer.RefusalCause())
	}
	full := preview(t, customerContractPayload)
	undeclaredCanonical, _ := answer.Canonical()
	fullCanonical, _ := full.Canonical()
	if undeclaredCanonical.Digest() == fullCanonical.Digest() {
		t.Fatal("dropping the contract-level declaration must change the digest")
	}
}

// Covers: 一册的正文冒不了另一册的名——壳说 CREDIT_POLICY、正文是 customerContract，预览答`未受理`带
// ErrPublicationContentKindMismatch；两册正文同时在场同理。
func TestCustomerContractPayloadUnderAnotherKindIsRefused(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	mislabelled := strings.Replace(customerContractPayload, `"kind": "CUSTOMER_CONTRACT"`, `"kind": "CREDIT_POLICY"`, 1)
	command, err := decodePublication(t, mislabelled).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	answer, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), command)
	if err != nil || answer.Outcome() != application.CommercialPublicationPreviewNotAccepted || !errors.Is(answer.RefusalCause(), domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("outcome = %q, cause = %v, err = %v", answer.Outcome(), answer.RefusalCause(), err)
	}
}
