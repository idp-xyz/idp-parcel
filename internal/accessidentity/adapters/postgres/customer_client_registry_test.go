package postgres_test

import (
	"context"
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

// 本文件对真实 PostgreSQL 16 证客户渠道册（access_identity 0010、ADR-0139 决定二）：登记、重放、撞键不覆盖、绑定不可改指、
// 跨租户主体拒收、授予只在区间内且撤销前生效，以及表结构自己守得住那几条不变量。

const (
	customerAccountOne    = "SYN-ACCOUNT-01"
	customerCredentialRef = "SYN-CREDENTIAL-REF/customer-01"
)

type customerRegister struct {
	registry   *adapter.CustomerClientRegistry
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newCustomerRegister(t *testing.T) customerRegister {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := adapter.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatalf("构造客户渠道册：%v", err)
	}
	return customerRegister{registry: registry, transactor: db.Transactor(), pool: pool}
}

func (register customerRegister) within(
	t *testing.T,
	act func(ctx context.Context) (accessidentity.CustomerClientRegistrationOutcome, error),
) accessidentity.CustomerClientRegistrationOutcome {
	t.Helper()
	var outcome accessidentity.CustomerClientRegistrationOutcome
	if err := register.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var actErr error
		outcome, actErr = act(txCtx)
		return actErr
	}); err != nil {
		t.Fatalf("登记事务：%v", err)
	}
	return outcome
}

func (register customerRegister) bind(t *testing.T, binding accessidentity.CustomerClientBinding) accessidentity.CustomerClientRegistrationOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.CustomerClientRegistrationOutcome, error) {
		return register.registry.RegisterCustomerClient(ctx, binding)
	})
}

func (register customerRegister) grant(t *testing.T, grant accessidentity.CustomerClientGrant) accessidentity.CustomerClientRegistrationOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.CustomerClientRegistrationOutcome, error) {
		return register.registry.RegisterCustomerClientGrant(ctx, grant)
	})
}

func (register customerRegister) revoke(t *testing.T, revocation accessidentity.GrantRevocation) accessidentity.CustomerClientRegistrationOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.CustomerClientRegistrationOutcome, error) {
		return register.registry.RegisterCustomerClientRevocation(ctx, revocation)
	})
}

func (register customerRegister) standing(t *testing.T, subject accessidentity.CustomerClientSubject) accessidentity.CustomerClientStanding {
	t.Helper()
	standing, found, err := register.registry.FindCustomerClient(t.Context(), subject)
	if err != nil || !found {
		t.Fatalf("FindCustomerClient = found %v, err %v; want 在册", found, err)
	}
	return standing
}

func customerSubjectOf(t *testing.T, issuer, sub string) accessidentity.CustomerClientSubject {
	t.Helper()
	subject, err := accessidentity.NewCustomerClientSubject(issuer, sub)
	if err != nil {
		t.Fatalf("客户端主体：%v", err)
	}
	return subject
}

type customerBindingSpec struct {
	tenant, account, credential, basis string
	certificateBound                   bool
}

func defaultCustomerBinding(tenant string) customerBindingSpec {
	return customerBindingSpec{tenant: tenant, account: customerAccountOne, credential: customerCredentialRef, basis: "SYN-CUSTOMER-BASIS-01"}
}

func customerBindingOf(t *testing.T, subject accessidentity.CustomerClientSubject, spec customerBindingSpec) accessidentity.CustomerClientBinding {
	t.Helper()
	reference, err := accessidentity.NewCredentialReference(spec.credential)
	if err != nil {
		t.Fatalf("凭据引用：%v", err)
	}
	binding, err := accessidentity.NewCustomerClientBinding(subject, spec.tenant, spec.account, reference, spec.certificateBound, spec.basis)
	if err != nil {
		t.Fatalf("绑定：%v", err)
	}
	return binding
}

func customerGrantOf(
	t *testing.T,
	tenant, id string,
	subject accessidentity.CustomerClientSubject,
	startsAt, endsAt time.Time,
) accessidentity.CustomerClientGrant {
	t.Helper()
	interval, err := accessidentity.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("区间：%v", err)
	}
	grant, err := accessidentity.NewCustomerClientGrant(tenant, id, subject,
		accessidentity.CustomerCapabilityShipmentRequestSubmission, interval, "SYN-CUSTOMER-GRANT-BASIS-"+id)
	if err != nil {
		t.Fatalf("授予：%v", err)
	}
	return grant
}

func expectCustomerOutcome(t *testing.T, label string, got, want accessidentity.CustomerClientRegistrationOutcome) {
	t.Helper()
	if got != want {
		t.Fatalf("%s：答复 = %s, want %s", label, got, want)
	}
}

// Covers: ADR-0139 决定二「绑定一次登定、不可改指」——首登落册、各件原样读回；同内容重放答原结果；同主体同租户而客户账户
// 或任何别的一件不同都答冲突，册上那一行原样不动；不在册的主体答 found=false 而不是 error。
func TestCustomerClientRegistrationRecordsReplaysAndNeverRepoints(t *testing.T) {
	register := newCustomerRegister(t)
	subject := customerSubjectOf(t, syntheticIssuer, "SYN-CUSTOMER-CLIENT-01")
	spec := defaultCustomerBinding(tenantOne)
	spec.certificateBound = true
	binding := customerBindingOf(t, subject, spec)

	expectCustomerOutcome(t, "首登", register.bind(t, binding), accessidentity.CustomerClientRegistrationRecorded)
	expectCustomerOutcome(t, "重放", register.bind(t, binding), accessidentity.CustomerClientRegistrationAlreadyRegistered)
	for name, mutate := range map[string]func(*customerBindingSpec){
		"改指另一个客户账户": func(s *customerBindingSpec) { s.account = "SYN-ACCOUNT-02" },
		"异凭据引用":     func(s *customerBindingSpec) { s.credential = "SYN-CREDENTIAL-REF/customer-01-v2" },
		"异证书绑定要求":   func(s *customerBindingSpec) { s.certificateBound = false },
		"异登记依据":     func(s *customerBindingSpec) { s.basis = "SYN-CUSTOMER-BASIS-02" },
	} {
		changed := spec
		mutate(&changed)
		expectCustomerOutcome(t, name, register.bind(t, customerBindingOf(t, subject, changed)), accessidentity.CustomerClientRegistrationContentConflict)
	}

	standing := register.standing(t, subject)
	if standing.Binding() != binding || len(standing.Grants()) != 0 {
		t.Fatalf("册上现状 = %+v, want 原登记且无授予", standing)
	}
	if _, found, err := register.registry.FindCustomerClient(t.Context(), customerSubjectOf(t, syntheticIssuer, "SYN-CUSTOMER-GHOST")); found || err != nil {
		t.Fatalf("不在册的主体：found %v, err %v; want false, nil", found, err)
	}
}

// Covers: ADR-0139 决定二「客户端主体绑定唯一（租户、客户账户）」——已绑在甲租户的主体，乙租户登不进、也授不了；表结构自己也
// 守：主体第二行与跨租户授予进不了库。客户渠道册与集成客户端册是两本：同一个主体登在集成客户端册上，客户渠道册里照旧
// 查不到它（ADR-0139 决定三「不得互相顶替」）。
func TestCustomerClientBoundToOneTenantIsRefusedByEveryOtherTenant(t *testing.T) {
	register := newCustomerRegister(t)
	subject := customerSubjectOf(t, syntheticIssuer, "SYN-CUSTOMER-CLIENT-01")

	expectCustomerOutcome(t, "甲租户首登", register.bind(t, customerBindingOf(t, subject, defaultCustomerBinding(tenantOne))),
		accessidentity.CustomerClientRegistrationRecorded)
	expectCustomerOutcome(t, "乙租户登同一主体", register.bind(t, customerBindingOf(t, subject, defaultCustomerBinding(tenantTwo))),
		accessidentity.CustomerClientBoundToAnotherTenant)
	expectCustomerOutcome(t, "乙租户授甲的主体",
		register.grant(t, customerGrantOf(t, tenantTwo, "SYN-CUSTOMER-GRANT-01", subject, grantStartsAt, time.Time{})),
		accessidentity.CustomerClientNotRegistered)

	expectSQLState(t, register.pool, "主体第二行", uniqueViolation,
		`INSERT INTO access_identity.customer_client
			(issuer, subject, tenant_id, customer_account_id, credential_ref, certificate_bound_token_required, basis_ref)
		 VALUES ($1, $2, $3, 'SYN-X', 'SYN-X', false, 'SYN-X')`,
		syntheticIssuer, "SYN-CUSTOMER-CLIENT-01", tenantTwo)
	expectSQLState(t, register.pool, "跨租户授予", foreignKeyViolation,
		`INSERT INTO access_identity.customer_client_grant
			(tenant_id, grant_id, issuer, subject, capability, effective_starts_at, basis_ref)
		 VALUES ($1, 'SYN-CUSTOMER-GRANT-X', $2, $3, 'SHIPMENT_REQUEST_SUBMISSION', now(), 'SYN-X')`,
		tenantTwo, syntheticIssuer, "SYN-CUSTOMER-CLIENT-01")

	db := registerDB(t, register.pool)
	integration, err := adapter.NewIntegrationClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	integrationSubject := clientSubjectOf(t, syntheticIssuer, "SYN-INTEGRATION-ONLY-01")
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := integration.RegisterIntegrationClient(ctx, clientBindingOf(t, integrationSubject, defaultClientBinding(tenantOne)))
		return err
	}); err != nil {
		t.Fatalf("登集成客户端：%v", err)
	}
	if _, found, err := register.registry.FindCustomerClient(t.Context(), customerSubjectOf(t, syntheticIssuer, "SYN-INTEGRATION-ONLY-01")); found || err != nil {
		t.Fatalf("集成客户端册上的主体在客户渠道册里查到了：found %v, err %v", found, err)
	}
}

// Covers: ADR-0139 决定二「授予按客户面能力显式登记、可撤销、带生效区间」——授予只在区间内、撤销时刻前生效；同内容重放
// 答原结果，同标识异区间答冲突；撤不存在的授予答未登记；表结构挡下未知能力与倒置区间。
func TestCustomerClientGrantIsEffectiveOnlyInsideItsIntervalAndUntilRevoked(t *testing.T) {
	register := newCustomerRegister(t)
	subject := customerSubjectOf(t, syntheticIssuer, "SYN-CUSTOMER-CLIENT-01")
	expectCustomerOutcome(t, "登主体", register.bind(t, customerBindingOf(t, subject, defaultCustomerBinding(tenantOne))),
		accessidentity.CustomerClientRegistrationRecorded)

	endsAt := grantStartsAt.Add(90 * 24 * time.Hour)
	grant := customerGrantOf(t, tenantOne, "SYN-CUSTOMER-GRANT-01", subject, grantStartsAt, endsAt)
	expectCustomerOutcome(t, "首授", register.grant(t, grant), accessidentity.CustomerClientRegistrationRecorded)
	expectCustomerOutcome(t, "重放", register.grant(t, grant), accessidentity.CustomerClientRegistrationAlreadyRegistered)
	expectCustomerOutcome(t, "同标识异区间",
		register.grant(t, customerGrantOf(t, tenantOne, "SYN-CUSTOMER-GRANT-01", subject, grantStartsAt, time.Time{})),
		accessidentity.CustomerClientRegistrationContentConflict)

	revokedAt := grantStartsAt.Add(30 * 24 * time.Hour)
	revocation := revocationOf(t, tenantOne, "SYN-CUSTOMER-GRANT-01", revokedAt, "SYN-REVOKE-BASIS-01")
	expectCustomerOutcome(t, "撤销", register.revoke(t, revocation), accessidentity.CustomerClientRegistrationRecorded)
	expectCustomerOutcome(t, "撤销重放", register.revoke(t, revocation), accessidentity.CustomerClientRegistrationAlreadyRegistered)
	expectCustomerOutcome(t, "异时刻撤销",
		register.revoke(t, revocationOf(t, tenantOne, "SYN-CUSTOMER-GRANT-01", revokedAt.Add(time.Hour), "SYN-REVOKE-BASIS-01")),
		accessidentity.CustomerClientRegistrationContentConflict)
	expectCustomerOutcome(t, "撤不存在的授予",
		register.revoke(t, revocationOf(t, tenantOne, "SYN-CUSTOMER-GRANT-GHOST", revokedAt, "SYN-REVOKE-BASIS-01")),
		accessidentity.CustomerClientGrantNotRegistered)

	standing := register.standing(t, subject)
	submission := accessidentity.CustomerCapabilityShipmentRequestSubmission
	for name, probe := range map[string]struct {
		at   time.Time
		want bool
	}{
		"起点前一刻":   {grantStartsAt.Add(-time.Microsecond), false},
		"起点":      {grantStartsAt, true},
		"撤销时刻前一刻": {revokedAt.Add(-time.Microsecond), true},
		"撤销时刻":    {revokedAt, false},
	} {
		if got := standing.HoldsAt(submission, probe.at); got != probe.want {
			t.Fatalf("%s：HoldsAt = %v, want %v", name, got, probe.want)
		}
	}

	expectSQLState(t, register.pool, "未知能力", checkViolation,
		`INSERT INTO access_identity.customer_client_grant
			(tenant_id, grant_id, issuer, subject, capability, effective_starts_at, basis_ref)
		 VALUES ($1, 'SYN-CUSTOMER-GRANT-Y', $2, $3, 'CLAIM_SUBMISSION', now(), 'SYN-X')`,
		tenantOne, syntheticIssuer, "SYN-CUSTOMER-CLIENT-01")
	expectSQLState(t, register.pool, "倒置区间", checkViolation,
		`INSERT INTO access_identity.customer_client_grant
			(tenant_id, grant_id, issuer, subject, capability, effective_starts_at, effective_ends_at, basis_ref)
		 VALUES ($1, 'SYN-CUSTOMER-GRANT-Z', $2, $3, 'SHIPMENT_REQUEST_SUBMISSION', now(), now() - interval '1 day', 'SYN-X')`,
		tenantOne, syntheticIssuer, "SYN-CUSTOMER-CLIENT-01")
	expectSQLState(t, register.pool, "缺客户账户", checkViolation,
		`INSERT INTO access_identity.customer_client
			(issuer, subject, tenant_id, customer_account_id, credential_ref, certificate_bound_token_required, basis_ref)
		 VALUES ($1, 'SYN-CUSTOMER-CLIENT-09', $2, ' ', 'SYN-X', false, 'SYN-X')`,
		syntheticIssuer, tenantOne)
}

func registerDB(t *testing.T, pool *pgxpool.Pool) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db
}
