package commercialhttp_test

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件对接单规则包册的运营操作者面载荷（票 admin-write-faces/12）证传输面：一格分节、键名镜像受控批文；逐格问题收齐；
// 跨格的规则（同键两行、空组、只有有效期没有终局行、未封闭零格）由领域在预览上答成`未受理`带成因；五步在四口走通，
// 发布答案里每一节各占一条声明通道。

const acceptanceRulePackagePayload = `{
  "kind": "ACCEPTANCE_RULE_PACKAGE",
  "objectId": "rules-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "acceptanceRulePackage": {
    "rulePackageBody": {
      "serviceProduct": "product-1",
      "contract": "contract-1",
      "legalEntity": "legal-1",
      "scope": "SYN-SCOPE-PC02C",
      "effectiveStartsAt": "2026-08-01T00:00:00Z",
      "rules": [
        {"category": "MINIMUM_INGRESS_IDENTITY", "reference": "RULE/ingress-identity"},
        {"category": "REGULATORY_SOURCE_DOCUMENT", "reference": "RULE/customs-doc"}
      ]
    },
    "asOfPolicies": [
      {"judgment": "NETWORK_REACHABILITY", "semantics": "AT_SUBMISSION", "policyVersion": "asof-policy/v1"}
    ],
    "acceptanceContent": {"applicableGroups": ["REQUIRED_DOCUMENT", "CUSTOMER_RELATIONSHIP"], "manualReview": "REQUIRED"},
    "intakeQualification": {"sources": ["NODE_INTAKE", "OFFSITE_PICKUP"], "qualifications": ["INTAKE-QUAL/realname"]},
    "finalRules": [
      {"outcome": "EFFECTIVE_DELIVERY", "finalKind": "FINAL/delivery"}
    ],
    "finalRuleValidity": {"anchor": "CHANNEL_RESULT_OBSERVED", "duration": "P3DT12H"},
    "sourceDataAmendment": {"closed": false, "rules": [
      {"dataGroup": "consignee.address", "stage": "ACCEPTED_NOT_YET_RECEIVED", "intent": "CORRECTION", "allowance": "ALLOWED"}
    ]}
  }
}`

// Covers: 载荷一格分节折成领域正文：五维与两条规则、时点锚、接受内容、收寄资格、终局规则与有效期、资料修订允许逐格到达；
// 预览算出 PCC-1 摘要，且与直接用领域值对象算出的逐字节相同——载荷层没有第二处算摘要。
func TestAcceptanceRulePackagePayloadTranslatesEverySection(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	shell, content, err := decodePublication(t, acceptanceRulePackagePayload).Publication(tenant)
	if err != nil {
		t.Fatalf("publication: %v", err)
	}
	if shell.Kind != domain.AcceptanceRulePackageObject || content.Kind != domain.AcceptanceRulePackageObject ||
		content.AcceptanceRulePackage == nil || content.CustomerContract != nil {
		t.Fatalf("shell kind = %s, content = %#v", shell.Kind, content)
	}
	body := content.AcceptanceRulePackage
	if body.Applicability.ServiceProduct().String() != "product-1" || body.Applicability.Contract().String() != "contract-1" ||
		body.Applicability.Scope().String() != "SYN-SCOPE-PC02C" || len(body.Rules) != 2 ||
		body.Rules[1].Category() != domain.RegulatorySourceDocumentRules || body.Rules[1].Reference().String() != "RULE/customs-doc" {
		t.Fatalf("rule package body = %#v", body)
	}
	if len(body.AsOfPolicies) != 1 || body.AsOfPolicies[0].Judgment() != domain.NetworkReachabilityJudgment ||
		body.AsOfPolicies[0].Semantics().String() != "AT_SUBMISSION" || body.AsOfPolicies[0].PolicyVersion().String() != "asof-policy/v1" {
		t.Fatalf("as-of policies = %#v", body.AsOfPolicies)
	}
	if body.AcceptanceContent == nil || body.AcceptanceContent.ManualReview != domain.ManualReviewRequired ||
		len(body.AcceptanceContent.ApplicableGroups) != 2 || body.AcceptanceContent.ApplicableGroups[0] != domain.RequiredDocumentCheckGroup {
		t.Fatalf("acceptance content = %#v", body.AcceptanceContent)
	}
	if body.IntakeQualification == nil || len(body.IntakeQualification.Sources) != 2 || body.IntakeQualification.Sources[1] != domain.DeclaredOffsitePickup ||
		len(body.IntakeQualification.Qualifications) != 1 || body.IntakeQualification.Qualifications[0].String() != "INTAKE-QUAL/realname" {
		t.Fatalf("intake qualification = %#v", body.IntakeQualification)
	}
	if len(body.FinalRules) != 1 || body.FinalRules[0].Outcome != domain.DeclaredEffectiveDelivery || body.FinalRules[0].FinalKind.String() != "FINAL/delivery" {
		t.Fatalf("final rules = %#v", body.FinalRules)
	}
	if body.FinalRuleValidity == nil || body.FinalRuleValidity.Anchor() != domain.ChannelResultObservedAnchor || body.FinalRuleValidity.Duration() != 84*time.Hour {
		t.Fatalf("final rule validity = %#v", body.FinalRuleValidity)
	}
	if body.SourceDataAmendment == nil || body.SourceDataAmendment.Closed || len(body.SourceDataAmendment.Rules) != 1 ||
		body.SourceDataAmendment.Rules[0].Stage != domain.DeclaredAcceptedNotYetReceived || body.SourceDataAmendment.Rules[0].Allowance != domain.AmendmentAllowed {
		t.Fatalf("source data amendment = %#v", body.SourceDataAmendment)
	}

	previewCommand, err := decodePublication(t, acceptanceRulePackagePayload).PreviewCommand(tenant)
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

// Covers: 逐格问题收齐——五维留白、集合外的分类 / 判断类型 / 校验组 / 人工复核 / 来源 / 责任结果 / 起算种类 / 阶段 / 意图、
// 允许性写成 NOT_DECLARED、时长带月段、closed 缺席，各记在自己那一格（行带下标）；成立的格不被点名；正文整节缺席只点名那一节。
func TestAcceptanceRulePackagePayloadCollectsEveryFieldProblem(t *testing.T) {
	raw := `{
	  "kind": "ACCEPTANCE_RULE_PACKAGE",
	  "objectId": "rules-1",
	  "version": "v1",
	  "scope": "SYN-SCOPE-PC02C",
	  "effectiveStartsAt": "2026-08-01T00:00:00Z",
	  "acceptanceRulePackage": {
	    "rulePackageBody": {
	      "serviceProduct": " ",
	      "contract": "contract-1",
	      "legalEntity": "legal-1",
	      "scope": "SYN-SCOPE-PC02C",
	      "effectiveStartsAt": "2026-08-01T00:00:00Z",
	      "rules": [
	        {"category": "NOT_A_CATEGORY", "reference": "RULE/x"},
	        {"category": "MINIMUM_INGRESS_IDENTITY", "reference": ""},
	        {"category": "MINIMUM_INGRESS_IDENTITY", "reference": "RULE/ok"}
	      ]
	    },
	    "asOfPolicies": [
	      {"judgment": "WEATHER", "semantics": "AT_SUBMISSION", "policyVersion": ""}
	    ],
	    "acceptanceContent": {"applicableGroups": ["REQUIRED_DOCUMENT", "NOPE"], "manualReview": "UNDECLARED"},
	    "intakeQualification": {"sources": ["NODE_INTAKE", "DRONE"], "qualifications": [" "]},
	    "finalRules": [
	      {"outcome": "LOST", "finalKind": "FINAL/x"},
	      {"outcome": "EFFECTIVE_DELIVERY", "finalKind": ""}
	    ],
	    "finalRuleValidity": {"anchor": "LABEL_ISSUED", "duration": "P1M"},
	    "sourceDataAmendment": {"rules": [
	      {"dataGroup": "", "stage": "SHIPPED", "intent": "TWEAK", "allowance": "NOT_DECLARED"},
	      {"dataGroup": "consignee.address", "stage": "CUSTOMS_SUBMITTED", "intent": "CORRECTION", "allowance": "DISALLOWED"}
	    ]}
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
		"acceptanceRulePackage.rulePackageBody.serviceProduct",
		"acceptanceRulePackage.rulePackageBody.rules[0].category",
		"acceptanceRulePackage.rulePackageBody.rules[1].reference",
		"acceptanceRulePackage.asOfPolicies[0].judgment",
		"acceptanceRulePackage.asOfPolicies[0].policyVersion",
		"acceptanceRulePackage.acceptanceContent.applicableGroups[1]",
		"acceptanceRulePackage.acceptanceContent.manualReview",
		"acceptanceRulePackage.intakeQualification.sources[1]",
		"acceptanceRulePackage.intakeQualification.qualifications[0]",
		"acceptanceRulePackage.finalRules[0].outcome",
		"acceptanceRulePackage.finalRules[1].finalKind",
		"acceptanceRulePackage.finalRuleValidity.anchor",
		"acceptanceRulePackage.finalRuleValidity.duration",
		"acceptanceRulePackage.sourceDataAmendment.closed",
		"acceptanceRulePackage.sourceDataAmendment.rules[0].dataGroup",
		"acceptanceRulePackage.sourceDataAmendment.rules[0].stage",
		"acceptanceRulePackage.sourceDataAmendment.rules[0].intent",
		"acceptanceRulePackage.sourceDataAmendment.rules[0].allowance",
	} {
		if !fields[want] {
			t.Errorf("problem for %q missing; got %v", want, sortedFields(fields))
		}
	}
	for _, unwanted := range []string{
		"acceptanceRulePackage.rulePackageBody.contract",
		"acceptanceRulePackage.rulePackageBody.rules[2]",
		"acceptanceRulePackage.rulePackageBody.rules[2].category",
		"acceptanceRulePackage.acceptanceContent.applicableGroups[0]",
		"acceptanceRulePackage.intakeQualification.sources[0]",
		"acceptanceRulePackage.sourceDataAmendment.rules[1].stage",
		"acceptanceRulePackage.sourceDataAmendment.rules[1].allowance",
	} {
		if fields[unwanted] {
			t.Errorf("a valid field was reported: %v", sortedFields(fields))
		}
	}

	absent := strings.Replace(acceptanceRulePackagePayload, `"rulePackageBody": {
      "serviceProduct": "product-1",
      "contract": "contract-1",
      "legalEntity": "legal-1",
      "scope": "SYN-SCOPE-PC02C",
      "effectiveStartsAt": "2026-08-01T00:00:00Z",
      "rules": [
        {"category": "MINIMUM_INGRESS_IDENTITY", "reference": "RULE/ingress-identity"},
        {"category": "REGULATORY_SOURCE_DOCUMENT", "reference": "RULE/customs-doc"}
      ]
    },
    `, "", 1)
	_, _, err = decodePublication(t, absent).Publication(pcNew(t, domain.NewTenantID, pcTenant))
	if !errors.As(err, &problems) {
		t.Fatalf("payload without rulePackageBody: err = %v", err)
	}
	if len(problems.Problems) != 1 || problems.Problems[0].Field != "acceptanceRulePackage.rulePackageBody" {
		t.Fatalf("problems = %#v, want rulePackageBody alone", problems.Problems)
	}

	// 待路由许可不归本册：写进来是未知键，严格解码拒。
	withPendingRouting := strings.Replace(acceptanceRulePackagePayload, `"asOfPolicies": [`, `"pendingRoutingBasis": "PRODUCT-CLAUSE/PENDING-OK", "asOfPolicies": [`, 1)
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(withPendingRouting)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("pendingRoutingBasis inside the rule package body: err = %v, want ErrMalformedRequest", err)
	}
}

func sortedFields(fields map[string]bool) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Covers: 票 12「恰一 / 缺件 / 冲突由服务端裁，表单不代判」——每格各自立得住而跨格不成立的输入不是解码问题，是领域构造门的
// 拒绝：预览答`未受理`带成因、不带摘要——同一判断两条时点锚、接受内容空组、只有有效期没有终局行、未封闭零格。各声明节
// 整节缺席则照常算——那是「本通道未声明」，且与齐全那份不同串。
func TestAcceptanceRulePackagePreviewAnswersTheDomainGateForCrossFieldRules(t *testing.T) {
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
	refused := func(t *testing.T, name, raw string, want error) {
		t.Helper()
		answer := preview(t, raw)
		if answer.Outcome() != application.CommercialPublicationPreviewNotAccepted || !errors.Is(answer.RefusalCause(), want) {
			t.Fatalf("%s: outcome = %q, cause = %v; want NOT_ACCEPTED / %v", name, answer.Outcome(), answer.RefusalCause(), want)
		}
		if _, ok := answer.Canonical(); ok {
			t.Fatalf("%s: a refused preview must not carry a digest", name)
		}
	}

	refused(t, "同一判断两条时点锚", strings.Replace(acceptanceRulePackagePayload,
		`{"judgment": "NETWORK_REACHABILITY", "semantics": "AT_SUBMISSION", "policyVersion": "asof-policy/v1"}`,
		`{"judgment": "NETWORK_REACHABILITY", "semantics": "AT_SUBMISSION", "policyVersion": "asof-policy/v1"},
      {"judgment": "NETWORK_REACHABILITY", "semantics": "AT_ACCEPTANCE", "policyVersion": "asof-policy/v1"}`, 1),
		domain.ErrConflictingAsOfPolicy)
	refused(t, "接受内容空组", strings.Replace(acceptanceRulePackagePayload,
		`"applicableGroups": ["REQUIRED_DOCUMENT", "CUSTOMER_RELATIONSHIP"], `, "", 1),
		domain.ErrAcceptanceContentNotConfigured)
	refused(t, "只有有效期没有终局行", strings.Replace(acceptanceRulePackagePayload,
		`"finalRules": [
      {"outcome": "EFFECTIVE_DELIVERY", "finalKind": "FINAL/delivery"}
    ],
    `, "", 1),
		domain.ErrFinalContentNotConfigured)
	refused(t, "未封闭零格", strings.Replace(acceptanceRulePackagePayload,
		`"sourceDataAmendment": {"closed": false, "rules": [
      {"dataGroup": "consignee.address", "stage": "ACCEPTED_NOT_YET_RECEIVED", "intent": "CORRECTION", "allowance": "ALLOWED"}
    ]}`, `"sourceDataAmendment": {"closed": false}`, 1),
		domain.ErrSourceDataAmendmentNotConfigured)
	refused(t, "零规则", strings.Replace(acceptanceRulePackagePayload,
		`"rules": [
        {"category": "MINIMUM_INGRESS_IDENTITY", "reference": "RULE/ingress-identity"},
        {"category": "REGULATORY_SOURCE_DOCUMENT", "reference": "RULE/customs-doc"}
      ]`, `"rules": []`, 1),
		domain.ErrInvalidAcceptanceRulePackage)

	full := preview(t, acceptanceRulePackagePayload)
	if full.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("full payload: outcome = %q, cause = %v", full.Outcome(), full.RefusalCause())
	}
	bodyOnly := `{
  "kind": "ACCEPTANCE_RULE_PACKAGE",
  "objectId": "rules-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "acceptanceRulePackage": {
    "rulePackageBody": {
      "serviceProduct": "product-1",
      "contract": "contract-1",
      "legalEntity": "legal-1",
      "scope": "SYN-SCOPE-PC02C",
      "effectiveStartsAt": "2026-08-01T00:00:00Z",
      "rules": [
        {"category": "MINIMUM_INGRESS_IDENTITY", "reference": "RULE/ingress-identity"},
        {"category": "REGULATORY_SOURCE_DOCUMENT", "reference": "RULE/customs-doc"}
      ]
    }
  }
}`
	undeclared := preview(t, bodyOnly)
	if undeclared.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("body only: outcome = %q, cause = %v", undeclared.Outcome(), undeclared.RefusalCause())
	}
	fullCanonical, _ := full.Canonical()
	undeclaredCanonical, _ := undeclared.Canonical()
	if fullCanonical.Digest() == undeclaredCanonical.Digest() {
		t.Fatal("dropping every declaration section must change the digest")
	}
	closedEmpty := preview(t, strings.Replace(bodyOnly, `      ]
    }
  }`, `      ]
    },
    "sourceDataAmendment": {"closed": true}
  }`, 1))
	if closedEmpty.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("closed with no rules: outcome = %q, cause = %v", closedEmpty.Outcome(), closedEmpty.RefusalCause())
	}
}

// Covers: 一册的正文冒不了另一册的名——壳说 CUSTOMER_CONTRACT、正文是 acceptanceRulePackage，预览答`未受理`带
// ErrPublicationContentKindMismatch。
func TestAcceptanceRulePackagePayloadUnderAnotherKindIsRefused(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	mislabelled := strings.Replace(acceptanceRulePackagePayload, `"kind": "ACCEPTANCE_RULE_PACKAGE"`, `"kind": "CUSTOMER_CONTRACT"`, 1)
	command, err := decodePublication(t, mislabelled).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	answer, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), command)
	if err != nil || answer.Outcome() != application.CommercialPublicationPreviewNotAccepted || !errors.Is(answer.RefusalCause(), domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("outcome = %q, cause = %v, err = %v", answer.Outcome(), answer.RefusalCause(), err)
	}
}

// Covers: 票 12「完成判据」的服务端半边 — 五步在四口走通：预览 PREVIEWED；录入 201 DRAFT_SUBMITTED 同摘要；批准 DRAFT_APPROVED
// 同摘要；发布 201 DRAFT_PUBLISHED，嵌的发布答案是 PUBLISHED_EFFECTIVE 且每一节各占一条声明通道（正文一通道，有效期随
// 终局规则同一通道，没有待路由那一通道）。
func TestAcceptanceRulePackageWalksTheFiveStepsWithEverySection(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	store := newDraftStore()
	rule, err := domain.NewApprovalDutyRule(tenant, true, domain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	rules := dutyRuleStore{rule: rule, found: true}
	payload := decodePublication(t, acceptanceRulePackagePayload)

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

	reference, err := commercialhttp.DecodePublicationDraftReferencePayload(strings.NewReader(`{"kind":"ACCEPTANCE_RULE_PACKAGE","objectId":"rules-1","version":"v1"}`))
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
	channels := make([]string, 0, len(body.Publication.Declarations))
	for _, declaration := range body.Publication.Declarations {
		if declaration.Outcome != "SAVED" {
			t.Fatalf("declaration %s landed as %s", declaration.Channel, declaration.Outcome)
		}
		channels = append(channels, declaration.Channel)
	}
	if got := strings.Join(channels, ","); got != "AS_OF_POLICY,ACCEPTANCE_CONTENT,INTAKE_QUALIFICATION,FINAL_RULE,RULE_PACKAGE_BODY,SOURCE_DATA_AMENDMENT" {
		t.Fatalf("declaration channels = %s", got)
	}
}
