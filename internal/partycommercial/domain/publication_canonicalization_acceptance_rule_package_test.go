package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件对接单规则包册接进 PCC-1 证四件（票 admin-write-faces/12；ADR-0126 Decision 一、三）：摘要稳定且与各节行序
// 无关；每一节都是正文的一部分；折成文档前过的是与发布时同一套门；文档折得回正文。外加本册特有的两件：面单有效期
// 时长的 ISO-8601 子集语法，与封闭集反查同 String() 一份名单。

// canonicalApplicability 造五维适用性，区间上界可选：与 acceptance_rule_package_test.go 的 rulePackageApplicability 分开，
// 因为本文件要证「给区间加上界」也是另一个串。
func canonicalApplicability(t *testing.T, endsAt time.Time) domain.RulePackageApplicability {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), endsAt)
	if err != nil {
		t.Fatalf("effective interval: %v", err)
	}
	applicability, err := domain.NewRulePackageApplicability(
		commercialValue(t, domain.NewCommercialObjectID, "product-1"),
		commercialValue(t, domain.NewCommercialObjectID, "contract-1"),
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewCommercialScopeReference, "scope-1"),
		interval,
	)
	if err != nil {
		t.Fatalf("applicability: %v", err)
	}
	return applicability
}

func assembledRule(t *testing.T, category domain.RuleCategory, reference string) domain.AssembledRule {
	t.Helper()
	rule, err := domain.NewAssembledRule(category, commercialValue(t, domain.NewRuleReference, reference))
	if err != nil {
		t.Fatalf("assembled rule %s: %v", reference, err)
	}
	return rule
}

func finalRule(t *testing.T, outcome domain.DeclaredResponsibilityOutcome, kind string) domain.FinalizationDeclaration {
	t.Helper()
	return domain.FinalizationDeclaration{Outcome: outcome, FinalKind: commercialValue(t, domain.NewRuleReference, kind)}
}

func labelValidity(t *testing.T, duration time.Duration) *domain.LabelValidityDeclaration {
	t.Helper()
	validity, err := domain.NewLabelValidityDeclaration(domain.ChannelResultObservedAnchor, duration)
	if err != nil {
		t.Fatalf("label validity: %v", err)
	}
	return &validity
}

// fullRulePackageBody 造一份七节齐全的正文：五维 + 两条规则、两条时点锚、接受内容、收寄资格（两来源一资格）、
// 两条终局规则 + 有效期、未封闭两格的资料修订允许。
func fullRulePackageBody(t *testing.T) domain.AcceptanceRulePackageBody {
	t.Helper()
	return domain.AcceptanceRulePackageBody{
		Applicability: canonicalApplicability(t, time.Time{}),
		Rules: []domain.AssembledRule{
			assembledRule(t, domain.MinimumIngressIdentityRules, "RULE/ingress-identity"),
			assembledRule(t, domain.RegulatorySourceDocumentRules, "RULE/customs-doc"),
		},
		AsOfPolicies: []domain.AsOfPolicy{
			asOfPolicy(t, domain.NetworkReachabilityJudgment, "AT_SUBMISSION", "asof-policy/v1"),
			asOfPolicy(t, domain.PreAcceptanceFinancialControlJudgment, "AT_ACCEPTANCE", "asof-policy/v1"),
		},
		AcceptanceContent: &domain.AcceptanceContentBody{
			ApplicableGroups: []domain.AcceptanceCheckGroupType{domain.RequiredDocumentCheckGroup, domain.CustomerRelationshipCheckGroup},
			ManualReview:     domain.ManualReviewRequired,
		},
		IntakeQualification: &domain.IntakeQualificationBody{
			Sources:        []domain.DeclaredIntakeSource{domain.DeclaredOffsitePickup, domain.DeclaredNodeIntake},
			Qualifications: []domain.RuleReference{commercialValue(t, domain.NewRuleReference, "INTAKE-QUAL/realname")},
		},
		FinalRules: []domain.FinalizationDeclaration{
			finalRule(t, domain.DeclaredReturnCompleted, "FINAL/return"),
			finalRule(t, domain.DeclaredEffectiveDelivery, "FINAL/delivery"),
		},
		FinalRuleValidity: labelValidity(t, 84*time.Hour),
		SourceDataAmendment: &domain.SourceDataAmendmentBody{
			Closed: false,
			Rules: []domain.SourceDataAmendmentRule{
				amendmentRule(t, "consignee.address", domain.DeclaredCustomsSubmitted, domain.DeclaredCorrectionIntent, domain.AmendmentDisallowed),
				amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed),
			},
		},
	}
}

func canonicalRulePackage(t *testing.T, body domain.AcceptanceRulePackageBody) domain.CanonicalPublicationContent {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:                  domain.AcceptanceRulePackageObject,
		AcceptanceRulePackage: &body,
	})
	if err != nil {
		t.Fatalf("canonicalize acceptance rule package: %v", err)
	}
	return canonical
}

func reversed[T any](items []T) []T {
	out := make([]T, 0, len(items))
	for index := len(items) - 1; index >= 0; index-- {
		out = append(out, items[index])
	}
	return out
}

// Covers: pc-gaps/12 裁决 3「不换号」——DeclaredResponsibilityOutcome 加面单渠道两格只扩大可选值集合，不改变任何已发布
// 文档的字节（ADR-0126 决定一「加键不换号」的同一精神；同形先例 pc-gaps/09 加 Validity() 槽、awf/25 加 omitempty 键都未换号）。
// 字面摘要取自本票改动前（基线 dede3c2e）对同一正文算得的值：既有各格声明的文档在本票前后逐字节同，仍按 PCC-1 重放。
// 与它并列的第二段证新两格真进了文档且折得回正文——加格是加可选值，不是加键。
func TestLabelServiceOutcomesDoNotRenumberTheCanonicalization(t *testing.T) {
	const digestBeforeThisTicket = "PCC-1:03bd7ee53502ffb6dd91bc41560ac092a5490d2fd62a7f3a2354a3265cd305a1"
	networkOnly := canonicalRulePackage(t, fullRulePackageBody(t))
	if got := networkOnly.Digest().String(); got != digestBeforeThisTicket {
		t.Fatalf("既有各格声明的文档摘要变了：got %s, want %s（本票只加可选值，不得换号也不得改字节）", got, digestBeforeThisTicket)
	}

	withLabelRows := fullRulePackageBody(t)
	withLabelRows.FinalRules = append(withLabelRows.FinalRules,
		finalRule(t, domain.DeclaredLabelServiceFailed, "FINAL/label-failed"),
		finalRule(t, domain.DeclaredLabelServiceCompleted, "FINAL/label-completed"),
	)
	canonical := canonicalRulePackage(t, withLabelRows)
	if canonical.Canonicalization() != "PCC-1" {
		t.Fatalf("canonicalization = %s, want PCC-1", canonical.Canonicalization())
	}
	document := string(canonical.Document())
	completedAt := strings.Index(document, `{"outcome":"LABEL_SERVICE_COMPLETED","finalKind":"FINAL/label-completed"}`)
	failedAt := strings.Index(document, `{"outcome":"LABEL_SERVICE_FAILED","finalKind":"FINAL/label-failed"}`)
	if completedAt < 0 || failedAt < 0 || failedAt < completedAt {
		t.Fatalf("面单两行没按声明顺序进文档：%s", document)
	}
	rehydrated, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	rows := rehydrated.AcceptanceRulePackage.FinalRules
	if len(rows) != 4 || rows[2].Outcome != domain.DeclaredLabelServiceCompleted || rows[3].Outcome != domain.DeclaredLabelServiceFailed {
		t.Fatalf("rehydrated final rules = %#v", rows)
	}
}

// Covers: ADR-0126 Decision 一（加册不换号）— 接单规则包接进 PCC-1：同一正文两次算逐字节同串；规则、时点锚、校验组、
// 来源、终局行、修订格各自换行序不换摘要——归一是服务端的事，表单里的行序不是正文。
func TestAcceptanceRulePackageDigestIsStableAndOrderInsensitive(t *testing.T) {
	body := fullRulePackageBody(t)
	first := canonicalRulePackage(t, body)
	if first.Canonicalization() != "PCC-1" || !strings.HasPrefix(first.Digest().String(), "PCC-1:") {
		t.Fatalf("acceptance rule package must be canonicalized under PCC-1, got %s", first.Digest())
	}
	if again := canonicalRulePackage(t, fullRulePackageBody(t)); again.Digest() != first.Digest() {
		t.Fatalf("same body produced different digests: %s / %s", first.Digest(), again.Digest())
	}

	shuffled := fullRulePackageBody(t)
	shuffled.Rules = reversed(shuffled.Rules)
	shuffled.AsOfPolicies = reversed(shuffled.AsOfPolicies)
	shuffled.AcceptanceContent.ApplicableGroups = reversed(shuffled.AcceptanceContent.ApplicableGroups)
	shuffled.IntakeQualification.Sources = reversed(shuffled.IntakeQualification.Sources)
	shuffled.FinalRules = reversed(shuffled.FinalRules)
	shuffled.SourceDataAmendment.Rules = reversed(shuffled.SourceDataAmendment.Rules)
	if reordered := canonicalRulePackage(t, shuffled); reordered.Digest() != first.Digest() {
		t.Fatalf("row order changed the digest: %s vs %s", reordered.Digest(), first.Digest())
	}
	if !domain.IsRegisterCanonicalized(domain.AcceptanceRulePackageObject) {
		t.Fatal("IsRegisterCanonicalized must answer true for ACCEPTANCE_RULE_PACKAGE once this register is wired")
	}
}

// Covers: 票 12「正文与全部声明在同一份载荷里、同一个摘要下」— 每一节都是正文的一部分：去掉任何一节、改人工复核指令、
// 改封闭标记、去掉有效期，各自都是另一个串；空节与缺席（nil 与空切片）折成同一份字节。
func TestAcceptanceRulePackageEverySectionIsPartOfTheDigest(t *testing.T) {
	base := canonicalRulePackage(t, fullRulePackageBody(t)).Digest()
	variants := map[string]func(body *domain.AcceptanceRulePackageBody){
		"去掉时点锚":  func(body *domain.AcceptanceRulePackageBody) { body.AsOfPolicies = nil },
		"去掉接受内容": func(body *domain.AcceptanceRulePackageBody) { body.AcceptanceContent = nil },
		"去掉收寄资格": func(body *domain.AcceptanceRulePackageBody) { body.IntakeQualification = nil },
		"去掉终局规则": func(body *domain.AcceptanceRulePackageBody) { body.FinalRules, body.FinalRuleValidity = nil, nil },
		"去掉有效期":  func(body *domain.AcceptanceRulePackageBody) { body.FinalRuleValidity = nil },
		"去掉资料修订": func(body *domain.AcceptanceRulePackageBody) { body.SourceDataAmendment = nil },
		"改人工复核": func(body *domain.AcceptanceRulePackageBody) {
			body.AcceptanceContent.ManualReview = domain.ManualReviewNotRequired
		},
		"改封闭标记":  func(body *domain.AcceptanceRulePackageBody) { body.SourceDataAmendment.Closed = true },
		"改有效期时长": func(body *domain.AcceptanceRulePackageBody) { body.FinalRuleValidity = labelValidity(t, 72*time.Hour) },
		"去掉一条规则": func(body *domain.AcceptanceRulePackageBody) { body.Rules = body.Rules[:1] },
		"去掉资格清单": func(body *domain.AcceptanceRulePackageBody) { body.IntakeQualification.Qualifications = nil },
		"给区间加上界": func(body *domain.AcceptanceRulePackageBody) {
			body.Applicability = canonicalApplicability(t, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
		},
	}
	seen := map[domain.CommercialContentDigest]string{base: "完整正文"}
	for name, mutate := range variants {
		t.Run(name, func(t *testing.T) {
			body := fullRulePackageBody(t)
			mutate(&body)
			digest := canonicalRulePackage(t, body).Digest()
			if other, taken := seen[digest]; taken {
				t.Fatalf("shares a digest with %q: %s", other, digest)
			}
			seen[digest] = name
		})
	}

	nilSections := fullRulePackageBody(t)
	nilSections.AsOfPolicies, nilSections.FinalRules, nilSections.FinalRuleValidity = nil, nil, nil
	emptySections := fullRulePackageBody(t)
	emptySections.AsOfPolicies, emptySections.FinalRules, emptySections.FinalRuleValidity = []domain.AsOfPolicy{}, []domain.FinalizationDeclaration{}, nil
	if canonicalRulePackage(t, nilSections).Digest() != canonicalRulePackage(t, emptySections).Digest() {
		t.Fatal("an empty section and an absent section must be the same bytes")
	}
}

// Covers: 票 12「canonical 文档键名镜像受控批文」— 文档里各节的键与 cmd/parcel-commercial 批文 declarations 下的
// rulePackageBody / asOfPolicies / acceptanceContent / intakeQualification / finalRules / finalRuleValidity / sourceDataAmendment
// 同名；行按稳定顺序写出；有效期时长写成子集的规范形；缺席的节整键缺席；待路由许可没有键（它不归本册）。
func TestAcceptanceRulePackageDocumentMirrorsTheBatchKeys(t *testing.T) {
	canonical := canonicalRulePackage(t, fullRulePackageBody(t))
	var document struct {
		Canonicalization string `json:"canonicalization"`
		Kind             string `json:"kind"`
		Package          struct {
			RulePackageBody struct {
				ServiceProduct    string              `json:"serviceProduct"`
				Contract          string              `json:"contract"`
				LegalEntity       string              `json:"legalEntity"`
				Scope             string              `json:"scope"`
				EffectiveStartsAt string              `json:"effectiveStartsAt"`
				Rules             []map[string]string `json:"rules"`
			} `json:"rulePackageBody"`
			AsOfPolicies      []map[string]string `json:"asOfPolicies"`
			AcceptanceContent struct {
				ApplicableGroups []string `json:"applicableGroups"`
				ManualReview     string   `json:"manualReview"`
			} `json:"acceptanceContent"`
			IntakeQualification struct {
				Sources        []string `json:"sources"`
				Qualifications []string `json:"qualifications"`
			} `json:"intakeQualification"`
			FinalRules          []map[string]string `json:"finalRules"`
			FinalRuleValidity   map[string]string   `json:"finalRuleValidity"`
			SourceDataAmendment struct {
				Closed bool                `json:"closed"`
				Rules  []map[string]string `json:"rules"`
			} `json:"sourceDataAmendment"`
		} `json:"acceptanceRulePackage"`
	}
	if err := json.Unmarshal(canonical.Document(), &document); err != nil {
		t.Fatalf("decode document %s: %v", canonical.Document(), err)
	}
	if document.Canonicalization != "PCC-1" || document.Kind != "ACCEPTANCE_RULE_PACKAGE" {
		t.Fatalf("document header = %q / %q", document.Canonicalization, document.Kind)
	}
	body := document.Package.RulePackageBody
	if body.ServiceProduct != "product-1" || body.Contract != "contract-1" || body.LegalEntity != "legal-1" ||
		body.Scope != "scope-1" || body.EffectiveStartsAt != "2026-10-01T00:00:00Z" {
		t.Fatalf("rulePackageBody = %#v", body)
	}
	if len(body.Rules) != 2 || body.Rules[0]["category"] != "MINIMUM_INGRESS_IDENTITY" || body.Rules[0]["reference"] != "RULE/ingress-identity" ||
		body.Rules[1]["category"] != "REGULATORY_SOURCE_DOCUMENT" {
		t.Fatalf("rules = %v", body.Rules)
	}
	if len(document.Package.AsOfPolicies) != 2 || document.Package.AsOfPolicies[0]["judgment"] != "NETWORK_REACHABILITY" ||
		document.Package.AsOfPolicies[0]["semantics"] != "AT_SUBMISSION" || document.Package.AsOfPolicies[1]["judgment"] != "PRE_ACCEPTANCE_FINANCIAL_CONTROL" ||
		document.Package.AsOfPolicies[1]["policyVersion"] != "asof-policy/v1" {
		t.Fatalf("asOfPolicies = %v", document.Package.AsOfPolicies)
	}
	if groups := document.Package.AcceptanceContent.ApplicableGroups; len(groups) != 2 || groups[0] != "CUSTOMER_RELATIONSHIP" || groups[1] != "REQUIRED_DOCUMENT" ||
		document.Package.AcceptanceContent.ManualReview != "REQUIRED" {
		t.Fatalf("acceptanceContent = %#v", document.Package.AcceptanceContent)
	}
	if sources := document.Package.IntakeQualification.Sources; len(sources) != 2 || sources[0] != "NODE_INTAKE" || sources[1] != "OFFSITE_PICKUP" ||
		len(document.Package.IntakeQualification.Qualifications) != 1 || document.Package.IntakeQualification.Qualifications[0] != "INTAKE-QUAL/realname" {
		t.Fatalf("intakeQualification = %#v", document.Package.IntakeQualification)
	}
	if finals := document.Package.FinalRules; len(finals) != 2 || finals[0]["outcome"] != "EFFECTIVE_DELIVERY" || finals[0]["finalKind"] != "FINAL/delivery" ||
		finals[1]["outcome"] != "RETURN_COMPLETED" {
		t.Fatalf("finalRules = %v", finals)
	}
	if document.Package.FinalRuleValidity["anchor"] != "CHANNEL_RESULT_OBSERVED" || document.Package.FinalRuleValidity["duration"] != "P3DT12H" {
		t.Fatalf("finalRuleValidity = %v", document.Package.FinalRuleValidity)
	}
	amendment := document.Package.SourceDataAmendment
	if amendment.Closed || len(amendment.Rules) != 2 || amendment.Rules[0]["stage"] != "ACCEPTED_NOT_YET_RECEIVED" ||
		amendment.Rules[0]["allowance"] != "ALLOWED" || amendment.Rules[1]["stage"] != "CUSTOMS_SUBMITTED" || amendment.Rules[1]["allowance"] != "DISALLOWED" {
		t.Fatalf("sourceDataAmendment = %#v", amendment)
	}
	if strings.Contains(string(canonical.Document()), "pendingRouting") {
		t.Fatalf("pending routing permission belongs to the service product, not this register: %s", canonical.Document())
	}

	bare := fullRulePackageBody(t)
	bare.AsOfPolicies, bare.AcceptanceContent, bare.IntakeQualification = nil, nil, nil
	bare.FinalRules, bare.FinalRuleValidity, bare.SourceDataAmendment = nil, nil, nil
	bareDocument := string(canonicalRulePackage(t, bare).Document())
	for _, key := range []string{"asOfPolicies", "acceptanceContent", "intakeQualification", "finalRules", "finalRuleValidity", "sourceDataAmendment"} {
		if strings.Contains(bareDocument, key) {
			t.Fatalf("an undeclared section must omit its key %q: %s", key, bareDocument)
		}
	}
	closedEmpty := fullRulePackageBody(t)
	closedEmpty.SourceDataAmendment = &domain.SourceDataAmendmentBody{Closed: true}
	closedDocument := string(canonicalRulePackage(t, closedEmpty).Document())
	if !strings.Contains(closedDocument, `"sourceDataAmendment":{"closed":true}`) {
		t.Fatalf("a closed, empty amendment declaration must be written as closed alone: %s", closedDocument)
	}
}

// Covers: 票 12「表单不裁任何门，恰一 / 缺件 / 冲突由服务端裁」— 折成文档前过的是与发布时同一套门，各节的拒绝哨兵与
// 各自构造门相同；外加本册在库上主键前先拒的两格（同一规则两行、同一资格引用两行）与只属规范化的一格（非整秒时长）。
// 正文缺席与冒别册的名照旧分开。
func TestAcceptanceRulePackageCanonicalizationRefusesWhatPublicationWouldRefuse(t *testing.T) {
	_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.AcceptanceRulePackageObject})
	if !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("no body: err = %v, want ErrPublicationContentAbsent", err)
	}
	body := fullRulePackageBody(t)
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CustomerContractObject, AcceptanceRulePackage: &body})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("rule package body under contract kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}

	refusals := map[string]struct {
		mutate func(body *domain.AcceptanceRulePackageBody)
		want   error
	}{
		"五维适用性零值": {func(body *domain.AcceptanceRulePackageBody) { body.Applicability = domain.RulePackageApplicability{} }, domain.ErrInvalidRulePackageApplicability},
		"零规则":     {func(body *domain.AcceptanceRulePackageBody) { body.Rules = nil }, domain.ErrInvalidAcceptanceRulePackage},
		"零值规则":    {func(body *domain.AcceptanceRulePackageBody) { body.Rules = []domain.AssembledRule{{}} }, domain.ErrInvalidAssembledRule},
		"同一规则两行": {func(body *domain.AcceptanceRulePackageBody) {
			body.Rules = append(body.Rules, assembledRule(t, domain.MinimumIngressIdentityRules, "RULE/ingress-identity"))
		}, domain.ErrInvalidAcceptanceRulePackage},
		"同一判断两条时点锚": {func(body *domain.AcceptanceRulePackageBody) {
			body.AsOfPolicies = append(body.AsOfPolicies, asOfPolicy(t, domain.NetworkReachabilityJudgment, "AT_ACCEPTANCE", "asof-policy/v1"))
		}, domain.ErrConflictingAsOfPolicy},
		"零值时点锚":  {func(body *domain.AcceptanceRulePackageBody) { body.AsOfPolicies = []domain.AsOfPolicy{{}} }, domain.ErrAsOfPolicyNotConfigured},
		"接受内容空组": {func(body *domain.AcceptanceRulePackageBody) { body.AcceptanceContent.ApplicableGroups = nil }, domain.ErrAcceptanceContentNotConfigured},
		"接受内容未声明人工复核": {func(body *domain.AcceptanceRulePackageBody) {
			body.AcceptanceContent.ManualReview = domain.ManualReviewUndeclared
		}, domain.ErrAcceptanceContentNotConfigured},
		"同一校验组两行": {func(body *domain.AcceptanceRulePackageBody) {
			body.AcceptanceContent.ApplicableGroups = append(body.AcceptanceContent.ApplicableGroups, domain.RequiredDocumentCheckGroup)
		}, domain.ErrConflictingCheckGroup},
		"收寄资格零来源": {func(body *domain.AcceptanceRulePackageBody) { body.IntakeQualification.Sources = nil }, domain.ErrIntakeContentNotConfigured},
		"同一来源两行": {func(body *domain.AcceptanceRulePackageBody) {
			body.IntakeQualification.Sources = append(body.IntakeQualification.Sources, domain.DeclaredNodeIntake)
		}, domain.ErrConflictingIntakeSource},
		"同一资格引用两行": {func(body *domain.AcceptanceRulePackageBody) {
			body.IntakeQualification.Qualifications = append(body.IntakeQualification.Qualifications, body.IntakeQualification.Qualifications[0])
		}, domain.ErrIntakeContentNotConfigured},
		"同一责任结果两行": {func(body *domain.AcceptanceRulePackageBody) {
			body.FinalRules = append(body.FinalRules, finalRule(t, domain.DeclaredEffectiveDelivery, "FINAL/other"))
		}, domain.ErrConflictingFinalization},
		"零值终局行":      {func(body *domain.AcceptanceRulePackageBody) { body.FinalRules = []domain.FinalizationDeclaration{{}} }, domain.ErrFinalContentNotConfigured},
		"只给有效期不给终局行": {func(body *domain.AcceptanceRulePackageBody) { body.FinalRules = nil }, domain.ErrFinalContentNotConfigured},
		"零值有效期": {func(body *domain.AcceptanceRulePackageBody) {
			body.FinalRuleValidity = &domain.LabelValidityDeclaration{}
		}, domain.ErrInvalidLabelValidity},
		"非整秒有效期": {func(body *domain.AcceptanceRulePackageBody) {
			body.FinalRuleValidity = labelValidity(t, 1500*time.Millisecond)
		}, domain.ErrInvalidLabelValidity},
		"未封闭零格": {func(body *domain.AcceptanceRulePackageBody) { body.SourceDataAmendment.Rules = nil }, domain.ErrSourceDataAmendmentNotConfigured},
		"允许性写成未声明": {func(body *domain.AcceptanceRulePackageBody) {
			body.SourceDataAmendment.Rules[0].Allowance = domain.AmendmentAllowanceNotDeclared
		}, domain.ErrSourceDataAmendmentNotConfigured},
		"同一格两行": {func(body *domain.AcceptanceRulePackageBody) {
			body.SourceDataAmendment.Rules = append(body.SourceDataAmendment.Rules, body.SourceDataAmendment.Rules[0])
		}, domain.ErrConflictingSourceDataAmendment},
	}
	for name, refusal := range refusals {
		t.Run(name, func(t *testing.T) {
			body := fullRulePackageBody(t)
			refusal.mutate(&body)
			_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.AcceptanceRulePackageObject, AcceptanceRulePackage: &body})
			if !errors.Is(err, refusal.want) {
				t.Fatalf("err = %v, want %v", err, refusal.want)
			}
		})
	}
}

// Covers: ADR-0126 Decision 三（正文快照）— 规范化文档折得回正文：七节逐格回到领域值对象，折回去再算一遍与列里的摘要
// 相等；缺席的节折回 nil 不折回零值；快照里坏一格（同一责任结果两行）折不回。
func TestAcceptanceRulePackageDocumentRehydratesToTheSameBody(t *testing.T) {
	body := fullRulePackageBody(t)
	canonical := canonicalRulePackage(t, body)

	content, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if content.Kind != domain.AcceptanceRulePackageObject || content.AcceptanceRulePackage == nil || content.CustomerContract != nil {
		t.Fatalf("rehydrated content = %#v", content)
	}
	rehydrated := content.AcceptanceRulePackage
	if rehydrated.Applicability != body.Applicability || len(rehydrated.Rules) != 2 {
		t.Fatalf("rehydrated body = %#v", rehydrated)
	}
	if len(rehydrated.AsOfPolicies) != 2 || rehydrated.AsOfPolicies[1].Judgment() != domain.PreAcceptanceFinancialControlJudgment ||
		rehydrated.AsOfPolicies[1].Semantics().String() != "AT_ACCEPTANCE" {
		t.Fatalf("as-of policies = %#v", rehydrated.AsOfPolicies)
	}
	if rehydrated.AcceptanceContent == nil || rehydrated.AcceptanceContent.ManualReview != domain.ManualReviewRequired ||
		len(rehydrated.AcceptanceContent.ApplicableGroups) != 2 {
		t.Fatalf("acceptance content = %#v", rehydrated.AcceptanceContent)
	}
	if rehydrated.IntakeQualification == nil || len(rehydrated.IntakeQualification.Sources) != 2 ||
		len(rehydrated.IntakeQualification.Qualifications) != 1 || rehydrated.IntakeQualification.Qualifications[0].String() != "INTAKE-QUAL/realname" {
		t.Fatalf("intake qualification = %#v", rehydrated.IntakeQualification)
	}
	if len(rehydrated.FinalRules) != 2 || rehydrated.FinalRuleValidity == nil || rehydrated.FinalRuleValidity.Duration() != 84*time.Hour ||
		rehydrated.FinalRuleValidity.Anchor() != domain.ChannelResultObservedAnchor {
		t.Fatalf("final rules = %#v validity = %#v", rehydrated.FinalRules, rehydrated.FinalRuleValidity)
	}
	if rehydrated.SourceDataAmendment == nil || rehydrated.SourceDataAmendment.Closed || len(rehydrated.SourceDataAmendment.Rules) != 2 {
		t.Fatalf("source data amendment = %#v", rehydrated.SourceDataAmendment)
	}
	recomputed, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("recanonicalize: %v", err)
	}
	if recomputed.Digest() != canonical.Digest() {
		t.Fatalf("rehydrated body digests to %s, column says %s", recomputed.Digest(), canonical.Digest())
	}

	bare := fullRulePackageBody(t)
	bare.AsOfPolicies, bare.AcceptanceContent, bare.IntakeQualification = nil, nil, nil
	bare.FinalRules, bare.FinalRuleValidity, bare.SourceDataAmendment = nil, nil, nil
	bareCanonical := canonicalRulePackage(t, bare)
	content, err = domain.RehydratePublicationContent(bareCanonical.Canonicalization(), bareCanonical.Document())
	if err != nil {
		t.Fatalf("rehydrate bare body: %v", err)
	}
	if pack := content.AcceptanceRulePackage; pack == nil || pack.AsOfPolicies != nil || pack.AcceptanceContent != nil ||
		pack.IntakeQualification != nil || pack.FinalRules != nil || pack.FinalRuleValidity != nil || pack.SourceDataAmendment != nil {
		t.Fatalf("absent sections must rehydrate to nil, got %#v", content.AcceptanceRulePackage)
	}

	corrupted := strings.Replace(string(canonical.Document()), `"outcome":"RETURN_COMPLETED"`, `"outcome":"EFFECTIVE_DELIVERY"`, 1)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(corrupted)); !errors.Is(err, domain.ErrConflictingFinalization) {
		t.Fatalf("corrupted snapshot: err = %v, want ErrConflictingFinalization", err)
	}
}

// Covers: ADR-0119 与票 12「duration 收 ISO-8601 子集 P[nD][T[nH][nM][nS]]，年 / 月 / 周不收」— 子集内的串解析到同一
// time.Duration，`PT36H` 与 `P1DT12H` 是同一时长；年 / 月 / 周 / 小数 / 逆序 / 重复 / 空段 / 缺 P 一律拒且 Is
// ErrInvalidLabelValidity；零时长解析放行、由声明构造门拒。
func TestLabelValidityDurationParsesTheISOSubsetOnly(t *testing.T) {
	accepted := map[string]time.Duration{
		"P3DT12H":    84 * time.Hour,
		"PT36H":      36 * time.Hour,
		"P1DT12H":    36 * time.Hour,
		"P1D":        24 * time.Hour,
		"PT90M":      90 * time.Minute,
		"PT1H30M15S": time.Hour + 30*time.Minute + 15*time.Second,
		"P0D":        0,
	}
	for raw, want := range accepted {
		got, err := domain.ParseLabelValidityDuration(raw)
		if err != nil || got != want {
			t.Fatalf("%s: got %v, %v; want %v", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "P", "PT", "3D", "P1M", "P1Y", "P1W", "PT1.5S", "PT12H3D", "P3DT", "PT30M1H", "PT1H1H", "P-1D", "P1DT12", "1D"} {
		if _, err := domain.ParseLabelValidityDuration(raw); !errors.Is(err, domain.ErrInvalidLabelValidity) {
			t.Fatalf("%q must be refused with ErrInvalidLabelValidity, got %v", raw, err)
		}
	}
	if _, err := domain.NewLabelValidityDeclaration(domain.ChannelResultObservedAnchor, 0); !errors.Is(err, domain.ErrInvalidLabelValidity) {
		t.Fatalf("P0D must be refused by the declaration gate, got %v", err)
	}
}

// Covers: 反查与 String() 同一份名单——规范化文档、运营载荷与词表读口里的各封闭集都是那一个词；集合外与空串答 false；
// 「未声明」（人工复核零值、允许性 NOT_DECLARED）不是一格能写的取值，反查答 false。
func TestAcceptanceRulePackageClosedSetsAreNamedFromTheirStringForms(t *testing.T) {
	for _, category := range []domain.RuleCategory{domain.MinimumIngressIdentityRules, domain.ShipmentInvariantRules, domain.ProductAndContractDocumentRules, domain.RegulatorySourceDocumentRules, domain.CrossFieldConditionRules} {
		if named, known := domain.RuleCategoryNamed(category.String()); !known || named != category {
			t.Fatalf("%s: RuleCategoryNamed = %v, %v", category, named, known)
		}
	}
	for _, judgment := range []domain.JudgmentType{domain.NetworkReachabilityJudgment, domain.PreAcceptanceFinancialControlJudgment} {
		if named, known := domain.JudgmentTypeNamed(judgment.String()); !known || named != judgment {
			t.Fatalf("%s: JudgmentTypeNamed = %v, %v", judgment, named, known)
		}
	}
	for _, group := range []domain.AcceptanceCheckGroupType{domain.CustomerRelationshipCheckGroup, domain.NetworkReachabilityCheckGroup} {
		if named, known := domain.AcceptanceCheckGroupTypeNamed(group.String()); !known || named != group {
			t.Fatalf("%s: AcceptanceCheckGroupTypeNamed = %v, %v", group, named, known)
		}
	}
	for _, review := range []domain.ManualReviewDirective{domain.ManualReviewNotRequired, domain.ManualReviewRequired} {
		if named, known := domain.ManualReviewDirectiveNamed(review.String()); !known || named != review {
			t.Fatalf("%s: ManualReviewDirectiveNamed = %v, %v", review, named, known)
		}
	}
	for _, source := range []domain.DeclaredIntakeSource{domain.DeclaredNodeIntake, domain.DeclaredOffsitePickup} {
		if named, known := domain.DeclaredIntakeSourceNamed(source.String()); !known || named != source {
			t.Fatalf("%s: DeclaredIntakeSourceNamed = %v, %v", source, named, known)
		}
	}
	for _, outcome := range []domain.DeclaredResponsibilityOutcome{domain.DeclaredEffectiveDelivery, domain.DeclaredRegulatoryDisposition} {
		if named, known := domain.DeclaredResponsibilityOutcomeNamed(outcome.String()); !known || named != outcome {
			t.Fatalf("%s: DeclaredResponsibilityOutcomeNamed = %v, %v", outcome, named, known)
		}
	}
	if named, known := domain.ValidityAnchorKindNamed("CHANNEL_RESULT_OBSERVED"); !known || named != domain.ChannelResultObservedAnchor {
		t.Fatalf("ValidityAnchorKindNamed = %v, %v", named, known)
	}
	for _, stage := range []domain.DeclaredAmendmentStage{domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCaseClosedOrServiceCompleted} {
		if named, known := domain.DeclaredAmendmentStageNamed(stage.String()); !known || named != stage {
			t.Fatalf("%s: DeclaredAmendmentStageNamed = %v, %v", stage, named, known)
		}
	}
	for _, intent := range []domain.DeclaredAmendmentIntent{domain.DeclaredSupplementIntent, domain.DeclaredExplicitClearIntent} {
		if named, known := domain.DeclaredAmendmentIntentNamed(intent.String()); !known || named != intent {
			t.Fatalf("%s: DeclaredAmendmentIntentNamed = %v, %v", intent, named, known)
		}
	}
	for _, allowance := range []domain.AmendmentAllowance{domain.AmendmentAllowed, domain.AmendmentDisallowed} {
		if named, known := domain.AmendmentAllowanceNamed(allowance.String()); !known || named != allowance {
			t.Fatalf("%s: AmendmentAllowanceNamed = %v, %v", allowance, named, known)
		}
	}
	for _, refused := range []struct {
		name  string
		known bool
	}{
		{"", func() bool { _, known := domain.RuleCategoryNamed(""); return known }()},
		{"minimum_ingress_identity", func() bool { _, known := domain.RuleCategoryNamed("minimum_ingress_identity"); return known }()},
		{"UNDECLARED", func() bool { _, known := domain.ManualReviewDirectiveNamed("UNDECLARED"); return known }()},
		{"NOT_DECLARED", func() bool { _, known := domain.AmendmentAllowanceNamed("NOT_DECLARED"); return known }()},
		{"LABEL_ISSUED", func() bool { _, known := domain.ValidityAnchorKindNamed("LABEL_ISSUED"); return known }()},
	} {
		if refused.known {
			t.Fatalf("%q must not name a closed-set value", refused.name)
		}
	}
}
