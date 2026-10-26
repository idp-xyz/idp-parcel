package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证资料修订编排的重放（ADR-0256 决定一第 2 格「重放须接着办」，UC-PS-002「未决」一行）：
// 来源保全自成一笔事务（preservationBoundary），版本落库是另一笔（amendmentBoundary），所以停在版本落库之前的各格——
// 授权未决、允许声明未声明答等复核、保存失败、并发抢先——来源已落库而委托上没有这次请求的版本。重放同一请求要按首次
// 请求接着办形成版本并入队恰好一封意图，且只形成一次；再重放才是已有结果，带那份版本。
// 装配与 authorizedAmendmentAssembly 同形（矩阵读法、阶段读面、仓储、交接壳都是生产件），只有授权口是替身。

// scriptedAmendmentAuthorizer 是资料修订授权替身（隔离合成 `S`）：能拨成答不出，放行时同 grantingAmendmentAuthorizer。
type scriptedAmendmentAuthorizer struct {
	granting grantingAmendmentAuthorizer
	err      error
	calls    atomic.Int32
}

func (double *scriptedAmendmentAuthorizer) AuthorizeSourceDataAmendment(
	ctx context.Context,
	query ports.SourceDataAmendmentAuthorizationQuery,
) (ports.SourceDataAmendmentAuthorization, error) {
	double.calls.Add(1)
	if double.err != nil {
		return ports.SourceDataAmendmentAuthorization{}, double.err
	}
	return double.granting.AuthorizeSourceDataAmendment(ctx, query)
}

var _ ports.SourceDataAmendmentAuthorizer = (*scriptedAmendmentAuthorizer)(nil)

// overtakingRules 在第一次问矩阵时让另一个连接抢先写这份委托（版本号加一）：矩阵那一问发生在编排读到委托之后、保存之前，
// 并发写入抢先在真库上就落在这个缝里。
type overtakingRules struct {
	inner    ports.SourceDataRuleDeclaration
	pool     *pgxpool.Pool
	identity domain.SourceIdentity
	spent    atomic.Bool
}

func (rules *overtakingRules) DeclareSourceDataAmendment(
	ctx context.Context,
	query ports.SourceDataAmendmentQuery,
) (ports.SourceDataAmendmentAllowance, error) {
	if rules.spent.CompareAndSwap(false, true) {
		overtakeRequest(ctx, rules.pool, rules.identity)
	}
	return rules.inner.DeclareSourceDataAmendment(ctx, query)
}

var _ ports.SourceDataRuleDeclaration = (*overtakingRules)(nil)

// refuseRequestWrites 让委托表的每一次 UPDATE 都在库里被拒：这是「保存失败」在真库上的样子（存储拒写，Save 报错），
// 不是替身凭空返回一个错。allowRequestWrites 撤掉它，模拟存储恢复。
func refuseRequestWrites(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, statement := range []string{
		`CREATE FUNCTION public.syn_refuse_request_write() RETURNS trigger LANGUAGE plpgsql AS
		 $$ BEGIN RAISE EXCEPTION 'syn: storage refuses the write'; END $$`,
		`CREATE TRIGGER syn_refuse_request_write BEFORE UPDATE ON parcel_shipment.shipment_request
		 FOR EACH ROW EXECUTE FUNCTION public.syn_refuse_request_write()`,
	} {
		if _, err := pool.Exec(t.Context(), statement); err != nil {
			t.Fatalf("拒写委托表：%v", err)
		}
	}
}

func allowRequestWrites(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `DROP TRIGGER syn_refuse_request_write ON parcel_shipment.shipment_request`); err != nil {
		t.Fatalf("恢复委托表写入：%v", err)
	}
}

func allowSupplementOfTheConsigneeGroup(t *testing.T, db *bentopg.DB, rules pcdomain.CommercialVersion) {
	t.Helper()
	declareAmendmentAllowance(t, db, rules, false,
		amendmentCell(t, "SYN-GROUP-CONSIGNEE", pcdomain.DeclaredAcceptedNotYetReceived, pcdomain.DeclaredSupplementIntent, pcdomain.AmendmentAllowed),
	)
}

// amendmentReplayDatabase 备好一份已接受委托与它接受时固定的规则包闭包。declared 为真时再登记一份允许收件人资料组补充的声明。
func amendmentReplayDatabase(t *testing.T, declared bool) (*pgxpool.Pool, *bentopg.DB, pcdomain.CommercialVersion) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	acceptedOnRealAssembly(t, db)
	rules := synAdoptedRulePackage(t)
	seedAdoptedClosure(t, db, rules)
	if declared {
		allowSupplementOfTheConsigneeGroup(t, db, rules)
	}
	return pool, db, rules
}

// replayableAmendment 装一份「授权与矩阵由调用方给、其余全真」的资料修订编排。
func replayableAmendment(
	t *testing.T,
	db *bentopg.DB,
	authorizer ports.SourceDataAmendmentAuthorizer,
	wrapRules func(ports.SourceDataRuleDeclaration) ports.SourceDataRuleDeclaration,
) shipmenthttp.AmendmentHandler {
	t.Helper()
	customs, consolidation, err := buildAmendmentStageFactViews(db)
	if err != nil {
		t.Fatalf("装配阶段事实读口：%v", err)
	}
	rules, err := buildSourceDataAmendmentAllowance(db)
	if err != nil {
		t.Fatalf("装配矩阵读法：%v", err)
	}
	if wrapRules != nil {
		rules = wrapRules(rules)
	}
	amendment, err := assembleCustomerAmendmentOrchestration(db, authorizer, rules, customs, consolidation)
	if err != nil {
		t.Fatalf("装配资料修订编排：%v", err)
	}
	return amendment
}

func newScriptedAmendmentAuthorizer(t *testing.T) *scriptedAmendmentAuthorizer {
	t.Helper()
	return &scriptedAmendmentAuthorizer{granting: grantingAmendmentAuthorizer{t: t}}
}

func versionsOnTheSeededRequest(t *testing.T, db *bentopg.DB) []domain.CustomerSourceDataVersion {
	t.Helper()
	return loadSeededRequest(t, db, submissionCommand(t).Identity).CustomerSourceDataVersions()
}

// assertStoppedWithoutAVersion 钉住停下的那一轮：未决或等复核，不带版本，库里没有版本、没有意图，来源保全落下。
func assertStoppedWithoutAVersion(
	t *testing.T,
	db *bentopg.DB,
	command shipmentapp.AmendCustomerSourceDataCommand,
	stopped shipmentapp.AmendCustomerSourceDataResult,
	outcome shipmentapp.AmendmentOutcome,
	reason shipmentapp.JudgmentPendingReason,
) {
	t.Helper()
	if stopped.Outcome() != outcome || stopped.PendingReason() != reason {
		t.Fatalf("停下的那一轮 outcome = %q reason = %q, want %q / %q", stopped.Outcome(), stopped.PendingReason(), outcome, reason)
	}
	if _, present := stopped.Version(); present {
		t.Fatal("停下的那一轮带了版本")
	}
	assertNoSourceDataVersionFormed(t, db)
	if rows := preservedSourceRows(t, db, command.AmendmentIdentity); rows != 1 {
		t.Fatalf("来源保全行数 = %d, want 1——停点之后保全仍在，重放才认得出", rows)
	}
}

// assertReplayFormsTheVersionOnce 是各格共用的后半段：条件恢复之后的重放形成版本、入队恰好一封意图；之后的重放读回
// 同一份版本，版本与意图都不再多。
func assertReplayFormsTheVersionOnce(
	t *testing.T,
	db *bentopg.DB,
	amendment shipmenthttp.AmendmentHandler,
	command shipmentapp.AmendCustomerSourceDataCommand,
) {
	t.Helper()
	replayed, err := amendment.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if got := replayed.Outcome(); got != shipmentapp.AmendmentRecorded {
		t.Fatalf("重放 outcome = %q（原因 %q）, want RECORDED——保全之后没形成的版本，重放要接着办，不是答已有结果", got, replayed.PendingReason())
	}
	formed, present := replayed.Version()
	if !present {
		t.Fatal("重放形成了版本，答复却没带")
	}
	if stored := versionsOnTheSeededRequest(t, db); len(stored) != 1 || stored[0].VersionID() != formed.VersionID() {
		t.Fatalf("委托上的资料版本 = %d 份, want 恰好重放形成的那一份", len(stored))
	}
	if got := envelopeCountOfType(t, db, sourceDataVersionEnvelopeType); got != 1 {
		t.Fatalf("「资料版本已形成」意图 = %d 封, want 恰好 1", got)
	}
	if rows := preservedSourceRows(t, db, command.AmendmentIdentity); rows != 1 {
		t.Fatalf("重放之后来源保全行数 = %d, want 仍是 1", rows)
	}
	if rows := sourceObservationRows(t, db, command.AmendmentIdentity); rows != 1 {
		t.Fatalf("来源观察行数 = %d, want 1——重放追加一次观察", rows)
	}

	for round := 0; round < 2; round++ {
		again, err := amendment.Handle(t.Context(), command)
		if err != nil {
			t.Fatalf("再重放 %d：%v", round, err)
		}
		if got := again.Outcome(); got != shipmentapp.AmendmentAlreadyHandled {
			t.Fatalf("再重放 %d outcome = %q, want ALREADY_HANDLED", round, got)
		}
		version, present := again.Version()
		if !present || version.VersionID() != formed.VersionID() {
			t.Fatalf("再重放 %d 版本 = %q（present=%v）, want 重放形成的 %q", round, version.VersionID(), present, formed.VersionID())
		}
	}
	if stored := versionsOnTheSeededRequest(t, db); len(stored) != 1 {
		t.Fatalf("重复重放之后委托上的资料版本 = %d 份, want 仍是 1", len(stored))
	}
	if got := envelopeCountOfType(t, db, sourceDataVersionEnvelopeType); got != 1 {
		t.Fatalf("重复重放之后意图 = %d 封, want 仍是 1——重发的是同一份", got)
	}
}

// Covers: ADR-0256 决定一第 2 格、UC-PS-002「未决」一行——版本落库之前停下的四格各走一遍真库：授权答不出，允许声明未声明
// 答等复核，保存失败（存储拒写，`AMENDED_REQUEST_NOT_SAVED`），并发写入抢先（真库上撞出版本冲突，
// `STALE_SHIPMENT_REQUEST_REVISION`）。每一格停下时来源保全已经落库、委托上没有版本；条件恢复后重放同一请求形成版本并入队恰好
// 一封意图，再重放读回同一份版本，版本与意图都只有一份。
func TestAnAmendmentReplayedAfterAStopBeforeTheVersionIsStoredFormsItOnceOnARealDatabase(t *testing.T) {
	t.Run("授权未决", func(t *testing.T) {
		_, db, _ := amendmentReplayDatabase(t, true)
		authorizer := newScriptedAmendmentAuthorizer(t)
		amendment := replayableAmendment(t, db, authorizer, nil)
		command := amendmentCommand(t, "SYN-KEY-1-AMENDMENT")

		authorizer.err = errors.New("authority unreachable")
		stopped, err := amendment.Handle(t.Context(), command)
		if err != nil {
			t.Fatalf("停下的那一轮：%v", err)
		}
		assertStoppedWithoutAVersion(t, db, command, stopped, shipmentapp.AmendmentUndecided, shipmentapp.SourceDataAmendmentAuthorityUnavailable)

		authorizer.err = nil
		assertReplayFormsTheVersionOnce(t, db, amendment, command)
	})

	t.Run("允许声明未声明答等复核", func(t *testing.T) {
		_, db, rules := amendmentReplayDatabase(t, false)
		amendment := replayableAmendment(t, db, newScriptedAmendmentAuthorizer(t), nil)
		command := amendmentCommand(t, "SYN-KEY-1-AMENDMENT")

		stopped, err := amendment.Handle(t.Context(), command)
		if err != nil {
			t.Fatalf("停下的那一轮：%v", err)
		}
		assertStoppedWithoutAVersion(t, db, command, stopped, shipmentapp.AmendmentAwaitingReview, shipmentapp.PendingReasonNone)

		allowSupplementOfTheConsigneeGroup(t, db, rules)
		assertReplayFormsTheVersionOnce(t, db, amendment, command)
	})

	t.Run("保存失败", func(t *testing.T) {
		pool, db, _ := amendmentReplayDatabase(t, true)
		amendment := replayableAmendment(t, db, newScriptedAmendmentAuthorizer(t), nil)
		command := amendmentCommand(t, "SYN-KEY-1-AMENDMENT")

		refuseRequestWrites(t, pool)
		stopped, err := amendment.Handle(t.Context(), command)
		if err != nil {
			t.Fatalf("停下的那一轮：%v", err)
		}
		assertStoppedWithoutAVersion(t, db, command, stopped, shipmentapp.AmendmentUndecided, shipmentapp.AmendedRequestNotSaved)

		allowRequestWrites(t, pool)
		assertReplayFormsTheVersionOnce(t, db, amendment, command)
	})

	t.Run("并发写入抢先", func(t *testing.T) {
		pool, db, _ := amendmentReplayDatabase(t, true)
		identity := submissionCommand(t).Identity
		amendment := replayableAmendment(t, db, newScriptedAmendmentAuthorizer(t),
			func(inner ports.SourceDataRuleDeclaration) ports.SourceDataRuleDeclaration {
				return &overtakingRules{inner: inner, pool: pool, identity: identity}
			})
		command := amendmentCommand(t, "SYN-KEY-1-AMENDMENT")

		stopped, err := amendment.Handle(t.Context(), command)
		if err != nil {
			t.Fatalf("被抢先的那一轮：%v", err)
		}
		assertStoppedWithoutAVersion(t, db, command, stopped, shipmentapp.AmendmentUndecided, shipmentapp.StaleShipmentRequestRevision)

		assertReplayFormsTheVersionOnce(t, db, amendment, command)
	})
}

// Covers: AT-PS-017——停在未决之后，同请求身份携带不同内容仍答请求冲突，不被重放逻辑吞成接着办：不形成版本、不问授权，
// 原请求的内容摘要不变，冲突也不追加为原请求的观察。
func TestAnAmendmentWithDifferentContentAfterAStoppedRoundStillConflictsOnARealDatabase(t *testing.T) {
	_, db, _ := amendmentReplayDatabase(t, true)
	authorizer := newScriptedAmendmentAuthorizer(t)
	amendment := replayableAmendment(t, db, authorizer, nil)
	command := amendmentCommand(t, "SYN-KEY-1-AMENDMENT")

	authorizer.err = errors.New("authority unreachable")
	if _, err := amendment.Handle(t.Context(), command); err != nil {
		t.Fatalf("停下的那一轮：%v", err)
	}
	authorizer.err = nil
	asked := authorizer.calls.Load()

	conflicting := command
	conflicting.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "syn-amendment-digest-2")
	result, err := amendment.Handle(t.Context(), conflicting)
	if err != nil {
		t.Fatalf("同身份不同内容：%v", err)
	}
	if got := result.Outcome(); got != shipmentapp.AmendmentSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", got)
	}
	if authorizer.calls.Load() != asked {
		t.Fatal("请求冲突的修订还去问了授权")
	}
	assertNoSourceDataVersionFormed(t, db)
	if rows := sourceObservationRows(t, db, command.AmendmentIdentity); rows != 0 {
		t.Fatalf("来源观察行数 = %d, want 0——冲突不追加为原请求的观察", rows)
	}
}

// Covers: 「各自重复重放只形成一次」在并发下也成立——停下之后几个重放同时到达：委托行的版本闸门只放一个，其余要么读到已形成的版本
// （已有结果），要么撞版本冲突答未决；库里只有一份版本、一封意图。
func TestConcurrentReplaysOfAStoppedAmendmentFormTheVersionOnlyOnceOnARealDatabase(t *testing.T) {
	_, db, _ := amendmentReplayDatabase(t, true)
	authorizer := newScriptedAmendmentAuthorizer(t)
	amendment := replayableAmendment(t, db, authorizer, nil)
	command := amendmentCommand(t, "SYN-KEY-1-AMENDMENT")

	authorizer.err = errors.New("authority unreachable")
	if _, err := amendment.Handle(t.Context(), command); err != nil {
		t.Fatalf("停下的那一轮：%v", err)
	}
	authorizer.err = nil

	const replays = 4
	results := make([]shipmentapp.AmendCustomerSourceDataResult, replays)
	errs := make([]error, replays)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range replays {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[index], errs[index] = amendment.Handle(context.Background(), command)
		}()
	}
	close(start)
	wg.Wait()

	recorded := 0
	for index, result := range results {
		if errs[index] != nil {
			t.Fatalf("并发重放 %d：%v", index, errs[index])
		}
		switch result.Outcome() {
		case shipmentapp.AmendmentRecorded:
			recorded++
		case shipmentapp.AmendmentAlreadyHandled:
			if _, present := result.Version(); !present {
				t.Fatalf("并发重放 %d 答已有结果却没带版本", index)
			}
		case shipmentapp.AmendmentUndecided:
			if result.PendingReason() != shipmentapp.StaleShipmentRequestRevision {
				t.Fatalf("并发重放 %d 未决原因 = %q, want STALE_SHIPMENT_REQUEST_REVISION", index, result.PendingReason())
			}
		default:
			t.Fatalf("并发重放 %d outcome = %q", index, result.Outcome())
		}
	}
	if recorded != 1 {
		t.Fatalf("形成版本的重放 = %d 个, want 恰好 1", recorded)
	}
	if stored := versionsOnTheSeededRequest(t, db); len(stored) != 1 {
		t.Fatalf("委托上的资料版本 = %d 份, want 1", len(stored))
	}
	if got := envelopeCountOfType(t, db, sourceDataVersionEnvelopeType); got != 1 {
		t.Fatalf("「资料版本已形成」意图 = %d 封, want 恰好 1", got)
	}
}
