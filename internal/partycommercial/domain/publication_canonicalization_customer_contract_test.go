package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func requiredControl() *domain.PreAcceptanceControlBody {
	return &domain.PreAcceptanceControlBody{Requirement: domain.PreAcceptanceControlRequired}
}

func notApplicableControl(t *testing.T) *domain.PreAcceptanceControlBody {
	t.Helper()
	return &domain.PreAcceptanceControlBody{
		Requirement: domain.PreAcceptanceControlNotApplicable,
		Basis:       notApplicableBasis(t),
	}
}

func customerContractBody(t *testing.T, control *domain.PreAcceptanceControlBody, bindings ...domain.FinancialControlBinding) domain.CustomerContractBody {
	t.Helper()
	return domain.CustomerContractBody{
		RulePackage: commercialValue(t, domain.NewCommercialObjectID, "rules-1"),
		Bindings:    bindings,
		Control:     control,
	}
}

func canonicalCustomerContract(t *testing.T, body domain.CustomerContractBody) domain.CanonicalPublicationContent {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:             domain.CustomerContractObject,
		CustomerContract: &body,
	})
	if err != nil {
		t.Fatalf("canonicalize customer contract: %v", err)
	}
	return canonical
}

// Covers: ADR-0126 Decision 一（加册不换号）— 客户合同接进 PCC-1：同一正文两次算逐字节同串；约定表按费用范围
// 归一，换行序不换摘要；两层各是正文的一部分——合同级声明在不在、约定是指名策略还是显式不适用，各自都是另一个串。
func TestCustomerContractDigestIsStableAndCoversBothLayers(t *testing.T) {
	prepaid := appliedControl(t, "charge-prepaid")
	cod := inapplicableControl(t, "charge-cod", "COD-NA-01")

	first := canonicalCustomerContract(t, customerContractBody(t, requiredControl(), prepaid, cod))
	again := canonicalCustomerContract(t, customerContractBody(t, requiredControl(), prepaid, cod))
	reordered := canonicalCustomerContract(t, customerContractBody(t, requiredControl(), cod, prepaid))
	if first.Digest() != again.Digest() || first.Digest() != reordered.Digest() {
		t.Fatalf("same contract body produced different digests: %s / %s / %s", first.Digest(), again.Digest(), reordered.Digest())
	}
	if first.Canonicalization() != "PCC-1" || !strings.HasPrefix(first.Digest().String(), "PCC-1:") {
		t.Fatalf("customer contract must be canonicalized under PCC-1, got %s", first.Digest())
	}

	withoutControl := canonicalCustomerContract(t, customerContractBody(t, nil, prepaid, cod))
	if withoutControl.Digest() == first.Digest() {
		t.Fatal("dropping the contract-level control declaration must change the digest")
	}
	notApplicable := canonicalCustomerContract(t, customerContractBody(t, notApplicableControl(t), prepaid, cod))
	if notApplicable.Digest() == first.Digest() {
		t.Fatal("REQUIRED and NOT_APPLICABLE declarations must not share a digest")
	}
	codApplied := canonicalCustomerContract(t, customerContractBody(t, requiredControl(), prepaid, appliedControl(t, "charge-cod")))
	if codApplied.Digest() == first.Digest() {
		t.Fatal("an applied binding and an inapplicable one on the same scope must not share a digest")
	}
	noBindings := canonicalCustomerContract(t, customerContractBody(t, requiredControl()))
	if noBindings.Digest() == first.Digest() {
		t.Fatal("a contract with no bindings must not share a digest with one that binds two scopes")
	}
	if !domain.IsRegisterCanonicalized(domain.CustomerContractObject) {
		t.Fatal("IsRegisterCanonicalized must answer true for CUSTOMER_CONTRACT once this register is wired")
	}
}

// Covers: 票 admin-write-faces/10「canonical 文档键名镜像受控批文」— 文档里两层的键与
// cmd/parcel-commercial 批文 declarations 下 contractContent / preAcceptanceControl 同名；约定行只带在场的那一格；
// 声明缺席时整键缺席。
func TestCustomerContractDocumentMirrorsTheBatchKeys(t *testing.T) {
	canonical := canonicalCustomerContract(t, customerContractBody(t, notApplicableControl(t),
		inapplicableControl(t, "charge-cod", "COD-NA-01"), appliedControl(t, "charge-prepaid")))

	var document struct {
		Canonicalization string `json:"canonicalization"`
		Kind             string `json:"kind"`
		CustomerContract struct {
			ContractContent struct {
				RulePackage string                       `json:"rulePackage"`
				Bindings    []map[string]json.RawMessage `json:"bindings"`
			} `json:"contractContent"`
			PreAcceptanceControl map[string]string `json:"preAcceptanceControl"`
		} `json:"customerContract"`
	}
	if err := json.Unmarshal(canonical.Document(), &document); err != nil {
		t.Fatalf("decode document %s: %v", canonical.Document(), err)
	}
	if document.Canonicalization != "PCC-1" || document.Kind != "CUSTOMER_CONTRACT" {
		t.Fatalf("document header = %q / %q", document.Canonicalization, document.Kind)
	}
	content := document.CustomerContract.ContractContent
	if content.RulePackage != "rules-1" || len(content.Bindings) != 2 {
		t.Fatalf("contractContent = %#v", content)
	}
	// 排序后 charge-cod 在前：显式不适用那一行只带 inapplicabilityBasis，指名策略那一行只带 policy。
	if string(content.Bindings[0]["chargeScope"]) != `"charge-cod"` || string(content.Bindings[0]["inapplicabilityBasis"]) != `"COD-NA-01"` {
		t.Fatalf("first binding = %s", content.Bindings[0])
	}
	if _, present := content.Bindings[0]["policy"]; present {
		t.Fatalf("an inapplicable binding must not carry a policy key: %s", content.Bindings[0])
	}
	if string(content.Bindings[1]["chargeScope"]) != `"charge-prepaid"` || string(content.Bindings[1]["policy"]) != `"policy-charge-prepaid"` {
		t.Fatalf("second binding = %s", content.Bindings[1])
	}
	if _, present := content.Bindings[1]["inapplicabilityBasis"]; present {
		t.Fatalf("an applied binding must not carry an inapplicabilityBasis key: %s", content.Bindings[1])
	}
	control := document.CustomerContract.PreAcceptanceControl
	if control["requirement"] != "NOT_APPLICABLE" || control["notApplicableBasis"] != notApplicableBasis(t).String() {
		t.Fatalf("preAcceptanceControl = %#v", control)
	}

	required := canonicalCustomerContract(t, customerContractBody(t, requiredControl()))
	if strings.Contains(string(required.Document()), "notApplicableBasis") || strings.Contains(string(required.Document()), `"bindings"`) {
		t.Fatalf("REQUIRED without bindings must omit notApplicableBasis and bindings: %s", required.Document())
	}
	undeclared := canonicalCustomerContract(t, customerContractBody(t, nil, appliedControl(t, "charge-prepaid")))
	if strings.Contains(string(undeclared.Document()), "preAcceptanceControl") {
		t.Fatalf("an undeclared control must omit the whole key: %s", undeclared.Document())
	}
}

// Covers: 票 admin-write-faces/10「恰一与『不适用必带依据』由服务端裁」— 折成文档前过的是与发布时同一套门：
// 规则包缺席拒（ErrInvalidCustomerContract）、同一范围约定两次拒（ErrConflictingFinancialControlBinding）、
// 零值约定拒（ErrInvalidFinancialControlBinding）、`不适用`无依据与`要求控制`带依据都拒
// （ErrPreAcceptanceControlNotDeclared）；正文缺席与冒别册的名照旧三格分开。
func TestCustomerContractCanonicalizationRefusesWhatPublicationWouldRefuse(t *testing.T) {
	_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CustomerContractObject})
	if !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("no body: err = %v, want ErrPublicationContentAbsent", err)
	}
	body := customerContractBody(t, requiredControl(), appliedControl(t, "charge-prepaid"))
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CreditPolicyObject, CustomerContract: &body})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("contract body under credit kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}

	refusals := map[string]struct {
		body domain.CustomerContractBody
		want error
	}{
		"规则包缺席": {
			body: domain.CustomerContractBody{Bindings: []domain.FinancialControlBinding{appliedControl(t, "charge-prepaid")}},
			want: domain.ErrInvalidCustomerContract,
		},
		"同一范围约定两次": {
			body: customerContractBody(t, requiredControl(), appliedControl(t, "charge-cod"), inapplicableControl(t, "charge-cod", "COD-NA-01")),
			want: domain.ErrConflictingFinancialControlBinding,
		},
		"零值约定": {
			body: customerContractBody(t, requiredControl(), domain.FinancialControlBinding{}),
			want: domain.ErrInvalidFinancialControlBinding,
		},
		"不适用无依据": {
			body: customerContractBody(t, &domain.PreAcceptanceControlBody{Requirement: domain.PreAcceptanceControlNotApplicable}),
			want: domain.ErrPreAcceptanceControlNotDeclared,
		},
		"要求控制却带依据": {
			body: customerContractBody(t, &domain.PreAcceptanceControlBody{Requirement: domain.PreAcceptanceControlRequired, Basis: notApplicableBasis(t)}),
			want: domain.ErrPreAcceptanceControlNotDeclared,
		},
		"声明在场却未声明要求": {
			body: customerContractBody(t, &domain.PreAcceptanceControlBody{}),
			want: domain.ErrPreAcceptanceControlNotDeclared,
		},
	}
	for name, refusal := range refusals {
		t.Run(name, func(t *testing.T) {
			body := refusal.body
			_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CustomerContractObject, CustomerContract: &body})
			if !errors.Is(err, refusal.want) {
				t.Fatalf("err = %v, want %v", err, refusal.want)
			}
		})
	}
}

// Covers: ADR-0126 Decision 三（正文快照）— 客户合同的规范化文档折得回正文：规则包、按范围的约定（含各自是指名
// 还是不适用）与合同级声明逐格回到领域值对象，折回去再算一遍与列里的摘要相等；声明缺席折回 nil 不折回零值。
func TestCustomerContractDocumentRehydratesToTheSameBody(t *testing.T) {
	body := customerContractBody(t, notApplicableControl(t),
		appliedControl(t, "charge-prepaid"), inapplicableControl(t, "charge-cod", "COD-NA-01"))
	canonical := canonicalCustomerContract(t, body)

	content, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if content.Kind != domain.CustomerContractObject || content.CustomerContract == nil || content.CreditPolicy != nil {
		t.Fatalf("rehydrated content = %#v", content)
	}
	rehydrated := content.CustomerContract
	if rehydrated.RulePackage != body.RulePackage {
		t.Fatalf("rule package = %s, want %s", rehydrated.RulePackage, body.RulePackage)
	}
	if len(rehydrated.Bindings) != 2 {
		t.Fatalf("bindings = %#v", rehydrated.Bindings)
	}
	byScope := map[string]domain.FinancialControlBinding{}
	for _, binding := range rehydrated.Bindings {
		byScope[binding.Scope().String()] = binding
	}
	if policy, applies := byScope["charge-prepaid"].Policy(); !applies || policy.String() != "policy-charge-prepaid" {
		t.Fatalf("charge-prepaid binding = %#v", byScope["charge-prepaid"])
	}
	if cod := byScope["charge-cod"]; !cod.ExplicitlyInapplicable() || cod.InapplicabilityBasis().String() != "COD-NA-01" {
		t.Fatalf("charge-cod binding = %#v", cod)
	}
	if rehydrated.Control == nil || rehydrated.Control.Requirement != domain.PreAcceptanceControlNotApplicable ||
		rehydrated.Control.Basis != notApplicableBasis(t) {
		t.Fatalf("control = %#v", rehydrated.Control)
	}
	recomputed, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("recanonicalize: %v", err)
	}
	if recomputed.Digest() != canonical.Digest() {
		t.Fatalf("rehydrated body digests to %s, column says %s", recomputed.Digest(), canonical.Digest())
	}

	undeclared := canonicalCustomerContract(t, customerContractBody(t, nil, appliedControl(t, "charge-prepaid")))
	content, err = domain.RehydratePublicationContent(undeclared.Canonicalization(), undeclared.Document())
	if err != nil {
		t.Fatalf("rehydrate undeclared control: %v", err)
	}
	if content.CustomerContract == nil || content.CustomerContract.Control != nil {
		t.Fatalf("an absent declaration must rehydrate to nil, got %#v", content.CustomerContract)
	}

	// 快照里坏一格（同一范围两行）折不回：快照是数据，正文立不立得住仍由构造门说。
	corrupted := strings.Replace(string(canonical.Document()), `"chargeScope":"charge-prepaid"`, `"chargeScope":"charge-cod"`, 1)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(corrupted)); !errors.Is(err, domain.ErrConflictingFinancialControlBinding) {
		t.Fatalf("corrupted snapshot: err = %v, want ErrConflictingFinancialControlBinding", err)
	}
}

// Covers: 反查与 String() 同一份名单——规范化文档与运营载荷里的控制要求都是那一个词；集合外与空串答 false，
// 不把「未声明」读成一格取值。
func TestPreAcceptanceControlRequirementNamedRoundTrips(t *testing.T) {
	for _, requirement := range []domain.PreAcceptanceControlRequirement{domain.PreAcceptanceControlRequired, domain.PreAcceptanceControlNotApplicable} {
		named, known := domain.PreAcceptanceControlRequirementNamed(requirement.String())
		if !known || named != requirement {
			t.Fatalf("%s: Named = %v, %v", requirement, named, known)
		}
	}
	for _, name := range []string{"", "UNDECLARED", "required", "NO_CONTROL"} {
		if _, known := domain.PreAcceptanceControlRequirementNamed(name); known {
			t.Fatalf("%q must not name a requirement", name)
		}
	}
}
