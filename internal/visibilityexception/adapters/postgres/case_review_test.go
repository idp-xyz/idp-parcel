package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件对真实 PostgreSQL 16 证案件侧三页查阅目录的读面（票 admin-skeleton-closure-
// batch/06）：上列经由写侧适配器真实落库的行（案件表尚无写入方，循 active_case_test
// 直接写行），照实转写不重建、发作期与分诊结论同键左联、补充期限按子表计版本、追偿
// 动作各种类取最近节点、租户隔离进 SQL 条件、排序稳定、空册如实交回空列表。读方与
// 写方共用同一个测试库——pgtest.Pool 每次调用都是一个新库，分开建会读到两个世界。

type caseReviewFixture struct {
	review        *adapter.CaseReview
	episodes      *adapter.SignalEpisodes
	requests      *adapter.DispositionRequests
	notifications *adapter.CustomerNotifications
	claims        *adapter.Claims
	recoveries    *adapter.Recoveries
	transactor    bentoapp.Transactor
	pool          *pgxpool.Pool
}

func newCaseReviewFixture(t *testing.T) *caseReviewFixture {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	review, err := adapter.NewCaseReview(db)
	if err != nil {
		t.Fatalf("构造案件查阅目录：%v", err)
	}
	episodes, err := adapter.NewSignalEpisodes(db)
	if err != nil {
		t.Fatalf("构造发作期库：%v", err)
	}
	requests, err := adapter.NewDispositionRequests(db)
	if err != nil {
		t.Fatalf("构造处置请求库：%v", err)
	}
	notifications, err := adapter.NewCustomerNotifications(db)
	if err != nil {
		t.Fatalf("构造通知库：%v", err)
	}
	claims, err := adapter.NewClaims(db)
	if err != nil {
		t.Fatalf("构造索赔库：%v", err)
	}
	recoveries, err := adapter.NewRecoveries(db)
	if err != nil {
		t.Fatalf("构造追偿库：%v", err)
	}
	return &caseReviewFixture{
		review:        review,
		episodes:      episodes,
		requests:      requests,
		notifications: notifications,
		claims:        claims,
		recoveries:    recoveries,
		transactor:    db.Transactor(),
		pool:          pool,
	}
}

func (fixture *caseReviewFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func reviewValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

// TestCaseReviewListsSignalEpisodesWithConclusions 证发作期册：发作期连同同笔提交的
// 分诊结论一行转写、新近首启在前、租户隔离与页大小都由读口执行。夹具复用
// accepted_fact_signal_episode_test 的 openedEpisode / raisedRecord（结论定格在
// factBaseAt+5m、规则 triage-rule/v1）。
func TestCaseReviewListsSignalEpisodesWithConclusions(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	ctx := t.Context()
	tenant := reviewValue(t, domain.NewTenantID, "tenant-a")

	first := openedEpisode(t, "ep-1", "parcel-1", "STALLED", factBaseAt)
	second := openedEpisode(t, "ep-2", "parcel-2", "GAP", factBaseAt.Add(time.Hour))
	foreign := openedEpisode(t, "ep-9", "parcel-9", "STALLED", factBaseAt)
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.episodes.SaveRaised(txCtx,
			raisedRecord(t, first, "tenant-a", "parcel-1", "STALLED", domain.AutoEstablishCase)); err != nil {
			return err
		}
		if err := fixture.episodes.SaveRaised(txCtx,
			raisedRecord(t, second, "tenant-a", "parcel-2", "GAP", domain.ManualReviewRequired)); err != nil {
			return err
		}
		return fixture.episodes.SaveRaised(txCtx,
			raisedRecord(t, foreign, "tenant-b", "parcel-9", "STALLED", domain.NoCaseNeeded))
	})

	rows, err := fixture.review.ListSignalEpisodes(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列发作期册：%v", err)
	}
	if len(rows) != 2 || rows[0].EpisodeID != "ep-2" || rows[1].EpisodeID != "ep-1" {
		t.Fatalf("册面行序变形：%+v", rows)
	}

	latest := rows[0]
	if latest.Parcel != "parcel-2" ||
		latest.Kind != "GAP" ||
		latest.Rule != "signal-rule/v1" ||
		latest.Confidence != "confidence/high" ||
		latest.Hits != 1 ||
		!latest.StartedAt.Equal(factBaseAt.Add(time.Hour)) ||
		!latest.LastHitAt.Equal(factBaseAt.Add(time.Hour)) ||
		latest.ReleaseBasis != "" ||
		latest.EndedAt != nil ||
		latest.PriorEpisode != "" {
		t.Errorf("发作期行转写变形：%+v", latest)
	}
	if latest.Outcome != "MANUAL_REVIEW" ||
		latest.OutcomeRule != "triage-rule/v1" ||
		latest.TriagedAt == nil ||
		!latest.TriagedAt.Equal(factBaseAt.Add(5*time.Minute)) {
		t.Errorf("分诊结论转写变形：%+v", latest)
	}
	if rows[1].Outcome != "AUTO_ESTABLISH" {
		t.Errorf("首启行结论变形：%+v", rows[1])
	}

	limited, err := fixture.review.ListSignalEpisodes(ctx, tenant, 1)
	if err != nil || len(limited) != 1 || limited[0].EpisodeID != "ep-2" {
		t.Errorf("页大小未生效：rows=%+v err=%v", limited, err)
	}
}

// TestCaseReviewListsDispositionRequests 证处置请求册：请求六要件与接受窗口照行
// 转写，尚无答复的行判断格如实缺席，另一个租户的请求不可见。
func TestCaseReviewListsDispositionRequests(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	ctx := t.Context()
	tenant := reviewValue(t, domain.NewTenantID, "tenant-a")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Save(txCtx, tenant, sentRequest(t, "request-1", 1)); err != nil {
			return err
		}
		_, err := fixture.requests.Save(txCtx,
			reviewValue(t, domain.NewTenantID, "tenant-b"), sentRequest(t, "request-9", 1))
		return err
	})

	rows, err := fixture.review.ListDispositionRequests(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列处置请求册：%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("册面行数 = %d：%+v", len(rows), rows)
	}
	request := rows[0]
	if request.RequestID != "request-1" ||
		request.CaseID != "case-1" ||
		request.TargetContext != "NETWORK_ROUTING" ||
		request.Action != "REROUTE_REMAINING_JOURNEY" ||
		request.Scope != "parcel-1/remaining" ||
		request.Reason != "route deviation confirmed" ||
		request.Evidence != "evidence/route-deviation-1" ||
		request.IntentVersion != 1 ||
		!request.SentAt.Equal(dispositionBaseAt) ||
		request.AcceptanceWindow == nil ||
		!request.AcceptanceWindow.Equal(dispositionBaseAt.Add(24*time.Hour)) {
		t.Errorf("请求行转写变形：%+v", request)
	}
	if request.Judgment != "" || request.JudgedAt != nil ||
		request.Cancellation != "" || request.SupersededBy != "" {
		t.Errorf("未答复行凭空长出交互历史：%+v", request)
	}
}

// insertReviewCase 直接写案件行：案件仓储尚未落地（0008 只落表），判据同
// active_case_test——证的是读面读得对，不是某个还不存在的写入方。
func (fixture *caseReviewFixture) insertReviewCase(
	t *testing.T,
	tenant, caseID, phase string,
	establishedAt time.Time,
	firstResponse, closedAt *time.Time,
	conclusion, mergedInto *string,
) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.exception_case
			(tenant_id, case_id, root_parcel, impact_scope, responsible_team,
			 phase, established_at, first_response, closed_at, conclusion, merged_into)
		 VALUES ($1, $2, 'parcel-1', 'scope/v1', 'team-a', $3, $4, $5, $6, $7, $8)`,
		tenant, caseID, phase, establishedAt, firstResponse, closedAt, conclusion, mergedInto,
	); err != nil {
		t.Fatalf("写入案件 %s/%s：%v", tenant, caseID, err)
	}
}

// TestCaseReviewListsExceptionCases 证异常案件册：三相各自的在场件照行转写（已关闭
// 带结论、归并指回主案件、待响应无首次响应），新近建立在前，另一个租户的案件不可见。
func TestCaseReviewListsExceptionCases(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	ctx := t.Context()
	tenant := reviewValue(t, domain.NewTenantID, "tenant-a")

	fixture.insertReviewCase(t, "tenant-a", "case-1", "AWAITING_RESPONSE",
		caseBaseAt, nil, nil, nil, nil)

	firstResponse := caseBaseAt.Add(90 * time.Minute)
	fixture.insertReviewCase(t, "tenant-a", "case-2", "IN_PROGRESS",
		caseBaseAt.Add(time.Hour), &firstResponse, nil, nil, nil)

	closedAt := caseBaseAt.Add(3 * time.Hour)
	merged := "MERGED"
	mergedInto := "case-2"
	fixture.insertReviewCase(t, "tenant-a", "case-3", "CLOSED",
		caseBaseAt.Add(2*time.Hour), nil, &closedAt, &merged, &mergedInto)

	fixture.insertReviewCase(t, "tenant-b", "case-9", "AWAITING_RESPONSE",
		caseBaseAt, nil, nil, nil, nil)

	rows, err := fixture.review.ListExceptionCases(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列案件册：%v", err)
	}
	if len(rows) != 3 || rows[0].CaseID != "case-3" || rows[1].CaseID != "case-2" || rows[2].CaseID != "case-1" {
		t.Fatalf("册面行序变形：%+v", rows)
	}

	closed := rows[0]
	if closed.Phase != "CLOSED" ||
		closed.RootParcel != "parcel-1" ||
		closed.ImpactScope != "scope/v1" ||
		closed.ResponsibleTeam != "team-a" ||
		closed.ClosedAt == nil || !closed.ClosedAt.Equal(closedAt) ||
		closed.Conclusion != "MERGED" ||
		closed.MergedInto != "case-2" ||
		closed.FirstResponse != nil {
		t.Errorf("归并关闭行转写变形：%+v", closed)
	}
	inProgress := rows[1]
	if inProgress.Phase != "IN_PROGRESS" ||
		inProgress.FirstResponse == nil ||
		!inProgress.FirstResponse.Equal(firstResponse) ||
		inProgress.ClosedAt != nil || inProgress.Conclusion != "" || inProgress.MergedInto != "" {
		t.Errorf("处理中行转写变形：%+v", inProgress)
	}
	awaiting := rows[2]
	if awaiting.Phase != "AWAITING_RESPONSE" || awaiting.FirstResponse != nil {
		t.Errorf("待响应行转写变形：%+v", awaiting)
	}
}

// reviewNotification 造一份可指定披露身份三维的通知（etaGap 夹具的 generatedNotification
// 钉死了三维，证不出排序与隔离）。
func reviewNotification(
	t *testing.T,
	id, customer, episode string,
	decidedAt time.Time,
) *domain.CustomerNotification {
	t.Helper()
	decision, err := domain.DecideDisclosure(
		reviewValue(t, domain.NewEpisodeID, episode),
		reviewValue(t, domain.NewCustomerAccountReference, customer),
		reviewValue(t, domain.NewDisclosurePolicyReference, "disclosure-policy/v1"),
		domain.DiscloseToCustomer,
		reviewValue(t, domain.NewDisclosureContentReference, "content/v1"),
		decidedAt,
	)
	if err != nil {
		t.Fatalf("构造披露决定：%v", err)
	}
	notification, err := domain.GenerateNotification(
		reviewValue(t, domain.NewNotificationID, id),
		decision,
		decidedAt.Add(24*time.Hour),
		reviewValue(t, domain.NewNotificationChannelReference, "portal-message/v1"),
		reviewValue(t, domain.NewDisclosurePolicyReference, "notify-policy/v1"),
		decidedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("构造通知：%v", err)
	}
	return notification
}

// TestCaseReviewListsCustomerNotifications 证客户通知册：披露身份三维、依据与渠道照
// 行转写，过程节点整列按登记序透出（渠道接受、送达是各自的节点，不折成「已通知」），
// 新近决定在前，另一个租户的通知不可见。
func TestCaseReviewListsCustomerNotifications(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	ctx := t.Context()
	tenant := reviewValue(t, domain.NewTenantID, "tenant-a")

	delivered := reviewNotification(t, "notice-1", "customer-1", "episode-1", etaGapBaseAt)
	if err := delivered.RecordMilestone(domain.NotificationChannelAccepted, etaGapBaseAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("记录渠道接受：%v", err)
	}
	if err := delivered.RecordMilestone(domain.NotificationDelivered, etaGapBaseAt.Add(3*time.Minute)); err != nil {
		t.Fatalf("记录送达：%v", err)
	}
	generated := reviewNotification(t, "notice-2", "customer-2", "episode-2", etaGapBaseAt.Add(time.Hour))
	foreign := reviewNotification(t, "notice-9", "customer-9", "episode-9", etaGapBaseAt)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.notifications.Save(txCtx, tenant, delivered); err != nil {
			return err
		}
		if _, err := fixture.notifications.Save(txCtx, tenant, generated); err != nil {
			return err
		}
		_, err := fixture.notifications.Save(txCtx,
			reviewValue(t, domain.NewTenantID, "tenant-b"), foreign)
		return err
	})

	rows, err := fixture.review.ListCustomerNotifications(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列通知册：%v", err)
	}
	if len(rows) != 2 || rows[0].NotificationID != "notice-2" || rows[1].NotificationID != "notice-1" {
		t.Fatalf("册面行序变形：%+v", rows)
	}

	full := rows[1]
	if full.Customer != "customer-1" ||
		full.Episode != "episode-1" ||
		!full.DecidedAt.Equal(etaGapBaseAt) ||
		full.Policy != "disclosure-policy/v1" ||
		full.Content != "content/v1" ||
		!full.Deadline.Equal(etaGapBaseAt.Add(24*time.Hour)) ||
		full.Channel != "portal-message/v1" ||
		full.Obligation != "notify-policy/v1" {
		t.Errorf("通知行转写变形：%+v", full)
	}
	if len(full.Milestones) != 3 ||
		full.Milestones[0].Milestone != "GENERATED" ||
		!full.Milestones[0].RecordedAt.Equal(etaGapBaseAt.Add(time.Minute)) ||
		full.Milestones[1].Milestone != "CHANNEL_ACCEPTED" ||
		full.Milestones[2].Milestone != "DELIVERED" ||
		!full.Milestones[2].RecordedAt.Equal(etaGapBaseAt.Add(3*time.Minute)) {
		t.Errorf("过程节点转写变形：%+v", full.Milestones)
	}
	if len(rows[0].Milestones) != 1 || rows[0].Milestones[0].Milestone != "GENERATED" {
		t.Errorf("仅生成行节点变形：%+v", rows[0].Milestones)
	}
}

// TestCaseReviewListsClaimItems 证索赔项册：三判各自的格照行转写不压并——未审行
// 判断格全缺席、等待补充行带四件与期限版本数、复核换版行带前版、撤回行带撤回时刻；
// 同刻提交按（批次，项）稳定排序；另一个租户的索赔不可见。
func TestCaseReviewListsClaimItems(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	ctx := t.Context()
	tenant := reviewValue(t, domain.NewTenantID, "tenant-a")
	saveClaim := func(claim *domain.ClaimItem, tenantName string) {
		t.Helper()
		fixture.inTx(t, ctx, func(txCtx context.Context) error {
			_, err := fixture.claims.Save(txCtx,
				reviewValue(t, domain.NewTenantID, tenantName), claim)
			return err
		})
	}

	saveClaim(receivedClaim(t, "batch-1", "item-1"), "tenant-a")

	awaiting := receivedClaim(t, "batch-1", "item-2")
	if err := awaiting.AwaitSupplement("materials incomplete",
		claimSupplement(t, claimBaseAt.Add(72*time.Hour)), claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("进入等待补充：%v", err)
	}
	if err := awaiting.ExtendSupplementDeadline(
		claimBaseAt.Add(120*time.Hour), claimBaseAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("获批延期：%v", err)
	}
	saveClaim(awaiting, "tenant-a")

	reviewed := receivedClaim(t, "batch-2", "item-3")
	if err := reviewed.ScreenEligibility(domain.ClaimEligible, "authorized; within window", claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("资格审核：%v", err)
	}
	if err := reviewed.ConcludeLiability(domain.LiabilityNotEstablished,
		claimBaseAt.Add(240*time.Hour), claimBaseAt.Add(3*time.Hour)); err != nil {
		t.Fatalf("形成结论：%v", err)
	}
	if err := reviewed.ReviewConclusion(domain.LiabilityPartiallyEstablished,
		claimBaseAt.Add(4*time.Hour)); err != nil {
		t.Fatalf("复核换版：%v", err)
	}
	saveClaim(reviewed, "tenant-a")

	withdrawn := receivedClaim(t, "batch-2", "item-4")
	if err := withdrawn.Withdraw(claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("撤回：%v", err)
	}
	saveClaim(withdrawn, "tenant-a")

	saveClaim(receivedClaim(t, "batch-1", "item-9"), "tenant-b")

	rows, err := fixture.review.ListClaimItems(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列索赔项册：%v", err)
	}
	if len(rows) != 4 ||
		rows[0].ItemID != "item-1" || rows[1].ItemID != "item-2" ||
		rows[2].ItemID != "item-3" || rows[3].ItemID != "item-4" {
		t.Fatalf("册面行序变形：%+v", rows)
	}

	received := rows[0]
	if received.Batch != "batch-1" ||
		received.Customer != "customer-1" ||
		received.Applicant != "applicant-1" ||
		received.Contract != "contract-scope/v1" ||
		received.Target != "parcel-1/loss" ||
		received.Kind != "LOSS" ||
		!received.SubmittedAt.Equal(claimBaseAt) ||
		received.Revision != 1 ||
		received.Screen != "" || received.ScreenBasis != "" ||
		received.SupplementDeadline != nil || received.DeadlineVersions != 0 ||
		received.Conclusion != "" || received.Withdrawn {
		t.Errorf("未审行转写变形：%+v", received)
	}

	supplement := rows[1]
	if supplement.Screen != "AWAITING_SUPPLEMENT" ||
		supplement.ScreenBasis != "materials incomplete" ||
		supplement.MissingMaterials == "" ||
		supplement.SupplementScope == "" ||
		supplement.SupplementNotice == "" ||
		supplement.SupplementDeadline == nil ||
		!supplement.SupplementDeadline.Equal(claimBaseAt.Add(120*time.Hour)) ||
		supplement.DeadlineVersions != 2 {
		t.Errorf("等待补充行转写变形：%+v", supplement)
	}

	concluded := rows[2]
	if concluded.Screen != "ELIGIBLE" ||
		concluded.Conclusion != "PARTIALLY_ESTABLISHED" ||
		concluded.PriorConclusion != "NOT_ESTABLISHED" ||
		concluded.ConcludedAt == nil ||
		concluded.ReviewBy == nil ||
		!concluded.ReviewBy.Equal(claimBaseAt.Add(240*time.Hour)) {
		t.Errorf("复核换版行转写变形：%+v", concluded)
	}

	if !rows[3].Withdrawn || rows[3].WithdrawnAt == nil ||
		!rows[3].WithdrawnAt.Equal(claimBaseAt.Add(time.Hour)) {
		t.Errorf("撤回行转写变形：%+v", rows[3])
	}
}

// TestCaseReviewListsRecoveryMatters 证追偿事项册：事项要件照行转写，两个动作种类
// 各取最近一个过程节点、尚无动作的种类成对缺席，另一个租户的事项不可见。
func TestCaseReviewListsRecoveryMatters(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	ctx := t.Context()
	tenant := reviewValue(t, domain.NewTenantID, "tenant-a")
	appendAction := func(matter string, kind domain.RecoveryActionKind, milestone domain.RecoveryActionMilestone, attempt int, at time.Time) {
		t.Helper()
		action, err := domain.RecordRecoveryAction(
			reviewValue(t, domain.NewRecoveryMatterID, matter),
			kind, "content/v1", milestone,
			reviewValue(t, domain.NewLiabilityBasisReference, "supplier-agreement/v1"),
			at, attempt)
		if err != nil {
			t.Fatalf("构造追偿动作：%v", err)
		}
		fixture.inTx(t, ctx, func(txCtx context.Context) error {
			return fixture.recoveries.AppendAction(txCtx, tenant, action)
		})
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.recoveries.Save(txCtx, tenant,
			openedMatter(t, "recovery-1", "case-1", "supplier-1", "parcel-1/loss")); err != nil {
			return err
		}
		if _, err := fixture.recoveries.Save(txCtx, tenant,
			openedMatter(t, "recovery-2", "case-2", "insurer-1", "parcel-2/damage")); err != nil {
			return err
		}
		_, err := fixture.recoveries.Save(txCtx,
			reviewValue(t, domain.NewTenantID, "tenant-b"),
			openedMatter(t, "recovery-9", "case-9", "supplier-9", "parcel-9/loss"))
		return err
	})

	appendAction("recovery-1", domain.PreliminaryNotice, domain.ActionPrepared, 1, claimBaseAt.Add(time.Hour))
	appendAction("recovery-1", domain.PreliminaryNotice, domain.ActionSubmitted, 1, claimBaseAt.Add(2*time.Hour))
	appendAction("recovery-1", domain.FormalAssertion, domain.SubmissionFailed, 1, claimBaseAt.Add(3*time.Hour))
	appendAction("recovery-1", domain.FormalAssertion, domain.ActionSubmitted, 2, claimBaseAt.Add(4*time.Hour))

	rows, err := fixture.review.ListRecoveryMatters(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列追偿事项册：%v", err)
	}
	if len(rows) != 2 || rows[0].MatterID != "recovery-1" || rows[1].MatterID != "recovery-2" {
		t.Fatalf("册面行序变形：%+v", rows)
	}

	acted := rows[0]
	if acted.CaseID != "case-1" ||
		acted.Counterparty != "supplier-1" ||
		acted.Scope != "parcel-1/loss" ||
		acted.Basis != "supplier-agreement/v1" ||
		acted.LegalEntity != "entity-1" ||
		acted.Evidence != "evidence/loss-1" ||
		!acted.OpenedAt.Equal(claimBaseAt) ||
		!acted.Deadline.Equal(claimBaseAt.Add(14*24*time.Hour)) {
		t.Errorf("事项要件转写变形：%+v", acted)
	}
	if acted.PreliminaryNotice == nil ||
		acted.PreliminaryNotice.Milestone != "SUBMITTED" ||
		acted.PreliminaryNotice.Attempt != 1 ||
		!acted.PreliminaryNotice.OccurredAt.Equal(claimBaseAt.Add(2*time.Hour)) {
		t.Errorf("预先通知格没取到最近节点：%+v", acted.PreliminaryNotice)
	}
	if acted.FormalAssertion == nil ||
		acted.FormalAssertion.Milestone != "SUBMITTED" ||
		acted.FormalAssertion.Attempt != 2 ||
		!acted.FormalAssertion.OccurredAt.Equal(claimBaseAt.Add(4*time.Hour)) {
		t.Errorf("正式主张格没取到最近节点（失败后重试是新记录）：%+v", acted.FormalAssertion)
	}
	if untouched := rows[1]; untouched.PreliminaryNotice != nil || untouched.FormalAssertion != nil {
		t.Errorf("无动作事项凭空长出动作格：%+v", untouched)
	}
}

// TestCaseReviewRejectsNonPositiveLimitAndAnswersEmptyHonestly 证读口只拒绝无意义的
// 页大小；空登记册如实交回空列表——空册是内容不是错误（ADR-0077 Decision 四）：案件
// 侧的写入方是信号→分诊→建案的编排，事实在接入渠道墙后面，空册正是三页的预期状态。
func TestCaseReviewRejectsNonPositiveLimitAndAnswersEmptyHonestly(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	ctx := t.Context()
	tenant := reviewValue(t, domain.NewTenantID, "tenant-a")

	if _, err := fixture.review.ListSignalEpisodes(ctx, tenant, 0); err == nil {
		t.Error("零页大小的发作期上列没有被拒")
	}
	if _, err := fixture.review.ListDispositionRequests(ctx, tenant, -1); err == nil {
		t.Error("负页大小的处置请求上列没有被拒")
	}
	if _, err := fixture.review.ListExceptionCases(ctx, tenant, 0); err == nil {
		t.Error("零页大小的案件上列没有被拒")
	}
	if _, err := fixture.review.ListCustomerNotifications(ctx, tenant, -1); err == nil {
		t.Error("负页大小的通知上列没有被拒")
	}
	if _, err := fixture.review.ListClaimItems(ctx, tenant, 0); err == nil {
		t.Error("零页大小的索赔上列没有被拒")
	}
	if _, err := fixture.review.ListRecoveryMatters(ctx, tenant, -1); err == nil {
		t.Error("负页大小的追偿上列没有被拒")
	}

	episodes, err := fixture.review.ListSignalEpisodes(ctx, tenant, 5)
	if err != nil || len(episodes) != 0 {
		t.Errorf("空发作期册：rows=%+v err=%v", episodes, err)
	}
	requests, err := fixture.review.ListDispositionRequests(ctx, tenant, 5)
	if err != nil || len(requests) != 0 {
		t.Errorf("空处置请求册：rows=%+v err=%v", requests, err)
	}
	cases, err := fixture.review.ListExceptionCases(ctx, tenant, 5)
	if err != nil || len(cases) != 0 {
		t.Errorf("空案件册：rows=%+v err=%v", cases, err)
	}
	notifications, err := fixture.review.ListCustomerNotifications(ctx, tenant, 5)
	if err != nil || len(notifications) != 0 {
		t.Errorf("空通知册：rows=%+v err=%v", notifications, err)
	}
	claims, err := fixture.review.ListClaimItems(ctx, tenant, 5)
	if err != nil || len(claims) != 0 {
		t.Errorf("空索赔册：rows=%+v err=%v", claims, err)
	}
	recoveries, err := fixture.review.ListRecoveryMatters(ctx, tenant, 5)
	if err != nil || len(recoveries) != 0 {
		t.Errorf("空追偿册：rows=%+v err=%v", recoveries, err)
	}
}
