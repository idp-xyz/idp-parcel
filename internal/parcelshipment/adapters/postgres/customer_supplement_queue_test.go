package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 16 证「等待受控补充」队列读口（ADR-0106 Consequences，票
// first-tenant-runway/09）：`waitingOn = CUSTOMER_SUPPLEMENT` 即在列且只有它在列。这一格在
// ADR-0106 之前结构上恒空（等待态整笔回滚），本文件是它第一次在库里筛得出行的取证。

// awaitSupplement 让一轮停在「等待受控补充」：某一成员的可达性`证据不足`译成未决校验且续办路径
// 为客户补充，依据不要求复核。
func awaitSupplement(t *testing.T, request domain.ShipmentRequest, decisionID string) domain.ShipmentRequest {
	t.Helper()
	check, err := domain.NewUndeterminedAcceptanceCheck(
		domain.NetworkReachabilityCheck,
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"),
		mustBuild(t, domain.NewCheckReason, "REACHABILITY_INSUFFICIENT_EVIDENCE"),
		domain.ResumeByCustomerSupplement,
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
		t.Fatalf("形成等待受控补充的一轮：%v", err)
	}
	if path, waiting := decided.AcceptanceDecisionTask().WaitingOn(); !waiting || path != domain.ResumeByCustomerSupplement {
		t.Fatalf("前置不成立：waitingOn = %q waiting = %v, want CUSTOMER_SUPPLEMENT", path, waiting)
	}
	return decided
}

// Covers: 「队列 = 事实」——`waitingOn = CUSTOMER_SUPPLEMENT` 即在列且只有它在列：没起判断的、
// 停等内部续办的、停等人工复核的、他租户与作用域外账户停等补充的都不出现；先停的在前；最近
// 处理记录照登记转写。
func TestListWaitingOnCustomerSupplementFiltersOrdersAndTranscribes(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	// 老的等待行：带一条处理记录。
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-1", "csq-a", "REQ-SUP-A",
		submittedAtFixture, func(request domain.ShipmentRequest) domain.ShipmentRequest {
			appended, err := request.RecordProcessingAttempt(processingAttempt(t))
			if err != nil {
				t.Fatalf("记处理尝试：%v", err)
			}
			return awaitSupplement(t, appended, "supplement-a")
		})
	// 新的等待行：没记过处理尝试。
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-2", "csq-b", "REQ-SUP-B",
		submittedAtFixture.Add(time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitSupplement(t, request, "supplement-b")
		})
	// 反例五行：没起判断、停等内部续办、停等人工复核、他租户停等补充、作用域外账户停等补充。
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-1", "csq-c", "REQ-SUP-C",
		submittedAtFixture.Add(2*time.Hour), nil)
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-2", "csq-d", "REQ-SUP-D",
		submittedAtFixture.Add(3*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitInternalRetry(t, request, "retry-d")
		})
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-1", "csq-e", "REQ-SUP-E",
		submittedAtFixture.Add(4*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitReview(t, request, "review-e")
		})
	seedAdvanced(t, repository, transactor, "tenant-2", "customer-1", "csq-f", "REQ-SUP-F",
		submittedAtFixture.Add(5*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitSupplement(t, request, "supplement-f")
		})
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-3", "csq-g", "REQ-SUP-G",
		submittedAtFixture.Add(6*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitSupplement(t, request, "supplement-g")
		})

	rows, err := views.ListWaitingOnCustomerSupplement(ctx,
		viewScope(t, "tenant-1", "customer-1", "customer-2"), 10)
	if err != nil {
		t.Fatalf("查受控补充队列：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2（未起判断、停等内部续办、停等复核、他租户、作用域外账户都不该在列）", len(rows))
	}
	if rows[0].ShipmentRequestID.String() != "REQ-SUP-A" || rows[1].ShipmentRequestID.String() != "REQ-SUP-B" {
		t.Fatalf("排序 = [%s %s], want [REQ-SUP-A REQ-SUP-B]（先停的在前）",
			rows[0].ShipmentRequestID, rows[1].ShipmentRequestID)
	}

	oldest := rows[0]
	if oldest.CustomerAccountID.String() != "customer-1" ||
		oldest.Source.String() != "portal" ||
		oldest.SourceRequestKey.String() != "csq-a" ||
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
	if rows[1].HasAttempt {
		t.Fatalf("REQ-SUP-B 没记过处理尝试，却带了记录：%+v", rows[1])
	}
}

// Covers: 出队靠事实不靠读侧折叠——客户补充形成新提交版本后任务换代、等待态清零，这份委托从
// 队列消失；它没有别的出口（ADR-0106 Consequences：续办方是客户，动作是新版本）。
func TestANewSubmissionVersionLeavesTheCustomerSupplementQueue(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()
	identity := requestScopedIdentity(t, "tenant-1", "customer-1", "csq-h")

	seedAdvanced(t, repository, transactor, "tenant-1", "customer-1", "csq-h", "REQ-SUP-H",
		submittedAtFixture, func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitSupplement(t, request, "supplement-h")
		})
	before, err := views.ListWaitingOnCustomerSupplement(ctx, viewScope(t, "tenant-1", "customer-1"), 10)
	if err != nil {
		t.Fatalf("查受控补充队列：%v", err)
	}
	if len(before) != 1 || before[0].ShipmentRequestID.String() != "REQ-SUP-H" {
		t.Fatalf("补充前队列 = %+v, want 恰好 REQ-SUP-H", before)
	}

	waiting, found, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回等待补充的委托：found=%v err=%v", found, err)
	}
	superseded, err := waiting.FormNewSubmissionVersion(domain.NewSubmissionVersionSpec{
		VersionID:         mustBuild(t, domain.NewSubmissionVersionID, "version-2"),
		TaskID:            mustBuild(t, domain.NewAcceptanceDecisionTaskID, "task-2"),
		SourceSubmission:  requestFingerprint(t, "csq-h-supplement", "digest-2"),
		DeclaredParcelIDs: waiting.CurrentSubmissionVersion().DeclaredParcelIDs(),
		EstablishedAt:     submittedAtFixture.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成新提交版本：%v", err)
	}
	mustSave(t, transactor, ctx, repository, identity, superseded)

	after, err := views.ListWaitingOnCustomerSupplement(ctx, viewScope(t, "tenant-1", "customer-1"), 10)
	if err != nil {
		t.Fatalf("查受控补充队列：%v", err)
	}
	if len(after) != 0 {
		t.Fatalf("新版本形成后队列仍列出 %d 行——换代没有清掉上一版的等待态", len(after))
	}
}

func TestListWaitingOnCustomerSupplementHonorsTheLimit(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()

	seedAdvanced(t, repository, transactor, "tenant-1", "customer-1", "csq-i", "REQ-SUP-I",
		submittedAtFixture, func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitSupplement(t, request, "supplement-i")
		})
	seedAdvanced(t, repository, transactor, "tenant-1", "customer-1", "csq-j", "REQ-SUP-J",
		submittedAtFixture.Add(time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitSupplement(t, request, "supplement-j")
		})

	rows, err := views.ListWaitingOnCustomerSupplement(ctx, viewScope(t, "tenant-1", "customer-1"), 1)
	if err != nil {
		t.Fatalf("查受控补充队列：%v", err)
	}
	if len(rows) != 1 || rows[0].ShipmentRequestID.String() != "REQ-SUP-I" {
		t.Fatalf("rows = %+v, want 只有最老的 REQ-SUP-I", rows)
	}

	if _, err := views.ListWaitingOnCustomerSupplement(ctx, viewScope(t, "tenant-1", "customer-1"), 0); err == nil {
		t.Fatal("非正 limit 是调用方编程错误，不该静默答一页")
	}
}
