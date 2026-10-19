package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件在真库上钉集运导入的单元号上限（票 operator-channel/54 裁定补）：单元号恰在封装快照意图信封的上限内，开启、移入、
// 封装三行照常落地并交出快照意图；越过一个字节、恰为 Subject 上限而分区键越界、带控制字符的那一份文件在译装时整批拒，单元
// 不在册、意图零封。导入链照 buildConsolidationImporter 原样装，意图交付就是生产那一只。夹具全为合成（S 级）。

func boundedConsolidationImporter(t *testing.T) (*pgxpool.Pool, consolidationImporter, *nopostgres.ConsolidationUnits) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	importer, err := buildConsolidationImporter(db, fixedClock{at: mustInstant(t, "2026-09-08T18:00:00+08:00")})
	if err != nil {
		t.Fatalf("装配集运导入口：%v", err)
	}
	units, err := nopostgres.NewConsolidationUnits(db)
	if err != nil {
		t.Fatalf("构造集运单元库读口：%v", err)
	}
	return pool, importer, units
}

func sealedSnapshotIntents(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'node-operations.sealed-snapshot.recorded'`,
	).Scan(&count); err != nil {
		t.Fatalf("统计封装快照意图：%v", err)
	}
	return count
}

// sealedUnitTemplate 是开启、移入、封装同一个单元的三行。
func sealedUnitTemplate(unit string) []byte {
	return []byte(consolidationHeader +
		consolidationLineFor("SYN-WORK-1", "OPEN_UNIT", unit, "SYN-CAGE-1", "", "", "") +
		consolidationLineFor("SYN-WORK-2", "ADD_MEMBER", unit, "", "SYN-HU-1", "", "") +
		consolidationLineFor("SYN-WORK-3", "SEAL_UNIT", unit, "", "", "SYN-SEAL-1", "SYN-BASIS-1"))
}

// Covers: 票 operator-channel/54 裁定补「单元号恰在上限内照常封装并交出快照意图（真库）」。
func TestFrontlineConsolidationHandsOffTheSnapshotForAUnitRefRightAtTheEnvelopeLimit(t *testing.T) {
	pool, importer, units := boundedConsolidationImporter(t)

	unit := unitRefFilling(t, unitRefLimit())
	results := importConsolidationTemplate(t, importer, sealedUnitTemplate(unit))
	assertDisposition(t, results, 0, rowLanded, "OPENED")
	assertDisposition(t, results, 1, rowLanded, "MEMBER_ADDED")
	assertDisposition(t, results, 2, rowLanded, "SEALED")
	if strings.Contains(results[2].Detail, "封装快照意图未交出") {
		t.Fatalf("恰在上限内的单元没交出快照意图：%s", strings.ReplaceAll(results[2].Detail, unit, "<unitRef>"))
	}
	if sealed := findUnit(t, units, unit); !sealed.Sealed() {
		t.Fatal("恰在上限内的单元没封装")
	}
	if got := sealedSnapshotIntents(t, pool); got != 1 {
		t.Fatalf("封装快照意图 %d 封，要 1", got)
	}
}

// Covers: 票 operator-channel/54 裁定补「越界或带控制字符整批拒，报告点名 unitRef；册上不落」。修前这几份文件照常开启、
// 封装，封装行明细写「封装快照意图未交出，续办引用 …」，而重跑重发的那一封每次都被信封拒收。
func TestFrontlineConsolidationRejectsAUnitRefTheSnapshotEnvelopeCannotCarry(t *testing.T) {
	cases := []struct {
		name  string
		unit  func(t *testing.T) string
		wants string
	}{
		{"越过一个字节", func(t *testing.T) string { return unitRefFilling(t, unitRefLimit()+1) }, "请缩短单元号"},
		{"恰为 Subject 上限而分区键越界", func(t *testing.T) string { return unitRefFilling(t, eventing.MaxSubjectLength) }, "请缩短单元号"},
		{"带控制字符", func(*testing.T) string { return "SYN-CU-\a1" }, "请删掉单元号里的不可见字符"},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			pool, importer, units := boundedConsolidationImporter(t)

			unit := spec.unit(t)
			batch, err := decodeConsolidationTemplate(synTenant(t), sealedUnitTemplate(unit))
			if err == nil {
				results, _ := importConsolidation(t.Context(), batch, importer)
				sealed := results[len(results)-1]
				t.Fatalf("译装放过了信封装不下的单元号（%d 字节），封装行答：第 %d 行 → %s [%s] %s",
					len(unit), sealed.Line, sealed.Outcome, sealed.Disposition, strings.ReplaceAll(sealed.Detail, unit, "<unitRef>"))
			}
			if !errors.Is(err, ErrConsolidationTemplate) || !strings.Contains(err.Error(), "第 2 行：unitRef") || !strings.Contains(err.Error(), spec.wants) {
				t.Fatalf("报文 = %q，要带集运模板不合格、第 2 行 unitRef 与「%s」", err, spec.wants)
			}
			unitID, err := domain.NewConsolidationUnitID(unit)
			if err != nil {
				t.Fatalf("构造单元标识：%v", err)
			}
			if _, found, err := units.FindByID(t.Context(), synTenant(t), unitID); err != nil || found {
				t.Fatalf("坏行的单元落了册（读错 %v）", err)
			}
			if got := sealedSnapshotIntents(t, pool); got != 0 {
				t.Fatalf("封装快照意图 %d 封，要 0", got)
			}
		})
	}
}
