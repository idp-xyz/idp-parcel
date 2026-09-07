package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证合同委派册（票 party-commercial-context-gaps/08，ADR-0116，0025 迁移）：
// 委派随合同版本往返、缺声明是合法缺席、同内容重放、异内容冲突且原行不动、按范围时点只装载有效
// 的委派、租户是身份不是过滤器、库上 CHECK 守住动作与委派方种类两个封闭集，以及裁定编排经本册
// 为运营角色代录解出实际决定方。
//
// 夹具里的账户、法人、等级取值只是取值，不作断言依据——委派属实例半边，仓库不持有任何一份。

type delegationFixture struct {
	repository  *adapter.CommercialPublications
	delegations *adapter.ContractDelegations
	grants      *adapter.AuthorityGrants
	transactor  bentoapp.Transactor
}

func newDelegationFixture(t *testing.T) delegationFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	delegations, err := adapter.NewContractDelegations(db)
	if err != nil {
		t.Fatalf("构造合同委派读口：%v", err)
	}
	grants, err := adapter.NewAuthorityGrants(db)
	if err != nil {
		t.Fatalf("构造授权治理册：%v", err)
	}
	return delegationFixture{
		repository:  repository,
		delegations: delegations,
		grants:      grants,
		transactor:  db.Transactor(),
	}
}

func accountDelegatorRow(t *testing.T, account string) domain.Delegator {
	t.Helper()
	delegator, err := domain.DelegatedByCustomerAccount(pcValue(t, domain.NewCustomerAccountID, account))
	if err != nil {
		t.Fatalf("customer account delegator: %v", err)
	}
	return delegator
}

func legalEntityDelegatorRow(t *testing.T, entity string) domain.Delegator {
	t.Helper()
	delegator, err := domain.DelegatedByLegalEntity(pcValue(t, domain.NewLegalEntityReference, entity))
	if err != nil {
		t.Fatalf("legal entity delegator: %v", err)
	}
	return delegator
}

func delegationRow(t *testing.T, delegator domain.Delegator, level, scope string, effective domain.EffectiveInterval) domain.ContractDelegationDeclaration {
	t.Helper()
	return domain.ContractDelegationDeclaration{
		Delegator: delegator,
		Action:    domain.SourceDataAmendmentAction,
		Scope:     pcValue(t, domain.NewCommercialScopeReference, scope),
		Level:     pcValue(t, domain.NewAuthorityLevel, level),
		Effective: effective,
	}
}

func delegationInterval(t *testing.T) domain.EffectiveInterval {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(60*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	return interval
}

func delegationsOn(t *testing.T, contract domain.CommercialVersion, rows ...domain.ContractDelegationDeclaration) domain.ContractDelegationContent {
	t.Helper()
	content, err := domain.NewContractDelegationContent(contract, rows)
	if err != nil {
		t.Fatalf("new contract delegation content: %v", err)
	}
	return content
}

func saveDelegations(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	content domain.ContractDelegationContent,
) ports.DeclarationSaveOutcome {
	t.Helper()
	var outcome ports.DeclarationSaveOutcome
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.SaveContractDelegations(txCtx, content)
		return err
	})
	return outcome
}

func mustSaveDelegations(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	content domain.ContractDelegationContent,
) {
	t.Helper()
	if outcome := saveDelegations(t, transactor, ctx, repository, content); outcome != ports.DeclarationSaved {
		t.Fatalf("save outcome = %q, want SAVED", outcome)
	}
}

// Covers: ADR-0116 Decision 二——委派五维随合同版本逐行往返（委派方种类与引用、动作、范围、等级、区间），
// 读回经领域构造门重建、挂回同一合同版本、按稳定顺序交回。
func TestContractDelegationsRoundTripWithTheirContractVersion(t *testing.T) {
	fixture := newDelegationFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	contract := effectiveContract(t, "contract-1", "v1", "digest-c1")
	mustSaveVersion(t, fixture.transactor, ctx, fixture.repository, contract)
	mustSaveDelegations(t, fixture.transactor, ctx, fixture.repository, delegationsOn(t, contract,
		delegationRow(t, legalEntityDelegatorRow(t, "legal-1"), "level-clerk", "scope-1", delegationInterval(t)),
		delegationRow(t, accountDelegatorRow(t, "account-1"), "level-commercial", "scope-1", delegationInterval(t)),
	))

	content, found, err := fixture.delegations.LoadContractDelegations(ctx, tenant, contract)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	if !content.Contract().SameVersionAs(contract) {
		t.Fatal("委派挂回了另一个合同版本")
	}
	delegations := content.Delegations()
	if len(delegations) != 2 {
		t.Fatalf("委派 %d 条, want 2", len(delegations))
	}
	first, second := delegations[0], delegations[1]
	if first.Level().String() != "level-clerk" || first.Delegator().Kind() != domain.LegalEntityDelegator ||
		first.Delegator().Reference() != "legal-1" || first.Action() != domain.SourceDataAmendmentAction ||
		first.Scope().String() != "scope-1" || !first.Effective().StartsAt().Equal(effectiveAtRow) {
		t.Fatalf("第一条变形：%#v", first)
	}
	if ends, bounded := first.Effective().EndsAt(); !bounded || !ends.Equal(effectiveAtRow.Add(60*24*time.Hour)) {
		t.Fatalf("第一条区间终点变形：%v %v", ends, bounded)
	}
	if second.Level().String() != "level-commercial" || second.Delegator().Kind() != domain.CustomerAccountDelegator ||
		second.Delegator().Reference() != "account-1" || !second.Contract().SameVersionAs(contract) {
		t.Fatalf("第二条变形：%#v", second)
	}
}

// Covers: 缺声明是合法缺席（found=false），不是 error，也不是任何默认委派——Authorize 据以答 ErrDelegationAbsent。
func TestAContractVersionWithoutDelegationsIsNotFound(t *testing.T) {
	fixture := newDelegationFixture(t)
	ctx := t.Context()

	contract := effectiveContract(t, "contract-bare", "v1", "digest-bare")
	mustSaveVersion(t, fixture.transactor, ctx, fixture.repository, contract)

	content, found, err := fixture.delegations.LoadContractDelegations(ctx, pcTenant(t, "tenant-1"), contract)
	if err != nil {
		t.Fatalf("没声明被当成了错误：%v", err)
	}
	if found || len(content.Delegations()) != 0 {
		t.Fatalf("没登记委派却读回了一份：%#v", content)
	}
}

// Covers: ADR-0031——同内容重放答`已登记`；换委派方、多一条少一条都是`内容冲突`；两者都不是 error，且原声明
// 一行不动。冲突路径一行不写，否则「绝不覆盖」只对父行成立。
func TestSavingDelegationsTwiceIsAReplayAndAChangeConflicts(t *testing.T) {
	fixture := newDelegationFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	contract := effectiveContract(t, "contract-1", "v1", "digest-c1")
	mustSaveVersion(t, fixture.transactor, ctx, fixture.repository, contract)
	original := delegationsOn(t, contract,
		delegationRow(t, accountDelegatorRow(t, "account-1"), "level-commercial", "scope-1", delegationInterval(t)),
	)
	mustSaveDelegations(t, fixture.transactor, ctx, fixture.repository, original)

	if outcome := saveDelegations(t, fixture.transactor, ctx, fixture.repository, original); outcome != ports.DeclarationAlreadyRegistered {
		t.Fatalf("replay outcome = %q, want ALREADY_REGISTERED", outcome)
	}

	changedDelegator := delegationsOn(t, contract,
		delegationRow(t, legalEntityDelegatorRow(t, "legal-1"), "level-commercial", "scope-1", delegationInterval(t)),
	)
	if outcome := saveDelegations(t, fixture.transactor, ctx, fixture.repository, changedDelegator); outcome != ports.DeclarationContentConflict {
		t.Fatalf("changed delegator outcome = %q, want CONTENT_CONFLICT", outcome)
	}

	extraRow := delegationsOn(t, contract,
		delegationRow(t, accountDelegatorRow(t, "account-1"), "level-commercial", "scope-1", delegationInterval(t)),
		delegationRow(t, accountDelegatorRow(t, "account-1"), "level-clerk", "scope-1", delegationInterval(t)),
	)
	if outcome := saveDelegations(t, fixture.transactor, ctx, fixture.repository, extraRow); outcome != ports.DeclarationContentConflict {
		t.Fatalf("extra row outcome = %q, want CONTENT_CONFLICT", outcome)
	}

	content, found, err := fixture.delegations.LoadContractDelegations(ctx, tenant, contract)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	delegations := content.Delegations()
	if len(delegations) != 1 || delegations[0].Delegator().Reference() != "account-1" || delegations[0].Level().String() != "level-commercial" {
		t.Fatalf("冲突路径改动了原声明：%#v", delegations)
	}
}

// Covers: EffectiveContractDelegationView——按（租户 + 范围 + 时点）只装载当时有效的委派：区间外、别的范围、
// 别的租户都读不到；同范围多份合同的委派一次交回，各带自己的合同版本。
func TestEffectiveDelegationsAreLoadedByScopeAndTime(t *testing.T) {
	fixture := newDelegationFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")
	scope := pcValue(t, domain.NewCommercialScopeReference, "scope-1")

	first := effectiveContract(t, "contract-1", "v1", "digest-c1")
	second := effectiveContract(t, "contract-2", "v1", "digest-c2")
	mustSaveVersion(t, fixture.transactor, ctx, fixture.repository, first)
	mustSaveVersion(t, fixture.transactor, ctx, fixture.repository, second)

	shortLived, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("短区间：%v", err)
	}
	mustSaveDelegations(t, fixture.transactor, ctx, fixture.repository, delegationsOn(t, first,
		delegationRow(t, accountDelegatorRow(t, "account-1"), "level-commercial", "scope-1", delegationInterval(t)),
		delegationRow(t, accountDelegatorRow(t, "account-1"), "level-clerk", "scope-1", shortLived),
		delegationRow(t, accountDelegatorRow(t, "account-1"), "level-commercial", "scope-other", delegationInterval(t)),
	))
	mustSaveDelegations(t, fixture.transactor, ctx, fixture.repository, delegationsOn(t, second,
		delegationRow(t, legalEntityDelegatorRow(t, "legal-1"), "level-commercial", "scope-1", delegationInterval(t)),
	))

	at := effectiveAtRow.Add(10 * 24 * time.Hour)
	loaded, err := fixture.delegations.LoadEffectiveDelegations(ctx, tenant, scope, at)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d delegations, want 2（短区间已过、别的范围不算）：%#v", len(loaded), loaded)
	}
	if !loaded[0].Contract().SameVersionAs(first) || loaded[0].Delegator().Reference() != "account-1" ||
		!loaded[1].Contract().SameVersionAs(second) || loaded[1].Delegator().Reference() != "legal-1" {
		t.Fatalf("装载顺序或归属变形：%#v", loaded)
	}

	early := effectiveAtRow.Add(time.Hour)
	if loaded, err := fixture.delegations.LoadEffectiveDelegations(ctx, tenant, scope, early); err != nil || len(loaded) != 3 {
		t.Fatalf("短区间仍有效的时点：len=%d err=%v, want 3", len(loaded), err)
	}
	if loaded, err := fixture.delegations.LoadEffectiveDelegations(ctx, pcTenant(t, "tenant-b"), scope, at); err != nil || len(loaded) != 0 {
		t.Fatalf("跨租户：len=%d err=%v, want 0", len(loaded), err)
	}
	if loaded, err := fixture.delegations.LoadEffectiveDelegations(ctx, tenant, scope, effectiveAtRow.Add(400*24*time.Hour)); err != nil || len(loaded) != 0 {
		t.Fatalf("全部过期：len=%d err=%v, want 0", len(loaded), err)
	}
}

// Covers: ADR-0003 / ADR-0040 租户是身份不是过滤器——显式租户与合同版本不是同一身份时报错且不交内容。
func TestAnotherTenantCannotReadThisContractsDelegations(t *testing.T) {
	fixture := newDelegationFixture(t)
	ctx := t.Context()

	contract := effectiveContract(t, "contract-1", "v1", "digest-c1")
	mustSaveVersion(t, fixture.transactor, ctx, fixture.repository, contract)
	mustSaveDelegations(t, fixture.transactor, ctx, fixture.repository, delegationsOn(t, contract,
		delegationRow(t, accountDelegatorRow(t, "account-1"), "level-commercial", "scope-1", delegationInterval(t)),
	))

	_, found, err := fixture.delegations.LoadContractDelegations(ctx, pcTenant(t, "tenant-b"), contract)
	if err == nil || found {
		t.Fatalf("另一租户按别人的合同读到了委派：found=%v err=%v", found, err)
	}
}

// Covers: 委派写口无事务拒（PBC-08 行为面）；库上 CHECK 守动作与委派方种类两个封闭集、外键要求合同版本先入册。
func TestDelegationWritesAreGuardedByTransactionAndSchema(t *testing.T) {
	fixture := newDelegationFixture(t)
	ctx := t.Context()

	if _, err := fixture.repository.SaveContractDelegations(ctx, domain.ContractDelegationContent{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记合同委派应返回 ErrTransactionRequired，实得：%v", err)
	}

	unpublished := effectiveContract(t, "contract-ghost", "v1", "digest-ghost")
	err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := fixture.repository.SaveContractDelegations(txCtx, delegationsOn(t, unpublished,
			delegationRow(t, accountDelegatorRow(t, "account-1"), "level-commercial", "scope-1", delegationInterval(t)),
		))
		return err
	})
	if err == nil {
		t.Fatal("合同版本未入册却登记了它的委派——外键没守住")
	}
}

// Covers: ADR-0116 Decision 三端到端——授权规则在册、委派在册，运营角色代客户请求资料修订经裁定编排解出委派方为
// 实际决定方；同一范围没有委派的另一客户被拒而不是未配置。
func TestAdjudicationResolvesTheDeciderFromThePersistedDelegation(t *testing.T) {
	fixture := newDelegationFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	contract := effectiveContract(t, "contract-1", "v1", "digest-c1")
	mustSaveVersion(t, fixture.transactor, ctx, fixture.repository, contract)
	mustSaveDelegations(t, fixture.transactor, ctx, fixture.repository, delegationsOn(t, contract,
		delegationRow(t, accountDelegatorRow(t, "account-1"), "level-commercial", "scope-1", delegationInterval(t)),
	))
	mustSaveGrant(t, fixture.transactor, ctx, fixture.grants,
		persistedGrant(t, "tenant-1", "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-1"))

	handler := application.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(fixture.grants, fixture.delegations)
	at := effectiveAtRow.Add(10 * 24 * time.Hour)

	authorized, err := handler.Handle(ctx, tenant, operatorAmendmentRequestRow(t, "operator-1", "account-1", "scope-1", at))
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	decider, named := authorized.Decider()
	if !named || decider.Kind() != domain.CustomerAccountDecider || decider.Reference() != "account-1" {
		t.Fatalf("decider = %s %q (named=%v), want CUSTOMER_ACCOUNT account-1", decider.Kind(), decider.Reference(), named)
	}
	if authorized.GrantVersion().ObjectID().String() != "auth-amend" {
		t.Fatal("已授权却没有指名所采用的授权规则版本")
	}

	_, err = handler.Handle(ctx, tenant, operatorAmendmentRequestRow(t, "operator-1", "account-2", "scope-1", at))
	if !errors.Is(err, domain.ErrDelegationAbsent) {
		t.Fatalf("error = %v, want ErrDelegationAbsent（另一客户没委派）", err)
	}
}

func operatorAmendmentRequestRow(t *testing.T, operator, onBehalfOf, scope string, at time.Time) domain.AuthorizationRequest {
	t.Helper()
	requester, err := domain.RequestedByOperatorRole(
		pcValue(t, domain.NewOperatorRoleReference, operator),
		pcValue(t, domain.NewCustomerAccountID, onBehalfOf),
	)
	if err != nil {
		t.Fatalf("requester: %v", err)
	}
	request, err := domain.NewAuthorizationRequestBy(
		requester,
		domain.SourceDataAmendmentAction,
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewAuthorityLevel, "level-commercial"),
		pcValue(t, domain.NewCommercialScopeReference, scope),
		pcValue(t, domain.NewStructuredReason, "CUSTOMER_CORRECTION"),
		pcValue(t, domain.NewEvidenceReference, "evidence-1"),
		at,
	)
	if err != nil {
		t.Fatalf("authorization request: %v", err)
	}
	return request
}
