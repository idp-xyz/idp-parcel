package postgres_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// Covers: AT-CC-457、AT-CC-458（真库半边，ADR-0353 决定六）——登记编排经真写口与真读口：
// 名称原样落库、点读与上列都读回；同一起点重登时名称并入比对（同名`已存在`、换名与补名`内容冲突`、册面不动），没登名称
// 的版本原样重登照旧`已存在`；改名走更晚起点的新版本，前版照留旧名称。
func TestCandidatePortDisplayNamesRoundTripAndJoinTheReplayComparison(t *testing.T) {
	registry, view, fixture := newPortsPathsRegistry(t)
	catalogue, err := adapter.NewPortsPathsCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造口岸路径册列表读口：%v", err)
	}
	handler := customsapp.NewRegisterPortsPathsHandler(customsapp.RegisterPortsPathsDeps{Registry: registry, View: view})
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	command := func(port, name string, appliesFrom time.Time) customsapp.RegisterCandidatePortCommand {
		registration := customsapp.RegisterCandidatePortCommand{
			TenantID: tenant, Port: viewValue(t, domain.NewCustomsPortReference, port), AppliesFrom: appliesFrom,
		}
		if name != "" {
			registration.DisplayName = viewValue(t, domain.NewCustomsPortDisplayName, name)
		}
		return registration
	}
	register := func(registration customsapp.RegisterCandidatePortCommand) customsapp.CaseConfigurationOutcome {
		t.Helper()
		var outcome customsapp.CaseConfigurationOutcome
		if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var handleErr error
			outcome, handleErr = handler.RegisterCandidatePort(txCtx, registration)
			return handleErr
		}); err != nil {
			t.Fatalf("登记口岸：%v", err)
		}
		return outcome
	}

	renamedAt := viewBaseAt.Add(30 * 24 * time.Hour)
	steps := []struct {
		label        string
		registration customsapp.RegisterCandidatePortCommand
		want         customsapp.CaseConfigurationOutcome
	}{
		{"带名称首登", command("SYN-PORT-SZX-01", "深圳湾口岸", viewBaseAt), customsapp.ConfigurationRegistered},
		{"同名重登", command("SYN-PORT-SZX-01", "深圳湾口岸", viewBaseAt), customsapp.ConfigurationExisting},
		{"同一起点换名", command("SYN-PORT-SZX-01", "皇岗口岸", viewBaseAt), customsapp.ConfigurationContentConflict},
		{"同一起点去掉名称", command("SYN-PORT-SZX-01", "", viewBaseAt), customsapp.ConfigurationContentConflict},
		{"改名走新版本", command("SYN-PORT-SZX-01", "皇岗口岸", renamedAt), customsapp.ConfigurationRegistered},
		{"不带名称首登", command("SYN-PORT-SIN-01", "", viewBaseAt), customsapp.ConfigurationRegistered},
		{"不带名称原样重登", command("SYN-PORT-SIN-01", "", viewBaseAt), customsapp.ConfigurationExisting},
		{"给没登名称的版本补名称", command("SYN-PORT-SIN-01", "樟宜口岸", viewBaseAt), customsapp.ConfigurationContentConflict},
	}
	for _, step := range steps {
		if got := register(step.registration); got != step.want {
			t.Fatalf("%s：答 %v，想要 %v", step.label, got, step.want)
		}
	}

	entry, found, err := view.LoadCandidatePort(t.Context(), tenant,
		viewValue(t, domain.NewCustomsPortReference, "SYN-PORT-SZX-01"), viewBaseAt)
	if err != nil || !found || entry.DisplayName.String() != "深圳湾口岸" || !entry.AppliesUntil.Equal(renamedAt) {
		t.Fatalf("前版点读：found=%v err=%v entry=%+v", found, err, entry)
	}
	entries, err := rowsOf(catalogue.ListCandidatePorts(t.Context(), tenant, 10, firstPage(t, ports.CandidatePortPages)))
	if err != nil {
		t.Fatalf("上列口岸：%v", err)
	}
	var names []string
	for _, listed := range entries {
		names = append(names, listed.Port.String()+"="+listed.DisplayName.String())
	}
	want := []string{"SYN-PORT-SIN-01=", "SYN-PORT-SZX-01=深圳湾口岸", "SYN-PORT-SZX-01=皇岗口岸"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("上列名称 = %v，想要 %v", names, want)
	}
}

// Covers: AT-CC-458（适用性半边）——名称不进关务适用性判断：两个租户登同一份口岸与路径、只差
// 名称，同一候选的作答、不可用理由与目录版本引用逐格相同。
func TestCandidatePortDisplayNamesStayOutOfTheApplicabilityJudgment(t *testing.T) {
	registry, _, fixture := newPortsPathsRegistry(t)
	from := viewBaseAt
	for tenant, name := range map[string]string{"tenant-named": "深圳湾口岸", "tenant-plain": ""} {
		if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
			displayName := domain.CustomsPortDisplayName{}
			if name != "" {
				displayName = viewValue(t, domain.NewCustomsPortDisplayName, name)
			}
			return registry.RegisterCandidatePort(ctx, viewValue(t, domain.NewTenantID, tenant),
				viewValue(t, domain.NewCustomsPortReference, "SYN-PORT-01"), displayName, from)
		}); err != nil {
			t.Fatalf("登记 %s 的口岸：%v", tenant, err)
		}
		registerSynPath(t, fixture, registry, tenant, "SYN-PATH-01", "SYN-PORT-01", "SYN-MODE-GENERAL", domain.ImportManifest, from)
	}

	asOf := from.Add(time.Hour)
	named := judgeTenant(t, fixture, "tenant-named", "SYN-CAND-01", "CN", "SG", true, true, asOf)
	plain := judgeTenant(t, fixture, "tenant-plain", "SYN-CAND-01", "CN", "SG", true, true, asOf)
	if named.Outcome() != plain.Outcome() || named.UnavailableReason() != plain.UnavailableReason() ||
		!reflect.DeepEqual(named.Versions(), plain.Versions()) {
		t.Fatalf("只差名称的两份目录作答不同：带名称 %v %v %v，不带 %v %v %v",
			named.Outcome(), named.UnavailableReason(), named.Versions(), plain.Outcome(), plain.UnavailableReason(), plain.Versions())
	}
	for _, version := range named.Versions() {
		if strings.Contains(version, "深圳湾") {
			t.Fatalf("目录版本引用带进了名称：%v", named.Versions())
		}
	}
}

// Covers: AT-CC-458（迁移半边）——迁移 0031 施加在已有口岸版本的库上：不回填、不改写，
// 旧行的名称为 NULL（读成没登），其余各列逐格不变；之后库上 CHECK 作为第二道网拒空白、首尾空白、超长与控制字符。
func TestMigration0031LeavesExistingCandidatePortsUnnamedAndUnchanged(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA customs_compliance`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.CustomsCompliance()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0031_candidate_port_display_name"
	var pending []migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			pending = assets[index:]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if len(pending) == 0 {
		t.Fatalf("没有找到迁移 %s", target)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO customs_compliance.candidate_port (tenant_id, port_ref, applies_from, applies_until)
		VALUES ('SYN-TENANT-CC31', 'SYN-PORT-CC31', '2026-01-01T00:00:00Z', '2026-03-01T00:00:00Z')`); err != nil {
		t.Fatalf("落 0031 之前的口岸版本：%v", err)
	}
	readRow := func() [2]time.Time {
		var row [2]time.Time
		if err := conn.QueryRow(ctx, `SELECT applies_from, applies_until FROM customs_compliance.candidate_port
			WHERE port_ref = 'SYN-PORT-CC31'`).Scan(&row[0], &row[1]); err != nil {
			t.Fatalf("读口岸版本：%v", err)
		}
		return row
	}
	before := readRow()
	for _, asset := range pending {
		if _, err := conn.Exec(ctx, asset.SQL); err != nil {
			t.Fatalf("在有数据的库上施加 %s：%v", asset.Name, err)
		}
	}
	if after := readRow(); after != before {
		t.Fatalf("迁移改写了旧行：之前 %v，之后 %v", before, after)
	}
	var displayName *string
	if err := conn.QueryRow(ctx, `SELECT display_name FROM customs_compliance.candidate_port
		WHERE port_ref = 'SYN-PORT-CC31'`).Scan(&displayName); err != nil || displayName != nil {
		t.Fatalf("旧行的名称应为 NULL：%v %v", displayName, err)
	}

	insert := func(port, name string) error {
		_, err := conn.Exec(ctx, `INSERT INTO customs_compliance.candidate_port (tenant_id, port_ref, applies_from, display_name)
			VALUES ('SYN-TENANT-CC31', $1, '2026-01-01T00:00:00Z', $2)`, port, name)
		return err
	}
	long := strings.Repeat("岸", 101)
	for index, bad := range []string{"", "  ", " 深圳湾口岸", "深圳湾口岸\t", "深圳\n湾", long} {
		if err := insert("SYN-PORT-CC31-BAD-"+string(rune('A'+index)), bad); err == nil {
			t.Fatalf("库上 CHECK 应拒名称 %q", bad)
		}
	}
	if err := insert("SYN-PORT-CC31-OK", strings.Repeat("岸", 100)); err != nil {
		t.Fatalf("恰 100 个字符的名称应收下：%v", err)
	}
}
