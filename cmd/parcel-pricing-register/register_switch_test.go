package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: AT-PP-048，ADR-0352 在受控登记口上的那一面——未启用较新规范化版本单列退出码 4，不并进治理答案 2 或未决 3；开关只认
// 恰为 `true`。
func TestTheNewerCanonicalizationRefusalHasItsOwnExitCode(t *testing.T) {
	codes := map[application.RegisterPriceCardOutcome]int{
		application.PriceCardRecorded:                                    exitRegistered,
		application.PriceCardAlreadyOnRegister:                           exitRegistered,
		application.PriceCardRegistrationConflict:                        exitGovernance,
		application.PriceCardRegistrationIncomparable:                    exitGovernance,
		application.PriceCardRegistrationNotAccepted:                     exitUsage,
		application.PriceCardRegistrationUndecided:                       exitUndecided,
		application.PriceCardRegistrationNewerCanonicalizationNotEnabled: exitNotEnabled,
	}
	for outcome, want := range codes {
		if code := priceCardExitCode(outcome); code != want {
			t.Fatalf("%s 译成 %d，want %d", outcome, code, want)
		}
	}
	for value, want := range map[string]bool{"": false, "true": true, "TRUE": false, "1": false} {
		if got := newerCanonicalizationEnabled(func(string) string { return value }); got != want {
			t.Fatalf("%q 读成 %v，want %v", value, got, want)
		}
	}
}

// Covers: AT-PP-048，ADR-0352——种子 FULL-03（PPC-6）经受控登记口：开关关着退 4、册上没有这张卡；开关打开退 0 入册；基础版的种子卡
// 在开关关着时照常退 0。真库，输入是隔离合成，只记 `S`。
func TestTheControlledRegistrationOfANewerCanonicalizationWaitsForTheSwitch(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seeds := filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing")
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(seeds, name))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	newer, base := read("price-card-cn-sg-full-03.json"), read("price-card-cn-sg.json")
	off, err := assemble(db, false)
	if err != nil {
		t.Fatal(err)
	}
	on, err := assemble(db, true)
	if err != nil {
		t.Fatal(err)
	}
	run := func(wired assembly, raw []byte) (string, int) {
		return execute(t.Context(), kindPriceCard, raw, wired.cards, wired.series, wired.reviewer, wired.catalogues, db.Transactor())
	}

	if message, code := run(off, newer); code != exitNotEnabled || !strings.Contains(message, "NEWER_CANONICALIZATION_NOT_ENABLED") {
		t.Fatalf("开关关着：%q code=%d", message, code)
	}
	registration, err := domain.RehydratePriceCardRegistration(newer)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := adapter.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := cards.ReadPriceCardContent(t.Context(), registration.Tenant(), registration.Plan().Reference()); err != nil || found {
		t.Fatalf("开关关着时册上不该有这张卡：found=%v err=%v", found, err)
	}
	if message, code := run(on, newer); code != exitRegistered || !strings.Contains(message, "RECORDED") {
		t.Fatalf("开关开着：%q code=%d", message, code)
	}
	if message, code := run(off, base); code != exitRegistered || !strings.Contains(message, "RECORDED") {
		t.Fatalf("基础版的卡在开关关着时：%q code=%d", message, code)
	}
}
