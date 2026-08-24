package postgres_test

import (
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证委托查阅读面：可见性过滤在 SQL 键上、`统一不可见结果`
// 对「不存在」与「作用域外」同答、读面跨生命周期覆盖重建门外的终局状态（ADR-0061 之外
// 正是本读口存在的理由）。

func newShipmentRequestViews(t *testing.T) (*adapter.ShipmentRequestViews, *adapter.ShipmentRequests, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
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

	rows, err := views.ListVisible(ctx, viewScope(t, "tenant-1", "customer-1", "customer-2"), 10)
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

	rows, err := views.ListVisible(ctx, viewScope(t, "tenant-1", "customer-1"), 1)
	if err != nil {
		t.Fatalf("查阅列表：%v", err)
	}
	if len(rows) != 1 || rows[0].ShipmentRequestID.String() != "REQ-B" {
		t.Fatalf("rows = %+v, want 只有最新的 REQ-B", rows)
	}

	if _, err := views.ListVisible(ctx, viewScope(t, "tenant-1", "customer-1"), 0); err == nil {
		t.Fatal("非正 limit 是调用方编程错误，不该静默答一页")
	}
}

// Covers: 详情把查阅要用的东西整份带回——含只存在于快照文档里的批次、信封时间、画像、
// 任务与决定；读面不走聚合重建，已接受产物照样读得出。
func TestFindVisibleReturnsTheWholeDetail(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	identity := requestScopedIdentity(t, "tenant-1", "customer-1", "view-d1")
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, "tenant-1", "customer-1", "view-d1", "REQ-D1", submittedAtFixture))
	loaded, _, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	withAttempt, err := loaded.RecordProcessingAttempt(processingAttempt(t))
	if err != nil {
		t.Fatalf("记处理尝试：%v", err)
	}
	accepted, err := withAttempt.Decide(acceptanceDecisionSpec(t))
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
		detail.Task.LastAttemptContinuation != "CONT-1" {
		t.Fatalf("处理记录变形：%+v", detail.Task)
	}
	if !detail.HasDecision || !detail.Decision.Accepted ||
		detail.Decision.DecisionID.String() != "accept-1" ||
		!detail.Decision.DecidedAt.Equal(submittedAtFixture.Add(time.Hour)) {
		t.Fatalf("决定摘要变形：%+v", detail.Decision)
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

// Covers: 查阅读面跨生命周期——`已撤回`在聚合重建门外（ADR-0061），但操作员必须能看到
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
