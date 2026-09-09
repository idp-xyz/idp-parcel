package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// serviceRuleBody 造一份挂在服务产品上的客户服务规则正文；两张子表由调用方给，各自可空。
func serviceRuleBody(t *testing.T, product string, deadlines []domain.ClaimDeadlineRule, materials []domain.MinimumMaterialsRule) domain.CustomerServiceRuleBody {
	t.Helper()
	return domain.CustomerServiceRuleBody{
		Applicability: domain.CustomerServiceRuleAppliesToServiceProduct(commercialValue(t, domain.NewCommercialObjectID, product)),
		Responsible:   commercialValue(t, domain.NewPartyID, "operator-1"),
		Scope:         commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		Deadlines:     deadlines,
		Materials:     materials,
	}
}

func canonicalServiceRule(t *testing.T, body domain.CustomerServiceRuleBody) domain.CanonicalPublicationContent {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:                domain.CustomerServiceRuleObject,
		CustomerServiceRule: &body,
	})
	if err != nil {
		t.Fatalf("canonicalize customer service rule: %v", err)
	}
	return canonical
}

// Covers: ADR-0126 Decision 一（加册不换号）— 客户服务规则接进 PCC-1：同一正文两次算逐字节同串；期限表按种类归一、材料表按索赔
// 类型归一、清单内按引用串归一，表单里换任何一处的行序都不换摘要；每一格都是正文的一部分——换天数、换日历、换适用对象、
// 加一行材料，各自都是另一个串；两张表任一为空仍是一版正文（另一张有行即可）。
func TestServiceRuleDigestIsStableAndOrderedByItsKeys(t *testing.T) {
	first := claimDeadline(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")
	review := claimDeadline(t, domain.ConclusionReviewDeadline, "event-conclusion-notified", 15, "calendar-cn")
	loss := minimumMaterials(t, "claim-loss", "material-photo", "material-invoice")
	damage := minimumMaterials(t, "claim-damage", "material-photo")

	base := canonicalServiceRule(t, serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first, review}, []domain.MinimumMaterialsRule{loss, damage}))
	again := canonicalServiceRule(t, serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first, review}, []domain.MinimumMaterialsRule{loss, damage}))
	reordered := canonicalServiceRule(t, serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{review, first}, []domain.MinimumMaterialsRule{damage, loss}))
	materialsSwapped := canonicalServiceRule(t, serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first, review},
		[]domain.MinimumMaterialsRule{minimumMaterials(t, "claim-loss", "material-invoice", "material-photo"), damage}))
	if base.Digest() != again.Digest() || base.Digest() != reordered.Digest() || base.Digest() != materialsSwapped.Digest() {
		t.Fatalf("same rule body produced different digests: %s / %s / %s / %s", base.Digest(), again.Digest(), reordered.Digest(), materialsSwapped.Digest())
	}
	if base.Canonicalization() != "PCC-1" || !strings.HasPrefix(base.Digest().String(), "PCC-1:") {
		t.Fatalf("customer service rule must be canonicalized under PCC-1, got %s", base.Digest())
	}

	longer := claimDeadline(t, domain.FirstClaimDeadline, "event-delivered", 45, "calendar-cn")
	if canonicalServiceRule(t, serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{longer, review}, []domain.MinimumMaterialsRule{loss, damage})).Digest() == base.Digest() {
		t.Fatal("changing a deadline's days must change the digest")
	}
	otherCalendar := claimDeadline(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-hk")
	if canonicalServiceRule(t, serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{otherCalendar, review}, []domain.MinimumMaterialsRule{loss, damage})).Digest() == base.Digest() {
		t.Fatal("changing a deadline's calendar must change the digest")
	}
	if canonicalServiceRule(t, serviceRuleBody(t, "product-2", []domain.ClaimDeadlineRule{first, review}, []domain.MinimumMaterialsRule{loss, damage})).Digest() == base.Digest() {
		t.Fatal("hanging the rule on another product must change the digest")
	}
	onContract := serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first, review}, []domain.MinimumMaterialsRule{loss, damage})
	onContract.Applicability = domain.CustomerServiceRuleAppliesToCustomerContract(commercialValue(t, domain.NewCommercialObjectID, "product-1"))
	if canonicalServiceRule(t, onContract).Digest() == base.Digest() {
		t.Fatal("the same identifier as a contract instead of a product must change the digest")
	}
	if canonicalServiceRule(t, serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first, review}, []domain.MinimumMaterialsRule{loss})).Digest() == base.Digest() {
		t.Fatal("dropping a materials row must change the digest")
	}

	deadlinesOnly := canonicalServiceRule(t, serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first}, nil))
	materialsOnly := canonicalServiceRule(t, serviceRuleBody(t, "product-1", nil, []domain.MinimumMaterialsRule{loss}))
	if deadlinesOnly.Digest() == materialsOnly.Digest() {
		t.Fatal("a deadlines-only body and a materials-only body must not share a digest")
	}
	if !domain.IsRegisterCanonicalized(domain.CustomerServiceRuleObject) {
		t.Fatal("IsRegisterCanonicalized must answer true for CUSTOMER_SERVICE_RULE once this register is wired")
	}
}

// Covers: 票 admin-write-faces/18「册与载荷」— 文档节键 customerServiceRule 下是 cmd/parcel-commercial 批文 customerServiceRuleBody
// 的键：适用对象 serviceProduct / customerContract 恰一在场（缺席省略）、responsible、scope，claimDeadlines 行内四格
// kind / startEvent / days / calendar，minimumMaterials 行内 claimKind / materials；期限按种类序、材料按索赔类型序写出，空表写 `[]`。
func TestServiceRuleDocumentMirrorsTheBatchKeys(t *testing.T) {
	canonical := canonicalServiceRule(t, serviceRuleBody(t, "product-1",
		[]domain.ClaimDeadlineRule{
			claimDeadline(t, domain.ConclusionReviewDeadline, "event-conclusion-notified", 15, "calendar-cn"),
			claimDeadline(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn"),
		},
		[]domain.MinimumMaterialsRule{
			minimumMaterials(t, "claim-loss", "material-photo", "material-invoice"),
			minimumMaterials(t, "claim-damage", "material-photo"),
		}))

	var document struct {
		Canonicalization string `json:"canonicalization"`
		Kind             string `json:"kind"`
		Rule             struct {
			ServiceProduct   string                       `json:"serviceProduct"`
			CustomerContract *string                      `json:"customerContract"`
			Responsible      string                       `json:"responsible"`
			Scope            string                       `json:"scope"`
			ClaimDeadlines   []map[string]json.RawMessage `json:"claimDeadlines"`
			MinimumMaterials []map[string]json.RawMessage `json:"minimumMaterials"`
		} `json:"customerServiceRule"`
	}
	if err := json.Unmarshal(canonical.Document(), &document); err != nil {
		t.Fatalf("decode document %s: %v", canonical.Document(), err)
	}
	if document.Canonicalization != "PCC-1" || document.Kind != "CUSTOMER_SERVICE_RULE" {
		t.Fatalf("document header = %q / %q", document.Canonicalization, document.Kind)
	}
	rule := document.Rule
	if rule.ServiceProduct != "product-1" || rule.CustomerContract != nil || rule.Responsible != "operator-1" || rule.Scope != "scope-a" {
		t.Fatalf("parent row = %#v", rule)
	}
	if strings.Contains(string(canonical.Document()), `"customerContract"`) {
		t.Fatalf("the absent applicability key must be omitted, not written empty: %s", canonical.Document())
	}
	if len(rule.ClaimDeadlines) != 2 || len(rule.MinimumMaterials) != 2 {
		t.Fatalf("tables = %d deadlines / %d materials", len(rule.ClaimDeadlines), len(rule.MinimumMaterials))
	}
	first, second := rule.ClaimDeadlines[0], rule.ClaimDeadlines[1]
	if string(first["kind"]) != `"FIRST_CLAIM"` || string(first["startEvent"]) != `"event-delivered"` || string(first["days"]) != "30" || string(first["calendar"]) != `"calendar-cn"` {
		t.Fatalf("first deadline = %s", first)
	}
	if string(second["kind"]) != `"CONCLUSION_REVIEW"` || string(second["days"]) != "15" {
		t.Fatalf("second deadline = %s", second)
	}
	for _, row := range rule.ClaimDeadlines {
		if len(row) != 4 {
			t.Fatalf("a deadline row must carry exactly its four keys: %s", row)
		}
	}
	damage, loss := rule.MinimumMaterials[0], rule.MinimumMaterials[1]
	if string(damage["claimKind"]) != `"claim-damage"` || string(damage["materials"]) != `["material-photo"]` {
		t.Fatalf("first materials row = %s", damage)
	}
	if string(loss["claimKind"]) != `"claim-loss"` || string(loss["materials"]) != `["material-invoice","material-photo"]` {
		t.Fatalf("second materials row = %s", loss)
	}
	for _, row := range rule.MinimumMaterials {
		if len(row) != 2 {
			t.Fatalf("a materials row must carry exactly its two keys: %s", row)
		}
	}
	// durationDays 是读面 HTTP 行体的键；文档镜像的是批文（days），两套词不得在这里混。
	for _, key := range []string{"creditPolicy", "supplierAgreement", "preAcceptanceFinancialControlPolicy", "durationDays"} {
		if strings.Contains(string(canonical.Document()), `"`+key+`"`) {
			t.Fatalf("a foreign key leaked into the document: %s", canonical.Document())
		}
	}

	materialsOnly := canonicalServiceRule(t, serviceRuleBody(t, "product-1", nil, []domain.MinimumMaterialsRule{minimumMaterials(t, "claim-loss", "material-photo")}))
	if !strings.Contains(string(materialsOnly.Document()), `"claimDeadlines":[]`) {
		t.Fatalf("an empty deadlines table must be written as [] not omitted: %s", materialsOnly.Document())
	}
}

// Covers: ADR-0104 Decision 三与 CONTEXT「必须按服务产品和客户合同明确适用范围、有效期间及责任方」— 折成文档前过的是与发布时
// 同一套门：两项都空拒（ErrInvalidCustomerServiceRuleVersion）、适用对象零值拒、责任方 / 范围零值拒、同一种期限两行与同一索赔类型
// 两行拒（ErrDuplicateCustomerServiceRuleItem）、零值行拒（ErrInvalidClaimDeadlineRule / ErrInvalidMinimumMaterialsRule）；正文缺席与
// 冒别册的名照旧分开两格。
func TestServiceRuleCanonicalizationRefusesWhatPublicationWouldRefuse(t *testing.T) {
	_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CustomerServiceRuleObject})
	if !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("no body: err = %v, want ErrPublicationContentAbsent", err)
	}
	first := claimDeadline(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")
	valid := serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first}, nil)
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CreditPolicyObject, CustomerServiceRule: &valid})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("rule body under credit kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}

	loss := minimumMaterials(t, "claim-loss", "material-photo")
	noApplicability := serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first}, nil)
	noApplicability.Applicability = domain.CustomerServiceRuleApplicability{}
	noResponsible := serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first}, nil)
	noResponsible.Responsible = domain.PartyID{}
	noScope := serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first}, nil)
	noScope.Scope = domain.CommercialScopeReference{}
	refusals := map[string]struct {
		body domain.CustomerServiceRuleBody
		want error
	}{
		"两项都空": {
			body: serviceRuleBody(t, "product-1", nil, nil),
			want: domain.ErrInvalidCustomerServiceRuleVersion,
		},
		"适用对象零值": {body: noApplicability, want: domain.ErrInvalidCustomerServiceRuleVersion},
		"责任方零值":  {body: noResponsible, want: domain.ErrInvalidCustomerServiceRuleVersion},
		"范围零值":   {body: noScope, want: domain.ErrInvalidCustomerServiceRuleVersion},
		"同一种期限两行": {
			body: serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first, claimDeadline(t, domain.FirstClaimDeadline, "event-signed", 7, "calendar-cn")}, nil),
			want: domain.ErrDuplicateCustomerServiceRuleItem,
		},
		"同一索赔类型两行": {
			body: serviceRuleBody(t, "product-1", nil, []domain.MinimumMaterialsRule{loss, minimumMaterials(t, "claim-loss", "material-invoice")}),
			want: domain.ErrDuplicateCustomerServiceRuleItem,
		},
		"零值期限行": {
			body: serviceRuleBody(t, "product-1", []domain.ClaimDeadlineRule{first, {}}, nil),
			want: domain.ErrInvalidClaimDeadlineRule,
		},
		"零值材料行": {
			body: serviceRuleBody(t, "product-1", nil, []domain.MinimumMaterialsRule{loss, {}}),
			want: domain.ErrInvalidMinimumMaterialsRule,
		},
	}
	for name, refusal := range refusals {
		t.Run(name, func(t *testing.T) {
			body := refusal.body
			_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
				Kind: domain.CustomerServiceRuleObject, CustomerServiceRule: &body,
			})
			if !errors.Is(err, refusal.want) {
				t.Fatalf("err = %v, want %v", err, refusal.want)
			}
		})
	}
}

// Covers: ADR-0126 Decision 三（正文快照）— 规则的规范化文档折得回正文：适用对象两键折回两格封闭、父行两格与两张表逐行回到领域
// 值对象，折回去再算一遍与列里的摘要相等；快照里坏一格（同一种期限两行、集外期限种类、两个适用键并存、两项都空）折不回——
// 快照是数据，正文立不立得住仍由构造门说。
func TestServiceRuleDocumentRehydratesToTheSameBody(t *testing.T) {
	body := serviceRuleBody(t, "product-1",
		[]domain.ClaimDeadlineRule{
			claimDeadline(t, domain.ConclusionReviewDeadline, "event-conclusion-notified", 15, "calendar-cn"),
			claimDeadline(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn"),
		},
		[]domain.MinimumMaterialsRule{minimumMaterials(t, "claim-loss", "material-photo", "material-invoice")})
	canonical := canonicalServiceRule(t, body)

	content, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if content.Kind != domain.CustomerServiceRuleObject || content.CustomerServiceRule == nil ||
		content.CreditPolicy != nil || content.PreAcceptanceFinancialControlPolicy != nil {
		t.Fatalf("rehydrated content = %#v", content)
	}
	rehydrated := content.CustomerServiceRule
	if product, applies := rehydrated.Applicability.ServiceProduct(); !applies || product.String() != "product-1" {
		t.Fatalf("rehydrated applicability = %#v", rehydrated.Applicability)
	}
	if _, applies := rehydrated.Applicability.CustomerContract(); applies {
		t.Fatal("a rule hung on a product must not rehydrate as hung on a contract as well")
	}
	if rehydrated.Responsible.String() != "operator-1" || rehydrated.Scope.String() != "scope-a" {
		t.Fatalf("rehydrated parent row = %s / %s", rehydrated.Responsible, rehydrated.Scope)
	}
	// 文档按种类序写出，折回来也按种类序：第一行是首次索赔期限。
	if len(rehydrated.Deadlines) != 2 || rehydrated.Deadlines[0].Kind() != domain.FirstClaimDeadline || rehydrated.Deadlines[0].DurationDays() != 30 ||
		rehydrated.Deadlines[0].StartEvent().String() != "event-delivered" || rehydrated.Deadlines[0].Calendar().String() != "calendar-cn" ||
		rehydrated.Deadlines[1].Kind() != domain.ConclusionReviewDeadline || rehydrated.Deadlines[1].DurationDays() != 15 {
		t.Fatalf("rehydrated deadlines = %#v", rehydrated.Deadlines)
	}
	if len(rehydrated.Materials) != 1 || rehydrated.Materials[0].ClaimKind().String() != "claim-loss" || len(rehydrated.Materials[0].Materials()) != 2 {
		t.Fatalf("rehydrated materials = %#v", rehydrated.Materials)
	}
	recomputed, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("recanonicalize: %v", err)
	}
	if recomputed.Digest() != canonical.Digest() {
		t.Fatalf("rehydrated body digests to %s, column says %s", recomputed.Digest(), canonical.Digest())
	}

	duplicateKind := strings.Replace(string(canonical.Document()), `"kind":"CONCLUSION_REVIEW"`, `"kind":"FIRST_CLAIM"`, 1)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(duplicateKind)); !errors.Is(err, domain.ErrDuplicateCustomerServiceRuleItem) {
		t.Fatalf("duplicate deadline kind snapshot: err = %v, want ErrDuplicateCustomerServiceRuleItem", err)
	}
	unknownKind := strings.Replace(string(canonical.Document()), `"kind":"CONCLUSION_REVIEW"`, `"kind":"APPEAL"`, 1)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(unknownKind)); !errors.Is(err, domain.ErrInvalidClaimDeadlineRule) {
		t.Fatalf("APPEAL snapshot: err = %v, want ErrInvalidClaimDeadlineRule", err)
	}
	bothKeys := strings.Replace(string(canonical.Document()), `"serviceProduct":"product-1"`, `"serviceProduct":"product-1","customerContract":"contract-1"`, 1)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(bothKeys)); !errors.Is(err, domain.ErrInvalidCustomerServiceRuleVersion) {
		t.Fatalf("both applicability keys snapshot: err = %v, want ErrInvalidCustomerServiceRuleVersion", err)
	}
	emptied := strings.Replace(strings.Replace(string(canonical.Document()),
		`"claimDeadlines":[{"kind":"FIRST_CLAIM","startEvent":"event-delivered","days":30,"calendar":"calendar-cn"},{"kind":"CONCLUSION_REVIEW","startEvent":"event-conclusion-notified","days":15,"calendar":"calendar-cn"}]`,
		`"claimDeadlines":[]`, 1),
		`"minimumMaterials":[{"claimKind":"claim-loss","materials":["material-invoice","material-photo"]}]`, `"minimumMaterials":[]`, 1)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(emptied)); !errors.Is(err, domain.ErrInvalidCustomerServiceRuleVersion) {
		t.Fatalf("emptied snapshot: err = %v, want ErrInvalidCustomerServiceRuleVersion (document was %s)", err, emptied)
	}
}

// Covers: 期限种类的反查与 String() 同一份名单——规范化文档、运营载荷、词表读口说的都是那一个词；集合外与空串答 false，
// 三种期限彼此独立（visibility-exception CONTEXT），打错的词不折进任何一格。
func TestClaimDeadlineKindRoundTripsThroughItsName(t *testing.T) {
	for _, kind := range []domain.ClaimDeadlineKind{domain.FirstClaimDeadline, domain.MaterialSupplementDeadline, domain.ConclusionReviewDeadline} {
		if named, known := domain.ClaimDeadlineKindNamed(kind.String()); !known || named != kind {
			t.Fatalf("%s: Named = %v, %v", kind, named, known)
		}
	}
	for _, name := range []string{"", "first_claim", "APPEAL", "FIRST CLAIM"} {
		if _, known := domain.ClaimDeadlineKindNamed(name); known {
			t.Fatalf("%q must not name a claim deadline kind", name)
		}
	}
}
