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

func demoTransportSeedFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(demoTransportSeeds, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("演示种子 data/transport 下没有登记输入：%v", err)
	}
	sort.Strings(files)
	return files
}

// Covers: 票 demo-chain-gaps/16 演示判据——演示种子照 seed.sh 原样登进真库：三条 SYN 周期安排各答已登记、重跑答已存在；
// 查阅口的周期安排册列出这三条；按运行日期显式登一个班次答已登记、再登答已存在。夹具全是 SYN 合成值（S 级）。
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

	files := demoTransportSeedFiles(t)
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

	materialize := []byte(`{"tenantId": "SYN-TENANT-01", "template": "SYN-SCHED-SZX-SIN-DAILY", "runningDate": "2026-10-21"}`)
	mustExecute(commandSchedule, materialize, exitRegistered, "SCHEDULE_REGISTERED（SYN-SCHED-SZX-SIN-DAILY/2026-10-21 v1）")
	mustExecute(commandSchedule, materialize, exitRegistered, "EXISTING_SCHEDULE")
	mustExecute(commandSchedule, []byte(`{"tenantId": "SYN-TENANT-01", "template": "SYN-SCHED-404", "runningDate": "2026-10-21"}`),
		exitUsage, "SCHEDULE_TEMPLATE_NOT_FOUND")
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
	for _, file := range demoTransportSeedFiles(t) {
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
