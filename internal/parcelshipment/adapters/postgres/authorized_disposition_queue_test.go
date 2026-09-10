package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「等待授权处置」队列读口（ADR-0132 决定二，票 sa-preacceptance-policy-view/04）：
// `waitingOn = AUTHORIZED_DISPOSITION` 即在列且只有它在列；逐行透出本版当前采用的控制结果里的受限项、
// 正文登记的失败处置与责任引用、受限原因；处置一记下即出队。

// dispositionQueueFixture 把查阅读面、仓储与判断登记册装在同一只库上：受限项从判断表取，与委托行必须同库。
type dispositionQueueFixture struct {
	views      *adapter.ShipmentRequestViews
	repository *adapter.ShipmentRequests
	judgments  *adapter.AcceptanceJudgments
	transactor bentoapp.Transactor
}

func newDispositionQueueFixture(t *testing.T) dispositionQueueFixture {
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
	judgments, err := adapter.NewAcceptanceJudgments(db)
	if err != nil {
		t.Fatalf("构造判断登记册：%v", err)
	}
	return dispositionQueueFixture{views: views, repository: repository, judgments: judgments, transactor: db.Transactor()}
}

// awaitDisposition 让一轮停在「等待授权处置」：其余组全部通过，接受前财务控制译成`无法判定`+ 续办路径
// 「授权处置」（那正是全部受限项登 AUTHORIZED_DISPOSITION 时 FinancialControlCheckFor 的译法）。
func awaitDisposition(t *testing.T, request domain.ShipmentRequest, decisionID string) domain.ShipmentRequest {
	t.Helper()
	checks := make([]domain.AcceptanceCheck, 0, 8)
	for _, check := range allGroupsPassing(t) {
		if check.Group() == domain.PreAcceptanceFinancialControlCheck {
			continue
		}
		checks = append(checks, check)
	}
	control, err := domain.NewUndeterminedAcceptanceCheck(
		domain.PreAcceptanceFinancialControlCheck,
		domain.DeclaredParcelID{},
		mustBuild(t, domain.NewCheckReason, "AVAILABLE_CREDIT_INSUFFICIENT"),
		domain.ResumeByAuthorizedDisposition,
	)
	if err != nil {
		t.Fatalf("未决校验：%v", err)
	}
	decided, err := request.Decide(domain.AcceptanceDecisionSpec{
		DecisionID: mustBuild(t, domain.NewAcceptanceDecisionID, decisionID),
		Checks:     append(checks, control),
		Basis:      acceptanceBasis(t),
		DecidedAt:  submittedAtFixture.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成等待授权处置的一轮：%v", err)
	}
	if path, waiting := decided.AcceptanceDecisionTask().WaitingOn(); !waiting || path != domain.ResumeByAuthorizedDisposition {
		t.Fatalf("前置不成立：waitingOn = %q waiting = %v, want AUTHORIZED_DISPOSITION", path, waiting)
	}
	return decided
}

// recordAdoptedRestrictedControl 往判断登记册记一份「预付冻结成立、信用受限且采用了`进入授权处置`」的控制结果。
func recordAdoptedRestrictedControl(
	t *testing.T,
	fixture dispositionQueueFixture,
	tenant, requestID, version, resultID string,
) {
	t.Helper()
	adopted, err := domain.NewAdoptedControlDisposition(
		domain.AuthorizedDispositionOnControlFailure,
		mustBuild(t, domain.NewControlResponsibilityReference, "CONTRACT-CLAUSE-7"))
	if err != nil {
		t.Fatalf("形成采用引用：%v", err)
	}
	control, err := executedControlResult(t, resultID, taskAsOfFirst,
		controlItem(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied, ""),
		controlItem(t, domain.CreditCheckControlItem, 2, domain.ControlItemRestricted, "AVAILABLE_CREDIT_INSUFFICIENT"),
	).AdoptControlDispositions(map[domain.ControlItemKind]domain.AdoptedControlDisposition{
		domain.CreditCheckControlItem: adopted,
	})
	if err != nil {
		t.Fatalf("采用处置：%v", err)
	}
	mustWithinTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		return fixture.judgments.RecordFinancialControlResult(txCtx,
			mustBuild(t, domain.NewTenantID, tenant), taskRequestID(t, requestID), taskVersion(t, version), control)
	})
}

// Covers: 「队列 = 事实」——`waitingOn = AUTHORIZED_DISPOSITION` 即在列且只有它在列：停等复核的、他租户停等处置的、
// 作用域外账户停等处置的都不出现；受限项、失败处置、责任引用与受限原因从本版当前采用的控制结果照登记透出，
// 成立项不上列；最近处理记录照登记转写。
func TestListAwaitingAuthorizedDispositionFiltersAndTranscribesRestrictedItems(t *testing.T) {
	fixture := newDispositionQueueFixture(t)
	ctx := t.Context()

	seedAdvanced(t, fixture.repository, fixture.transactor, "tenant-1", "customer-1", "adq-a", "REQ-DISP-A",
		submittedAtFixture, func(request domain.ShipmentRequest) domain.ShipmentRequest {
			appended, err := request.RecordProcessingAttempt(processingAttempt(t))
			if err != nil {
				t.Fatalf("记处理尝试：%v", err)
			}
			return awaitDisposition(t, appended, "dispose-a")
		})
	recordAdoptedRestrictedControl(t, fixture, "tenant-1", "REQ-DISP-A", "version-1", "REQ-DISP-A/version-1")
	// 反例三行：停等复核、他租户停等处置、作用域外账户停等处置。
	seedAdvanced(t, fixture.repository, fixture.transactor, "tenant-1", "customer-2", "adq-b", "REQ-DISP-B",
		submittedAtFixture.Add(time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitReview(t, request, "review-b")
		})
	seedAdvanced(t, fixture.repository, fixture.transactor, "tenant-2", "customer-1", "adq-c", "REQ-DISP-C",
		submittedAtFixture.Add(2*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitDisposition(t, request, "dispose-c")
		})
	seedAdvanced(t, fixture.repository, fixture.transactor, "tenant-1", "customer-3", "adq-d", "REQ-DISP-D",
		submittedAtFixture.Add(3*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitDisposition(t, request, "dispose-d")
		})

	rows, err := fixture.views.ListAwaitingAuthorizedDisposition(ctx, viewScope(t, "tenant-1", "customer-1", "customer-2"), 10)
	if err != nil {
		t.Fatalf("查授权处置队列：%v", err)
	}
	if len(rows) != 1 || rows[0].ShipmentRequestID.String() != "REQ-DISP-A" {
		t.Fatalf("rows = %+v, want only REQ-DISP-A（停等复核、他租户、作用域外账户都不该在列）", rows)
	}
	row := rows[0]
	if row.CustomerAccountID.String() != "customer-1" || row.State != domain.ShipmentRequestSubmitted ||
		row.SubmissionVersionID.String() != "version-1" || !row.SubmittedAt.Equal(submittedAtFixture) {
		t.Fatalf("概要变形：%+v", row)
	}
	if !row.HasAttempt || row.LastAttemptReason != "COMMERCIAL_BASIS_UNAVAILABLE" {
		t.Fatalf("最近处理记录变形：%+v", row)
	}
	if row.ControlResultID.String() != "REQ-DISP-A/version-1" {
		t.Fatalf("control result id = %q", row.ControlResultID)
	}
	if len(row.RestrictedItems) != 1 {
		t.Fatalf("restricted items = %+v, want the single restricted CREDIT_CHECK（成立项不上列）", row.RestrictedItems)
	}
	item := row.RestrictedItems[0]
	if item.Kind != domain.CreditCheckControlItem || item.Order != 2 ||
		item.Basis.String() != "AVAILABLE_CREDIT_INSUFFICIENT" ||
		item.FailureDisposition != domain.AuthorizedDispositionOnControlFailure ||
		item.Responsibility.String() != "CONTRACT-CLAUSE-7" {
		t.Fatalf("受限项透出 %+v", item)
	}
}

// Covers: 出队靠事实不靠读侧折叠——处置一记下（`交客户补充`），等待态转到`等待受控补充`，这份委托从授权处置
// 队列消失、出现在受控补充队列；`拒绝`去向则委托离开`已提交`，同样不在列。
func TestADispositionLeavesTheAuthorizedDispositionQueue(t *testing.T) {
	fixture := newDispositionQueueFixture(t)
	ctx := t.Context()

	seedAdvanced(t, fixture.repository, fixture.transactor, "tenant-1", "customer-1", "adq-h", "REQ-DISP-H",
		submittedAtFixture, func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitDisposition(t, request, "dispose-h")
		})
	recordAdoptedRestrictedControl(t, fixture, "tenant-1", "REQ-DISP-H", "version-1", "REQ-DISP-H/version-1")
	seedAdvanced(t, fixture.repository, fixture.transactor, "tenant-1", "customer-1", "adq-r", "REQ-DISP-R",
		submittedAtFixture.Add(time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
			return awaitDisposition(t, request, "dispose-r")
		})
	recordAdoptedRestrictedControl(t, fixture, "tenant-1", "REQ-DISP-R", "version-1", "REQ-DISP-R/version-1")

	scope := viewScope(t, "tenant-1", "customer-1")
	before, err := fixture.views.ListAwaitingAuthorizedDisposition(ctx, scope, 10)
	if err != nil || len(before) != 2 {
		t.Fatalf("处置前 rows = %d err = %v, want 2", len(before), err)
	}

	handedIdentity := requestScopedIdentity(t, "tenant-1", "customer-1", "adq-h")
	handed, found, err := fixture.repository.FindBySourceIdentity(ctx, handedIdentity)
	if err != nil || !found {
		t.Fatalf("读回 REQ-DISP-H：found=%v err=%v", found, err)
	}
	disposed, err := handed.DisposeUnderAuthority(domain.DisposeUnderAuthoritySpec{
		Disposition: authorizedDispositionFixture(t, domain.DisposeByCustomerSupplement),
	})
	if err != nil {
		t.Fatalf("处置为交客户补充：%v", err)
	}
	mustSave(t, fixture.transactor, ctx, fixture.repository, handedIdentity, disposed)

	rejectedIdentity := requestScopedIdentity(t, "tenant-1", "customer-1", "adq-r")
	toReject, found, err := fixture.repository.FindBySourceIdentity(ctx, rejectedIdentity)
	if err != nil || !found {
		t.Fatalf("读回 REQ-DISP-R：found=%v err=%v", found, err)
	}
	rejected, err := toReject.DisposeUnderAuthority(domain.DisposeUnderAuthoritySpec{
		Disposition: authorizedDispositionFixture(t, domain.DisposeByRejection),
		DecisionID:  mustBuild(t, domain.NewAcceptanceDecisionID, "decision-dispose-r"),
	})
	if err != nil {
		t.Fatalf("处置为拒绝：%v", err)
	}
	mustSave(t, fixture.transactor, ctx, fixture.repository, rejectedIdentity, rejected)

	after, err := fixture.views.ListAwaitingAuthorizedDisposition(ctx, scope, 10)
	if err != nil {
		t.Fatalf("处置后查队列：%v", err)
	}
	if len(after) != 0 {
		t.Fatalf("处置后 rows = %+v, want 0——两个去向都该让委托出队", after)
	}
	supplement, err := fixture.views.ListWaitingOnCustomerSupplement(ctx, scope, 10)
	if err != nil {
		t.Fatalf("查受控补充队列：%v", err)
	}
	if len(supplement) != 1 || supplement[0].ShipmentRequestID.String() != "REQ-DISP-H" {
		t.Fatalf("supplement rows = %+v, want REQ-DISP-H（交客户补充转到了等待受控补充）", supplement)
	}
}
