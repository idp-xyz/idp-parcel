package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	adapter "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证作业设备册与「作业事实登记」授予（access_identity 0004、ADR-0149 决定二）：登记、
// 重放、停用、按租户分开，以及表结构自己守得住那几条不变量——不靠登记口记得去查。

type deviceRegister struct {
	registry  *adapter.DeviceRegistry
	operators operatorRegister
}

func newDeviceRegister(t *testing.T) deviceRegister {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	devices, err := adapter.NewDeviceRegistry(db)
	if err != nil {
		t.Fatalf("构造设备册：%v", err)
	}
	operators, err := adapter.NewOperatorRegistry(db)
	if err != nil {
		t.Fatalf("构造操作者册：%v", err)
	}
	return deviceRegister{
		registry:  devices,
		operators: operatorRegister{registry: operators, transactor: db.Transactor(), pool: pool},
	}
}

func (register deviceRegister) within(
	t *testing.T,
	act func(ctx context.Context) (accessidentity.DeviceRegistrationOutcome, error),
) accessidentity.DeviceRegistrationOutcome {
	t.Helper()
	var outcome accessidentity.DeviceRegistrationOutcome
	if err := register.operators.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var actErr error
		outcome, actErr = act(txCtx)
		return actErr
	}); err != nil {
		t.Fatalf("登记事务：%v", err)
	}
	return outcome
}

func (register deviceRegister) register(t *testing.T, registration accessidentity.DeviceRegistration) accessidentity.DeviceRegistrationOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.DeviceRegistrationOutcome, error) {
		return register.registry.RegisterDevice(ctx, registration)
	})
}

func (register deviceRegister) deactivate(t *testing.T, deactivation accessidentity.DeviceDeactivation) accessidentity.DeviceRegistrationOutcome {
	t.Helper()
	return register.within(t, func(ctx context.Context) (accessidentity.DeviceRegistrationOutcome, error) {
		return register.registry.RegisterDeactivation(ctx, deactivation)
	})
}

func (register deviceRegister) find(t *testing.T, tenant, deviceID string) accessidentity.RecordedDevice {
	t.Helper()
	device, found, err := register.registry.FindDevice(t.Context(), tenant, deviceID)
	if err != nil || !found {
		t.Fatalf("FindDevice(%s, %s) = found %v, err %v; want 在册", tenant, deviceID, found, err)
	}
	return device
}

func scopeOf(t *testing.T, kind accessidentity.WorkScopeKind, reference string) accessidentity.WorkScope {
	t.Helper()
	scope, err := accessidentity.NewWorkScope(kind, reference)
	if err != nil {
		t.Fatalf("作业范围：%v", err)
	}
	return scope
}

func deviceOf(t *testing.T, tenant, deviceID string, scope accessidentity.WorkScope, basis string) accessidentity.DeviceRegistration {
	t.Helper()
	registration, err := accessidentity.NewDeviceRegistration(tenant, deviceID, scope, basis)
	if err != nil {
		t.Fatalf("设备：%v", err)
	}
	return registration
}

func deactivationOf(t *testing.T, tenant, deviceID string, at time.Time, basis string) accessidentity.DeviceDeactivation {
	t.Helper()
	deactivation, err := accessidentity.NewDeviceDeactivation(tenant, deviceID, at, basis)
	if err != nil {
		t.Fatalf("停用：%v", err)
	}
	return deactivation
}

func expectDeviceOutcome(t *testing.T, label string, got, want accessidentity.DeviceRegistrationOutcome) {
	t.Helper()
	if got != want {
		t.Fatalf("%s：答复 = %s, want %s", label, got, want)
	}
}

// Covers: 票 operator-channel/10 做什么「设备册：设备标识、所属租户与作业范围」——设备首登落册；同内容重放答原结果；
// 同标识异作业范围或异依据答冲突且册上那一行原样不动；别的租户登同一标识是另一台设备，照常登；不在册答 found=false 而不是 error。
func TestDeviceRegistrationRecordsReplaysAndNeverOverwrites(t *testing.T) {
	register := newDeviceRegister(t)
	node := scopeOf(t, accessidentity.WorkScopeNode, "SYN-NODE-SZX-01")
	device := deviceOf(t, tenantOne, "SYN-DEVICE-01", node, "SYN-DEVICE-BASIS-01")

	expectDeviceOutcome(t, "首登", register.register(t, device), accessidentity.DeviceRegistrationRecorded)
	expectDeviceOutcome(t, "重放", register.register(t, device), accessidentity.DeviceRegistrationAlreadyRegistered)
	expectDeviceOutcome(t, "只换范围种类",
		register.register(t, deviceOf(t, tenantOne, "SYN-DEVICE-01", scopeOf(t, accessidentity.WorkScopeOffsite, "SYN-NODE-SZX-01"), "SYN-DEVICE-BASIS-01")),
		accessidentity.DeviceRegistrationContentConflict)
	expectDeviceOutcome(t, "只换范围引用",
		register.register(t, deviceOf(t, tenantOne, "SYN-DEVICE-01", scopeOf(t, accessidentity.WorkScopeNode, "SYN-NODE-SIN-01"), "SYN-DEVICE-BASIS-01")),
		accessidentity.DeviceRegistrationContentConflict)
	expectDeviceOutcome(t, "异依据",
		register.register(t, deviceOf(t, tenantOne, "SYN-DEVICE-01", node, "SYN-DEVICE-BASIS-02")),
		accessidentity.DeviceRegistrationContentConflict)
	expectDeviceOutcome(t, "别的租户登同一标识",
		register.register(t, deviceOf(t, tenantTwo, "SYN-DEVICE-01", node, "SYN-DEVICE-BASIS-02")),
		accessidentity.DeviceRegistrationRecorded)

	recorded := register.find(t, tenantOne, "SYN-DEVICE-01")
	if recorded.Registration() != device {
		t.Fatalf("册上现状 = %+v, want 原登记", recorded.Registration())
	}
	if _, deactivated := recorded.Deactivation(); deactivated {
		t.Fatalf("未停用的设备读回带着停用")
	}
	if other := register.find(t, tenantTwo, "SYN-DEVICE-01"); other.Registration().Basis() != "SYN-DEVICE-BASIS-02" {
		t.Fatalf("乙租户的同名设备 = %+v", other.Registration())
	}
	if _, found, err := register.registry.FindDevice(t.Context(), tenantOne, "SYN-DEVICE-GHOST"); found || err != nil {
		t.Fatalf("不在册的设备：found %v, err %v; want false, nil", found, err)
	}
}

// Covers: 票 operator-channel/10 做什么「状态（登记 / 停用）」——停用自停用时刻起生效、之前照常（带亚微秒的时刻
// 重放也认得出）；停用同内容重放答原结果、异时刻答冲突；停一台本租户册上没有的设备答未登记，别的租户拿同一标识也
// 停不动。
func TestDeviceDeactivationEndsUseFromItsInstant(t *testing.T) {
	register := newDeviceRegister(t)
	node := scopeOf(t, accessidentity.WorkScopeNode, "SYN-NODE-SZX-01")
	expectDeviceOutcome(t, "登设备", register.register(t, deviceOf(t, tenantOne, "SYN-DEVICE-01", node, "SYN-DEVICE-BASIS-01")),
		accessidentity.DeviceRegistrationRecorded)

	deactivatedAt := grantStartsAt.Add(48 * time.Hour)
	deactivation := deactivationOf(t, tenantOne, "SYN-DEVICE-01", deactivatedAt, "SYN-DEVICE-OFF-BASIS-01")
	expectDeviceOutcome(t, "停用", register.deactivate(t, deactivation), accessidentity.DeviceRegistrationRecorded)
	expectDeviceOutcome(t, "停用重放", register.deactivate(t, deactivation), accessidentity.DeviceRegistrationAlreadyRegistered)
	expectDeviceOutcome(t, "异时刻停用",
		register.deactivate(t, deactivationOf(t, tenantOne, "SYN-DEVICE-01", deactivatedAt.Add(time.Hour), "SYN-DEVICE-OFF-BASIS-01")),
		accessidentity.DeviceRegistrationContentConflict)
	expectDeviceOutcome(t, "停不存在的设备",
		register.deactivate(t, deactivationOf(t, tenantOne, "SYN-DEVICE-GHOST", deactivatedAt, "SYN-DEVICE-OFF-BASIS-01")),
		accessidentity.DeviceNotRegistered)
	expectDeviceOutcome(t, "别的租户停同一标识",
		register.deactivate(t, deactivationOf(t, tenantTwo, "SYN-DEVICE-01", deactivatedAt, "SYN-DEVICE-OFF-BASIS-01")),
		accessidentity.DeviceNotRegistered)

	device := register.find(t, tenantOne, "SYN-DEVICE-01")
	stored := deactivatedAt.Truncate(time.Microsecond)
	if !device.InUseAt(stored.Add(-time.Microsecond)) || device.InUseAt(stored) {
		t.Fatal("停用应自停用时刻起生效、之前照常")
	}
	recorded, deactivated := device.Deactivation()
	if !deactivated || !recorded.DeactivatedAt().Equal(stored) || recorded.Basis() != "SYN-DEVICE-OFF-BASIS-01" {
		t.Fatalf("册上停用 = %+v, %v", recorded, deactivated)
	}
}

// Covers: 票 operator-channel/10 做什么「授予按租户 × 作业范围」——「作业事实登记」授予带着作业范围落册与读回；同内容重放答原结果，同标识
// 只换范围种类、只换范围引用各答冲突（两列各自都在比）；读回的现状只在授予的那个作业范围上答持有，整格问一律答否。
func TestWorkFactRegistrationGrantRoundTripsItsWorkScope(t *testing.T) {
	register := newDeviceRegister(t).operators
	subject := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-01")
	expectOutcome(t, "登主体", register.bind(t, bindingOf(t, subject, tenantOne, "SYN-OPERATOR-BASIS-01")),
		accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(grantStartsAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	node := scopeOf(t, accessidentity.WorkScopeNode, "SYN-NODE-SZX-01")
	offsite := scopeOf(t, accessidentity.WorkScopeOffsite, "SYN-OFFSITE-SZX-01")
	workGrant := func(scope accessidentity.WorkScope) accessidentity.OperatorGrant {
		grant, err := accessidentity.NewWorkFactRegistrationGrant(tenantOne, "SYN-GRANT-WF-01", subject, scope, interval, "SYN-GRANT-BASIS-WF")
		if err != nil {
			t.Fatal(err)
		}
		return grant
	}

	expectOutcome(t, "授予", register.grant(t, workGrant(node)), accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "重放", register.grant(t, workGrant(node)), accessidentity.OperatorRegistrationAlreadyRegistered)
	expectOutcome(t, "同标识只换范围种类",
		register.grant(t, workGrant(scopeOf(t, accessidentity.WorkScopeOffsite, "SYN-NODE-SZX-01"))),
		accessidentity.OperatorRegistrationContentConflict)
	expectOutcome(t, "同标识只换范围引用",
		register.grant(t, workGrant(scopeOf(t, accessidentity.WorkScopeNode, "SYN-NODE-SIN-01"))),
		accessidentity.OperatorRegistrationContentConflict)
	expectOutcome(t, "同标识异作业范围", register.grant(t, workGrant(offsite)), accessidentity.OperatorRegistrationContentConflict)

	standing := register.standing(t, subject)
	at := grantStartsAt.Add(time.Hour)
	if !standing.HoldsWorkFactAt(node, at) || standing.HoldsWorkFactAt(offsite, at) {
		t.Fatalf("作业范围没有原样往返：授予 = %+v", standing.Grants())
	}
	if standing.HoldsAt(accessidentity.CapabilityWorkFactRegistration, at) {
		t.Fatal("整格问答了「持有」")
	}
	if grants := standing.Grants(); len(grants) != 1 || grants[0].Grant().WorkScope() != node {
		t.Fatalf("册上授予 = %+v, want 只有首授那一笔、范围为节点", grants)
	}
}

// Covers: 票 operator-channel/10 做什么「授予按租户 × 作业范围」与「设备册：设备标识、所属租户与作业范围」的表结构一侧，
// 守法同票 operator-channel/01 做什么「结构与 CHECK 守上面的不变量」——
// 绕过登记口直接写：作业事实登记授予缺作业范围或缺引用、别的格带作业范围、未知范围种类；设备缺件、未知范围种类；停一台
// 不存在的设备、一台设备停两次、停用空依据——都进不了库。
func TestWorkFactAndDeviceTablesGuardTheInvariantsThemselves(t *testing.T) {
	register := newDeviceRegister(t)
	pool := register.operators.pool
	subject := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-01")
	expectOutcome(t, "登主体", register.operators.bind(t, bindingOf(t, subject, tenantOne, "SYN-OPERATOR-BASIS-01")),
		accessidentity.OperatorRegistrationRecorded)

	const insertGrant = `INSERT INTO access_identity.operator_grant
		(tenant_id, grant_id, issuer, subject, capability_face, effective_starts_at, basis_ref, work_scope_kind, work_scope_ref)
		VALUES ($1, 'SYN-GRANT-09', $2, $3, $4, $5, 'SYN-X', $6, $7)`
	for name, row := range map[string]struct {
		face      string
		kind, ref any
	}{
		"作业事实登记缺作业范围": {face: "WORK_FACT_REGISTRATION", kind: nil, ref: nil},
		"作业事实登记缺引用":   {face: "WORK_FACT_REGISTRATION", kind: "NODE", ref: nil},
		"作业事实登记空引用":   {face: "WORK_FACT_REGISTRATION", kind: "NODE", ref: " "},
		"未知范围种类":      {face: "WORK_FACT_REGISTRATION", kind: "ZONE", ref: "SYN-ZONE-01"},
		"别的格带作业范围":    {face: "REGISTRY_CONFIGURATION_WRITE", kind: "NODE", ref: "SYN-NODE-01"},
	} {
		expectSQLState(t, pool, name, checkViolation, insertGrant,
			tenantOne, syntheticIssuer, "SYN-OPERATOR-01", row.face, grantStartsAt, row.kind, row.ref)
	}

	const insertDevice = `INSERT INTO access_identity.device (tenant_id, device_id, work_scope_kind, work_scope_ref, basis_ref)
		VALUES ($1, $2, $3, $4, $5)`
	for name, row := range map[string][5]string{
		"设备空租户":    {" ", "SYN-DEVICE-09", "NODE", "SYN-NODE-01", "SYN-X"},
		"设备空标识":    {tenantOne, "", "NODE", "SYN-NODE-01", "SYN-X"},
		"设备空范围引用":  {tenantOne, "SYN-DEVICE-09", "NODE", " ", "SYN-X"},
		"设备空依据":    {tenantOne, "SYN-DEVICE-09", "NODE", "SYN-NODE-01", ""},
		"设备未知范围种类": {tenantOne, "SYN-DEVICE-09", "ZONE", "SYN-ZONE-01", "SYN-X"},
	} {
		expectSQLState(t, pool, name, checkViolation, insertDevice, row[0], row[1], row[2], row[3], row[4])
	}

	node := scopeOf(t, accessidentity.WorkScopeNode, "SYN-NODE-SZX-01")
	expectDeviceOutcome(t, "登设备", register.register(t, deviceOf(t, tenantOne, "SYN-DEVICE-01", node, "SYN-DEVICE-BASIS-01")),
		accessidentity.DeviceRegistrationRecorded)
	expectDeviceOutcome(t, "停用",
		register.deactivate(t, deactivationOf(t, tenantOne, "SYN-DEVICE-01", grantStartsAt, "SYN-DEVICE-OFF-BASIS-01")),
		accessidentity.DeviceRegistrationRecorded)

	const insertDeactivation = `INSERT INTO access_identity.device_deactivation (tenant_id, device_id, deactivated_at, basis_ref)
		VALUES ($1, $2, $3, $4)`
	expectSQLState(t, pool, "停不存在的设备", foreignKeyViolation, insertDeactivation,
		tenantOne, "SYN-DEVICE-GHOST", grantStartsAt, "SYN-X")
	expectSQLState(t, pool, "别的租户停同一标识", foreignKeyViolation, insertDeactivation,
		tenantTwo, "SYN-DEVICE-01", grantStartsAt, "SYN-X")
	expectSQLState(t, pool, "一台设备停两次", uniqueViolation, insertDeactivation,
		tenantOne, "SYN-DEVICE-01", grantStartsAt.Add(time.Hour), "SYN-X")
	expectSQLState(t, pool, "停用空依据", checkViolation, insertDeactivation,
		tenantOne, "SYN-DEVICE-01", grantStartsAt, " ")
}

// Covers: PBC-08——设备册两个写口只经框架事务，拿不到事务句柄时答 ErrTransactionRequired，不退回连接池自动提交。
func TestDeviceRegistrationWritesRefuseToRunOutsideATransaction(t *testing.T) {
	register := newDeviceRegister(t)
	ctx := t.Context()
	node := scopeOf(t, accessidentity.WorkScopeNode, "SYN-NODE-SZX-01")

	if _, err := register.registry.RegisterDevice(ctx,
		deviceOf(t, tenantOne, "SYN-DEVICE-01", node, "SYN-DEVICE-BASIS-01")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记设备：err = %v, want ErrTransactionRequired", err)
	}
	if _, err := register.registry.RegisterDeactivation(ctx,
		deactivationOf(t, tenantOne, "SYN-DEVICE-01", grantStartsAt, "SYN-DEVICE-OFF-BASIS-01")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记停用：err = %v, want ErrTransactionRequired", err)
	}
	if _, found, err := register.registry.FindDevice(ctx, tenantOne, "SYN-DEVICE-01"); found || err != nil {
		t.Fatalf("无事务登记之后：found %v, err %v; want 未落册", found, err)
	}
}

// Covers: 票 operator-channel/21 做什么「设备册增一个按租户列出已登记设备的读口」——按租户列出已登记设备，含已停用的；
// 别的租户的同名设备不在其中；空册答空切片；空租户拒绝。查阅行不带依据。
func TestListDevicesReturnsOnlyThatTenantsRows(t *testing.T) {
	register := newDeviceRegister(t)
	node := scopeOf(t, accessidentity.WorkScopeNode, "SYN-NODE-SZX-01")
	offsite := scopeOf(t, accessidentity.WorkScopeOffsite, "SYN-OFFSITE-SZX-01")
	expectDeviceOutcome(t, "后登的标识",
		register.register(t, deviceOf(t, tenantOne, "SYN-DEVICE-B", offsite, "SYN-DEVICE-BASIS-B")),
		accessidentity.DeviceRegistrationRecorded)
	expectDeviceOutcome(t, "先序的标识",
		register.register(t, deviceOf(t, tenantOne, "SYN-DEVICE-A", node, "SYN-DEVICE-BASIS-A")),
		accessidentity.DeviceRegistrationRecorded)
	deactivatedAt := grantStartsAt.Add(48 * time.Hour)
	expectDeviceOutcome(t, "停用 B",
		register.deactivate(t, deactivationOf(t, tenantOne, "SYN-DEVICE-B", deactivatedAt, "SYN-DEVICE-OFF-B")),
		accessidentity.DeviceRegistrationRecorded)
	expectDeviceOutcome(t, "乙租户同名",
		register.register(t, deviceOf(t, tenantTwo, "SYN-DEVICE-A", node, "SYN-DEVICE-BASIS-OTHER")),
		accessidentity.DeviceRegistrationRecorded)

	rows, err := register.registry.ListDevices(t.Context(), tenantOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].DeviceID() != "SYN-DEVICE-A" || rows[1].DeviceID() != "SYN-DEVICE-B" {
		t.Fatalf("甲租户的册 = %d 行，首尾 %v", len(rows), deviceIDs(rows))
	}
	if rows[0].Status() != accessidentity.DeviceCatalogueRegistered || rows[0].WorkScope() != node {
		t.Fatalf("A = %s %s", rows[0].Status(), rows[0].WorkScope().Kind())
	}
	if _, ok := rows[0].DeactivatedAt(); ok {
		t.Fatal("登记行带着停用时刻")
	}
	at, ok := rows[1].DeactivatedAt()
	if rows[1].Status() != accessidentity.DeviceCatalogueDeactivated || !ok || !at.Equal(deactivatedAt.Truncate(time.Microsecond)) {
		t.Fatalf("B 停用 = %s %s ok %v", rows[1].Status(), at, ok)
	}
	if rows[1].WorkScope() != offsite {
		t.Fatalf("B 的作业范围 = %s %s", rows[1].WorkScope().Kind(), rows[1].WorkScope().Reference())
	}

	other, err := register.registry.ListDevices(t.Context(), tenantTwo)
	if err != nil || len(other) != 1 || other[0].DeviceID() != "SYN-DEVICE-A" || other[0].Status() != accessidentity.DeviceCatalogueRegistered {
		t.Fatalf("乙租户的册 = %v, err %v", deviceIDs(other), err)
	}
	empty, err := register.registry.ListDevices(t.Context(), "SYN-TENANT-EMPTY")
	if err != nil || len(empty) != 0 || empty == nil {
		t.Fatalf("空册 = %#v, err %v; want 空切片", empty, err)
	}
	if _, err := register.registry.ListDevices(t.Context(), " "); !errors.Is(err, accessidentity.ErrDeviceCatalogueTenantRequired) {
		t.Fatalf("空租户 err = %v", err)
	}
}

func deviceIDs(rows []accessidentity.DeviceCatalogueRow) []string {
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.DeviceID()
	}
	return ids
}
