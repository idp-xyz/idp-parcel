package main

import (
	"testing"
	"testing/fstest"
)

func fixture() fstest.MapFS {
	file := func(body string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(body)} }
	return fstest.MapFS{
		"internal/parcelshipment/domain/parcel.go":                       file("package domain"),
		"internal/parcelshipment/domain/parcel_test.go":                  file("package domain"),
		"internal/parcelshipment/application/submit.go":                  file("package application"),
		"internal/parcelshipment/adapters/postgres/store.go":             file("package postgres"),
		"internal/parcelshipment/adapters/postgres/submitted_handoff.go": file("package postgres\n\ntype OutboxSubmittedHandoff struct{}\n"),
		"internal/parcelshipment/adapters/http/submit_handler.go":        file("package http"),
		"internal/parcelshipment/adapters/partycommercial/basis.go":      file("package partycommercial"),
		"internal/parcelshipment/adapters/partycommercial/as_of.go":      file("package partycommercial"),
		"internal/parcelshipment/adapters/settlementaccounting/hold.go":  file("package settlementaccounting"),
		"internal/partycommercial/domain/product.go":                     file("package domain"),
		"internal/settlementaccounting/domain/ledger.go":                 file("package domain"),
		"internal/platform/outboxintent/enqueue.go":                      file("package outboxintent"),
		"internal/platform/outboxintent/enqueue_test.go":                 file("package outboxintent"),
		"cmd/parcel-api/main.go":                                         file("package main"),
		"cmd/parcel-api/assemble_test.go":                                file("package main"),
		"migrations/parcel_shipment/0001_source.sql":                     file(""),
		"migrations/parcel_shipment/0002_request.sql":                    file(""),
		"migrations/party_commercial/0001_publication.sql":               file(""),
		"migrations/party_commercial/README.md":                          file(""),
	}
}

func find(t *testing.T, census FileCensus, name string) ContextCount {
	t.Helper()
	for _, ctx := range census.Contexts {
		if ctx.Name == name {
			return ctx
		}
	}
	t.Fatalf("清点结果里没有上下文 %q", name)
	return ContextCount{}
}

func TestCensusCountsProductionAndTestSeparately(t *testing.T) {
	census, err := CensusFiles(fixture())
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}

	ps := find(t, census, "parcelshipment")
	if ps.Production != 8 {
		t.Errorf("parcelshipment 生产文件 = %d，想要 8", ps.Production)
	}
	if ps.Test != 1 {
		t.Errorf("parcelshipment 测试文件 = %d，想要 1", ps.Test)
	}
	if ps.Application != 1 {
		t.Errorf("应用编排 = %d，想要 1", ps.Application)
	}
	if ps.Postgres != 2 {
		t.Errorf("postgres 适配器 = %d，想要 2", ps.Postgres)
	}
	if ps.HTTP != 1 {
		t.Errorf("http 适配器 = %d，想要 1", ps.HTTP)
	}
}

// Outbox 投递按类型声明认。同目录下另一个文件只是普通仓储，不能因为挨着就被算进去。
func TestOutboxHandoffCountedByTypeDeclaration(t *testing.T) {
	census, err := CensusFiles(fixture())
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}
	if got := find(t, census, "parcelshipment").OutboxHandoff; got != 1 {
		t.Errorf("Outbox 投递适配器 = %d，想要 1", got)
	}
}

// 提到名字不算声明——否则一份引用了投递类型的装配文件会被计成第二个适配器。
func TestOutboxHandoffIgnoresMereMention(t *testing.T) {
	fs := fixture()
	fs["internal/parcelshipment/adapters/postgres/wiring.go"] = &fstest.MapFile{
		Data: []byte("package postgres\n\nvar _ = OutboxSubmittedHandoff{}\n"),
	}
	census, err := CensusFiles(fs)
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}
	if got := find(t, census, "parcelshipment").OutboxHandoff; got != 1 {
		t.Errorf("Outbox 投递适配器 = %d，想要 1（引用不是声明）", got)
	}
}

// 缝按「提供方也是一个业务上下文」认。postgres 与 http 是技术适配器，同层但不成缝。
func TestCrossSeamsExcludeTechnicalAdapters(t *testing.T) {
	census, err := CensusFiles(fixture())
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}
	groups, files := census.CrossSeamTotals()
	if groups != 2 {
		t.Errorf("缝组数 = %d，想要 2（partycommercial 与 settlementaccounting 各一组）", groups)
	}
	if files != 3 {
		t.Errorf("缝内文件数 = %d，想要 3", files)
	}
	for _, seam := range census.CrossSeams {
		if seam.Provider == "postgres" || seam.Provider == "http" {
			t.Errorf("技术适配器 %q 被当成了跨上下文缝", seam.Provider)
		}
	}
}

// 同一个消费方对两个提供方是两组缝，不是一组。按消费方去重会把这一格数错。
func TestCrossSeamsGroupByConsumerProviderPair(t *testing.T) {
	census, err := CensusFiles(fixture())
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}
	seen := map[string]int{}
	for _, seam := range census.CrossSeams {
		if seam.Consumer != "parcelshipment" {
			continue
		}
		seen[seam.Provider] = seam.Files
	}
	if seen["partycommercial"] != 2 {
		t.Errorf("→partycommercial 文件数 = %d，想要 2", seen["partycommercial"])
	}
	if seen["settlementaccounting"] != 1 {
		t.Errorf("→settlementaccounting 文件数 = %d，想要 1", seen["settlementaccounting"])
	}
}

// adapters 与目标名必须紧邻。一个不在 adapters 下、只是碰巧叫 postgres 的目录不算适配器。
func TestAdapterMatchRequiresAdjacency(t *testing.T) {
	fs := fixture()
	fs["internal/partycommercial/postgres/helper.go"] = &fstest.MapFile{Data: []byte("package postgres")}
	census, err := CensusFiles(fs)
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}
	if got := find(t, census, "partycommercial").Postgres; got != 0 {
		t.Errorf("partycommercial postgres 适配器 = %d，想要 0（该目录不在 adapters 下）", got)
	}
}

func TestNonBusinessDirsAreMarkedButStillCounted(t *testing.T) {
	census, err := CensusFiles(fixture())
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}
	platform := find(t, census, "platform")
	if platform.Business {
		t.Error("platform 应被标为非业务目录")
	}
	if platform.Production != 1 || platform.Test != 1 {
		t.Errorf("platform 生产/测试 = %d/%d，想要 1/1", platform.Production, platform.Test)
	}
	if got := len(census.BusinessContexts()); got != 3 {
		t.Errorf("业务上下文数 = %d，想要 3", got)
	}
	if total := census.Totals(); total.Production != 11 {
		t.Errorf("合计生产文件 = %d，想要 11（含非业务目录）", total.Production)
	}
}

func TestCmdAndMigrationsCounted(t *testing.T) {
	census, err := CensusFiles(fixture())
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}
	if census.CmdProd != 1 || census.CmdTest != 1 {
		t.Errorf("cmd 生产/测试 = %d/%d，想要 1/1", census.CmdProd, census.CmdTest)
	}
	if len(census.Migrations) != 2 {
		t.Fatalf("迁移模块数 = %d，想要 2", len(census.Migrations))
	}
	for _, m := range census.Migrations {
		switch m.Name {
		case "parcel_shipment":
			if m.Files != 2 {
				t.Errorf("parcel_shipment 迁移 = %d，想要 2", m.Files)
			}
		case "party_commercial":
			if m.Files != 1 {
				t.Errorf("party_commercial 迁移 = %d，想要 1（README.md 不是迁移）", m.Files)
			}
		}
	}
}
