package main

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipostgres "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件证作业设备册的受控登记口与「作业事实登记」授予批（票 operator-channel/10 做什么第 1、2 条）：翻译纪律——作业范围
// 缺席、种类未知、别的格带作业范围、设备缺件、停用缺时刻在触库前拒收，一格不代填；批推进——对真库端到端跑
// device-register → device-deactivate 与带作业范围的 operator-grant，证落定、整批重放、停用生效，以及别的租户停同一
// 标识、同标识异作业范围各报请人看且册上不变。

const deviceBatch = `{
  "tenantId": "SYN-TENANT-01",
  "devices": [
    {"deviceId": "SYN-DEVICE-01", "workScope": {"kind": "NODE", "reference": "SYN-NODE-SZX-01"}, "basis": "SYN-DEVICE-BASIS-01"},
    {"deviceId": "SYN-DEVICE-02", "workScope": {"kind": "OFFSITE", "reference": "SYN-OFFSITE-SZX-01"}, "basis": "SYN-DEVICE-BASIS-02"}
  ]
}`

const deviceDeactivationBatch = `{
  "tenantId": "SYN-TENANT-01",
  "deactivations": [
    {"deviceId": "SYN-DEVICE-02", "deactivatedAt": "2026-03-01T00:00:00Z", "basis": "SYN-DEVICE-OFF-BASIS-01"}
  ]
}`

const workFactGrantBatch = `{
  "tenantId": "SYN-TENANT-01",
  "grants": [
    {"grantId": "SYN-GRANT-WF-01", "issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-01",
     "capabilityFace": "WORK_FACT_REGISTRATION", "workScope": {"kind": "NODE", "reference": "SYN-NODE-SZX-01"},
     "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-GRANT-BASIS-WF-01"}
  ]
}`

func recordedDeviceOf(t *testing.T, dsn, tenant, deviceID string) accessidentity.RecordedDevice {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池核对：%v", err)
	}
	t.Cleanup(pool.Close)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := aipostgres.NewDeviceRegistry(db)
	if err != nil {
		t.Fatalf("构造设备册：%v", err)
	}
	device, found, err := registry.FindDevice(t.Context(), tenant, deviceID)
	if err != nil || !found {
		t.Fatalf("%s/%s 在册：found %v, err %v", tenant, deviceID, found, err)
	}
	return device
}

// Covers: 票 operator-channel/10 做什么第 2 条「受控 CLI 登记」——两个子命令在真库上登设备、停用，整批重放以原结果回答；
// 停用自停用时刻起生效；别的租户停同一标识、同标识异作业范围，各报请人看（退出码 2、答复名如实回显），册上不因它们变。
func TestDeviceRegisterAndDeactivateEndToEnd(t *testing.T) {
	dsn := freshMigratedDSN(t)
	for _, step := range []struct {
		name, subcommand, body string
	}{
		{"登设备", "device-register", deviceBatch},
		{"停用", "device-deactivate", deviceDeactivationBatch},
	} {
		path := batchFile(t, step.body)
		if code, out := runCLI(t, dsn, step.subcommand, "-input", path); code != exitLanded || !strings.Contains(out, "RECORDED") {
			t.Fatalf("%s：exit = %d, want %d 且回显 RECORDED", step.name, code, exitLanded)
		}
		if code, out := runCLI(t, dsn, step.subcommand, "-input", path); code != exitLanded || !strings.Contains(out, "ALREADY_REGISTERED") {
			t.Fatalf("%s重放：exit = %d, want %d 且回显 ALREADY_REGISTERED", step.name, code, exitLanded)
		}
	}

	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	node := recordedDeviceOf(t, dsn, "SYN-TENANT-01", "SYN-DEVICE-01")
	if scope := node.Registration().WorkScope(); scope.Kind() != accessidentity.WorkScopeNode || scope.Reference() != "SYN-NODE-SZX-01" ||
		!node.InUseAt(march.Add(1000*time.Hour)) {
		t.Fatalf("SYN-DEVICE-01 现状 = %+v, want 登在节点 SYN-NODE-SZX-01、一直在用", node)
	}
	offsite := recordedDeviceOf(t, dsn, "SYN-TENANT-01", "SYN-DEVICE-02")
	if !offsite.InUseAt(march.Add(-time.Second)) || offsite.InUseAt(march) {
		t.Fatalf("SYN-DEVICE-02 现状 = %+v, want 停用时刻前在用、起不在用", offsite)
	}

	for _, refused := range []struct {
		name, subcommand, body, outcome string
	}{
		{"别的租户停同一标识", "device-deactivate",
			`{"tenantId": "SYN-TENANT-02", "deactivations": [{"deviceId": "SYN-DEVICE-01", "deactivatedAt": "2026-03-01T00:00:00Z", "basis": "SYN-DEVICE-OFF-BASIS-09"}]}`,
			"DEVICE_NOT_REGISTERED"},
		{"同标识异作业范围", "device-register",
			`{"tenantId": "SYN-TENANT-01", "devices": [{"deviceId": "SYN-DEVICE-01", "workScope": {"kind": "OFFSITE", "reference": "SYN-NODE-SZX-01"}, "basis": "SYN-DEVICE-BASIS-01"}]}`,
			"CONTENT_CONFLICT"},
	} {
		code, out := runCLI(t, dsn, refused.subcommand, "-input", batchFile(t, refused.body))
		if code != exitAttention || !strings.Contains(out, refused.outcome) {
			t.Fatalf("%s：exit = %d, want %d 且回显 %s", refused.name, code, exitAttention, refused.outcome)
		}
	}
	if after := recordedDeviceOf(t, dsn, "SYN-TENANT-01", "SYN-DEVICE-01"); after.Registration() != node.Registration() || !after.InUseAt(march) {
		t.Fatalf("被拒之后 SYN-DEVICE-01 现状 = %+v, want 原登记、仍在用", after)
	}
}

// Covers: 票 operator-channel/10 做什么第 1 条——带作业范围的「作业事实登记」授予经 operator-grant 落册，读回只在那个作业范围上
// 持有，整格问一律答否。
func TestWorkFactRegistrationGrantBatchEndToEnd(t *testing.T) {
	dsn := freshMigratedDSN(t)
	if code, _ := runCLI(t, dsn, "operator-register", "-input", batchFile(t, operatorBatch)); code != exitLanded {
		t.Fatalf("登主体：exit = %d, want %d", code, exitLanded)
	}
	if code, out := runCLI(t, dsn, "operator-grant", "-input", batchFile(t, workFactGrantBatch)); code != exitLanded || !strings.Contains(out, "RECORDED") {
		t.Fatalf("授予：exit = %d, want %d 且回显 RECORDED", code, exitLanded)
	}

	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	scope := func(reference string) accessidentity.WorkScope {
		value, err := accessidentity.NewWorkScope(accessidentity.WorkScopeNode, reference)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	standing := standingOf(t, dsn, "SYN-OPERATOR-01")
	if !standing.HoldsWorkFactAt(scope("SYN-NODE-SZX-01"), march) || standing.HoldsWorkFactAt(scope("SYN-NODE-SIN-01"), march) ||
		standing.HoldsAt(accessidentity.CapabilityWorkFactRegistration, march) {
		t.Fatalf("SYN-OPERATOR-01 现状 = %+v, want 只在节点 SYN-NODE-SZX-01 上持有作业事实登记", standing)
	}
}

// Covers: 作业范围与设备两种批的缺件与形状错在触库之前拒收，绝不代填——没有缺省作业范围、没有缺省停用时刻、没有空依据。
func TestWorkFactAndDeviceBatchTranslationRefusesBeforeTouchingTheDatabase(t *testing.T) {
	grantItem := func(rest string) string {
		return `{"tenantId": "SYN-TENANT-01", "grants": [{"grantId": "SYN-GRANT-WF-01", "issuer": "` + syntheticIssuer +
			`", "subject": "SYN-OPERATOR-01", ` + rest + `}]}`
	}
	const startsAndBasis = `"effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "b"`
	deviceItem := func(fields string) string {
		return `{"tenantId": "SYN-TENANT-01", "devices": [{` + fields + `}]}`
	}
	const nodeScope = `"workScope": {"kind": "NODE", "reference": "SYN-NODE-SZX-01"}`
	for name, body := range map[string]string{
		"作业事实登记授予批": workFactGrantBatch,
	} {
		if _, err := grantBatchFromJSON([]byte(body)); err != nil {
			t.Fatalf("基准%s应译得出，否则下面的拒收证不了各自那一处：%v", name, err)
		}
	}
	if _, err := deviceBatchFromJSON([]byte(deviceBatch)); err != nil {
		t.Fatalf("基准设备批应译得出：%v", err)
	}
	if _, err := deviceDeactivationBatchFromJSON([]byte(deviceDeactivationBatch)); err != nil {
		t.Fatalf("基准停用批应译得出：%v", err)
	}

	cases := map[string]struct {
		translate func([]byte) error
		body      string
	}{
		"作业事实登记缺作业范围":  {grants, grantItem(`"capabilityFace": "WORK_FACT_REGISTRATION", ` + startsAndBasis)},
		"作业事实登记未知范围种类": {grants, grantItem(`"capabilityFace": "WORK_FACT_REGISTRATION", "workScope": {"kind": "ZONE", "reference": "SYN-ZONE-01"}, ` + startsAndBasis)},
		"作业事实登记空范围引用":  {grants, grantItem(`"capabilityFace": "WORK_FACT_REGISTRATION", "workScope": {"kind": "NODE", "reference": " "}, ` + startsAndBasis)},
		"作业范围未知字段":     {grants, grantItem(`"capabilityFace": "WORK_FACT_REGISTRATION", "workScope": {"kind": "NODE", "reference": "SYN-NODE-SZX-01", "parent": "SYN-HUB"}, ` + startsAndBasis)},
		"别的格带作业范围":     {grants, grantItem(`"capabilityFace": "REGISTRY_CONFIGURATION_WRITE", ` + nodeScope + `, ` + startsAndBasis)},
		"作业事实登记带种类":    {grants, grantItem(`"capabilityFace": "WORK_FACT_REGISTRATION", ` + nodeScope + `, "decisionKind": "SEGMENT_CLOSURE", ` + startsAndBasis)},
		"运营决定带作业范围":    {grants, grantItem(`"capabilityFace": "OPERATION_DECISION", "decisionKind": "SEGMENT_CLOSURE", ` + nodeScope + `, ` + startsAndBasis)},
		"设备批缺租户":       {devices, `{"devices": [{"deviceId": "d", ` + nodeScope + `, "basis": "b"}]}`},
		"设备批空数组":       {devices, `{"tenantId": "SYN-TENANT-01", "devices": []}`},
		"设备缺标识":        {devices, deviceItem(nodeScope + `, "basis": "b"`)},
		"设备缺作业范围":      {devices, deviceItem(`"deviceId": "d", "basis": "b"`)},
		"设备未知范围种类":     {devices, deviceItem(`"deviceId": "d", "workScope": {"kind": "ZONE", "reference": "z"}, "basis": "b"`)},
		"设备缺依据":        {devices, deviceItem(`"deviceId": "d", ` + nodeScope)},
		"设备未知字段":       {devices, deviceItem(`"deviceId": "d", ` + nodeScope + `, "basis": "b", "serialNumber": "SN-1"`)},
		"停用缺时刻":        {deactivations, `{"tenantId": "SYN-TENANT-01", "deactivations": [{"deviceId": "d", "basis": "b"}]}`},
		"停用缺依据":        {deactivations, `{"tenantId": "SYN-TENANT-01", "deactivations": [{"deviceId": "d", "deactivatedAt": "2026-03-01T00:00:00Z"}]}`},
		"停用批未知字段":      {deactivations, `{"tenantId": "SYN-TENANT-01", "deactivations": [{"deviceId": "d", "deactivatedAt": "2026-03-01T00:00:00Z", "basis": "b", "cause": "lost"}]}`},
		"停用批空数组":       {deactivations, `{"tenantId": "SYN-TENANT-01", "deactivations": []}`},
	}
	for name, probe := range cases {
		if err := probe.translate([]byte(probe.body)); err == nil {
			t.Fatalf("%s：翻译应在触库前拒收", name)
		}
	}
}

func devices(raw []byte) error       { _, err := deviceBatchFromJSON(raw); return err }
func deactivations(raw []byte) error { _, err := deviceDeactivationBatchFromJSON(raw); return err }
