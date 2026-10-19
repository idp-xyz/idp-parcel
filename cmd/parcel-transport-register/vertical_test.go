package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var (
	demoTransportSeeds = filepath.Join("..", "..", "scripts", "demo-seeds", "data", "transport")
	demoNetworkCatalog = filepath.Join("..", "..", "referenceconfig", "network-routing", "network-catalog", "SYN-CN-SG@1.json")
)

// demoTransportSeedFiles 取 data/transport 下某一种命令的输入：周期安排与物化登记的班次各按文件名里的那一段认，
// 免得把班次输入当周期安排去登。
func demoTransportSeedFiles(t *testing.T, kind string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(demoTransportSeeds, "*-"+kind+"-*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("演示种子 data/transport 下没有 %s 登记输入：%v", kind, err)
	}
	sort.Strings(files)
	return files
}

// Covers: 票 demo-chain-gaps/16 演示判据——演示种子照 seed.sh 原样登进真库：每份 SYN 周期安排各答已登记、重跑答已存在；
// 查阅口的周期安排册列出全部；按运行日期显式登一个班次答已登记、再登答已存在。夹具全是 SYN 合成值（S 级）。
func TestDemoTransportSeedsRegisterOnARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildRegistrar(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	ctx := t.Context()
	mustExecute := func(command string, raw []byte, wantCode int, wantAnswer string) {
		t.Helper()
		message, code := execute(ctx, command, raw, registrar)
		if code != wantCode || !strings.Contains(message, wantAnswer) {
			t.Fatalf("%s 答 %q（%d），要 %s（%d）", command, message, code, wantAnswer, wantCode)
		}
	}

	files := demoTransportSeedFiles(t, "schedule-template")
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		mustExecute(commandScheduleTemplate, raw, exitRegistered, "TEMPLATE_REGISTERED")
		mustExecute(commandScheduleTemplate, raw, exitRegistered, "EXISTING_TEMPLATE")
	}

	catalogue, err := tfpostgres.NewReviewCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := catalogue.ListScheduleTemplates(ctx, tenant, 50)
	if err != nil || len(rows) != len(files) {
		t.Fatalf("周期安排册 = %d 行，要 %d；%v", len(rows), len(files), err)
	}

	materialize := []byte(`{"tenantId": "SYN-TENANT-01", "template": "SYN-SCHED-SHA-SZX-DAILY", "runningDate": "2026-10-22"}`)
	mustExecute(commandSchedule, materialize, exitRegistered, "SCHEDULE_REGISTERED（SYN-SCHED-SHA-SZX-DAILY/2026-10-22 v1）")
	mustExecute(commandSchedule, materialize, exitRegistered, "EXISTING_SCHEDULE")
	mustExecute(commandSchedule, []byte(`{"tenantId": "SYN-TENANT-01", "template": "SYN-SCHED-404", "runningDate": "2026-10-21"}`),
		exitUsage, "SCHEDULE_TEMPLATE_NOT_FOUND")

	// 票 demo-chain-gaps/17 演示判据：seed.sh 显式物化登记的那一班随周期安排的逐维容量建池，查阅口 capacity-pool
	// 列出它上面的 SYN 池；重跑同一份班次与池都答已存在。
	for _, file := range demoTransportSeedFiles(t, "materialized-schedule") {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		mustExecute(commandSchedule, raw, exitRegistered, "SCHEDULE_REGISTERED")
		mustExecute(commandSchedule, raw, exitRegistered, "/WEIGHT EXISTING_POOL")
	}
	pools, err := catalogue.ListCapacityPools(ctx, tenant, 50)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]string{}
	for _, pool := range pools {
		listed[pool.PoolID] = pool.Dimension
	}
	for _, want := range []string{
		"SYN-SCHED-SHA-SZX-DAILY/2026-10-22/WEIGHT", "SYN-SCHED-SHA-SZX-DAILY/2026-10-22/PIECES",
		"SYN-SCHED-SZX-SIN-DAILY/2026-10-21/WEIGHT", "SYN-SCHED-SZX-SIN-DAILY/2026-10-21/PIECES",
	} {
		if listed[want] == "" {
			t.Fatalf("池册没有 %s：%v", want, listed)
		}
	}

	// capacity-pool 命令与在线建池口同一个编排：已登记的班次上另登一维答已建立，再登已存在，异内容池冲突退 2，
	// 指向没登过的班次退 1。
	bags := []byte(`{"tenantId": "SYN-TENANT-01", "schedule": "SYN-SCHED-SZX-SIN-DAILY/2026-10-21", "dimension": "BAG_POSITIONS",
		"unit": "BAG", "capacity": 40}`)
	mustExecute(commandCapacityPool, bags, exitRegistered, "POOL_ESTABLISHED（SYN-SCHED-SZX-SIN-DAILY/2026-10-21/BAG_POSITIONS BAG 40）")
	mustExecute(commandCapacityPool, bags, exitRegistered, "EXISTING_POOL")
	mustExecute(commandCapacityPool, []byte(strings.Replace(string(bags), "40", "41", 1)), exitConflict, "POOL_CONFLICT")
	mustExecute(commandCapacityPool, []byte(strings.Replace(string(bags), "2026-10-21", "2026-10-23", 1)), exitUsage, "SCHEDULE_NOT_FOUND")
}

// Covers: 票 demo-chain-gaps/17——演示种子的每份周期安排都逐维声明重量与件数两池：准备评估只认重量、体积、件数三维
// 有申报来源（ADR-0201 决定八），其中体积要看客户报没报外廓；只声明重量与件数两维，演示里的每一维池都有需求量可比。
func TestDemoTransportTemplatesDeclareWeightAndPiecePools(t *testing.T) {
	for _, file := range demoTransportSeedFiles(t, "schedule-template") {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var input templateInput
		if err := decodeStrict(raw, &input); err != nil {
			t.Fatalf("%s：%v", file, err)
		}
		dimensions := map[string]bool{}
		for _, declaration := range input.Capacities {
			if declaration.Capacity <= 0 {
				t.Fatalf("%s 的 %s 硬上限不为正", file, declaration.Dimension)
			}
			dimensions[declaration.Dimension] = true
		}
		if len(dimensions) != 2 || !dimensions["WEIGHT"] || !dimensions["PIECES"] {
			t.Fatalf("%s 的容量声明 = %v，要重量与件数两维", file, input.Capacities)
		}
	}
}

// Covers: 票 demo-chain-gaps/16「与演示网络的连接代码与起讫节点对上」——每条 SYN 周期安排的方向都是演示网络参考配置
// 里的一条连接（代码、起点、终点三格逐一相同），时区取那条连接的业务时区（演示约定）。连接改名或改起讫时这里先红，
// 不等准备评估在演示时静默答未配置。
func TestDemoTransportSeedsFollowTheDemoNetworkConnections(t *testing.T) {
	raw, err := os.ReadFile(demoNetworkCatalog)
	if err != nil {
		t.Fatalf("读演示网络参考配置：%v", err)
	}
	var catalog struct {
		Connections []struct {
			Code     string `json:"code"`
			From     string `json:"from_node"`
			To       string `json:"to_node"`
			Timezone string `json:"business_timezone"`
		} `json:"connections"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("解演示网络参考配置：%v", err)
	}
	covered := map[string]bool{}
	for _, file := range demoTransportSeedFiles(t, "schedule-template") {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var input templateInput
		if err := decodeStrict(raw, &input); err != nil {
			t.Fatalf("%s：%v", file, err)
		}
		matched := false
		for _, connection := range catalog.Connections {
			if connection.Code == input.Connection {
				matched = connection.From == input.FromNode && connection.To == input.ToNode && connection.Timezone == input.TimeZone
			}
		}
		if !matched {
			t.Fatalf("%s 的方向 %s %s→%s（%s）对不上演示网络的连接", file, input.Connection, input.FromNode, input.ToNode, input.TimeZone)
		}
		covered[input.Connection] = true
	}
	for _, connection := range catalog.Connections {
		if !covered[connection.Code] {
			t.Fatalf("演示网络的连接 %s 没有周期安排", connection.Code)
		}
	}
}
