package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	noidentity "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件钉前线导入路上意图入队失败的去向（票 operator-channel/45 裁定二）：导入报告有人读，编排吞成的续办引用写进本行
// 明细，收寄照常提交；重跑同一份文件答重放，重发同一份意图（ADR-0043 决定三）。设备收寄路在同样的失败上整笔回滚，两条
// 路为什么分开见 parcel-api 的 transactionalReception。编排照 buildIntakeImporter 的同一套件装，只把意图交付换成能
// 关掉的那一只。夹具全为合成（S 级）。

// errImportHandoffDown 是注入的入队失败：Go 层错误、不碰库，编排吞成续办的正是这一种。
var errImportHandoffDown = errors.New("SYN 注入：意图入队失败")

// switchableIntakeHandoff 关着时答注入的失败，开着时转给生产的意图交付。
type switchableIntakeHandoff struct {
	inner ports.NodeIntakeHandoff
	down  bool
}

func (handoff *switchableIntakeHandoff) HandOffNodeIntake(ctx context.Context, intent ports.NodeIntakeHandoffIntent) error {
	if handoff.down {
		return errImportHandoffDown
	}
	return handoff.inner.HandOffNodeIntake(ctx, intent)
}

// importerWithSwitchableHandoff 与 buildIntakeImporter 同一套件（收寄库、结果版本签发、Outbox 意图交付、服务结果读口、交付范围证据），
// 身份核对用能解析的替身让 `RECEIVED` 行形成收寄——只有形成了的收寄才交意图。意图交付起初关着。
func importerWithSwitchableHandoff(t *testing.T) (*pgxpool.Pool, intakeImporter, *switchableIntakeHandoff, *nopostgres.Receptions) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	clock := fixedClock{at: mustInstant(t, "2026-09-08T12:00:00+08:00")}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("收寄库：%v", err)
	}
	versions, err := noidentity.NewIntakeResultVersions()
	if err != nil {
		t.Fatalf("收寄结果版本签发：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("outbox 存储：%v", err)
	}
	downstream, err := nopostgres.NewOutboxNodeIntakeHandoff(db, store, clock)
	if err != nil {
		t.Fatalf("收寄意图交付：%v", err)
	}
	doorbell, err := nopostgres.NewOutboxIdentityRecheckDoorbell(db, store, clock)
	if err != nil {
		t.Fatalf("重核门铃：%v", err)
	}
	serviceResults, err := parcelServiceResultViewOf(db)
	if err != nil {
		t.Fatalf("服务结果读口：%v", err)
	}
	association, err := domain.NewParcelAssociationReference("SYN-PARCEL-0001")
	if err != nil {
		t.Fatalf("构造包裹关联：%v", err)
	}
	scope, err := nopostgres.NewDeliveryScopeEvidence(db)
	if err != nil {
		t.Fatalf("交付范围证据：%v", err)
	}
	switchable := &switchableIntakeHandoff{inner: downstream, down: true}
	handler, err := application.NewReceiveDeliveredUnitHandler(application.ReceiveDeliveredUnitDeps{
		Identity:       resolvingParcelIdentityView{association: association},
		ServiceResults: serviceResults,
		Receptions:     receptions,
		Versions:       versions,
		Downstream:     switchable,
		Doorbell:       doorbell,
		Scope:          scope,
		Clock:          clock,
	})
	if err != nil {
		t.Fatalf("收寄编排：%v", err)
	}
	return pool, intakeImporter{handler: handler, transactor: db.Transactor()}, switchable, receptions
}

func importedNodeIntakeIntents(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'node-operations.node-intake.formed'`,
	).Scan(&count); err != nil {
		t.Fatalf("统计意图：%v", err)
	}
	return count
}

// Covers: 票 operator-channel/45 完成判据「前线导入路照旧」——入队以 Go 层错误失败：`RECEIVED` 行照样落地、答 INTAKE_FORMED、
// 退出码不变，明细写明意图未交出并带续办引用；收寄已提交、意图零封。入队恢复后重跑同一份文件：那一行答重放、明细不再提
// 意图未交出，意图一封。
func TestFrontlineImportKeepsTheContinuationWhenTheIntentIsNotHandedOff(t *testing.T) {
	pool, importer, handoff, receptions := importerWithSwitchableHandoff(t)

	results := importTemplate(t, importer, sampleTemplate(t))
	assertDisposition(t, results, 0, rowLanded, "INTAKE_FORMED")
	if code := exitCodeFor(results); code != exitLanded {
		t.Fatalf("退出码 = %d，要 %d（%+v）", code, exitLanded, results)
	}
	if !strings.Contains(results[0].Detail, "意图未交出（CONT-") || !strings.Contains(results[0].Detail, "重跑同一文件会重发同一份") {
		t.Fatalf("明细没把续办写给内勤：%s", results[0].Detail)
	}
	if _, found := findReception(t, receptions, sourceIDReceived); !found {
		t.Fatalf("收寄没提交：%s", sourceIDReceived)
	}
	if got := importedNodeIntakeIntents(t, pool); got != 0 {
		t.Fatalf("意图 %d 封，要 0", got)
	}

	handoff.down = false
	replayed := importTemplate(t, importer, sampleTemplate(t))
	assertDisposition(t, replayed, 0, rowReplayed, "EXISTING_RESULT")
	if strings.Contains(replayed[0].Detail, "意图未交出") {
		t.Fatalf("入队恢复后重跑仍报意图未交出：%s", replayed[0].Detail)
	}
	if got := importedNodeIntakeIntents(t, pool); got != 1 {
		t.Fatalf("重跑后意图 %d 封，要 1——重发的是同一份", got)
	}
}
