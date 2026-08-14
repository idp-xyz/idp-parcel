package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证委托聚合仓储的行为：全聚合快照往返不丢字段、作用域
// 隔离由 SQL 条件承担、重复建单由主键拦住、并发保存由乐观版本拦住、重建门只开到
// `已提交`（ADR-0030）由读回侧如实透出。

func TestASubmittedRequestRoundTripsWholly(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	original := submittedShipmentRequest(t, "req-key-1", "request-1")
	mustInsert(t, transactor, ctx, repository, original)

	found, exists, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if err != nil {
		t.Fatalf("取回委托：%v", err)
	}
	if !exists {
		t.Fatal("已建单的委托读不回来")
	}
	if found.Revision() != 1 {
		t.Fatalf("revision = %d, want 1（Insert 写 1）", found.Revision())
	}
	if found.ShipmentRequestID() != original.ShipmentRequestID() ||
		found.BatchID() != original.BatchID() ||
		found.State() != original.State() ||
		!found.SubmittedAt().Equal(original.SubmittedAt()) {
		t.Fatalf("委托身份/状态往返变形：%+v", found)
	}

	foundVersion, originalVersion := found.CurrentSubmissionVersion(), original.CurrentSubmissionVersion()
	if foundVersion.VersionID() != originalVersion.VersionID() ||
		foundVersion.SourceSubmission() != originalVersion.SourceSubmission() ||
		len(foundVersion.DeclaredParcelIDs()) != 2 {
		t.Fatalf("提交版本往返变形：%+v", foundVersion)
	}
	// 画像（重量+外廓）随版本往返：丢了它，估价装配从此永远停在缺输入。
	profile, present := foundVersion.ProfileFor(mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !present {
		t.Fatal("成员声明画像没有读回来")
	}
	if profile.Measurement().Weight().Value().String() != "2.5" {
		t.Fatalf("weight = %q, want 2.5", profile.Measurement().Weight().Value())
	}
	if _, declared := profile.Measurement().Dimensions(); !declared {
		t.Fatal("外廓没有读回来")
	}

	foundTask, originalTask := found.AcceptanceDecisionTask(), original.AcceptanceDecisionTask()
	if foundTask.TaskID() != originalTask.TaskID() ||
		foundTask.SubmissionVersionID() != originalTask.SubmissionVersionID() ||
		foundTask.IsComplete() || foundTask.IsStopped() {
		t.Fatalf("接受判断任务往返变形：%+v", foundTask)
	}
}

// TestHistoryAndAttemptsSurviveTheRoundTrip 证受控补充历史（ADR-0045）与处理记录随
// 快照往返：丢历史，一份补充过的委托重建后看起来像从未补充过；丢处理记录，「卡过几轮」
// 从头数起。
func TestHistoryAndAttemptsSurviveTheRoundTrip(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	original := submittedShipmentRequest(t, "req-key-1", "request-1")
	mustInsert(t, transactor, ctx, repository, original)

	loaded, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	withAttempt, err := loaded.RecordProcessingAttempt(processingAttempt(t))
	if err != nil {
		t.Fatalf("记处理尝试：%v", err)
	}
	// 新版本必须以自己的来源身份到达（领域守卫）：换一个来源请求键。
	superseded, err := withAttempt.FormNewSubmissionVersion(domain.NewSubmissionVersionSpec{
		VersionID:        mustBuild(t, domain.NewSubmissionVersionID, "version-2"),
		TaskID:           mustBuild(t, domain.NewAcceptanceDecisionTaskID, "task-2"),
		SourceSubmission: requestFingerprint(t, "req-key-1-supplement", "digest-2"),
		// 受控补充不改成员集合（改了要走关联新委托）：与当前版本等值的两个成员。
		DeclaredParcelIDs: []domain.DeclaredParcelID{
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"),
		},
		EstablishedAt: submittedAtFixture.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成新提交版本：%v", err)
	}

	var saved ports.ShipmentRequestSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		saved, err = repository.Save(txCtx, requestIdentity(t, "req-key-1"), superseded)
		return err
	})
	if saved != ports.ShipmentRequestSaved {
		t.Fatalf("save outcome = %s, want SAVED", saved)
	}

	found, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if err != nil {
		t.Fatalf("读回换代后的委托：%v", err)
	}
	if found.Revision() != 2 {
		t.Fatalf("revision = %d, want 2（一次保存推进一格）", found.Revision())
	}
	if found.CurrentSubmissionVersion().VersionID() != mustBuild(t, domain.NewSubmissionVersionID, "version-2") {
		t.Fatalf("当前版本 = %+v", found.CurrentSubmissionVersion().VersionID())
	}
	priors := found.PriorSubmissionVersions()
	if len(priors) != 1 || priors[0].VersionID() != mustBuild(t, domain.NewSubmissionVersionID, "version-1") {
		t.Fatalf("历史版本 = %+v；补充过的委托读回后像从未补充过", priors)
	}
	priorTasks := found.PriorAcceptanceTasks()
	if len(priorTasks) != 1 || len(priorTasks[0].ProcessingAttempts()) != 1 {
		t.Fatalf("历史任务或处理记录丢失：%+v", priorTasks)
	}
}

// TestOtherScopesAreInvisibleForRequests 证否定结果不泄露其他作用域是否存在该委托。
func TestOtherScopesAreInvisibleForRequests(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "shared-key", "request-1"))

	elsewhere := map[string]domain.SourceIdentity{
		"另一个租户":   requestScopedIdentity(t, "tenant-b", "customer-1", "shared-key"),
		"另一个客户账户": requestScopedIdentity(t, "tenant-1", "customer-b", "shared-key"),
	}
	for name, scope := range elsewhere {
		_, exists, err := repository.FindBySourceIdentity(ctx, scope)
		if err != nil {
			t.Fatalf("%s：查询出错 %v", name, err)
		}
		if exists {
			t.Errorf("%s：读到了不属于该作用域的委托", name)
		}
	}
}

// TestInsertingTwiceReportsAlreadyExists 证并发重复建单由主键拦住并译成业务答案
// （ADR-0031：`已存在`不是 error），先到者的快照原样保留。
func TestInsertingTwiceReportsAlreadyExists(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	first := submittedShipmentRequest(t, "req-key-1", "request-1")
	mustInsert(t, transactor, ctx, repository, first)

	second := submittedShipmentRequest(t, "req-key-1", "request-2")
	var outcome ports.ShipmentRequestInsertOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Insert(txCtx, requestIdentity(t, "req-key-1"), second)
		return err
	})
	if outcome != ports.ShipmentRequestAlreadyExists {
		t.Fatalf("outcome = %s, want ALREADY_EXISTS", outcome)
	}

	found, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if found.ShipmentRequestID() != first.ShipmentRequestID() {
		t.Fatal("后到者覆盖了先到者的委托")
	}
}

// TestSaveDetectsTheRevisionConflict 证并发保存由乐观版本拦住：拿着旧版本的一方零行
// 命中，译成`版本冲突`而不是覆盖赢家。
func TestSaveDetectsTheRevisionConflict(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "req-key-1", "request-1"))
	stale, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}

	// 赢家先保存一次（revision 1→2）。
	var winner ports.ShipmentRequestSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		winner, err = repository.Save(txCtx, requestIdentity(t, "req-key-1"), stale)
		return err
	})
	if winner != ports.ShipmentRequestSaved {
		t.Fatalf("winner outcome = %s", winner)
	}

	// 落败方拿着同一份 revision 1 的聚合再保存：零行命中。
	var loser ports.ShipmentRequestSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		loser, err = repository.Save(txCtx, requestIdentity(t, "req-key-1"), stale)
		return err
	})
	if loser != ports.ShipmentRequestRevisionConflict {
		t.Fatalf("loser outcome = %s, want REVISION_CONFLICT", loser)
	}
}

// TestRequestWritesRefuseToRunOutsideATransaction 证写入不会在缺少事务时改用连接池
// （`PBC-08`「不绕过 DB.RequireExecutor」的行为面）。
func TestRequestWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newShipmentRequests(t)
	ctx := t.Context()

	request := submittedShipmentRequest(t, "req-key-1", "request-1")
	if _, err := repository.Insert(ctx, requestIdentity(t, "req-key-1"), request); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务建单应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := repository.Save(ctx, requestIdentity(t, "req-key-1"), request); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestUnsupportedStatesSurfaceOnRead 证重建门只开到`已提交`这一边界在读回侧如实透出
// （ADR-0030 的登记缺口）：状态列被推进到门外取值时，Find 报错而不是拼一份缺判断产物
// 的聚合——一份空的接受基线看起来与真的一样，那正是要防的事。
func TestUnsupportedStatesSurfaceOnRead(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	ctx := t.Context()

	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "req-key-1", "request-1"))
	if _, err := pool.Exec(ctx,
		`UPDATE parcel_shipment.shipment_request SET state = $1 WHERE source_request_key = $2`,
		uint8(domain.ShipmentRequestAccepted), "req-key-1"); err != nil {
		t.Fatalf("推进状态列：%v", err)
	}

	_, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if !errors.Is(err, domain.ErrRehydrationStateNotSupported) {
		t.Fatalf("err = %v, want ErrRehydrationStateNotSupported（边界透出而不是悄悄拼聚合）", err)
	}
}

// ---- 夹具 ----

var submittedAtFixture = time.Date(2026, 9, 6, 11, 0, 0, 0, time.UTC)

func newShipmentRequests(t *testing.T) (*adapter.ShipmentRequests, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustInsert(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.ShipmentRequests,
	request domain.ShipmentRequest,
) {
	t.Helper()
	var outcome ports.ShipmentRequestInsertOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Insert(txCtx,
			request.CurrentSubmissionVersion().SourceSubmission().Identity(), request)
		return err
	})
	if outcome != ports.ShipmentRequestInserted {
		t.Fatalf("insert outcome = %s", outcome)
	}
}

func mustBuild[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func requestScopedIdentity(t *testing.T, tenant, customer, key string) domain.SourceIdentity {
	t.Helper()
	built, err := domain.NewSourceIdentity(
		mustBuild(t, domain.NewTenantID, tenant),
		mustBuild(t, domain.NewCustomerAccountID, customer),
		mustBuild(t, domain.NewSource, "portal"),
		mustBuild(t, domain.NewSourceRequestKey, key),
	)
	if err != nil {
		t.Fatalf("来源身份：%v", err)
	}
	return built
}

func requestIdentity(t *testing.T, key string) domain.SourceIdentity {
	t.Helper()
	return requestScopedIdentity(t, "tenant-1", "customer-1", key)
}

func requestFingerprint(t *testing.T, key, digest string) domain.SourceSubmissionFingerprint {
	t.Helper()
	built, err := domain.NewSourceSubmissionFingerprint(
		requestIdentity(t, key),
		mustBuild(t, domain.NewPayloadDigest, digest),
		submittedAtFixture.Add(-time.Hour),
		submittedAtFixture.Add(-time.Hour+time.Second),
	)
	if err != nil {
		t.Fatalf("来源指纹：%v", err)
	}
	return built
}

func processingAttempt(t *testing.T) domain.ProcessingAttempt {
	t.Helper()
	attempt, err := domain.NewProcessingAttempt(domain.ProcessingAttemptSpec{
		Reason:       mustBuild(t, domain.NewProcessingAttemptReason, "COMMERCIAL_BASIS_UNAVAILABLE"),
		ResumePath:   domain.ResumeByInternalRetry,
		Continuation: mustBuild(t, domain.NewOwnershipContinuationReference, "CONT-1"),
		AttemptedAt:  submittedAtFixture.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("处理记录：%v", err)
	}
	return attempt
}

// submittedShipmentRequest 经领域构造一份带两个成员、一张画像的`已提交`委托——与应用
// 测试的同名夹具同款配方，画像多带一张以证测量往返。
func submittedShipmentRequest(t *testing.T, key, requestID string) domain.ShipmentRequest {
	t.Helper()

	scope, err := domain.NewAdmissionScope(
		mustBuild(t, domain.NewAdmissionScopeReference, "scope-ref-1"),
		mustBuild(t, domain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("准入范围：%v", err)
	}
	validity, err := domain.NewOwnershipValidityInterval(
		submittedAtFixture.Add(-24*time.Hour),
		submittedAtFixture.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	decision, err := domain.NewProductionOwnershipDecision(domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustBuild(t, domain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        domain.ProductionAuthorityIDPParcel,
		AdmissionControl: domain.AdmissionControlOpen,
		RuleVersion:      mustBuild(t, domain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             submittedAtFixture,
		Validity:         validity,
		Revision:         mustBuild(t, domain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       submittedAtFixture,
	})
	if err != nil {
		t.Fatalf("归属决定：%v", err)
	}
	gate, err := domain.EvaluateFutureSubmissionGate(
		decision, scope.Digest(),
		mustBuild(t, domain.NewProductionOwnershipRevision, "rev-1"), submittedAtFixture)
	if err != nil {
		t.Fatalf("建单门禁：%v", err)
	}
	candidate, err := domain.NewSubmissionCandidate(
		requestFingerprint(t, key, "digest-1"),
		mustBuild(t, domain.NewSubmissionBatchID, "batch-1"),
		mustBuild(t, domain.NewShipmentRequestID, requestID),
		[]domain.DeclaredParcelID{
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"),
		},
	)
	if err != nil {
		t.Fatalf("提交候选：%v", err)
	}

	weight, err := domain.NewDeclaredWeight(
		mustBuild(t, domain.NewMeasurementValue, "2.5"),
		mustBuild(t, domain.NewMeasurementUnitReference, "kg"))
	if err != nil {
		t.Fatalf("申报重量：%v", err)
	}
	dimensions, err := domain.NewDeclaredDimensions(
		mustBuild(t, domain.NewMeasurementValue, "30"),
		mustBuild(t, domain.NewMeasurementValue, "20"),
		mustBuild(t, domain.NewMeasurementValue, "10"),
		mustBuild(t, domain.NewMeasurementUnitReference, "cm"))
	if err != nil {
		t.Fatalf("申报外廓：%v", err)
	}
	measurement, err := domain.NewDeclaredMeasurement(weight, dimensions)
	if err != nil {
		t.Fatalf("申报测量：%v", err)
	}
	profile, err := domain.NewDeclaredParcelProfile(
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"), measurement)
	if err != nil {
		t.Fatalf("成员画像：%v", err)
	}

	request, err := domain.SubmitShipmentRequest(domain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   mustBuild(t, domain.NewSubmissionVersionID, "version-1"),
		TaskID:      mustBuild(t, domain.NewAcceptanceDecisionTaskID, "task-1"),
		SubmittedAt: submittedAtFixture,
		Profiles:    []domain.DeclaredParcelProfile{profile},
	})
	if err != nil {
		t.Fatalf("建单：%v", err)
	}
	return request
}
