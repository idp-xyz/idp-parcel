package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证授权规则册接进 PCC-1 的形（票 admin-write-faces/17「Go 侧只加本册规范化一格」）：正文只有取消授权目录，
// 文档按请求方序写出，摘要不随行序变；目录立不立得住由与发布时相同的那道门说（同一请求方第二行由构造门拒）。

func cancellationRow(t *testing.T, party domain.DeclaredCancellationParty, rule string) domain.CancellationAuthorityDeclaration {
	t.Helper()
	return domain.CancellationAuthorityDeclaration{Party: party, Rule: commercialValue(t, domain.NewRuleReference, rule)}
}

func authorizationRuleContent(rows ...domain.CancellationAuthorityDeclaration) domain.PublicationContent {
	return domain.PublicationContent{
		Kind:              domain.AuthorizationRuleObject,
		AuthorizationRule: &domain.AuthorizationRuleBody{CancellationAuthority: rows},
	}
}

func canonicalAuthorizationRule(t *testing.T, rows ...domain.CancellationAuthorityDeclaration) domain.CanonicalPublicationContent {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(authorizationRuleContent(rows...))
	if err != nil {
		t.Fatalf("canonicalize authorization rule: %v", err)
	}
	return canonical
}

// Covers: 票 17 完成判据「Go 侧只加本册规范化一格」——接进同一个 PCC-1（加册不换号，ADR-0126 Decision 一）；文档
// 是 {canonicalization, kind, authorizationRule.cancellationAuthority[{party, rule}]}，行按请求方枚举序；同一份目录换行序
// 摘要不变、换一条规则引用摘要就变。
func TestAuthorizationRuleCanonicalizesTheCancellationAuthorityInPartyOrder(t *testing.T) {
	customer := cancellationRow(t, domain.DeclaredCustomerCancellation, "CANCEL/customer-before-intake")
	operations := cancellationRow(t, domain.DeclaredOperationsCancellation, "CANCEL/operations-any-time")

	ordered := canonicalAuthorizationRule(t, customer, operations)
	reordered := canonicalAuthorizationRule(t, operations, customer)
	if ordered.Digest() != reordered.Digest() {
		t.Fatalf("row order changed the digest: %s vs %s", ordered.Digest(), reordered.Digest())
	}
	if ordered.Canonicalization() != "PCC-1" || !strings.HasPrefix(ordered.Digest().String(), "PCC-1:") {
		t.Fatalf("canonicalization = %q, digest = %s; want PCC-1", ordered.Canonicalization(), ordered.Digest())
	}
	if !domain.IsRegisterCanonicalized(domain.AuthorizationRuleObject) {
		t.Fatal("IsRegisterCanonicalized(AUTHORIZATION_RULE) must answer true once the register is wired")
	}

	var document struct {
		Canonicalization  string `json:"canonicalization"`
		Kind              string `json:"kind"`
		AuthorizationRule struct {
			CancellationAuthority []struct {
				Party string `json:"party"`
				Rule  string `json:"rule"`
			} `json:"cancellationAuthority"`
		} `json:"authorizationRule"`
	}
	if err := json.Unmarshal(reordered.Document(), &document); err != nil {
		t.Fatalf("document is not JSON: %v", err)
	}
	rows := document.AuthorizationRule.CancellationAuthority
	if document.Kind != "AUTHORIZATION_RULE" || len(rows) != 2 ||
		rows[0].Party != "CUSTOMER" || rows[0].Rule != "CANCEL/customer-before-intake" ||
		rows[1].Party != "OPERATIONS" || rows[1].Rule != "CANCEL/operations-any-time" {
		t.Fatalf("document = %s; want the two rows in party order", reordered.Document())
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(reordered.Document(), &keys); err != nil {
		t.Fatalf("document keys: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("document = %s; want exactly {canonicalization, kind, authorizationRule}", reordered.Document())
	}

	looser := canonicalAuthorizationRule(t, customer, cancellationRow(t, domain.DeclaredOperationsCancellation, "CANCEL/operations-looser"))
	if looser.Digest() == ordered.Digest() {
		t.Fatal("changing a rule reference must change the digest")
	}
}

// Covers: 票 17「同一请求方第二行由构造门拒」——规范化过的是与 NewCancellationAuthorityContent 同一道门：零行是缺件、
// 请求方集外或规则空是缺件、同一请求方两行是冲突；正文缺席答缺席；别册的正文冒本册的名、本册正文挂别册的 kind 都答
// kind 不符。四格恢复动作各不相同，不折成一个「算不出」。
func TestAuthorizationRuleCanonicalizationRefusesWhatTheCatalogueGateRefuses(t *testing.T) {
	customer := cancellationRow(t, domain.DeclaredCustomerCancellation, "CANCEL/customer")

	for name, tc := range map[string]struct {
		content domain.PublicationContent
		want    error
	}{
		"absent body": {domain.PublicationContent{Kind: domain.AuthorizationRuleObject}, domain.ErrPublicationContentAbsent},
		"no rows":     {authorizationRuleContent(), domain.ErrCancellationAuthorityNotConfigured},
		"invalid party": {authorizationRuleContent(domain.CancellationAuthorityDeclaration{
			Party: domain.DeclaredCancellationPartyInvalid, Rule: customer.Rule,
		}), domain.ErrCancellationAuthorityNotConfigured},
		"blank rule": {authorizationRuleContent(domain.CancellationAuthorityDeclaration{
			Party: domain.DeclaredOperationsCancellation,
		}), domain.ErrCancellationAuthorityNotConfigured},
		"same party twice": {authorizationRuleContent(customer,
			cancellationRow(t, domain.DeclaredCustomerCancellation, "CANCEL/customer-again")), domain.ErrConflictingCancellationAuthority},
		"body under another kind": {domain.PublicationContent{
			Kind:              domain.SettlementPolicyObject,
			AuthorizationRule: &domain.AuthorizationRuleBody{CancellationAuthority: []domain.CancellationAuthorityDeclaration{customer}},
		}, domain.ErrPublicationContentKindMismatch},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.CanonicalizePublicationContent(tc.content); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}

	credit := creditPolicyBody(t, "freight", creditAmount(t, 100), time.Time{})
	_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.AuthorizationRuleObject, CreditPolicy: &credit})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("credit body under authorization rule kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}
}

// Covers: 快照折回——存下来的文档折回本册的正文输入面，每行过构造门；伪造的请求方词与同一请求方两行都被拒。
// DeclaredCancellationPartyNamed 只认 String() 原词，大小写与空串都在集外。
func TestAuthorizationRuleSnapshotRehydratesThroughTheSameGate(t *testing.T) {
	canonical := canonicalAuthorizationRule(t,
		cancellationRow(t, domain.DeclaredOperationsCancellation, "CANCEL/operations"),
		cancellationRow(t, domain.DeclaredCustomerCancellation, "CANCEL/customer"))

	content, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
	if err != nil {
		t.Fatalf("rehydrate authorization rule content: %v", err)
	}
	if content.Kind != domain.AuthorizationRuleObject || content.AuthorizationRule == nil || len(content.AuthorizationRule.CancellationAuthority) != 2 {
		t.Fatalf("rehydrated content = %#v", content)
	}
	again, err := domain.CanonicalizePublicationContent(content)
	if err != nil || again.Digest() != canonical.Digest() {
		t.Fatalf("re-canonicalized digest = %s (%v), want %s", again.Digest(), err, canonical.Digest())
	}

	forgedParty := strings.Replace(string(canonical.Document()), `"party":"OPERATIONS"`, `"party":"ANYONE"`, 1)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(forgedParty)); !errors.Is(err, domain.ErrCancellationAuthorityNotConfigured) {
		t.Fatalf("forged party: err = %v, want ErrCancellationAuthorityNotConfigured", err)
	}
	duplicated := strings.Replace(string(canonical.Document()), `"party":"OPERATIONS"`, `"party":"CUSTOMER"`, 1)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(duplicated)); !errors.Is(err, domain.ErrConflictingCancellationAuthority) {
		t.Fatalf("duplicated party: err = %v, want ErrConflictingCancellationAuthority", err)
	}

	for name, want := range map[string]domain.DeclaredCancellationParty{
		"CUSTOMER":   domain.DeclaredCustomerCancellation,
		"OPERATIONS": domain.DeclaredOperationsCancellation,
	} {
		if got, known := domain.DeclaredCancellationPartyNamed(name); !known || got != want {
			t.Fatalf("DeclaredCancellationPartyNamed(%q) = %v, %v", name, got, known)
		}
	}
	for _, name := range []string{"", "customer", "ANYONE"} {
		if got, known := domain.DeclaredCancellationPartyNamed(name); known || got != domain.DeclaredCancellationPartyInvalid {
			t.Fatalf("DeclaredCancellationPartyNamed(%q) = %v, %v; want invalid, false", name, got, known)
		}
	}
}

// Covers: 五步路径的领域半边对本册成立——预览算出的摘要就是录入载体记下的摘要；同壳同目录重发是重放，换行序也是重放。
func TestAuthorizationRuleDraftsCarryTheComputedDigest(t *testing.T) {
	submitter := commercialValue(t, domain.NewOperatorSubjectReference, "op-1")
	at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	customer := cancellationRow(t, domain.DeclaredCustomerCancellation, "CANCEL/customer")
	operations := cancellationRow(t, domain.DeclaredOperationsCancellation, "CANCEL/operations")
	shell := draftShell(t, domain.AuthorizationRuleObject, "authz-1", "v1")

	preview, err := domain.PreviewPublication(shell, authorizationRuleContent(customer, operations))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	draft, err := domain.SubmitPublicationDraft(shell, authorizationRuleContent(operations, customer), submitter, at)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if draft.Canonical().Digest() != preview.Digest() || draft.PublicationSpec().ContentDigest != preview.Digest() {
		t.Fatalf("draft digest %s / spec %s, preview %s", draft.Canonical().Digest(), draft.PublicationSpec().ContentDigest, preview.Digest())
	}
	again, err := domain.SubmitPublicationDraft(shell, authorizationRuleContent(customer, operations), submitter, at)
	if err != nil {
		t.Fatalf("submit again: %v", err)
	}
	if !draft.SameContentAs(again) || !draft.SameSubmissionAs(again) {
		t.Fatal("the same catalogue under the same shell must be the same submission regardless of row order")
	}
}
