package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func controlPolicyBody(items ...domain.PreAcceptanceControlItem) domain.PreAcceptanceFinancialControlPolicyBody {
	return domain.PreAcceptanceFinancialControlPolicyBody{JointPass: domain.AllControlsPass, Items: items}
}

func canonicalControlPolicy(t *testing.T, body domain.PreAcceptanceFinancialControlPolicyBody) domain.CanonicalPublicationContent {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:                                domain.PreAcceptanceFinancialControlPolicyObject,
		PreAcceptanceFinancialControlPolicy: &body,
	})
	if err != nil {
		t.Fatalf("canonicalize control policy: %v", err)
	}
	return canonical
}

// Covers: ADR-0126 Decision 一（加册不换号）— 接受前财务控制策略接进 PCC-1：同一正文两次算逐字节同串；控制项表按判断
// 顺序归一，表单里换行序不换摘要；每一格都是正文的一部分——换失败处置、换责任、换范围、加一行，各自都是另一个串。
func TestControlPolicyDigestIsStableAndOrderedByEvaluationOrder(t *testing.T) {
	freeze := controlItem(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure)
	check := controlItem(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.AuthorizedDispositionOnControlFailure)

	first := canonicalControlPolicy(t, controlPolicyBody(freeze, check))
	again := canonicalControlPolicy(t, controlPolicyBody(freeze, check))
	reordered := canonicalControlPolicy(t, controlPolicyBody(check, freeze))
	if first.Digest() != again.Digest() || first.Digest() != reordered.Digest() {
		t.Fatalf("same policy body produced different digests: %s / %s / %s", first.Digest(), again.Digest(), reordered.Digest())
	}
	if first.Canonicalization() != "PCC-1" || !strings.HasPrefix(first.Digest().String(), "PCC-1:") {
		t.Fatalf("control policy must be canonicalized under PCC-1, got %s", first.Digest())
	}

	rejectingCheck := controlItem(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.RejectOnControlFailure)
	if canonicalControlPolicy(t, controlPolicyBody(freeze, rejectingCheck)).Digest() == first.Digest() {
		t.Fatal("changing a failure disposition must change the digest")
	}
	otherScope := controlItem(t, domain.CreditCheckControl, "charge-scope-b", 2, domain.AuthorizedDispositionOnControlFailure)
	if canonicalControlPolicy(t, controlPolicyBody(freeze, otherScope)).Digest() == first.Digest() {
		t.Fatal("moving a control to another charge scope must change the digest")
	}
	if canonicalControlPolicy(t, controlPolicyBody(freeze)).Digest() == first.Digest() {
		t.Fatal("a single-control policy must not share a digest with a two-control one")
	}
	if !domain.IsRegisterCanonicalized(domain.PreAcceptanceFinancialControlPolicyObject) {
		t.Fatal("IsRegisterCanonicalized must answer true for PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY once this register is wired")
	}
}

// Covers: 票 admin-write-faces/13「canonical 文档键名镜像受控批文」— 文档节键 preAcceptanceFinancialControlPolicy 下是
// cmd/parcel-commercial 批文 preAcceptanceFinancialControlPolicyBody 的两键 jointPassCondition / controls，行内五格
// control / chargeScope / order / onFailure / responsibility 同名；行按 order 升序写出。
func TestControlPolicyDocumentMirrorsTheBatchKeys(t *testing.T) {
	canonical := canonicalControlPolicy(t, controlPolicyBody(
		controlItem(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.AuthorizedDispositionOnControlFailure),
		controlItem(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure),
	))

	var document struct {
		Canonicalization string `json:"canonicalization"`
		Kind             string `json:"kind"`
		Policy           struct {
			JointPassCondition string                       `json:"jointPassCondition"`
			Controls           []map[string]json.RawMessage `json:"controls"`
		} `json:"preAcceptanceFinancialControlPolicy"`
	}
	if err := json.Unmarshal(canonical.Document(), &document); err != nil {
		t.Fatalf("decode document %s: %v", canonical.Document(), err)
	}
	if document.Canonicalization != "PCC-1" || document.Kind != "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY" {
		t.Fatalf("document header = %q / %q", document.Canonicalization, document.Kind)
	}
	if document.Policy.JointPassCondition != "ALL_CONTROLS_PASS" || len(document.Policy.Controls) != 2 {
		t.Fatalf("policy section = %#v", document.Policy)
	}
	first, second := document.Policy.Controls[0], document.Policy.Controls[1]
	if string(first["order"]) != "1" || string(first["control"]) != `"PREPAID_FREEZE"` || string(first["onFailure"]) != `"REJECT"` ||
		string(first["chargeScope"]) != `"charge-scope-a"` || string(first["responsibility"]) != `"responsibility-charge-scope-a"` {
		t.Fatalf("first control = %s", first)
	}
	if string(second["order"]) != "2" || string(second["control"]) != `"CREDIT_CHECK"` || string(second["onFailure"]) != `"AUTHORIZED_DISPOSITION"` {
		t.Fatalf("second control = %s", second)
	}
	for _, row := range document.Policy.Controls {
		if len(row) != 5 {
			t.Fatalf("a control row must carry exactly its five keys: %s", row)
		}
	}
	for _, key := range []string{"creditPolicy", "supplierAgreement", "customerContract"} {
		if strings.Contains(string(canonical.Document()), `"`+key+`"`) {
			t.Fatalf("another register's key leaked into the document: %s", canonical.Document())
		}
	}
}

// Covers: 票 admin-write-faces/13「顺序唯一、范围 × 种类唯一、至少一项，都由构造门答」— 折成文档前过的是与发布时同一套门
// （filedPreAcceptanceControlItems）：零项拒、共同通过条件缺席拒（ErrInvalidPreAcceptanceFinancialControlPolicy）、两行抢一个
// 顺序拒、同一范围同一种控制两行拒（ErrDuplicatePreAcceptanceControlItem）、零值行拒（ErrInvalidPreAcceptanceControlItem）；
// 正文缺席与冒别册的名照旧分开两格。
func TestControlPolicyCanonicalizationRefusesWhatPublicationWouldRefuse(t *testing.T) {
	_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.PreAcceptanceFinancialControlPolicyObject})
	if !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("no body: err = %v, want ErrPublicationContentAbsent", err)
	}
	body := controlPolicyBody(controlItem(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure))
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CreditPolicyObject, PreAcceptanceFinancialControlPolicy: &body})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("policy body under credit kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}

	freeze := controlItem(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure)
	refusals := map[string]struct {
		body domain.PreAcceptanceFinancialControlPolicyBody
		want error
	}{
		"零项": {
			body: controlPolicyBody(),
			want: domain.ErrInvalidPreAcceptanceFinancialControlPolicy,
		},
		"共同通过条件缺席": {
			body: domain.PreAcceptanceFinancialControlPolicyBody{Items: []domain.PreAcceptanceControlItem{freeze}},
			want: domain.ErrInvalidPreAcceptanceFinancialControlPolicy,
		},
		"两行抢一个顺序": {
			body: controlPolicyBody(freeze, controlItem(t, domain.CreditCheckControl, "charge-scope-a", 1, domain.RejectOnControlFailure)),
			want: domain.ErrDuplicatePreAcceptanceControlItem,
		},
		"同一范围同一种控制两行": {
			body: controlPolicyBody(freeze, controlItem(t, domain.PrepaidFreezeControl, "charge-scope-a", 2, domain.AuthorizedDispositionOnControlFailure)),
			want: domain.ErrDuplicatePreAcceptanceControlItem,
		},
		"零值行": {
			body: controlPolicyBody(freeze, domain.PreAcceptanceControlItem{}),
			want: domain.ErrInvalidPreAcceptanceControlItem,
		},
	}
	for name, refusal := range refusals {
		t.Run(name, func(t *testing.T) {
			body := refusal.body
			_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
				Kind: domain.PreAcceptanceFinancialControlPolicyObject, PreAcceptanceFinancialControlPolicy: &body,
			})
			if !errors.Is(err, refusal.want) {
				t.Fatalf("err = %v, want %v", err, refusal.want)
			}
		})
	}
}

// Covers: ADR-0126 Decision 三（正文快照）— 策略的规范化文档折得回正文：共同通过条件与每一行五格回到领域值对象，折回去
// 再算一遍与列里的摘要相等；快照里坏一格（两行抢一个顺序、集外控制种类）折不回——快照是数据，正文立不立得住仍由构造门说。
func TestControlPolicyDocumentRehydratesToTheSameBody(t *testing.T) {
	body := controlPolicyBody(
		controlItem(t, domain.CreditCheckControl, "charge-scope-b", 2, domain.AuthorizedDispositionOnControlFailure),
		controlItem(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure),
	)
	canonical := canonicalControlPolicy(t, body)

	content, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if content.Kind != domain.PreAcceptanceFinancialControlPolicyObject || content.PreAcceptanceFinancialControlPolicy == nil ||
		content.CreditPolicy != nil || content.CustomerContract != nil {
		t.Fatalf("rehydrated content = %#v", content)
	}
	rehydrated := content.PreAcceptanceFinancialControlPolicy
	if rehydrated.JointPass != domain.AllControlsPass || len(rehydrated.Items) != 2 {
		t.Fatalf("rehydrated body = %#v", rehydrated)
	}
	// 文档按顺序写出，折回来也按顺序：第一行是顺序 1 的预付冻结。
	if first := rehydrated.Items[0]; first.EvaluationOrder() != 1 || first.Kind() != domain.PrepaidFreezeControl ||
		first.Scope().String() != "charge-scope-a" || first.FailureDisposition() != domain.RejectOnControlFailure ||
		first.Responsibility().String() != "responsibility-charge-scope-a" {
		t.Fatalf("first item = %#v", first)
	}
	if second := rehydrated.Items[1]; second.EvaluationOrder() != 2 || second.Kind() != domain.CreditCheckControl ||
		second.Scope().String() != "charge-scope-b" || second.FailureDisposition() != domain.AuthorizedDispositionOnControlFailure {
		t.Fatalf("second item = %#v", second)
	}
	recomputed, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("recanonicalize: %v", err)
	}
	if recomputed.Digest() != canonical.Digest() {
		t.Fatalf("rehydrated body digests to %s, column says %s", recomputed.Digest(), canonical.Digest())
	}

	duplicateOrder := strings.Replace(string(canonical.Document()), `"order":2`, `"order":1`, 1)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(duplicateOrder)); !errors.Is(err, domain.ErrDuplicatePreAcceptanceControlItem) {
		t.Fatalf("duplicate order snapshot: err = %v, want ErrDuplicatePreAcceptanceControlItem", err)
	}
	noControl := strings.Replace(string(canonical.Document()), `"control":"CREDIT_CHECK"`, `"control":"NO_CONTROL"`, 1)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(noControl)); !errors.Is(err, domain.ErrInvalidPreAcceptanceControlItem) {
		t.Fatalf("NO_CONTROL snapshot: err = %v, want ErrInvalidPreAcceptanceControlItem", err)
	}
}

// Covers: 三个封闭集的反查与 String() 同一份名单——规范化文档、运营载荷、词表读口说的都是那一个词；集合外与空串答 false。
// 「无控制」不在控制种类里不是漏（ADR-0115 Decision 一）；共同通过条件缺席不折成 ALL_CONTROLS_PASS（Decision 三）。
func TestControlPolicyClosedSetsRoundTripThroughTheirNames(t *testing.T) {
	for _, kind := range []domain.PreAcceptanceControlKind{domain.PrepaidFreezeControl, domain.CreditCheckControl} {
		if named, known := domain.PreAcceptanceControlKindNamed(kind.String()); !known || named != kind {
			t.Fatalf("%s: Named = %v, %v", kind, named, known)
		}
	}
	for _, name := range []string{"", "NO_CONTROL", "prepaid_freeze", "CREDIT"} {
		if _, known := domain.PreAcceptanceControlKindNamed(name); known {
			t.Fatalf("%q must not name a control kind", name)
		}
	}
	for _, disposition := range []domain.ControlFailureDisposition{domain.RejectOnControlFailure, domain.AuthorizedDispositionOnControlFailure} {
		if named, known := domain.ControlFailureDispositionNamed(disposition.String()); !known || named != disposition {
			t.Fatalf("%s: Named = %v, %v", disposition, named, known)
		}
	}
	for _, name := range []string{"", "ALLOW", "reject"} {
		if _, known := domain.ControlFailureDispositionNamed(name); known {
			t.Fatalf("%q must not name a failure disposition", name)
		}
	}
	if named, known := domain.JointPassConditionNamed(domain.AllControlsPass.String()); !known || named != domain.AllControlsPass {
		t.Fatalf("ALL_CONTROLS_PASS: Named = %v, %v", named, known)
	}
	for _, name := range []string{"", "ANY_CONTROL_PASSES", "all_controls_pass"} {
		if _, known := domain.JointPassConditionNamed(name); known {
			t.Fatalf("%q must not name a joint pass condition", name)
		}
	}
}
