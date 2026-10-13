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

// 本文件对真实 PostgreSQL 16 证准入范围对照册（access_identity 0005、ADR-0180 决定一、二）：登记、重放、撞键不覆盖，
// 同键生效期重叠在登记时拒，结束只增不改、首尾相接的换版登得进去，按键读回、别的租户与别的事实类型隔开，以及表结构
// 自己守得住的那几条。

var (
	mappingFrom       = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mappingSwitchover = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	loadAssignment    = accessidentity.AdmissionRequirement{
		Capability: accessidentity.CapabilityOperationDecision.String(),
		FactKind:   accessidentity.DecisionLoadAssignment.String(),
	}
	participationTermination = accessidentity.AdmissionRequirement{
		Capability: accessidentity.CapabilityOperationDecision.String(),
		FactKind:   accessidentity.DecisionParticipationTermination.String(),
	}
)

type mappingRegister struct {
	mappings   *adapter.AdmissionScopeMappings
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newMappingRegister(t *testing.T) mappingRegister {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	mappings, err := adapter.NewAdmissionScopeMappings(db)
	if err != nil {
		t.Fatalf("构造对照册：%v", err)
	}
	return mappingRegister{mappings: mappings, transactor: db.Transactor(), pool: pool}
}

func (register mappingRegister) within(
	t *testing.T,
	act func(ctx context.Context) (accessidentity.AdmissionScopeMappingOutcome, error),
) accessidentity.AdmissionScopeMappingOutcome {
	t.Helper()
	var outcome accessidentity.AdmissionScopeMappingOutcome
	if err := register.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var actErr error
		outcome, actErr = act(txCtx)
		return actErr
	}); err != nil {
		t.Fatalf("登记事务：%v", err)
	}
	return outcome
}

func (register mappingRegister) register(t *testing.T, mapping accessidentity.AdmissionScopeMapping) accessidentity.AdmissionScopeMappingOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.AdmissionScopeMappingOutcome, error) {
		return register.mappings.RegisterAdmissionScopeMapping(ctx, mapping)
	})
}

func (register mappingRegister) end(t *testing.T, end accessidentity.AdmissionScopeMappingEnd) accessidentity.AdmissionScopeMappingOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.AdmissionScopeMappingOutcome, error) {
		return register.mappings.RegisterAdmissionScopeMappingEnd(ctx, end)
	})
}

func (register mappingRegister) find(t *testing.T, tenant string, requirement accessidentity.AdmissionRequirement) []accessidentity.RecordedAdmissionScopeMapping {
	t.Helper()
	found, err := register.mappings.FindAdmissionScopeMappings(t.Context(), tenant, requirement)
	if err != nil {
		t.Fatalf("FindAdmissionScopeMappings：%v", err)
	}
	return found
}

func mappingOf(t *testing.T, tenant, id string, requirement accessidentity.AdmissionRequirement, scope string, startsAt time.Time) accessidentity.AdmissionScopeMapping {
	t.Helper()
	interval, err := accessidentity.NewEffectiveInterval(startsAt, time.Time{})
	if err != nil {
		t.Fatalf("生效区间：%v", err)
	}
	mapping, err := accessidentity.NewAdmissionScopeMapping(tenant, id, requirement, scope, interval, "SYN-BASIS-MAPPING")
	if err != nil {
		t.Fatalf("对照：%v", err)
	}
	return mapping
}

func mappingEndOf(t *testing.T, tenant, id string, at time.Time, basis string) accessidentity.AdmissionScopeMappingEnd {
	t.Helper()
	end, err := accessidentity.NewAdmissionScopeMappingEnd(tenant, id, at, basis)
	if err != nil {
		t.Fatalf("结束：%v", err)
	}
	return end
}

// Covers: ADR-0180 决定一——对照登记只增不改：同内容重放答原结果，同标识异内容答冲突，册上那一行原样不动。
func TestTheMappingRegisterRecordsReplaysAndNeverOverwrites(t *testing.T) {
	register := newMappingRegister(t)
	first := mappingOf(t, "SYN-TENANT-01", "SYN-MAP-LOAD-V1", loadAssignment, "SYN-PILOT-SCOPE/operation-decisions@v1", mappingFrom)

	if got := register.register(t, first); got != accessidentity.AdmissionScopeMappingRecorded {
		t.Fatalf("首登 = %s", got)
	}
	if got := register.register(t, first); got != accessidentity.AdmissionScopeMappingAlreadyRegistered {
		t.Fatalf("重放 = %s", got)
	}
	rescoped := mappingOf(t, "SYN-TENANT-01", "SYN-MAP-LOAD-V1", loadAssignment, "SYN-PILOT-SCOPE/operation-decisions@v2", mappingFrom)
	if got := register.register(t, rescoped); got != accessidentity.AdmissionScopeMappingContentConflict {
		t.Fatalf("同标识异内容 = %s", got)
	}

	found := register.find(t, "SYN-TENANT-01", loadAssignment)
	if len(found) != 1 || found[0].Mapping().ObjectScope() != "SYN-PILOT-SCOPE/operation-decisions@v1" {
		t.Fatalf("读回 = %+v，want 只有首登那一行", found)
	}
	if _, ended := found[0].End(); ended {
		t.Fatal("没登结束却读回了结束")
	}
}

// Covers: ADR-0180 决定二——同键（租户、能力、事实类型）的生效期重叠在登记时拒；别的事实类型、别的租户不算同键。
// 旧行在切换时刻结束之后，自同一时刻起的新行登得进去；读回两行，切换前后各解出各自的对象范围。结束只增不改：
// 重放答原结果，异时刻答冲突，册上没有的行（含别的租户拿同一标识）答未登记。
func TestOverlappingMappingsOnOneKeyAreRefusedAndASwitchoverIsNot(t *testing.T) {
	register := newMappingRegister(t)
	oldScope, newScope := "SYN-PILOT-SCOPE/operation-decisions@v1", "SYN-PILOT-SCOPE/operation-decisions@v2"
	if got := register.register(t, mappingOf(t, "SYN-TENANT-01", "SYN-MAP-V1", loadAssignment, oldScope, mappingFrom)); got != accessidentity.AdmissionScopeMappingRecorded {
		t.Fatalf("旧行 = %s", got)
	}
	next := mappingOf(t, "SYN-TENANT-01", "SYN-MAP-V2", loadAssignment, newScope, mappingSwitchover)
	if got := register.register(t, next); got != accessidentity.AdmissionScopeMappingOverlaps {
		t.Fatalf("旧行未结束时登新行 = %s, want MAPPING_OVERLAPS", got)
	}
	if got := register.register(t, mappingOf(t, "SYN-TENANT-01", "SYN-MAP-TERM", participationTermination, oldScope, mappingFrom)); got != accessidentity.AdmissionScopeMappingRecorded {
		t.Fatalf("别的事实类型 = %s", got)
	}
	if got := register.register(t, mappingOf(t, "SYN-TENANT-02", "SYN-MAP-V2", loadAssignment, newScope, mappingFrom)); got != accessidentity.AdmissionScopeMappingRecorded {
		t.Fatalf("别的租户 = %s", got)
	}

	ending := mappingEndOf(t, "SYN-TENANT-01", "SYN-MAP-V1", mappingSwitchover, "SYN-BASIS-SWITCHOVER")
	if got := register.end(t, ending); got != accessidentity.AdmissionScopeMappingRecorded {
		t.Fatalf("结束旧行 = %s", got)
	}
	if got := register.end(t, ending); got != accessidentity.AdmissionScopeMappingAlreadyRegistered {
		t.Fatalf("结束重放 = %s", got)
	}
	if got := register.end(t, mappingEndOf(t, "SYN-TENANT-01", "SYN-MAP-V1", mappingSwitchover.Add(time.Hour), "SYN-BASIS-SWITCHOVER")); got != accessidentity.AdmissionScopeMappingContentConflict {
		t.Fatalf("异时刻结束 = %s", got)
	}
	if got := register.end(t, mappingEndOf(t, "SYN-TENANT-01", "SYN-MAP-NONE", mappingSwitchover, "SYN-BASIS")); got != accessidentity.AdmissionScopeMappingNotRegistered {
		t.Fatalf("结束未登的行 = %s", got)
	}
	if got := register.end(t, mappingEndOf(t, "SYN-TENANT-03", "SYN-MAP-V1", mappingSwitchover, "SYN-BASIS")); got != accessidentity.AdmissionScopeMappingNotRegistered {
		t.Fatalf("别的租户拿同一标识结束 = %s", got)
	}

	if got := register.register(t, next); got != accessidentity.AdmissionScopeMappingRecorded {
		t.Fatalf("切换时刻起的新行 = %s", got)
	}
	found := register.find(t, "SYN-TENANT-01", loadAssignment)
	if len(found) != 2 {
		t.Fatalf("读回 %d 行，want 2", len(found))
	}
	for at, want := range map[time.Time]string{
		mappingFrom:                         oldScope,
		mappingSwitchover.Add(-time.Second): oldScope,
		mappingSwitchover:                   newScope,
	} {
		scope, resolved, err := accessidentity.ResolveAdmissionObjectScope(found, at)
		if err != nil || !resolved || scope != want {
			t.Fatalf("%s 解出 %q（%v, %v），want %q", at, scope, resolved, err, want)
		}
	}
	if other := register.find(t, "SYN-TENANT-02", loadAssignment); len(other) != 1 || other[0].Mapping().ObjectScope() != newScope {
		t.Fatalf("别的租户读回 = %+v", other)
	}
	if none := register.find(t, "SYN-TENANT-01", accessidentity.AdmissionRequirement{
		Capability: accessidentity.ExternalFactAdmissionCapability,
		FactKind:   accessidentity.FactExternalFunds.String(),
	}); len(none) != 0 {
		t.Fatalf("没登的键读回 %d 行", len(none))
	}
}

// Covers: 写口不在事务里就不跑：重叠判定与写入要关在调用方的同一个事务里，事务外那一笔被 RequireExecutor 拒，册上不落行。
func TestMappingRegistrationWritesRefuseToRunOutsideATransaction(t *testing.T) {
	register := newMappingRegister(t)
	ctx := t.Context()
	if _, err := register.mappings.RegisterAdmissionScopeMapping(ctx,
		mappingOf(t, "SYN-TENANT-01", "SYN-MAP-V1", loadAssignment, "SYN-PILOT-SCOPE/x@v1", mappingFrom)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记对照：err = %v, want ErrTransactionRequired", err)
	}
	if _, err := register.mappings.RegisterAdmissionScopeMappingEnd(ctx,
		mappingEndOf(t, "SYN-TENANT-01", "SYN-MAP-V1", mappingSwitchover, "SYN-BASIS")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记结束：err = %v, want ErrTransactionRequired", err)
	}
	if found := register.find(t, "SYN-TENANT-01", loadAssignment); len(found) != 0 {
		t.Fatalf("无事务登记之后读回 %d 行，want 未落册", len(found))
	}
}

// Covers: 表结构自己守得住的几条：空格、终点不晚于起点、结束挂在不存在的对照上，绕过登记口的直插也进不去。
func TestTheMappingTablesKeepTheirInvariants(t *testing.T) {
	register := newMappingRegister(t)
	ctx := t.Context()
	for name, statement := range map[string]string{
		"blank object scope": `INSERT INTO access_identity.admission_scope_mapping
			(tenant_id, mapping_id, capability, fact_kind, object_scope, effective_starts_at, basis_ref)
			VALUES ('SYN-TENANT-01', 'SYN-MAP-X', 'OPERATION_DECISION', 'LOAD_ASSIGNMENT', ' ', now(), 'SYN-BASIS')`,
		"end before start": `INSERT INTO access_identity.admission_scope_mapping
			(tenant_id, mapping_id, capability, fact_kind, object_scope, effective_starts_at, effective_ends_at, basis_ref)
			VALUES ('SYN-TENANT-01', 'SYN-MAP-X', 'OPERATION_DECISION', 'LOAD_ASSIGNMENT', 'SYN-PILOT-SCOPE/x@v1', now(), now() - interval '1 day', 'SYN-BASIS')`,
		"end of a missing mapping": `INSERT INTO access_identity.admission_scope_mapping_end (tenant_id, mapping_id, ended_at, basis_ref)
			VALUES ('SYN-TENANT-01', 'SYN-MAP-MISSING', now(), 'SYN-BASIS')`,
	} {
		if _, err := register.pool.Exec(ctx, statement); err == nil {
			t.Fatalf("%s：直插进去了", name)
		}
	}
}
