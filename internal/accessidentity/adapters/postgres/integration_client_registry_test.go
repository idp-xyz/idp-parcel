package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	adapter "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证集成客户端册（access_identity 0003、ADR-0149 决定三）：登记、重放、撞键不覆盖、跨租户
// 主体拒收、授予只在区间内且撤销前生效，以及表结构自己守得住那几条不变量。

const (
	clientSource        = "SYN-SOURCE/bank-01"
	clientCredentialRef = "SYN-CREDENTIAL-REF/bank-01"
)

type clientRegister struct {
	registry   *adapter.IntegrationClientRegistry
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newClientRegister(t *testing.T) clientRegister {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := adapter.NewIntegrationClientRegistry(db)
	if err != nil {
		t.Fatalf("构造集成客户端册：%v", err)
	}
	return clientRegister{registry: registry, transactor: db.Transactor(), pool: pool}
}

func (register clientRegister) within(
	t *testing.T,
	act func(ctx context.Context) (accessidentity.IntegrationClientRegistrationOutcome, error),
) accessidentity.IntegrationClientRegistrationOutcome {
	t.Helper()
	var outcome accessidentity.IntegrationClientRegistrationOutcome
	if err := register.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var actErr error
		outcome, actErr = act(txCtx)
		return actErr
	}); err != nil {
		t.Fatalf("登记事务：%v", err)
	}
	return outcome
}

func (register clientRegister) bind(t *testing.T, binding accessidentity.IntegrationClientBinding) accessidentity.IntegrationClientRegistrationOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.IntegrationClientRegistrationOutcome, error) {
		return register.registry.RegisterIntegrationClient(ctx, binding)
	})
}

func (register clientRegister) grant(t *testing.T, grant accessidentity.IntegrationClientGrant) accessidentity.IntegrationClientRegistrationOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.IntegrationClientRegistrationOutcome, error) {
		return register.registry.RegisterIntegrationClientGrant(ctx, grant)
	})
}

func (register clientRegister) revoke(t *testing.T, revocation accessidentity.GrantRevocation) accessidentity.IntegrationClientRegistrationOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.IntegrationClientRegistrationOutcome, error) {
		return register.registry.RegisterIntegrationClientRevocation(ctx, revocation)
	})
}

func (register clientRegister) standing(t *testing.T, subject accessidentity.IntegrationClientSubject) accessidentity.IntegrationClientStanding {
	t.Helper()
	standing, found, err := register.registry.FindIntegrationClient(t.Context(), subject)
	if err != nil || !found {
		t.Fatalf("FindIntegrationClient = found %v, err %v; want 在册", found, err)
	}
	return standing
}

func clientSubjectOf(t *testing.T, issuer, sub string) accessidentity.IntegrationClientSubject {
	t.Helper()
	subject, err := accessidentity.NewIntegrationClientSubject(issuer, sub)
	if err != nil {
		t.Fatalf("客户端主体：%v", err)
	}
	return subject
}

type clientBindingSpec struct {
	tenant, source, credential, basis string
	certificateBound                  bool
}

func defaultClientBinding(tenant string) clientBindingSpec {
	return clientBindingSpec{tenant: tenant, source: clientSource, credential: clientCredentialRef, basis: "SYN-CLIENT-BASIS-01"}
}

func clientBindingOf(t *testing.T, subject accessidentity.IntegrationClientSubject, spec clientBindingSpec) accessidentity.IntegrationClientBinding {
	t.Helper()
	reference, err := accessidentity.NewCredentialReference(spec.credential)
	if err != nil {
		t.Fatalf("凭据引用：%v", err)
	}
	binding, err := accessidentity.NewIntegrationClientBinding(subject, spec.tenant, spec.source, reference, spec.certificateBound, spec.basis)
	if err != nil {
		t.Fatalf("绑定：%v", err)
	}
	return binding
}

func clientGrantOf(
	t *testing.T,
	tenant, id string,
	subject accessidentity.IntegrationClientSubject,
	fact accessidentity.ExternalFactType,
	startsAt, endsAt time.Time,
) accessidentity.IntegrationClientGrant {
	t.Helper()
	interval, err := accessidentity.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("区间：%v", err)
	}
	grant, err := accessidentity.NewIntegrationClientGrant(tenant, id, subject, fact, interval, "SYN-CLIENT-GRANT-BASIS-"+id)
	if err != nil {
		t.Fatalf("授予：%v", err)
	}
	return grant
}

func expectClientOutcome(t *testing.T, label string, got, want accessidentity.IntegrationClientRegistrationOutcome) {
	t.Helper()
	if got != want {
		t.Fatalf("%s：答复 = %s, want %s", label, got, want)
	}
}

// Covers: 票面完成判据「客户端未登记……各答其格」的册这一侧——首登落册，绑定的各件（主体、租户、来源身份、凭据引用、
// 证书绑定要求、依据）原样读回；同内容重放答原结果；同主体同租户而任何一件不同答冲突，册上那一行原样不动；不在册的
// 主体答 found=false 而不是 error。
func TestIntegrationClientRegistrationRecordsReplaysAndNeverOverwrites(t *testing.T) {
	register := newClientRegister(t)
	subject := clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-01")
	spec := defaultClientBinding(tenantOne)
	spec.certificateBound = true
	binding := clientBindingOf(t, subject, spec)

	expectClientOutcome(t, "首登", register.bind(t, binding), accessidentity.IntegrationClientRegistrationRecorded)
	expectClientOutcome(t, "重放", register.bind(t, binding), accessidentity.IntegrationClientRegistrationAlreadyRegistered)
	for name, mutate := range map[string]func(*clientBindingSpec){
		"异来源身份":   func(s *clientBindingSpec) { s.source = "SYN-SOURCE/bank-02" },
		"异凭据引用":   func(s *clientBindingSpec) { s.credential = "SYN-CREDENTIAL-REF/bank-01-v2" },
		"异证书绑定要求": func(s *clientBindingSpec) { s.certificateBound = false },
		"异登记依据":   func(s *clientBindingSpec) { s.basis = "SYN-CLIENT-BASIS-02" },
	} {
		changed := spec
		mutate(&changed)
		expectClientOutcome(t, name, register.bind(t, clientBindingOf(t, subject, changed)), accessidentity.IntegrationClientRegistrationContentConflict)
	}

	standing := register.standing(t, subject)
	if standing.Binding() != binding || len(standing.Grants()) != 0 {
		t.Fatalf("册上现状 = %+v, want 原登记且无授予", standing)
	}
	if _, found, err := register.registry.FindIntegrationClient(t.Context(), clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-GHOST")); found || err != nil {
		t.Fatalf("不在册的主体：found %v, err %v; want false, nil", found, err)
	}
}

// Covers: ADR-0149 决定三「客户端标识绑定唯一租户」——已绑在甲租户的主体，乙租户登不进、也授不了；答复不披露绑在哪；
// 键是发行方 + sub，别的发行方下同名 sub 是另一个主体。表结构自己也守：主体第二行与跨租户授予都进不了库。
func TestIntegrationClientBoundToOneTenantIsRefusedByEveryOtherTenant(t *testing.T) {
	register := newClientRegister(t)
	subject := clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-01")

	expectClientOutcome(t, "甲租户首登", register.bind(t, clientBindingOf(t, subject, defaultClientBinding(tenantOne))),
		accessidentity.IntegrationClientRegistrationRecorded)
	expectClientOutcome(t, "乙租户登同一主体", register.bind(t, clientBindingOf(t, subject, defaultClientBinding(tenantTwo))),
		accessidentity.IntegrationClientBoundToAnotherTenant)
	expectClientOutcome(t, "乙租户授甲的主体",
		register.grant(t, clientGrantOf(t, tenantTwo, "SYN-CLIENT-GRANT-01", subject, accessidentity.FactExternalFunds, grantStartsAt, time.Time{})),
		accessidentity.IntegrationClientNotRegistered)
	if standing := register.standing(t, subject); standing.Binding().TenantID() != tenantOne || len(standing.Grants()) != 0 {
		t.Fatalf("册上现状 = %+v, want 仍只绑甲租户、无授予", standing)
	}

	elsewhere := clientSubjectOf(t, "https://syn-issuer-02.example.invalid", "SYN-CLIENT-01")
	expectClientOutcome(t, "别的发行方下同名 sub", register.bind(t, clientBindingOf(t, elsewhere, defaultClientBinding(tenantTwo))),
		accessidentity.IntegrationClientRegistrationRecorded)

	expectSQLState(t, register.pool, "主体第二行", uniqueViolation,
		`INSERT INTO access_identity.integration_client
			(issuer, subject, tenant_id, source_ref, credential_ref, certificate_bound_token_required, basis_ref)
		 VALUES ($1, $2, $3, 'SYN-X', 'SYN-X', false, 'SYN-X')`,
		syntheticIssuer, "SYN-CLIENT-01", tenantTwo)
	expectSQLState(t, register.pool, "跨租户授予", foreignKeyViolation,
		`INSERT INTO access_identity.integration_client_grant
			(tenant_id, grant_id, issuer, subject, fact_type, effective_starts_at, basis_ref)
		 VALUES ($1, 'SYN-CLIENT-GRANT-X', $2, $3, 'EXTERNAL_FUNDS_FACT', now(), 'SYN-X')`,
		tenantTwo, syntheticIssuer, "SYN-CLIENT-01")
}

// Covers: 票面「授予按事实类型登记、可撤销、带生效区间」——授予只在区间内生效、另一类事实不顶替；同内容重放（带亚微秒
// 的时点也认得出）答原结果，同标识异事实类型或异区间答冲突；撤销自撤销时刻起生效，同内容重放答原结果、异时刻答冲突；
// 撤一笔本租户册上没有的授予答未登记（别的租户拿同一个授予标识也撤不动）；撤了再授是另一笔，前一笔连同撤销留在册上。
func TestIntegrationClientGrantIsEffectiveOnlyInsideItsIntervalAndUntilRevoked(t *testing.T) {
	register := newClientRegister(t)
	subject := clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-01")
	expectClientOutcome(t, "登主体", register.bind(t, clientBindingOf(t, subject, defaultClientBinding(tenantOne))),
		accessidentity.IntegrationClientRegistrationRecorded)

	funds := accessidentity.FactExternalFunds
	endsAt := grantStartsAt.Add(90 * 24 * time.Hour)
	grant := clientGrantOf(t, tenantOne, "SYN-CLIENT-GRANT-01", subject, funds, grantStartsAt, endsAt)
	expectClientOutcome(t, "首授", register.grant(t, grant), accessidentity.IntegrationClientRegistrationRecorded)
	expectClientOutcome(t, "重放", register.grant(t, grant), accessidentity.IntegrationClientRegistrationAlreadyRegistered)
	expectClientOutcome(t, "同标识异事实类型",
		register.grant(t, clientGrantOf(t, tenantOne, "SYN-CLIENT-GRANT-01", subject, accessidentity.FactCustomsExternalResult, grantStartsAt, endsAt)),
		accessidentity.IntegrationClientRegistrationContentConflict)
	expectClientOutcome(t, "同标识异区间",
		register.grant(t, clientGrantOf(t, tenantOne, "SYN-CLIENT-GRANT-01", subject, funds, grantStartsAt, time.Time{})),
		accessidentity.IntegrationClientRegistrationContentConflict)
	expectClientOutcome(t, "不在册的主体",
		register.grant(t, clientGrantOf(t, tenantOne, "SYN-CLIENT-GRANT-02", clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-GHOST"), funds, grantStartsAt, time.Time{})),
		accessidentity.IntegrationClientNotRegistered)

	revokedAt := grantStartsAt.Add(30 * 24 * time.Hour)
	revocation := revocationOf(t, tenantOne, "SYN-CLIENT-GRANT-01", revokedAt, "SYN-REVOKE-BASIS-01")
	expectClientOutcome(t, "撤销", register.revoke(t, revocation), accessidentity.IntegrationClientRegistrationRecorded)
	expectClientOutcome(t, "撤销重放", register.revoke(t, revocation), accessidentity.IntegrationClientRegistrationAlreadyRegistered)
	expectClientOutcome(t, "异时刻撤销",
		register.revoke(t, revocationOf(t, tenantOne, "SYN-CLIENT-GRANT-01", revokedAt.Add(time.Hour), "SYN-REVOKE-BASIS-01")),
		accessidentity.IntegrationClientRegistrationContentConflict)
	expectClientOutcome(t, "撤不存在的授予",
		register.revoke(t, revocationOf(t, tenantOne, "SYN-CLIENT-GRANT-GHOST", revokedAt, "SYN-REVOKE-BASIS-01")),
		accessidentity.IntegrationClientGrantNotRegistered)
	expectClientOutcome(t, "别的租户撤同一标识",
		register.revoke(t, revocationOf(t, tenantTwo, "SYN-CLIENT-GRANT-01", revokedAt, "SYN-REVOKE-BASIS-01")),
		accessidentity.IntegrationClientGrantNotRegistered)

	standing := register.standing(t, subject)
	for name, probe := range map[string]struct {
		fact accessidentity.ExternalFactType
		at   time.Time
		want bool
	}{
		"起点前一刻":     {funds, grantStartsAt.Add(-time.Microsecond), false},
		"起点":        {funds, grantStartsAt, true},
		"撤销时刻前一刻":   {funds, revokedAt.Add(-time.Microsecond), true},
		"撤销时刻":      {funds, revokedAt, false},
		"未授予的那一类事实": {accessidentity.FactCustomsExternalResult, grantStartsAt, false},
	} {
		if got := standing.HoldsAt(probe.fact, probe.at); got != probe.want {
			t.Fatalf("%s：HoldsAt = %v, want %v", name, got, probe.want)
		}
	}
	recorded, revoked := standing.Grants()[0].Revocation()
	if !revoked || !recorded.RevokedAt().Equal(revokedAt.Truncate(time.Microsecond)) || recorded.Basis() != "SYN-REVOKE-BASIS-01" {
		t.Fatalf("册上撤销 = %+v, %v", recorded, revoked)
	}

	regrantedAt := revokedAt.Add(5 * 24 * time.Hour)
	expectClientOutcome(t, "撤了再授",
		register.grant(t, clientGrantOf(t, tenantOne, "SYN-CLIENT-GRANT-03", subject, funds, regrantedAt, time.Time{})),
		accessidentity.IntegrationClientRegistrationRecorded)
	standing = register.standing(t, subject)
	if len(standing.Grants()) != 2 || standing.HoldsAt(funds, regrantedAt.Add(-time.Microsecond)) || !standing.HoldsAt(funds, regrantedAt) {
		t.Fatalf("再授后现状 = %+v, want 两笔在册、空档不生效、再授起点起生效", standing)
	}
}

// Covers: 表结构自己守得住——绕过登记口直接写，缺件、未知事实类型、倒置区间、撤不存在的授予、一笔授予撤两次，都进不了库。
func TestIntegrationClientRegisterTablesGuardTheInvariantsThemselves(t *testing.T) {
	register := newClientRegister(t)
	subject := clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-01")
	expectClientOutcome(t, "登主体", register.bind(t, clientBindingOf(t, subject, defaultClientBinding(tenantOne))),
		accessidentity.IntegrationClientRegistrationRecorded)
	expectClientOutcome(t, "授予",
		register.grant(t, clientGrantOf(t, tenantOne, "SYN-CLIENT-GRANT-01", subject, accessidentity.FactExternalFunds, grantStartsAt, time.Time{})),
		accessidentity.IntegrationClientRegistrationRecorded)
	expectClientOutcome(t, "撤销",
		register.revoke(t, revocationOf(t, tenantOne, "SYN-CLIENT-GRANT-01", grantStartsAt.Add(time.Hour), "SYN-REVOKE-BASIS-01")),
		accessidentity.IntegrationClientRegistrationRecorded)

	const insertClient = `INSERT INTO access_identity.integration_client
		(issuer, subject, tenant_id, source_ref, credential_ref, certificate_bound_token_required, basis_ref)
		VALUES ($1, $2, $3, $4, $5, false, $6)`
	for name, row := range map[string][6]string{
		"空发行方":  {" ", "SYN-CLIENT-09", tenantOne, clientSource, clientCredentialRef, "SYN-X"},
		"空 sub": {syntheticIssuer, "", tenantOne, clientSource, clientCredentialRef, "SYN-X"},
		"空租户":   {syntheticIssuer, "SYN-CLIENT-09", " ", clientSource, clientCredentialRef, "SYN-X"},
		"空来源身份": {syntheticIssuer, "SYN-CLIENT-09", tenantOne, "", clientCredentialRef, "SYN-X"},
		"空凭据引用": {syntheticIssuer, "SYN-CLIENT-09", tenantOne, clientSource, " ", "SYN-X"},
		"空依据":   {syntheticIssuer, "SYN-CLIENT-09", tenantOne, clientSource, clientCredentialRef, ""},
	} {
		expectSQLState(t, register.pool, "主体"+name, checkViolation, insertClient, row[0], row[1], row[2], row[3], row[4], row[5])
	}

	const insertGrant = `INSERT INTO access_identity.integration_client_grant
		(tenant_id, grant_id, issuer, subject, fact_type, effective_starts_at, effective_ends_at, basis_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	for name, fact := range map[string]string{"未知事实类型": "ANYTHING", "更正不另立一类": "EXTERNAL_FUNDS_FACT_CORRECTION"} {
		expectSQLState(t, register.pool, name, checkViolation, insertGrant,
			tenantOne, "SYN-CLIENT-GRANT-09", syntheticIssuer, "SYN-CLIENT-01", fact, grantStartsAt, nil, "SYN-X")
	}
	expectSQLState(t, register.pool, "终点不晚于起点", checkViolation, insertGrant,
		tenantOne, "SYN-CLIENT-GRANT-09", syntheticIssuer, "SYN-CLIENT-01", "EXTERNAL_FUNDS_FACT", grantStartsAt, grantStartsAt, "SYN-X")
	expectSQLState(t, register.pool, "授予空依据", checkViolation, insertGrant,
		tenantOne, "SYN-CLIENT-GRANT-09", syntheticIssuer, "SYN-CLIENT-01", "EXTERNAL_FUNDS_FACT", grantStartsAt, nil, " ")

	const insertRevocation = `INSERT INTO access_identity.integration_client_grant_revocation
		(tenant_id, grant_id, revoked_at, basis_ref) VALUES ($1, $2, $3, $4)`
	expectSQLState(t, register.pool, "撤不存在的授予", foreignKeyViolation, insertRevocation,
		tenantOne, "SYN-CLIENT-GRANT-GHOST", grantStartsAt, "SYN-X")
	expectSQLState(t, register.pool, "一笔授予撤两次", uniqueViolation, insertRevocation,
		tenantOne, "SYN-CLIENT-GRANT-01", grantStartsAt.Add(2*time.Hour), "SYN-X")
	expectSQLState(t, register.pool, "撤销空依据", checkViolation, insertRevocation,
		tenantOne, "SYN-CLIENT-GRANT-01", grantStartsAt, "")
}

// Covers: PBC-08——三个写口只经框架事务，拿不到事务句柄时答 ErrTransactionRequired，不退回连接池自动提交。
func TestIntegrationClientRegistrationWritesRefuseToRunOutsideATransaction(t *testing.T) {
	register := newClientRegister(t)
	subject := clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-01")
	ctx := t.Context()

	if _, err := register.registry.RegisterIntegrationClient(ctx,
		clientBindingOf(t, subject, defaultClientBinding(tenantOne))); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记主体：err = %v, want ErrTransactionRequired", err)
	}
	if _, err := register.registry.RegisterIntegrationClientGrant(ctx,
		clientGrantOf(t, tenantOne, "SYN-CLIENT-GRANT-01", subject, accessidentity.FactExternalFunds, grantStartsAt, time.Time{})); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记授予：err = %v, want ErrTransactionRequired", err)
	}
	if _, err := register.registry.RegisterIntegrationClientRevocation(ctx,
		revocationOf(t, tenantOne, "SYN-CLIENT-GRANT-01", grantStartsAt, "SYN-REVOKE-BASIS-01")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记撤销：err = %v, want ErrTransactionRequired", err)
	}
	if _, found, err := register.registry.FindIntegrationClient(ctx, subject); found || err != nil {
		t.Fatalf("无事务登记之后：found %v, err %v; want 未落册", found, err)
	}
}
