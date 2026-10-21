package postgres_test

import (
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件对真实 PostgreSQL 16 证授权裁定折进所挂版本的收尾（ADR-0261）：授权规则版本经收尾口同起点退役或截止之后，
// 授予册的装载不再交出已退役的那一版、截止点之后不再交出旧版；客户合同版本收尾之后，挂在旧版上的委派同样离开新的
// 裁定；未收尾时两版并存答适用冲突。收尾经生产写口（CloseCommercialVersionHandler 与发布登记册）落库，不手写行。

type authorityClosureFixture struct {
	cutFixture
	grants      *adapter.AuthorityGrants
	delegations *adapter.ContractDelegations
}

func newAuthorityClosureFixture(t *testing.T) authorityClosureFixture {
	t.Helper()
	base := newCutFixture(t)
	grants, err := adapter.NewAuthorityGrants(base.db)
	if err != nil {
		t.Fatalf("构造授权治理册：%v", err)
	}
	delegations, err := adapter.NewContractDelegations(base.db)
	if err != nil {
		t.Fatalf("构造合同委派读口：%v", err)
	}
	return authorityClosureFixture{cutFixture: base, grants: grants, delegations: delegations}
}

// saveRuleWithGrant 登一版授权规则壳，再在它上面登一条主动拒绝授予，授予区间取壳的区间。
func (fixture authorityClosureFixture) saveRuleWithGrant(t *testing.T, label string, startsAt time.Time) domain.CommercialVersion {
	t.Helper()
	version := cutVersion(t, domain.AuthorizationRuleObject, "auth-reject", label, startsAt)
	fixture.save(t, version)
	fixture.saveGrantOn(t, version, domain.ActiveRejectionAction)
	return version
}

func (fixture authorityClosureFixture) saveGrantOn(t *testing.T, version domain.CommercialVersion, action domain.AuthorizedAction) {
	t.Helper()
	grant, err := domain.NewAuthorityGrant(version, action,
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewAuthorityLevel, "level-commercial"),
		pcScope(t),
		version.Effective())
	if err != nil {
		t.Fatalf("授权授予：%v", err)
	}
	mustSaveGrant(t, fixture.transactor, t.Context(), fixture.grants, grant)
}

func (fixture authorityClosureFixture) adjudicate(t *testing.T, request domain.AuthorizationRequest) (domain.Authorization, error) {
	t.Helper()
	handler := application.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(fixture.grants, fixture.delegations)
	return handler.Handle(t.Context(), pcTenant(t, "tenant-1"), request)
}

func (fixture authorityClosureFixture) loadedGrantLabels(t *testing.T, at time.Time) []string {
	t.Helper()
	loaded, err := fixture.grants.LoadEffectiveGrants(t.Context(), pcTenant(t, "tenant-1"), pcScope(t), at)
	if err != nil {
		t.Fatalf("装载授予：%v", err)
	}
	labels := make([]string, 0, len(loaded))
	for _, grant := range loaded {
		labels = append(labels, grant.Version().Version().String())
	}
	return labels
}

// Covers: `AT-PC-042` 真库那一半与 `AT-PC-059` 真库那一半——同起点重发 v2 而未收尾，裁定答适用冲突、不取首个；经收尾口
// 退役 v1 之后，装载只交出 v2，裁定指名 v2；退役不追溯已作出的那一次裁定（它指名的那一版仍是它当时采用的）。
func TestARetiredAuthorizationRuleVersionLeavesTheGrantBook(t *testing.T) {
	fixture := newAuthorityClosureFixture(t)
	old := fixture.saveRuleWithGrant(t, "v1", cutRowPredecessorStart)

	earlier, err := fixture.adjudicate(t, grantRejectionRequest(t, pcScope(t).String(), cutRowAfter))
	if err != nil || earlier.GrantVersion().Version().String() != "v1" {
		t.Fatalf("只有 v1 时 adjudicate = %v / %v, want 已授权指名 v1", earlier.GrantVersion().Version(), err)
	}

	fixture.saveRuleWithGrant(t, "v2", cutRowPredecessorStart)
	if _, err := fixture.adjudicate(t, grantRejectionRequest(t, pcScope(t).String(), cutRowAfter)); !errors.Is(err, domain.ErrAuthorityApplicabilityConflict) {
		t.Fatalf("同起点重发未收尾 error = %v, want ErrAuthorityApplicabilityConflict", err)
	}

	result := fixture.retire(t, retireCommand(t, old, "v2", "SYN-REISSUE-AUTH-1"))
	if result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("retire outcome = %q (cause %v), want REGISTERED", result.Outcome(), result.Cause())
	}

	for _, at := range []time.Time{cutRowBefore, cutRowAfter} {
		if got := fixture.loadedGrantLabels(t, at); len(got) != 1 || got[0] != "v2" {
			t.Fatalf("at %v 装载 = %v, want 只有 v2——已退役那一版的授予不交出", at, got)
		}
		authorized, err := fixture.adjudicate(t, grantRejectionRequest(t, pcScope(t).String(), at))
		if err != nil {
			t.Fatalf("at %v adjudicate: %v", at, err)
		}
		if got := authorized.GrantVersion().Version().String(); got != "v2" {
			t.Fatalf("at %v 采用 %q, want v2", at, got)
		}
	}
	if earlier.GrantVersion().Version().String() != "v1" {
		t.Fatal("退役改写了已作出的那一次裁定所采用的版本")
	}
}

// Covers: `AT-PC-060` 真库那一半——授权规则旧版经收尾口截止在后继的生效时点：截止点之后只剩旧版授予时这个范围没有现行
// 规则；后继的授予登上之后，截止点之前命中旧版、之后只命中后继。
func TestACutAuthorizationRuleVersionNarrowsItsGrant(t *testing.T) {
	fixture := newAuthorityClosureFixture(t)
	old := fixture.saveRuleWithGrant(t, "v1", cutRowPredecessorStart)
	successor := cutVersion(t, domain.AuthorizationRuleObject, "auth-reject", "v2", cutRowSuccessorStart)
	fixture.save(t, successor)

	result := fixture.cut(t, cutCommand(t, old, "v2", "SYN-CUT-AUTH-1"))
	if result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("cut outcome = %q (cause %v), want REGISTERED", result.Outcome(), result.Cause())
	}
	if got := fixture.loadedGrantLabels(t, cutRowAfter); len(got) != 0 {
		t.Fatalf("截止点之后装载 = %v, want 无——截止之后的旧版授予不交出", got)
	}
	if _, err := fixture.adjudicate(t, grantRejectionRequest(t, pcScope(t).String(), cutRowAfter)); !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v, want ErrAuthorityRulesNotConfigured——截止之后的旧版授予不算现行规则", err)
	}

	fixture.saveGrantOn(t, successor, domain.ActiveRejectionAction)
	for at, want := range map[time.Time]string{cutRowBefore: "v1", cutRowSuccessorStart: "v2", cutRowAfter: "v2"} {
		authorized, err := fixture.adjudicate(t, grantRejectionRequest(t, pcScope(t).String(), at))
		if err != nil {
			t.Fatalf("at %v adjudicate: %v", at, err)
		}
		if got := authorized.GrantVersion().Version().String(); got != want {
			t.Fatalf("at %v 采用 %q, want %q", at, got, want)
		}
	}
}

// Covers: `AT-PC-061` 真库那一半——客户合同版本经收尾口同起点退役或截止之后，挂在已退役那一版、或截止点之后的旧版上的
// 委派不再解出实际决定方（委派缺席，Is 不允许）；后继合同版本声明了同一委派即由它解出。
func TestADelegationLeavesWithTheClosureOfItsContractVersion(t *testing.T) {
	saveAmendmentGrant := func(t *testing.T, fixture authorityClosureFixture) {
		t.Helper()
		rule := cutVersion(t, domain.AuthorizationRuleObject, "auth-amend", "v1", cutRowPredecessorStart)
		fixture.save(t, rule)
		fixture.saveGrantOn(t, rule, domain.SourceDataAmendmentAction)
	}
	declare := func(t *testing.T, fixture authorityClosureFixture, contract domain.CommercialVersion, delegator domain.Delegator) {
		t.Helper()
		mustSaveDelegations(t, fixture.transactor, t.Context(), fixture.publications, delegationsOn(t, contract,
			delegationRow(t, delegator, "level-commercial", pcScope(t).String(), contract.Effective())))
	}
	amend := func(at time.Time) domain.AuthorizationRequest {
		return operatorAmendmentRequestRow(t, "operator-1", "account-1", pcScope(t).String(), at)
	}

	t.Run("same-start retirement", func(t *testing.T) {
		fixture := newAuthorityClosureFixture(t)
		saveAmendmentGrant(t, fixture)
		fixture.save(t, cutVersion(t, domain.AcceptanceRulePackageObject, "rules-1", "v1", cutRowPredecessorStart))
		old := cutVersion(t, domain.CustomerContractObject, "contract-1", "v1", cutRowPredecessorStart)
		next := cutVersion(t, domain.CustomerContractObject, "contract-1", "v2", cutRowPredecessorStart)
		fixture.save(t, old)
		fixture.save(t, next)
		declare(t, fixture, old, accountDelegatorRow(t, "account-1"))

		if result := fixture.retire(t, retireCommand(t, old, "v2", "SYN-REISSUE-CONTRACT-1")); result.Outcome() != application.VersionClosureRegistered {
			t.Fatalf("retire outcome = %q (cause %v)", result.Outcome(), result.Cause())
		}
		if _, err := fixture.adjudicate(t, amend(cutRowAfter)); !errors.Is(err, domain.ErrDelegationAbsent) {
			t.Fatalf("error = %v, want ErrDelegationAbsent——已退役合同版本上的委派不再解出决定方", err)
		}

		declare(t, fixture, next, legalEntityDelegatorRow(t, "legal-1"))
		authorized, err := fixture.adjudicate(t, amend(cutRowAfter))
		if err != nil {
			t.Fatalf("adjudicate: %v", err)
		}
		if decider, _ := authorized.Decider(); decider.Kind() != domain.LegalEntityDecider || decider.Reference() != "legal-1" {
			t.Fatalf("decider = %s %q, want 后继合同版本声明的 LEGAL_ENTITY legal-1", decider.Kind(), decider.Reference())
		}
	})

	t.Run("cut at the successor", func(t *testing.T) {
		fixture := newAuthorityClosureFixture(t)
		saveAmendmentGrant(t, fixture)
		fixture.save(t, cutVersion(t, domain.AcceptanceRulePackageObject, "rules-1", "v1", cutRowPredecessorStart))
		old := cutVersion(t, domain.CustomerContractObject, "contract-1", "v1", cutRowPredecessorStart)
		fixture.save(t, old)
		fixture.save(t, cutVersion(t, domain.CustomerContractObject, "contract-1", "v2", cutRowSuccessorStart))
		declare(t, fixture, old, accountDelegatorRow(t, "account-1"))

		if result := fixture.cut(t, cutCommand(t, old, "v2", "SYN-CUT-CONTRACT-1")); result.Outcome() != application.VersionClosureRegistered {
			t.Fatalf("cut outcome = %q (cause %v)", result.Outcome(), result.Cause())
		}
		if _, err := fixture.adjudicate(t, amend(cutRowBefore)); err != nil {
			t.Fatalf("截止点之前：%v", err)
		}
		if _, err := fixture.adjudicate(t, amend(cutRowAfter)); !errors.Is(err, domain.ErrDelegationAbsent) {
			t.Fatalf("error = %v, want ErrDelegationAbsent——截止点之后的旧版委派不再解出决定方", err)
		}
	})
}

// Covers: ADR-0056「选用区间取登记顺序上的最后一条」延伸到授予与正文——同一版先截晚（截在 v3 的起点）、再截早（截在 v2
// 的起点），授予与供应商协议正文都按后一条判：两者之间的时刻旧版已不现行。按前一条判，授予会与 v2 并存成适用冲突，
// 正文会照旧支持采购。
func TestTheLatestOfTwoCutsGovernsTheGrantAndTheBody(t *testing.T) {
	fixture := newAuthorityClosureFixture(t)

	old := fixture.saveRuleWithGrant(t, "v1", cutRowPredecessorStart)
	fixture.saveRuleWithGrant(t, "v2", cutRowSuccessorStart)
	fixture.saveRuleWithGrant(t, "v3", cutRowLaterStart)
	if result := fixture.cut(t, cutCommand(t, old, "v3", "SYN-CUT-AUTH-LATE")); result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("cut at v3 outcome = %q (cause %v)", result.Outcome(), result.Cause())
	}
	if result := fixture.cut(t, cutCommand(t, old, "v2", "SYN-CUT-AUTH-EARLY")); result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("cut at v2 outcome = %q (cause %v)", result.Outcome(), result.Cause())
	}
	if got := fixture.correctionRows(t); got != 2 {
		t.Fatalf("更正册 %d 行, want 2", got)
	}
	if got := fixture.loadedGrantLabels(t, cutRowAfter); len(got) != 1 || got[0] != "v2" {
		t.Fatalf("两次截止之间的时刻装载 = %v, want 只有 v2——按后一条（较早的截止点）判", got)
	}
	authorized, err := fixture.adjudicate(t, grantRejectionRequest(t, pcScope(t).String(), cutRowAfter))
	if err != nil || authorized.GrantVersion().Version().String() != "v2" {
		t.Fatalf("adjudicate = %v / %v, want 已授权指名 v2", authorized.GrantVersion().Version(), err)
	}

	contents, err := adapter.NewSupplierAgreementContents(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	a1 := cutVersion(t, domain.SupplierAgreementObject, "agreement-1", "v1", cutRowPredecessorStart)
	fixture.saveAgreement(t, a1)
	fixture.saveAgreement(t, cutVersion(t, domain.SupplierAgreementObject, "agreement-1", "v2", cutRowSuccessorStart))
	fixture.saveAgreement(t, cutVersion(t, domain.SupplierAgreementObject, "agreement-1", "v3", cutRowLaterStart))
	if result := fixture.cut(t, cutCommand(t, a1, "v3", "SYN-CUT-AGREEMENT-LATE")); result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("cut agreement at v3 outcome = %q (cause %v)", result.Outcome(), result.Cause())
	}
	if result := fixture.cut(t, cutCommand(t, a1, "v2", "SYN-CUT-AGREEMENT-EARLY")); result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("cut agreement at v2 outcome = %q (cause %v)", result.Outcome(), result.Cause())
	}
	body, found, err := contents.LoadSupplierAgreement(t.Context(), a1.Tenant(), a1)
	if err != nil || !found {
		t.Fatalf("正文点读 v1：found=%v err=%v", found, err)
	}
	if body.SupportsProcurementAt(cutRowAfter) || !body.SupportsProcurementAt(cutRowBefore) {
		t.Fatal("正文没有按后一条（较早的截止点）判")
	}
}
