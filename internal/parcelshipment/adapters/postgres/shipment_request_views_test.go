package postgres_test

import (
	"context"
	"slices"
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

// 本文件对真实 PostgreSQL 16 证委托查阅读面：可见性过滤在 SQL 键上、`统一不可见结果`
// 对「不存在」与「作用域外」同答、读面跨生命周期覆盖重建门外的终局状态（ADR-0061 之外
// 正是本读口存在的理由）。

func newShipmentRequestViews(t *testing.T) (*adapter.ShipmentRequestViews, *adapter.ShipmentRequests, bentoapp.Transactor) {
	t.Helper()
	return newShipmentRequestViewsOn(t, pgtest.Pool(t))
}

// newShipmentRequestViewsOn 同 newShipmentRequestViews，只是用调用方给的库：pgtest.Pool 每调一次就是一个新库，测试要在同一个库里
// 另写别的表时用它。
func newShipmentRequestViewsOn(t *testing.T, pool *pgxpool.Pool) (*adapter.ShipmentRequestViews, *adapter.ShipmentRequests, bentoapp.Transactor) {
	t.Helper()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	views, err := adapter.NewShipmentRequestViews(db)
	if err != nil {
		t.Fatalf("构造查阅读面：%v", err)
	}
	repository, err := adapter.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	return views, repository, db.Transactor()
}

// newShipmentRequestViewsWithJudgments 同 newShipmentRequestViews，另交同一个库上的判断登记册：委托详情与各委托队列的「上次处理」读自
// 处理尝试表（ADR-0383 决定四），要在读面上看见一条，得经生产写口记进那里。
func newShipmentRequestViewsWithJudgments(t *testing.T) (
	*adapter.ShipmentRequestViews, *adapter.ShipmentRequests, *adapter.AcceptanceJudgments, bentoapp.Transactor,
) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	views, err := adapter.NewShipmentRequestViews(db)
	if err != nil {
		t.Fatalf("构造查阅读面：%v", err)
	}
	repository, err := adapter.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	judgments, err := adapter.NewAcceptanceJudgments(db)
	if err != nil {
		t.Fatalf("构造判断登记册：%v", err)
	}
	return views, repository, judgments, db.Transactor()
}

// recordAttemptOnCurrentVersion 把一条处理尝试经生产写口记到这份委托此刻的当前版本上。
func recordAttemptOnCurrentVersion(
	t *testing.T,
	repository *adapter.ShipmentRequests,
	judgments *adapter.AcceptanceJudgments,
	transactor bentoapp.Transactor,
	identity domain.SourceIdentity,
	attempt domain.ProcessingAttempt,
) {
	t.Helper()
	ctx := t.Context()
	request, found, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回委托：found=%v err=%v", found, err)
	}
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return judgments.RecordProcessingAttempt(txCtx, identity.TenantID(), request.ShipmentRequestID(),
			request.CurrentSubmissionVersion().VersionID(), attempt)
	})
}

func viewScope(t *testing.T, tenant string, accounts ...string) domain.AuthorizedQueryScope {
	t.Helper()
	ids := make([]domain.CustomerAccountID, len(accounts))
	for index, account := range accounts {
		ids[index] = mustBuild(t, domain.NewCustomerAccountID, account)
	}
	scope, err := domain.NewAuthorizedQueryScope(
		mustBuild(t, domain.NewQueryScopeReference, "SCOPE-GRANT-1"),
		mustBuild(t, domain.NewTenantID, tenant),
		ids,
	)
	if err != nil {
		t.Fatalf("查询作用域：%v", err)
	}
	return scope
}

func scopedFingerprint(t *testing.T, tenant, customer, key string) domain.SourceSubmissionFingerprint {
	t.Helper()
	built, err := domain.NewSourceSubmissionFingerprint(
		requestScopedIdentity(t, tenant, customer, key),
		mustBuild(t, domain.NewPayloadDigest, "digest-1"),
		submittedAtFixture.Add(-time.Hour),
		submittedAtFixture.Add(-time.Hour+time.Second),
	)
	if err != nil {
		t.Fatalf("来源指纹：%v", err)
	}
	return built
}

// scopedSubmittedRequest 与 submittedShipmentRequest 同一配方，多出作用域与提交时间两个
// 参数：可见性与排序正是本读面要证的两样，夹具必须能把行铺到不同作用域与不同时刻上。
func scopedSubmittedRequest(
	t *testing.T,
	tenant, customer, key, requestID string,
	submittedAt time.Time,
) domain.ShipmentRequest {
	t.Helper()

	scope, err := domain.NewAdmissionScope(
		mustBuild(t, domain.NewAdmissionScopeReference, "scope-ref-1"),
		mustBuild(t, domain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("准入范围：%v", err)
	}
	validity, err := domain.NewOwnershipValidityInterval(
		submittedAt.Add(-24*time.Hour),
		submittedAt.Add(24*time.Hour),
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
		AsOf:             submittedAt,
		Validity:         validity,
		Revision:         mustBuild(t, domain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       submittedAt,
	})
	if err != nil {
		t.Fatalf("归属决定：%v", err)
	}
	gate, err := domain.EvaluateFutureSubmissionGate(
		decision, scope.Digest(),
		mustBuild(t, domain.NewProductionOwnershipRevision, "rev-1"), submittedAt)
	if err != nil {
		t.Fatalf("建单门禁：%v", err)
	}
	candidate, err := domain.NewSubmissionCandidate(
		scopedFingerprint(t, tenant, customer, key),
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
		SubmittedAt: submittedAt,
		Profiles:    []domain.DeclaredParcelProfile{profile},
	})
	if err != nil {
		t.Fatalf("建单：%v", err)
	}
	return request
}

// Covers: CONTEXT「授权查询作用域」——过滤在键上：作用域外的客户账户与其他租户的同名
// 账户都不出现在结果里；排序新的在前，操作员看的是「最近来了什么」。
func TestListVisibleFiltersByScopeNewestFirst(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-1", "view-a", "REQ-A", submittedAtFixture))
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-2", "view-b", "REQ-B", submittedAtFixture.Add(time.Hour)))
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-3", "view-c", "REQ-C", submittedAtFixture.Add(2*time.Hour)))
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-2", "customer-1", "view-d", "REQ-D", submittedAtFixture.Add(3*time.Hour)))

	rows, err := rowsOf(views.ListVisible(ctx, viewScope(t, "tenant-1", "customer-1", "customer-2"), 10, firstPage(t, ports.ShipmentRequestViewPages)))
	if err != nil {
		t.Fatalf("查阅列表：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2（customer-3 与 tenant-2 都不可见）", len(rows))
	}
	if rows[0].ShipmentRequestID.String() != "REQ-B" || rows[1].ShipmentRequestID.String() != "REQ-A" {
		t.Fatalf("排序 = [%s %s], want [REQ-B REQ-A]（新的在前）",
			rows[0].ShipmentRequestID, rows[1].ShipmentRequestID)
	}

	newest := rows[0]
	if newest.CustomerAccountID.String() != "customer-2" ||
		newest.Source.String() != "portal" ||
		newest.SourceRequestKey.String() != "view-b" {
		t.Fatalf("来源身份变形：%+v", newest)
	}
	if newest.State != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %s, want SUBMITTED", newest.State)
	}
	if newest.SubmissionVersionID.String() != "version-1" {
		t.Fatalf("version = %s, want version-1", newest.SubmissionVersionID)
	}
	if newest.DeclaredParcelCount != 2 {
		t.Fatalf("parcel count = %d, want 2", newest.DeclaredParcelCount)
	}
	if !newest.SubmittedAt.Equal(submittedAtFixture.Add(time.Hour)) {
		t.Fatalf("submittedAt = %s", newest.SubmittedAt)
	}
}

func TestListVisibleHonorsTheLimit(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-1", "view-a", "REQ-A", submittedAtFixture))
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-1", "view-b", "REQ-B", submittedAtFixture.Add(time.Hour)))

	rows, err := rowsOf(views.ListVisible(ctx, viewScope(t, "tenant-1", "customer-1"), 1, firstPage(t, ports.ShipmentRequestViewPages)))
	if err != nil {
		t.Fatalf("查阅列表：%v", err)
	}
	if len(rows) != 1 || rows[0].ShipmentRequestID.String() != "REQ-B" {
		t.Fatalf("rows = %+v, want 只有最新的 REQ-B", rows)
	}

	if _, err := views.ListVisible(ctx, viewScope(t, "tenant-1", "customer-1"), 0, firstPage(t, ports.ShipmentRequestViewPages)); err == nil {
		t.Fatal("非正 limit 是调用方编程错误，不该静默答一页")
	}
}

// Covers: 详情把查阅要用的东西整份带回——含只存在于快照文档里的批次、信封时间、画像、
// 任务与决定；读面不走聚合重建，已接受产物照样读得出。
func TestFindVisibleReturnsTheWholeDetail(t *testing.T) {
	views, repository, judgments, transactor := newShipmentRequestViewsWithJudgments(t)
	ctx := t.Context()

	identity := requestScopedIdentity(t, "tenant-1", "customer-1", "view-d1")
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-1", "view-d1", "REQ-D1", submittedAtFixture))
	recordAttemptOnCurrentVersion(t, repository, judgments, transactor, identity, processingAttempt(t))
	loaded, _, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	accepted, err := loaded.Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	mustSave(t, transactor, ctx, repository, identity, accepted)

	detail, found, err := views.FindVisibleByID(ctx,
		viewScope(t, "tenant-1", "customer-1"),
		mustBuild(t, domain.NewShipmentRequestID, "REQ-D1"))
	if err != nil {
		t.Fatalf("查阅详情：%v", err)
	}
	if !found {
		t.Fatal("作用域内的委托查不到")
	}
	if detail.State != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %s, want ACCEPTED", detail.State)
	}
	if detail.BatchID.String() != "batch-1" {
		t.Fatalf("batch = %s", detail.BatchID)
	}
	if !detail.OccurredAt.Equal(submittedAtFixture.Add(-time.Hour)) ||
		!detail.ReceivedAt.Equal(submittedAtFixture.Add(-time.Hour+time.Second)) {
		t.Fatalf("信封时间变形：occurred=%s received=%s", detail.OccurredAt, detail.ReceivedAt)
	}
	if detail.DeclaredParcelCount != 2 || len(detail.DeclaredParcels) != 2 {
		t.Fatalf("成员 = %d/%d, want 2/2", detail.DeclaredParcelCount, len(detail.DeclaredParcels))
	}
	first := detail.DeclaredParcels[0]
	if first.Parcel.String() != "parcel-1" ||
		first.WeightValue != "2.5" || first.WeightUnit != "kg" ||
		!first.HasDimensions || first.Length != "30" || first.DimensionsUnit != "cm" {
		t.Fatalf("成员画像变形：%+v", first)
	}
	second := detail.DeclaredParcels[1]
	if second.Parcel.String() != "parcel-2" || second.WeightValue != "" || second.HasDimensions {
		t.Fatalf("无画像成员该保持测量缺席：%+v", second)
	}
	if detail.PriorVersionCount != 0 {
		t.Fatalf("prior versions = %d, want 0", detail.PriorVersionCount)
	}
	if detail.Task.State != domain.AcceptanceTaskComplete {
		t.Fatalf("task state = %s, want COMPLETE", detail.Task.State)
	}
	if !detail.Task.HasAttempt ||
		detail.Task.LastAttemptReason != "COMMERCIAL_BASIS_UNAVAILABLE" ||
		detail.Task.LastAttemptResumePath != domain.ResumeByInternalRetry ||
		detail.Task.LastAttemptContinuation != "CONT-1" ||
		detail.Task.LastAttemptSettlementReason != "" || detail.Task.LastAttemptServiceProduct != "" {
		t.Fatalf("处理记录变形：%+v", detail.Task)
	}
	if !detail.HasDecision || !detail.Decision.Accepted ||
		detail.Decision.DecisionID.String() != "accept-1" ||
		!detail.Decision.DecidedAt.Equal(submittedAtFixture.Add(time.Hour)) {
		t.Fatalf("决定摘要变形：%+v", detail.Decision)
	}
}

// Covers: ADR-0383 决定四——委托详情的「上次处理」读处理尝试表上这份委托当前这一版任务最近落下的一条：
//   - 按发生时刻取最近，同一时刻按续办引用定先后，答案不随扫描顺序变；
//   - 别的版本上的尝试不算（任务随版本重立，ADR-0045）；
//   - 迁移 0038 之前落下、没有版本的旧行按任务建立时刻圈定：晚于它的算，早于它的属上一份任务；
//   - 续办方、结算原因与服务产品照那一行交出（ADR-0383 决定三）；缺项码同理（迁移 0039，ADR-0381 决定四）。
func TestTheDetailReadsTheLatestAttemptOfTheCurrentTask(t *testing.T) {
	pool := pgtest.Pool(t)
	views, repository, transactor := newShipmentRequestViewsOn(t, pool)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	judgments, err := adapter.NewAcceptanceJudgments(db)
	if err != nil {
		t.Fatalf("构造判断登记册：%v", err)
	}
	ctx := t.Context()
	tenant := mustBuild(t, domain.NewTenantID, "tenant-1")
	identity := requestScopedIdentity(t, "tenant-1", "customer-1", "view-la")
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-1", "view-la", "REQ-LA", submittedAtFixture))
	loaded, _, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	current := loaded.CurrentSubmissionVersion().VersionID()
	requestID := loaded.ShipmentRequestID()

	entry, err := domain.NewCanonicalContentEntry(domain.RequestedServiceProductEntryName, "SYN-PROD-01")
	if err != nil {
		t.Fatalf("canonical entry: %v", err)
	}
	attempt := func(reason string, path domain.ResumePath, continuation string, at time.Time, settlement string) domain.ProcessingAttempt {
		spec := domain.ProcessingAttemptSpec{
			Reason:       mustBuild(t, domain.NewProcessingAttemptReason, reason),
			ResumePath:   path,
			Continuation: mustBuild(t, domain.NewOwnershipContinuationReference, continuation),
			AttemptedAt:  at,
		}
		if settlement != "" {
			spec.SettlementReason = mustBuild(t, domain.NewSettlementNotFormedReason, settlement)
			spec.ServiceProduct = domain.RequestedServiceProductOf([]domain.CanonicalContentEntry{entry})
			spec.EstimateGapCodes = []string{"MEASUREMENT", "POSTAL_ROUTE"}
		}
		built, err := domain.NewProcessingAttempt(spec)
		if err != nil {
			t.Fatalf("形成处理尝试：%v", err)
		}
		return built
	}
	record := func(version domain.SubmissionVersionID, attempt domain.ProcessingAttempt) {
		mustWithinTransaction(t, db.Transactor(), ctx, func(txCtx context.Context) error {
			return judgments.RecordProcessingAttempt(txCtx, tenant, requestID, version, attempt)
		})
	}
	lastAttempt := func() ports.AcceptanceTaskViewRecord {
		detail, found, err := views.FindVisibleByID(ctx, viewScope(t, "tenant-1", "customer-1"), requestID)
		if err != nil || !found {
			t.Fatalf("查阅详情：found=%v err=%v", found, err)
		}
		return detail.Task
	}

	record(current, attempt("FINANCIAL_CONTROL_NOT_FORMED", domain.ResumeByInternalRetry, "CONT-early",
		submittedAtFixture.Add(time.Minute), ""))
	record(current, attempt("FINANCIAL_CONTROL_NOT_CONFIGURED", domain.ResumeByOperatorRegistration, "CONT-a",
		submittedAtFixture.Add(2*time.Minute), "CONTROL_AMOUNT_NOT_CONFIGURED"))
	record(current, attempt("FINANCIAL_CONTROL_NOT_CONFIGURED", domain.ResumeByOperatorRegistration, "CONT-b",
		submittedAtFixture.Add(2*time.Minute), "ESTIMATE_PRICE_CARD_NOT_CONFIGURED"))
	// 别的版本上更晚的一条：不属这一份任务。
	record(mustBuild(t, domain.NewSubmissionVersionID, "version-other"),
		attempt("COMMERCIAL_BASIS_UNAVAILABLE", domain.ResumeByInternalRetry, "CONT-other", submittedAtFixture.Add(time.Hour), ""))

	task := lastAttempt()
	if !task.HasAttempt || task.LastAttemptContinuation != "CONT-b" ||
		task.LastAttemptReason != "FINANCIAL_CONTROL_NOT_CONFIGURED" ||
		task.LastAttemptResumePath != domain.ResumeByOperatorRegistration ||
		task.LastAttemptSettlementReason != "ESTIMATE_PRICE_CARD_NOT_CONFIGURED" ||
		task.LastAttemptServiceProduct != "SYN-PROD-01" ||
		!task.LastAttemptedAt.Equal(submittedAtFixture.Add(2*time.Minute)) {
		t.Fatalf("上次处理 = %+v, want 同一时刻续办引用在后的 CONT-b，别的版本上更晚的那条不算", task)
	}
	if !slices.Equal(task.LastAttemptEstimateGapCodes, []string{"MEASUREMENT", "POSTAL_ROUTE"}) {
		t.Fatalf("上次处理的缺项码 = %v, want 迁移 0039 那一列照行交出（ADR-0381 决定四）", task.LastAttemptEstimateGapCodes)
	}

	// 旧行没有版本：早于任务建立的那条属上一份任务，不算；晚于它的那条算。
	for _, legacy := range []struct {
		continuation string
		at           time.Time
	}{
		{"CONT-legacy-before", submittedAtFixture.Add(-time.Hour)},
		{"CONT-legacy-after", submittedAtFixture.Add(3 * time.Minute)},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO parcel_shipment.acceptance_processing_attempt
				(tenant_id, shipment_request_id, continuation_ref, attempted_at, reason_ref, resume_path)
			 VALUES ('tenant-1', 'REQ-LA', $1, $2, 'COMMERCIAL_BASIS_UNAVAILABLE', 'INTERNAL_RETRY')`,
			legacy.continuation, legacy.at); err != nil {
			t.Fatalf("写旧行：%v", err)
		}
	}
	task = lastAttempt()
	if task.LastAttemptContinuation != "CONT-legacy-after" || task.LastAttemptResumePath != domain.ResumeByInternalRetry ||
		task.LastAttemptSettlementReason != "" || task.LastAttemptServiceProduct != "" ||
		len(task.LastAttemptEstimateGapCodes) != 0 {
		t.Fatalf("上次处理 = %+v, want 任务建立之后的那条旧行，几格都缺席", task)
	}

	// 只有一条早于任务建立的旧行：这份任务还没有处理尝试。
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-1", "view-lb", "REQ-LB", submittedAtFixture))
	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.acceptance_processing_attempt
			(tenant_id, shipment_request_id, continuation_ref, attempted_at, reason_ref, resume_path)
		 VALUES ('tenant-1', 'REQ-LB', 'CONT-prior-task', $1, 'COMMERCIAL_BASIS_UNAVAILABLE', 'INTERNAL_RETRY')`,
		submittedAtFixture.Add(-time.Hour)); err != nil {
		t.Fatalf("写旧行：%v", err)
	}
	other, found, err := views.FindVisibleByID(ctx, viewScope(t, "tenant-1", "customer-1"),
		mustBuild(t, domain.NewShipmentRequestID, "REQ-LB"))
	if err != nil || !found {
		t.Fatalf("查阅详情：found=%v err=%v", found, err)
	}
	if other.Task.HasAttempt {
		t.Fatalf("上一份任务的旧行被读成了这一份的上次处理：%+v", other.Task)
	}
}

// Covers: CONTEXT「统一不可见结果」——不存在、其他客户账户、其他租户三种情况从本读口
// 拿到完全相同的答复（found=false 且无错误），拆开任何一种都是跨作用域存在性预言机。
func TestFindVisibleAnswersAbsentAndOutOfScopeTheSame(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-1", "view-e", "REQ-E", submittedAtFixture))

	invisible := map[string]struct {
		scope     domain.AuthorizedQueryScope
		requestID string
	}{
		"真不存在":     {viewScope(t, "tenant-1", "customer-1"), "REQ-GHOST"},
		"其他客户账户":   {viewScope(t, "tenant-1", "customer-2"), "REQ-E"},
		"其他租户同号账户": {viewScope(t, "tenant-2", "customer-1"), "REQ-E"},
	}
	for name, probe := range invisible {
		_, found, err := views.FindVisibleByID(ctx, probe.scope,
			mustBuild(t, domain.NewShipmentRequestID, probe.requestID))
		if err != nil {
			t.Fatalf("%s：查询出错 %v", name, err)
		}
		if found {
			t.Errorf("%s：读到了不该可见的委托", name)
		}
	}
}

// Covers: 查阅读面跨生命周期——读面直读投影列与快照文档、不经重建门，操作员必须能看到
// 这份委托的存在与终局状态；决定缺席如实缺席，任务停在`已停止`。
func TestAWithdrawnRequestStaysVisibleToTheView(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	identity := requestScopedIdentity(t, "tenant-1", "customer-1", "view-f")
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-1", "view-f", "REQ-F", submittedAtFixture))
	loaded, _, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	withdrawn, err := loaded.WithdrawByCustomer(domain.WithdrawalSpec{
		DecisionID: mustBuild(t, domain.NewAcceptanceDecisionID, "withdraw-1"),
		Authority:  mustBuild(t, domain.NewWithdrawalAuthorityReference, "PC-WITHDRAW-ROLE-1"),
		Requester:  mustBuild(t, domain.NewWithdrawalRequesterReference, "CUSTOMER-CONTACT-1"),
		Reason:     mustBuild(t, domain.NewWithdrawalReasonReference, "CUSTOMER_NO_LONGER_REQUIRES_SERVICE"),
		DecidedAt:  submittedAtFixture.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("撤回：%v", err)
	}
	mustSave(t, transactor, ctx, repository, identity, withdrawn)

	detail, found, err := views.FindVisibleByID(ctx,
		viewScope(t, "tenant-1", "customer-1"),
		mustBuild(t, domain.NewShipmentRequestID, "REQ-F"))
	if err != nil {
		t.Fatalf("查阅详情：%v", err)
	}
	if !found {
		t.Fatal("已撤回的委托在查阅面上消失了")
	}
	if detail.State != domain.ShipmentRequestWithdrawn {
		t.Fatalf("state = %s, want WITHDRAWN", detail.State)
	}
	if detail.HasDecision {
		t.Fatal("撤回不是接受决定，详情不该长出决定摘要")
	}
	if detail.Task.State != domain.AcceptanceTaskStopped {
		t.Fatalf("task state = %s, want STOPPED", detail.Task.State)
	}
}

// Covers: ADR-0373 决定五、ADR-0374 决定四与决定六——委托详情摊出当前版本的货物明细、平台货件，逐件带客户件号与平台箱标号（票
// declared-goods-lines/04）；没报的照实缺席。
func TestFindVisibleCarriesGoodsAndPlatformDeclarations(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()
	request := numberedRequestWith(t, "tenant-1", "customer-1", "view-g1", "REQ-G1", []string{"parcel-1", "parcel-2"},
		map[string]string{"parcel-1": "CUST-0001"}, func(spec *domain.SubmitShipmentRequestSpec) {
			shipment, err := domain.DeclarePlatformShipment("AMAZON", mustBuild(t, domain.NewExternalIdentifierNumber, "FBA15SYNVIEW"), "SYN-REF-V")
			if err != nil {
				t.Fatalf("平台货件：%v", err)
			}
			spec.PlatformShipment = shipment
			label, err := domain.DeclarePlatformBoxLabel(mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"),
				mustBuild(t, domain.NewExternalIdentifierNumber, "FBA15SYNVIEWU000002"))
			if err != nil {
				t.Fatalf("箱标号：%v", err)
			}
			spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{label}
			spec.Goods = roundTripGoods(t)
		})
	mustInsert(t, transactor, ctx, repository, request)
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-1", "view-g2", "REQ-G2", submittedAtFixture))

	detail, found, err := views.FindVisibleByID(ctx, viewScope(t, "tenant-1", "customer-1"), mustBuild(t, domain.NewShipmentRequestID, "REQ-G1"))
	if err != nil || !found {
		t.Fatalf("查阅详情：found=%v err=%v", found, err)
	}
	if lines := detail.Goods.Lines(); len(lines) != 2 || len(detail.Goods.Packing()) != 3 {
		t.Fatalf("货物明细 %d 行 %d 条分配, want 2 / 3", len(lines), len(detail.Goods.Packing()))
	}
	if shipment := detail.PlatformShipment; !shipment.Declared() || shipment.ShipmentNumber().String() != "FBA15SYNVIEW" {
		t.Fatalf("平台货件 = %+v", shipment)
	}
	byParcel := map[string]ports.DeclaredParcelViewRecord{}
	for _, parcel := range detail.DeclaredParcels {
		byParcel[parcel.Parcel.String()] = parcel
	}
	if byParcel["parcel-1"].CustomerParcelNumber != "CUST-0001" || byParcel["parcel-1"].PlatformBoxLabel != "" ||
		byParcel["parcel-2"].PlatformBoxLabel != "FBA15SYNVIEWU000002" || byParcel["parcel-2"].CustomerParcelNumber != "" {
		t.Fatalf("逐件的号 = %+v", byParcel)
	}

	plain, _, err := views.FindVisibleByID(ctx, viewScope(t, "tenant-1", "customer-1"), mustBuild(t, domain.NewShipmentRequestID, "REQ-G2"))
	if err != nil {
		t.Fatalf("查阅没报货物的委托：%v", err)
	}
	if plain.Goods.Declared() || plain.PlatformShipment.Declared() {
		t.Fatal("没报货物明细与平台货件的委托读成了报了")
	}
}

// Covers: ADR-0374 决定六（票 declared-goods-lines/04）——详情逐件列跟踪号：此刻仍适用、标识这一件的渠道包裹件号；已作废的不列，
// 没号的件不列。号在外部标识关联册里，本例直接写两条关联，其一随后作废。
func TestFindVisibleListsEachParcelsTrackingNumbers(t *testing.T) {
	pool := pgtest.Pool(t)
	views, repository, transactor := newShipmentRequestViewsOn(t, pool)
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-1", "view-t1", "REQ-T1", submittedAtFixture))
	for _, row := range []struct{ number, basis string }{{"SYN-TRACK-OLD", "label-1"}, {"SYN-TRACK-NEW", "label-2"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO parcel_shipment.external_identifier_relation
		      (tenant_id, identifier_kind, allocator, identifier_number, object_kind, object_ref, scope, basis, formed_at)
		      VALUES ('tenant-1', 'CHANNEL_PARCEL_NUMBER', 'SYN-CHANNEL', $1, 'PARCEL', 'parcel-1', 'SYN-CHANNEL', $2, $3)`,
			row.number, row.basis, submittedAtFixture); err != nil {
			t.Fatalf("写渠道包裹件号关联：%v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO parcel_shipment.external_identifier_relation_change
	      (tenant_id, identifier_kind, allocator, identifier_number, object_ref, basis, change_kind, cause, changed_at)
	      VALUES ('tenant-1', 'CHANNEL_PARCEL_NUMBER', 'SYN-CHANNEL', 'SYN-TRACK-OLD', 'parcel-1', 'label-1', 'VOIDED', 'label-1', $1)`,
		submittedAtFixture); err != nil {
		t.Fatalf("作废旧号：%v", err)
	}

	detail, found, err := views.FindVisibleByID(ctx, viewScope(t, "tenant-1", "customer-1"), mustBuild(t, domain.NewShipmentRequestID, "REQ-T1"))
	if err != nil || !found {
		t.Fatalf("查阅详情：found=%v err=%v", found, err)
	}
	if len(detail.DeclaredParcels) != 2 {
		t.Fatalf("包裹 %d 件, want 2", len(detail.DeclaredParcels))
	}
	for _, parcel := range detail.DeclaredParcels {
		want := []string(nil)
		if parcel.Parcel.String() == "parcel-1" {
			want = []string{"SYN-TRACK-NEW"}
		}
		if len(parcel.TrackingNumbers) != len(want) || (len(want) == 1 && parcel.TrackingNumbers[0] != want[0]) {
			t.Fatalf("%s 跟踪号 = %v, want %v", parcel.Parcel, parcel.TrackingNumbers, want)
		}
	}
}
