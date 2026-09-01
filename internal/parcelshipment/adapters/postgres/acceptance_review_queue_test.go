package postgres_test

import (
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 16 证复核队列读面（票 acceptance-review-read-face/01）：
// 队列的定义是任务文档上的 `waitingOn = MANUAL_REVIEW`——过滤在 SQL 键上，等待别的
// 续办方、不等人、他租户的行都进不来；排序老的在前（先来先审）；复核已录完成但未
// 续办的行照列并带留痕。

// awaitReview 把一份委托推进到「等待人工复核」：全过 + 规则要求复核而复核未完成，
// Decide 写下 waitingOn 后本轮不形成决定（domain/acceptance_decision.go 341 行）。
func awaitReview(t *testing.T, request domain.ShipmentRequest, decisionID string) domain.ShipmentRequest {
	t.Helper()
	decided, err := request.Decide(domain.AcceptanceDecisionSpec{
		DecisionID: mustBuild(t, domain.NewAcceptanceDecisionID, decisionID),
		Checks:     allGroupsPassing(t),
		Basis:      basisWithReviewPolicy(t, domain.ManualReviewRequiredByRules),
		DecidedAt:  submittedAtFixture.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成等待复核的一轮：%v", err)
	}
	if path, waiting := decided.AcceptanceDecisionTask().WaitingOn(); !waiting || path != domain.ResumeByManualReview {
		t.Fatalf("前置不成立：waitingOn = %q waiting = %v, want MANUAL_REVIEW", path, waiting)
	}
	return decided
}

// awaitInternalRetry 让一轮停在「等待内部续办」：单条未决校验即可，依据不要求复核。
func awaitInternalRetry(t *testing.T, request domain.ShipmentRequest, decisionID string) domain.ShipmentRequest {
	t.Helper()
	check, err := domain.NewUndeterminedAcceptanceCheck(
		domain.CustomerRelationshipCheck,
		domain.DeclaredParcelID{},
		mustBuild(t, domain.NewCheckReason, "DEPENDENCY_TIMEOUT"),
		domain.ResumeByInternalRetry,
	)
	if err != nil {
		t.Fatalf("未决校验：%v", err)
	}
	decided, err := request.Decide(domain.AcceptanceDecisionSpec{
		DecisionID: mustBuild(t, domain.NewAcceptanceDecisionID, decisionID),
		Checks:     []domain.AcceptanceCheck{check},
		Basis:      acceptanceBasis(t),
		DecidedAt:  submittedAtFixture.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成停等内部续办的一轮：%v", err)
	}
	return decided
}

func completedReview(t *testing.T) domain.ManualReviewCompletion {
	t.Helper()
	completion, err := domain.NewManualReviewCompletion(domain.ManualReviewCompletionSpec{
		Authority:   mustBuild(t, domain.NewReviewAuthorityReference, "AUTH-RULE-9"),
		Reviewer:    mustBuild(t, domain.NewReviewerReference, "REVIEWER-OP-7"),
		Evidence:    mustBuild(t, domain.NewReviewEvidenceReference, "EVIDENCE-42"),
		CompletedAt: submittedAtFixture.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("复核完成留痕：%v", err)
	}
	return completion
}

// seedAdvanced 建单入库，按 advance 推进后保存。advance 为 nil 时只建单。
func seedAdvanced(
	t *testing.T,
	repository *adapter.ShipmentRequests,
	transactor bentoapp.Transactor,
	tenant, customer, key, requestID string,
	submittedAt time.Time,
	advance func(domain.ShipmentRequest) domain.ShipmentRequest,
) {
	t.Helper()
	ctx := t.Context()
	identity := requestScopedIdentity(t, tenant, customer, key)
	mustInsert(t, transactor, ctx, repository,
		scopedSubmittedRequest(t, tenant, customer, key, requestID, submittedAt))
	if advance == nil {
		return
	}
	loaded, found, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回 %s：found=%v err=%v", requestID, found, err)
	}
	mustSave(t, transactor, ctx, repository, identity, advance(loaded))
}

// Covers: 票 01「队列 = 事实」——`waitingOn = MANUAL_REVIEW` 即在列且只有它在列：
// 没起判断的、停等内部续办的、他租户停等复核的都不出现；先来先审老的在前；最近处理
// 记录与复核完成留痕照登记转写（复核已录完成但未续办的行照列并标示，不折成出队）。
func TestListAwaitingManualReviewFiltersOrdersAndTranscribes(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	// 老的等待行：带一条处理记录，复核未录完成。
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-1", "arq-a", "REQ-RVW-A",
		submittedAtFixture, func(request domain.ShipmentRequest) domain.ShipmentRequest {
			appended, err := request.RecordProcessingAttempt(processingAttempt(t))
			if err != nil {
				t.Fatalf("记处理尝试：%v", err)
			}
			return awaitReview(t, appended, "review-a")
		})
	// 新的等待行：复核已录完成、等下一轮判断续办。
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-2", "arq-b", "REQ-RVW-B",
		submittedAtFixture.Add(time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			waiting := awaitReview(t, request, "review-b")
			reviewed, err := waiting.CompleteManualReview(completedReview(t))
			if err != nil {
				t.Fatalf("录复核完成：%v", err)
			}
			return reviewed
		})
	// 反例三行：没起判断、停等内部续办、他租户停等复核。
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-1", "arq-c", "REQ-RVW-C",
		submittedAtFixture.Add(2*time.Hour), nil)
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-2", "arq-d", "REQ-RVW-D",
		submittedAtFixture.Add(3*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitInternalRetry(t, request, "retry-d")
		})
	seedAdvanced(t, repository, transactor, "tenant-2", "customer-1", "arq-e", "REQ-RVW-E",
		submittedAtFixture.Add(4*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitReview(t, request, "review-e")
		})

	rows, err := views.ListAwaitingManualReview(ctx,
		viewScope(t, "tenant-1", "customer-1", "customer-2"), 10)
	if err != nil {
		t.Fatalf("查复核队列：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2（未起判断、停等内部续办、他租户都不该在列）", len(rows))
	}
	if rows[0].ShipmentRequestID.String() != "REQ-RVW-A" || rows[1].ShipmentRequestID.String() != "REQ-RVW-B" {
		t.Fatalf("排序 = [%s %s], want [REQ-RVW-A REQ-RVW-B]（先来先审老的在前）",
			rows[0].ShipmentRequestID, rows[1].ShipmentRequestID)
	}

	oldest := rows[0]
	if oldest.CustomerAccountID.String() != "customer-1" ||
		oldest.Source.String() != "portal" ||
		oldest.SourceRequestKey.String() != "arq-a" ||
		oldest.State != domain.ShipmentRequestSubmitted ||
		oldest.SubmissionVersionID.String() != "version-1" ||
		oldest.DeclaredParcelCount != 2 ||
		!oldest.SubmittedAt.Equal(submittedAtFixture) {
		t.Fatalf("概要变形：%+v", oldest)
	}
	if !oldest.HasAttempt ||
		oldest.LastAttemptReason != "COMMERCIAL_BASIS_UNAVAILABLE" ||
		oldest.LastAttemptContinuation != "CONT-1" ||
		!oldest.LastAttemptedAt.Equal(submittedAtFixture.Add(time.Hour)) {
		t.Fatalf("最近处理记录变形：%+v", oldest)
	}
	if oldest.ReviewCompleted {
		t.Fatalf("REQ-RVW-A 未录复核完成，却带了留痕：%+v", oldest)
	}

	reviewed := rows[1]
	if !reviewed.ReviewCompleted ||
		reviewed.ReviewAuthority != "AUTH-RULE-9" ||
		reviewed.ReviewReviewer != "REVIEWER-OP-7" ||
		reviewed.ReviewEvidence != "EVIDENCE-42" ||
		!reviewed.ReviewCompletedAt.Equal(submittedAtFixture.Add(3*time.Hour)) {
		t.Fatalf("复核完成留痕变形：%+v", reviewed)
	}
	if reviewed.HasAttempt {
		t.Fatalf("REQ-RVW-B 没记过处理尝试，却带了记录：%+v", reviewed)
	}
}

func TestListAwaitingManualReviewHonorsTheLimit(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	seedAdvanced(t, repository, transactor, "tenant-1", "customer-1", "arq-f", "REQ-RVW-F",
		submittedAtFixture, func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitReview(t, request, "review-f")
		})
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-1", "arq-g", "REQ-RVW-G",
		submittedAtFixture.Add(time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitReview(t, request, "review-g")
		})

	rows, err := views.ListAwaitingManualReview(ctx, viewScope(t, "tenant-1", "customer-1"), 1)
	if err != nil {
		t.Fatalf("查复核队列：%v", err)
	}
	if len(rows) != 1 || rows[0].ShipmentRequestID.String() != "REQ-RVW-F" {
		t.Fatalf("rows = %+v, want 只有最老的 REQ-RVW-F", rows)
	}

	if _, err := views.ListAwaitingManualReview(ctx, viewScope(t, "tenant-1", "customer-1"), 0); err == nil {
		t.Fatal("非正 limit 是调用方编程错误，不该静默答一页")
	}
}

// Covers: AcceptanceTaskViewRecord 的加性扩展——详情读面把等待态与复核完成留痕如实
// 带回（复核详情复用委托查阅详情，页上「停在哪、复核录了没」从这里来）。
func TestFindVisibleCarriesWaitAndReviewCompletion(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	seedAdvanced(t, repository, transactor, "tenant-1", "customer-1", "arq-h", "REQ-RVW-H",
		submittedAtFixture, func(request domain.ShipmentRequest) domain.ShipmentRequest {
			waiting := awaitReview(t, request, "review-h")
			reviewed, err := waiting.CompleteManualReview(completedReview(t))
			if err != nil {
				t.Fatalf("录复核完成：%v", err)
			}
			return reviewed
		})

	detail, found, err := views.FindVisibleByID(ctx,
		viewScope(t, "tenant-1", "customer-1"),
		mustBuild(t, domain.NewShipmentRequestID, "REQ-RVW-H"))
	if err != nil || !found {
		t.Fatalf("查详情：found=%v err=%v", found, err)
	}
	if detail.Task.WaitingOn != domain.ResumeByManualReview {
		t.Fatalf("waitingOn = %q, want MANUAL_REVIEW", detail.Task.WaitingOn)
	}
	if !detail.Task.ReviewCompleted ||
		detail.Task.ReviewAuthority != "AUTH-RULE-9" ||
		detail.Task.ReviewReviewer != "REVIEWER-OP-7" ||
		detail.Task.ReviewEvidence != "EVIDENCE-42" ||
		!detail.Task.ReviewCompletedAt.Equal(submittedAtFixture.Add(3*time.Hour)) {
		t.Fatalf("复核完成留痕变形：%+v", detail.Task)
	}
}
