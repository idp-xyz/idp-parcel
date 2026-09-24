package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	adapter "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证操作者册（access_identity 0001、ADR-0100 决定二第三条）：登记、
// 重放、撤销、区间外不生效、跨租户主体拒收，以及表结构自己守得住那几条不变量——不靠登记口
// 记得去查。

const (
	syntheticIssuer = "https://syn-issuer-01.example.invalid"
	tenantOne       = "SYN-TENANT-01"
	tenantTwo       = "SYN-TENANT-02"
)

// grantStartsAt 带纳秒：库里 timestamptz 只到微秒，同一份输入重放时要照样认得出是重放。
var grantStartsAt = time.Date(2026, 1, 1, 8, 0, 0, 123456789, time.UTC)

type operatorRegister struct {
	registry   *adapter.OperatorRegistry
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newOperatorRegister(t *testing.T) operatorRegister {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := adapter.NewOperatorRegistry(db)
	if err != nil {
		t.Fatalf("构造操作者册：%v", err)
	}
	return operatorRegister{registry: registry, transactor: db.Transactor(), pool: pool}
}

func (register operatorRegister) within(
	t *testing.T,
	act func(ctx context.Context) (accessidentity.OperatorRegistrationOutcome, error),
) accessidentity.OperatorRegistrationOutcome {
	t.Helper()
	var outcome accessidentity.OperatorRegistrationOutcome
	if err := register.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var actErr error
		outcome, actErr = act(txCtx)
		return actErr
	}); err != nil {
		t.Fatalf("登记事务：%v", err)
	}
	return outcome
}

func (register operatorRegister) bind(t *testing.T, binding accessidentity.OperatorBinding) accessidentity.OperatorRegistrationOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.OperatorRegistrationOutcome, error) {
		return register.registry.RegisterOperator(ctx, binding)
	})
}

func (register operatorRegister) grant(t *testing.T, grant accessidentity.OperatorGrant) accessidentity.OperatorRegistrationOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.OperatorRegistrationOutcome, error) {
		return register.registry.RegisterGrant(ctx, grant)
	})
}

func (register operatorRegister) revoke(t *testing.T, revocation accessidentity.GrantRevocation) accessidentity.OperatorRegistrationOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.OperatorRegistrationOutcome, error) {
		return register.registry.RegisterRevocation(ctx, revocation)
	})
}

func (register operatorRegister) standing(t *testing.T, subject accessidentity.OperatorSubject) accessidentity.OperatorStanding {
	t.Helper()
	standing, found, err := register.registry.FindOperator(t.Context(), subject)
	if err != nil || !found {
		t.Fatalf("FindOperator = found %v, err %v; want 在册", found, err)
	}
	return standing
}

func subjectOf(t *testing.T, issuer, sub string) accessidentity.OperatorSubject {
	t.Helper()
	subject, err := accessidentity.NewOperatorSubject(issuer, sub)
	if err != nil {
		t.Fatalf("主体：%v", err)
	}
	return subject
}

func bindingOf(t *testing.T, subject accessidentity.OperatorSubject, tenant, basis string) accessidentity.OperatorBinding {
	t.Helper()
	binding, err := accessidentity.NewOperatorBinding(subject, tenant, basis)
	if err != nil {
		t.Fatalf("绑定：%v", err)
	}
	return binding
}

func grantOf(
	t *testing.T,
	tenant, id string,
	subject accessidentity.OperatorSubject,
	face accessidentity.CapabilityFace,
	startsAt, endsAt time.Time,
) accessidentity.OperatorGrant {
	t.Helper()
	interval, err := accessidentity.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("区间：%v", err)
	}
	grant, err := accessidentity.NewOperatorGrant(tenant, id, subject, face, interval, "SYN-GRANT-BASIS-"+id)
	if err != nil {
		t.Fatalf("授予：%v", err)
	}
	return grant
}

func revocationOf(t *testing.T, tenant, id string, revokedAt time.Time, basis string) accessidentity.GrantRevocation {
	t.Helper()
	revocation, err := accessidentity.NewGrantRevocation(tenant, id, revokedAt, basis)
	if err != nil {
		t.Fatalf("撤销：%v", err)
	}
	return revocation
}

func expectOutcome(t *testing.T, label string, got, want accessidentity.OperatorRegistrationOutcome) {
	t.Helper()
	if got != want {
		t.Fatalf("%s：答复 = %s, want %s", label, got, want)
	}
}

// Covers: 票 operator-channel/01 完成判据「登记、重放」——首登落册；同内容重放答原结果；同主体同租户
// 异依据答冲突且册上那一行原样不动；不在册的主体答 found=false 而不是 error。
func TestOperatorRegistrationRecordsReplaysAndNeverOverwrites(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-01")
	binding := bindingOf(t, subject, tenantOne, "SYN-OPERATOR-BASIS-01")

	expectOutcome(t, "首登", register.bind(t, binding), accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "重放", register.bind(t, binding), accessidentity.OperatorRegistrationAlreadyRegistered)
	expectOutcome(t, "异依据",
		register.bind(t, bindingOf(t, subject, tenantOne, "SYN-OPERATOR-BASIS-02")),
		accessidentity.OperatorRegistrationContentConflict)

	standing := register.standing(t, subject)
	if standing.Binding() != binding || len(standing.Grants()) != 0 {
		t.Fatalf("册上现状 = %+v, want 原登记且无授予", standing)
	}

	if _, found, err := register.registry.FindOperator(t.Context(), subjectOf(t, syntheticIssuer, "SYN-OPERATOR-GHOST")); found || err != nil {
		t.Fatalf("不在册的主体：found %v, err %v; want false, nil", found, err)
	}
}

// Covers: 票 operator-channel/01 完成判据「跨租户主体拒收」——已绑在甲租户的主体，乙租户登不进、也授不了；
// 答复不披露绑在哪；键是发行方 + sub，别的发行方下同名 sub 是另一个主体，照常登。表结构自己也守：
// 绕过登记口直接写，主体第二行与跨租户授予都进不了库。
func TestOperationDecisionGrantRoundTripsItsDecisionKind(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-07")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	decision := func(kind accessidentity.DecisionKind) accessidentity.OperatorGrant {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-01", subject, kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		return grant
	}

	expectOutcome(t, "decision grant", register.grant(t, decision(accessidentity.DecisionSegmentClosure)), accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "same content replayed", register.grant(t, decision(accessidentity.DecisionSegmentClosure)), accessidentity.OperatorRegistrationAlreadyRegistered)
	expectOutcome(t, "same grant, another kind", register.grant(t, decision(accessidentity.DecisionLoadAssignment)), accessidentity.OperatorRegistrationContentConflict)

	standing := register.standing(t, subject)
	at := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionSegmentClosure, at) || standing.HoldsDecisionAt(accessidentity.DecisionLoadAssignment, at) {
		t.Fatalf("decision kind did not round-trip: grants = %+v", standing.Grants())
	}
}

func TestSubjectBoundToOneTenantIsRefusedByEveryOtherTenant(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-01")

	expectOutcome(t, "甲租户首登", register.bind(t, bindingOf(t, subject, tenantOne, "SYN-OPERATOR-BASIS-01")),
		accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "乙租户登同一主体", register.bind(t, bindingOf(t, subject, tenantTwo, "SYN-OPERATOR-BASIS-01")),
		accessidentity.OperatorSubjectBoundToAnotherTenant)
	expectOutcome(t, "乙租户授甲的主体",
		register.grant(t, grantOf(t, tenantTwo, "SYN-GRANT-01", subject,
			accessidentity.CapabilityRegistryConfigurationWrite, grantStartsAt, time.Time{})),
		accessidentity.OperatorNotRegistered)

	standing := register.standing(t, subject)
	if standing.Binding().TenantID() != tenantOne || len(standing.Grants()) != 0 {
		t.Fatalf("册上现状 = %+v, want 仍只绑甲租户、无授予", standing)
	}

	elsewhere := subjectOf(t, "https://syn-issuer-02.example.invalid", "SYN-OPERATOR-01")
	expectOutcome(t, "别的发行方下同名 sub", register.bind(t, bindingOf(t, elsewhere, tenantTwo, "SYN-OPERATOR-BASIS-02")),
		accessidentity.OperatorRegistrationRecorded)

	expectSQLState(t, register.pool, "主体第二行", uniqueViolation,
		`INSERT INTO access_identity.operator (issuer, subject, tenant_id, basis_ref) VALUES ($1, $2, $3, 'SYN-X')`,
		syntheticIssuer, "SYN-OPERATOR-01", tenantTwo)
	expectSQLState(t, register.pool, "跨租户授予", foreignKeyViolation,
		`INSERT INTO access_identity.operator_grant
			(tenant_id, grant_id, issuer, subject, capability_face, effective_starts_at, basis_ref)
		 VALUES ($1, 'SYN-GRANT-X', $2, $3, 'REGISTRY_CONFIGURATION_WRITE', now(), 'SYN-X')`,
		tenantTwo, syntheticIssuer, "SYN-OPERATOR-01")
}

// Covers: 票 operator-channel/01 完成判据「区间外不生效」——授予只在自己的区间内生效（含起点、不含终点），
// 另一格不顶替；授予同内容重放（带亚微秒的时点也认得出）答原结果、同标识异区间答冲突；不在册的
// 主体授不了。
func TestGrantIsEffectiveOnlyInsideItsInterval(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-01")
	expectOutcome(t, "登主体", register.bind(t, bindingOf(t, subject, tenantOne, "SYN-OPERATOR-BASIS-01")),
		accessidentity.OperatorRegistrationRecorded)

	endsAt := grantStartsAt.Add(30 * 24 * time.Hour)
	grant := grantOf(t, tenantOne, "SYN-GRANT-01", subject, accessidentity.CapabilityRegistryConfigurationWrite, grantStartsAt, endsAt)
	expectOutcome(t, "首授", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "重放", register.grant(t, grant), accessidentity.OperatorRegistrationAlreadyRegistered)
	expectOutcome(t, "同标识异区间",
		register.grant(t, grantOf(t, tenantOne, "SYN-GRANT-01", subject,
			accessidentity.CapabilityRegistryConfigurationWrite, grantStartsAt, time.Time{})),
		accessidentity.OperatorRegistrationContentConflict)
	expectOutcome(t, "不在册的主体",
		register.grant(t, grantOf(t, tenantOne, "SYN-GRANT-02", subjectOf(t, syntheticIssuer, "SYN-OPERATOR-GHOST"),
			accessidentity.CapabilityRegistryConfigurationWrite, grantStartsAt, time.Time{})),
		accessidentity.OperatorNotRegistered)

	standing := register.standing(t, subject)
	write := accessidentity.CapabilityRegistryConfigurationWrite
	for name, probe := range map[string]struct {
		face accessidentity.CapabilityFace
		at   time.Time
		want bool
	}{
		"起点前一刻":   {write, grantStartsAt.Add(-time.Microsecond), false},
		"起点":      {write, grantStartsAt, true},
		"终点前一刻":   {write, endsAt.Add(-time.Microsecond), true},
		"终点":      {write, endsAt, false},
		"未授予的那一格": {accessidentity.CapabilityMasterDataAndOperationsRead, grantStartsAt, false},
	} {
		if got := standing.HoldsAt(probe.face, probe.at); got != probe.want {
			t.Fatalf("%s：HoldsAt = %v, want %v", name, got, probe.want)
		}
	}
	grants := standing.Grants()
	if len(grants) != 1 || grants[0].Grant().Basis() != "SYN-GRANT-BASIS-SYN-GRANT-01" {
		t.Fatalf("册上授予 = %+v, want 只有首授那一笔", grants)
	}
}

// Covers: 票 operator-channel/01 完成判据「撤销」——撤销自撤销时刻起生效，之前照常；撤销同内容重放答原结果、
// 异时刻答冲突；撤一笔本租户册上没有的授予答未登记（别的租户拿同一个授予标识也撤不动）；撤了再授是
// 另一笔，前一笔连同撤销原样留在册上。
func TestRevocationEndsTheGrantFromItsInstantAndKeepsHistory(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-01")
	expectOutcome(t, "登主体", register.bind(t, bindingOf(t, subject, tenantOne, "SYN-OPERATOR-BASIS-01")),
		accessidentity.OperatorRegistrationRecorded)
	write := accessidentity.CapabilityRegistryConfigurationWrite
	expectOutcome(t, "授予",
		register.grant(t, grantOf(t, tenantOne, "SYN-GRANT-01", subject, write, grantStartsAt, time.Time{})),
		accessidentity.OperatorRegistrationRecorded)

	revokedAt := grantStartsAt.Add(10 * 24 * time.Hour)
	revocation := revocationOf(t, tenantOne, "SYN-GRANT-01", revokedAt, "SYN-REVOKE-BASIS-01")
	expectOutcome(t, "撤销", register.revoke(t, revocation), accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "撤销重放", register.revoke(t, revocation), accessidentity.OperatorRegistrationAlreadyRegistered)
	expectOutcome(t, "异时刻撤销",
		register.revoke(t, revocationOf(t, tenantOne, "SYN-GRANT-01", revokedAt.Add(time.Hour), "SYN-REVOKE-BASIS-01")),
		accessidentity.OperatorRegistrationContentConflict)
	expectOutcome(t, "撤不存在的授予",
		register.revoke(t, revocationOf(t, tenantOne, "SYN-GRANT-GHOST", revokedAt, "SYN-REVOKE-BASIS-01")),
		accessidentity.OperatorGrantNotRegistered)
	expectOutcome(t, "别的租户撤同一标识",
		register.revoke(t, revocationOf(t, tenantTwo, "SYN-GRANT-01", revokedAt, "SYN-REVOKE-BASIS-01")),
		accessidentity.OperatorGrantNotRegistered)

	standing := register.standing(t, subject)
	if !standing.HoldsAt(write, revokedAt.Add(-time.Microsecond)) || standing.HoldsAt(write, revokedAt) {
		t.Fatal("撤销应自撤销时刻起生效、之前照常")
	}
	recorded, revoked := standing.Grants()[0].Revocation()
	if !revoked || !recorded.RevokedAt().Equal(revokedAt.Truncate(time.Microsecond)) || recorded.Basis() != "SYN-REVOKE-BASIS-01" {
		t.Fatalf("册上撤销 = %+v, %v", recorded, revoked)
	}

	regrantedAt := revokedAt.Add(5 * 24 * time.Hour)
	expectOutcome(t, "撤了再授",
		register.grant(t, grantOf(t, tenantOne, "SYN-GRANT-02", subject, write, regrantedAt, time.Time{})),
		accessidentity.OperatorRegistrationRecorded)
	standing = register.standing(t, subject)
	if len(standing.Grants()) != 2 || standing.HoldsAt(write, regrantedAt.Add(-time.Microsecond)) || !standing.HoldsAt(write, regrantedAt) {
		t.Fatalf("再授后现状 = %+v, want 两笔在册、空档不生效、再授起点起生效", standing)
	}
}

// Covers: 票 operator-channel/01「结构与 CHECK 守上面的不变量」——绕过登记口直接写，缺件、预留格与未知格、
// 倒置区间、撤不存在的授予、一笔授予撤两次，都进不了库。
func TestOperatorRegisterTablesGuardTheInvariantsThemselves(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-01")
	expectOutcome(t, "登主体", register.bind(t, bindingOf(t, subject, tenantOne, "SYN-OPERATOR-BASIS-01")),
		accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "授予",
		register.grant(t, grantOf(t, tenantOne, "SYN-GRANT-01", subject,
			accessidentity.CapabilityRegistryConfigurationWrite, grantStartsAt, time.Time{})),
		accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "撤销",
		register.revoke(t, revocationOf(t, tenantOne, "SYN-GRANT-01", grantStartsAt.Add(time.Hour), "SYN-REVOKE-BASIS-01")),
		accessidentity.OperatorRegistrationRecorded)

	const insertOperator = `INSERT INTO access_identity.operator (issuer, subject, tenant_id, basis_ref) VALUES ($1, $2, $3, $4)`
	for name, row := range map[string][4]string{
		"空发行方":  {" ", "SYN-OPERATOR-09", tenantOne, "SYN-X"},
		"空 sub": {syntheticIssuer, "", tenantOne, "SYN-X"},
		"空租户":   {syntheticIssuer, "SYN-OPERATOR-09", " ", "SYN-X"},
		"空依据":   {syntheticIssuer, "SYN-OPERATOR-09", tenantOne, ""},
	} {
		expectSQLState(t, register.pool, "主体"+name, checkViolation, insertOperator, row[0], row[1], row[2], row[3])
	}

	const insertGrant = `INSERT INTO access_identity.operator_grant
		(tenant_id, grant_id, issuer, subject, capability_face, effective_starts_at, effective_ends_at, basis_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	for name, face := range map[string]string{"预留的治理登记": "GOVERNANCE_REGISTRATION", "未知格": "ANYTHING"} {
		expectSQLState(t, register.pool, name, checkViolation, insertGrant,
			tenantOne, "SYN-GRANT-09", syntheticIssuer, "SYN-OPERATOR-01", face, grantStartsAt, nil, "SYN-X")
	}
	expectSQLState(t, register.pool, "终点不晚于起点", checkViolation, insertGrant,
		tenantOne, "SYN-GRANT-09", syntheticIssuer, "SYN-OPERATOR-01", "REGISTRY_CONFIGURATION_WRITE",
		grantStartsAt, grantStartsAt, "SYN-X")
	expectSQLState(t, register.pool, "授予空依据", checkViolation, insertGrant,
		tenantOne, "SYN-GRANT-09", syntheticIssuer, "SYN-OPERATOR-01", "REGISTRY_CONFIGURATION_WRITE",
		grantStartsAt, nil, " ")

	const insertRevocation = `INSERT INTO access_identity.operator_grant_revocation
		(tenant_id, grant_id, revoked_at, basis_ref) VALUES ($1, $2, $3, $4)`
	expectSQLState(t, register.pool, "撤不存在的授予", foreignKeyViolation, insertRevocation,
		tenantOne, "SYN-GRANT-GHOST", grantStartsAt, "SYN-X")
	expectSQLState(t, register.pool, "一笔授予撤两次", uniqueViolation, insertRevocation,
		tenantOne, "SYN-GRANT-01", grantStartsAt.Add(2*time.Hour), "SYN-X")
	expectSQLState(t, register.pool, "撤销空依据", checkViolation, insertRevocation,
		tenantOne, "SYN-GRANT-01", grantStartsAt, "")
}

// Covers: PBC-08——三个写口只经框架事务，拿不到事务句柄时答 ErrTransactionRequired，不退回连接池
// 自动提交。
func TestOperatorRegistrationWritesRefuseToRunOutsideATransaction(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-01")
	ctx := t.Context()

	if _, err := register.registry.RegisterOperator(ctx,
		bindingOf(t, subject, tenantOne, "SYN-OPERATOR-BASIS-01")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记主体：err = %v, want ErrTransactionRequired", err)
	}
	if _, err := register.registry.RegisterGrant(ctx,
		grantOf(t, tenantOne, "SYN-GRANT-01", subject, accessidentity.CapabilityRegistryConfigurationWrite,
			grantStartsAt, time.Time{})); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记授予：err = %v, want ErrTransactionRequired", err)
	}
	if _, err := register.registry.RegisterRevocation(ctx,
		revocationOf(t, tenantOne, "SYN-GRANT-01", grantStartsAt, "SYN-REVOKE-BASIS-01")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记撤销：err = %v, want ErrTransactionRequired", err)
	}
	if _, found, err := register.registry.FindOperator(ctx, subject); found || err != nil {
		t.Fatalf("无事务登记之后：found %v, err %v; want 未落册", found, err)
	}
}

const (
	checkViolation      = "23514"
	foreignKeyViolation = "23503"
	uniqueViolation     = "23505"
)

func expectSQLState(t *testing.T, pool *pgxpool.Pool, label, state, statement string, arguments ...any) {
	t.Helper()
	_, err := pool.Exec(t.Context(), statement, arguments...)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != state {
		t.Fatalf("%s：err = %v, want SQLSTATE %s", label, err, state)
	}
}
