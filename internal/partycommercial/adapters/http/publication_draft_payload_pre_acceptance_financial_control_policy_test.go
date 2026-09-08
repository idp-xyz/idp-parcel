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

// 本文件证接受前财务控制策略册在运营操作者面载荷上的那一格（票 admin-write-faces/13）：同一份载荷过预览与过录入逐字节
// 同摘要、表单里的行序不是正文（文档按判断顺序归一，换行序摘要不变）、逐格问题按 preAcceptanceFinancialControlPolicy.*
// 路径收齐且控制项逐行点名、跨行的门（顺序撞了、同键两行、零项）由领域在预览上答成`未受理`带成因而不是解码问题、
// 正文缺席与挂错类别同归`未受理`。

const (
	creditCheckRow   = `{"control": "CREDIT_CHECK", "chargeScope": "charge-terms", "order": 2, "onFailure": "AUTHORIZED_DISPOSITION", "responsibility": "operator-legal-1"}`
	prepaidFreezeRow = `{"control": "PREPAID_FREEZE", "chargeScope": "charge-prepaid", "order": 1, "onFailure": "REJECT", "responsibility": "customer-1"}`
)

// controlPolicyPayloadWith 把给定的控制项行装进同一份壳与共同通过条件里；行之间用逗号接。
func controlPolicyPayloadWith(rows ...string) string {
	return `{
  "kind": "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY",
  "objectId": "fcp-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "preAcceptanceFinancialControlPolicy": {
    "jointPassCondition": "ALL_CONTROLS_PASS",
    "controls": [` + strings.Join(rows, ", ") + `]
  }
}`
}

// 判断顺序 2 的行写在前面：文档按判断顺序归一，载荷里的行序不是正文。
var controlPolicyPayload = controlPolicyPayloadWith(creditCheckRow, prepaidFreezeRow)

// 只有壳没有正文：策略版本壳单独发布是合法的（受控批文那一半今天就这么做），但运营主路径要的是正文，预览对它答正文缺席。
const controlPolicyShellOnly = `{
  "kind": "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY",
  "objectId": "fcp-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z"
}`

func previewControlPolicy(t *testing.T, raw string) application.CommercialPublicationPreview {
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

// Covers: ADR-0126 Decision 四 — 策略载荷过预览与过录入从同一个 Publication 出发，摘要只在领域一处算、逐字节相同；
// 五格逐行到达领域正文（种类、范围、顺序、处置、责任原词往返）；表单里换两行的先后不是换正文——文档按判断顺序写，
// 摘要不跟着行序变（票 13：顺序版本内唯一，它就是这张表唯一的自然序）。
func TestControlPolicyPreviewAndSubmissionShareTheDigest(t *testing.T) {
	payload := decodePublication(t, controlPolicyPayload)
	tenant := pcNew(t, domain.NewTenantID, pcTenant)

	preview := previewControlPolicy(t, controlPolicyPayload)
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
	if draft.Kind() != domain.PreAcceptanceFinancialControlPolicyObject {
		t.Fatalf("draft kind = %s, want PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY", draft.Kind())
	}

	body := submitCommand.Content.PreAcceptanceFinancialControlPolicy
	if body == nil || body.JointPass != domain.AllControlsPass || len(body.Items) != 2 {
		t.Fatalf("content = %#v", submitCommand.Content)
	}
	// 翻译那一步只逐行过门、不排序：第一行仍是载荷里的第一行（判断顺序 2）；归一是文档那一层的事。
	first, second := body.Items[0], body.Items[1]
	if first.Kind() != domain.CreditCheckControl || first.Scope().String() != "charge-terms" || first.EvaluationOrder() != 2 ||
		first.FailureDisposition() != domain.AuthorizedDispositionOnControlFailure || first.Responsibility().String() != "operator-legal-1" {
		t.Fatalf("first item = %#v", first)
	}
	if second.Kind() != domain.PrepaidFreezeControl || second.Scope().String() != "charge-prepaid" || second.EvaluationOrder() != 1 ||
		second.FailureDisposition() != domain.RejectOnControlFailure || second.Responsibility().String() != "customer-1" {
		t.Fatalf("second item = %#v", second)
	}

	reordered := previewControlPolicy(t, controlPolicyPayloadWith(prepaidFreezeRow, creditCheckRow))
	if reordered.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("swapped preview = %q (cause %v)", reordered.Outcome(), reordered.RefusalCause())
	}
	if canonical, _ := reordered.Canonical(); canonical.Digest() != previewed.Digest() {
		t.Fatalf("swapping two rows changed the digest: %s ≠ %s", canonical.Digest(), previewed.Digest())
	}
}

// Covers: ADR-0126 Decision 四「逐格问题」— 共同通过条件集外落在 preAcceptanceFinancialControlPolicy.jointPassCondition；
// 控制项逐行点名到 controls[i].<键>：种类集外（含「无控制」那个词——它不在集合里是 ADR-0115 Decision 一，不是漏）、范围留白、
// 顺序非正、处置集外、责任留白各记在自己那一格；立得住的那一行一格都不报；顺序不是 JSON 整数按形状拒。
func TestControlPolicyPayloadCollectsEveryFieldProblem(t *testing.T) {
	raw := `{
	  "kind": "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY",
	  "objectId": "fcp-1",
	  "version": "v1",
	  "scope": "SYN-SCOPE-PC02C",
	  "effectiveStartsAt": "2026-08-01T00:00:00Z",
	  "preAcceptanceFinancialControlPolicy": {
	    "jointPassCondition": "ANY_CONTROL_PASSES",
	    "controls": [
	      {"control": "NO_CONTROL", "chargeScope": " ", "order": 0, "onFailure": "IGNORE", "responsibility": ""},
	      {"control": "PREPAID_FREEZE", "chargeScope": "charge-ok", "order": 1, "onFailure": "REJECT", "responsibility": "customer-1"}
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
		"preAcceptanceFinancialControlPolicy.jointPassCondition",
		"preAcceptanceFinancialControlPolicy.controls[0].control",
		"preAcceptanceFinancialControlPolicy.controls[0].chargeScope",
		"preAcceptanceFinancialControlPolicy.controls[0].order",
		"preAcceptanceFinancialControlPolicy.controls[0].onFailure",
		"preAcceptanceFinancialControlPolicy.controls[0].responsibility",
	} {
		if !fields[want] {
			t.Errorf("problem for %q missing; got %v", want, fields)
		}
	}
	for field := range fields {
		if strings.HasPrefix(field, "preAcceptanceFinancialControlPolicy.controls[1]") || field == "preAcceptanceFinancialControlPolicy" ||
			field == "preAcceptanceFinancialControlPolicy.controls[0]" || field == "scope" {
			t.Fatalf("a valid field %q was reported: %v", field, fields)
		}
	}

	orderAsText := strings.Replace(controlPolicyPayload, `"order": 2`, `"order": "2"`, 1)
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(orderAsText)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("order as text: err = %v, want ErrMalformedRequest", err)
	}
	orderAsFraction := strings.Replace(controlPolicyPayload, `"order": 2`, `"order": 1.5`, 1)
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(orderAsFraction)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("order as fraction: err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: 票 13「顺序唯一、范围 × 种类唯一、至少一项，都由构造门答；拒绝的话由服务端说」— 每一格各自立得住而跨行不成立的
// 正文不是解码问题，是领域的门：两行抢同一个判断顺序、同一范围上同一种控制两行答 ErrDuplicatePreAcceptanceControlItem，零项答
// ErrInvalidPreAcceptanceFinancialControlPolicy，都在预览上答`未受理`带成因、不带摘要；正文缺席答 ErrPublicationContentAbsent；
// 挂错类别答 ErrPublicationContentKindMismatch。
func TestControlPolicyPreviewAnswersTheCrossRowGatesAndTheKind(t *testing.T) {
	refused := func(t *testing.T, raw string, want error) {
		t.Helper()
		answer := previewControlPolicy(t, raw)
		if answer.Outcome() != application.CommercialPublicationPreviewNotAccepted || !errors.Is(answer.RefusalCause(), want) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / %v", answer.Outcome(), answer.RefusalCause(), want)
		}
		if _, ok := answer.Canonical(); ok {
			t.Fatal("a refused preview must not carry a digest")
		}
	}

	sameOrder := controlPolicyPayloadWith(creditCheckRow, strings.Replace(prepaidFreezeRow, `"order": 1`, `"order": 2`, 1))
	refused(t, sameOrder, domain.ErrDuplicatePreAcceptanceControlItem)

	sameKey := controlPolicyPayloadWith(creditCheckRow,
		strings.Replace(prepaidFreezeRow, `"control": "PREPAID_FREEZE", "chargeScope": "charge-prepaid"`, `"control": "CREDIT_CHECK", "chargeScope": "charge-terms"`, 1))
	refused(t, sameKey, domain.ErrDuplicatePreAcceptanceControlItem)

	refused(t, controlPolicyPayloadWith(), domain.ErrInvalidPreAcceptanceFinancialControlPolicy)

	refused(t, controlPolicyShellOnly, domain.ErrPublicationContentAbsent)

	underCreditKind := strings.Replace(controlPolicyPayload, `"kind": "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY"`, `"kind": "CREDIT_POLICY"`, 1)
	refused(t, underCreditKind, domain.ErrPublicationContentKindMismatch)
}
