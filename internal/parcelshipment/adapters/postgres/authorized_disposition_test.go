package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证授权处置在委托登记册上的往返（ADR-0132，迁移 0021）：`等待授权处置`经 Decide 真落库并投影到
// task_waiting_on（队列读面按它过滤），处置记录随任务快照整份往返、两去向各自的等待态与状态如实读回。
// 处置授权与占用释放不在这里：那是编排与它两只适配器的事，本文件只证仓储。

var disposedAtFixture = time.Date(2026, 8, 5, 12, 30, 0, 0, time.UTC)

// Covers: 迁移 0021 第一、二件——`等待授权处置`经 Decide 写下、随快照与投影列同一条 SQL 落库，投影值就是领域
// ResumePath 的数字（队列部分索引按它过滤），读回仍停在等处置且没有处置记录。
func TestAnAwaitingDispositionRequestProjectsItsWaitState(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	ctx := t.Context()

	waiting := awaitingDispositionShipmentRequest(t, "req-dispose-1", "request-dispose-1")
	mustInsert(t, transactor, ctx, repository, waiting)

	var projected int16
	if err := pool.QueryRow(ctx,
		`SELECT task_waiting_on FROM parcel_shipment.shipment_request WHERE shipment_request_id = 'request-dispose-1'`,
	).Scan(&projected); err != nil {
		t.Fatalf("读投影列：%v", err)
	}
	if projected != int16(domain.ResumeByAuthorizedDisposition) {
		t.Fatalf("task_waiting_on = %d, want %d（AUTHORIZED_DISPOSITION）——队列按这一列过滤，写错就列不出", projected, domain.ResumeByAuthorizedDisposition)
	}

	found := mustFind(t, repository, ctx, "req-dispose-1")
	path, present := found.AcceptanceDecisionTask().WaitingOn()
	if !present || path != domain.ResumeByAuthorizedDisposition {
		t.Fatalf("waitingOn 读回 %q (present=%v)，want AUTHORIZED_DISPOSITION", path, present)
	}
	if _, recorded := found.AcceptanceDecisionTask().AuthorizedDisposition(); recorded {
		t.Fatal("没处置过的任务读回带着一条处置记录")
	}
}

// Covers: 处置记录随任务快照整份往返（落法照复核完成留痕）——`交客户补充`去向记下后，读回的任务带全部引用与
// 时点、等待态转到`等待受控补充`、投影列同步、委托仍`已提交`；重建后的一版一次闸门仍守得住。
func TestACustomerSupplementDispositionRoundTripsOnTheTask(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	ctx := t.Context()

	waiting := awaitingDispositionShipmentRequest(t, "req-dispose-2", "request-dispose-2")
	mustInsert(t, transactor, ctx, repository, waiting)
	loaded := mustFind(t, repository, ctx, "req-dispose-2")

	handed, err := loaded.DisposeUnderAuthority(domain.DisposeUnderAuthoritySpec{
		Disposition: authorizedDispositionFixture(t, domain.DisposeByCustomerSupplement),
	})
	if err != nil {
		t.Fatalf("处置为交客户补充：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, "req-dispose-2"), handed)

	found := mustFind(t, repository, ctx, "req-dispose-2")
	if found.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED——交客户补充不是决定", found.State())
	}
	path, present := found.AcceptanceDecisionTask().WaitingOn()
	if !present || path != domain.ResumeByCustomerSupplement {
		t.Fatalf("waitingOn 读回 %q (present=%v)，want CUSTOMER_SUPPLEMENT", path, present)
	}
	var projected int16
	if err := pool.QueryRow(ctx,
		`SELECT task_waiting_on FROM parcel_shipment.shipment_request WHERE shipment_request_id = 'request-dispose-2'`,
	).Scan(&projected); err != nil {
		t.Fatalf("读投影列：%v", err)
	}
	if projected != int16(domain.ResumeByCustomerSupplement) {
		t.Fatalf("task_waiting_on = %d, want %d（CUSTOMER_SUPPLEMENT）", projected, domain.ResumeByCustomerSupplement)
	}
	recorded, present := found.AcceptanceDecisionTask().AuthorizedDisposition()
	if !present {
		t.Fatal("处置记录没有随快照读回——一版一次的闸门会放行第二次处置")
	}
	if recorded.Choice() != domain.DisposeByCustomerSupplement ||
		recorded.Authority().String() != "PC-DISPOSE-RULE-1/v1" ||
		recorded.Disposer().String() != "CREDIT-OFFICER-1" ||
		recorded.Reason().String() != "CREDIT_LIMIT_NOT_EXTENDED" ||
		recorded.Evidence().String() != "EVID-DISPOSE-1" ||
		!recorded.DisposedAt().Equal(disposedAtFixture) {
		t.Fatalf("处置记录读回 %+v", recorded)
	}
	if _, err := found.DisposeUnderAuthority(domain.DisposeUnderAuthoritySpec{
		Disposition: authorizedDispositionFixture(t, domain.DisposeByCustomerSupplement),
	}); err == nil {
		t.Fatal("重建后的任务放行了同一版本的第二次处置")
	}
}

// Covers: `拒绝`去向记下后委托转`已拒绝`并落库、投影列清零。读回那一半今天由重建门挡在门外（`已拒绝`尚未
// 开门，ADR-0030），本条只证写入成功与投影——与主动拒绝落库的既有证据同一深度。
func TestARejectionDispositionIsSavedWithItsWaitStateCleared(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	ctx := t.Context()

	waiting := awaitingDispositionShipmentRequest(t, "req-dispose-3", "request-dispose-3")
	mustInsert(t, transactor, ctx, repository, waiting)
	loaded := mustFind(t, repository, ctx, "req-dispose-3")

	rejected, err := loaded.DisposeUnderAuthority(domain.DisposeUnderAuthoritySpec{
		Disposition: authorizedDispositionFixture(t, domain.DisposeByRejection),
		DecisionID:  mustBuild(t, domain.NewAcceptanceDecisionID, "decision-dispose-3"),
	})
	if err != nil {
		t.Fatalf("处置为拒绝：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, "req-dispose-3"), rejected)

	var state, projected int16
	if err := pool.QueryRow(ctx,
		`SELECT state, task_waiting_on FROM parcel_shipment.shipment_request WHERE shipment_request_id = 'request-dispose-3'`,
	).Scan(&state, &projected); err != nil {
		t.Fatalf("读状态与投影列：%v", err)
	}
	if state != int16(domain.ShipmentRequestRejected) {
		t.Fatalf("state = %d, want %d（REJECTED）", state, domain.ShipmentRequestRejected)
	}
	if projected != 0 {
		t.Fatalf("task_waiting_on = %d, want 0——决定形成后等待态要清零，否则队列会把一份已决委托再列给处置角色", projected)
	}
}

// awaitingDispositionShipmentRequest 经 Decide 把一份`已提交`委托停到`等待授权处置`：其余组全部通过，接受前财务
// 控制是`无法判定`+ 续办路径「授权处置」（那正是 FinancialControlCheckFor 对全部受限项登 AUTHORIZED_DISPOSITION
// 的译法，这里直接造校验，仓储用例不再重走翻译）。
func awaitingDispositionShipmentRequest(t *testing.T, key, requestID string) domain.ShipmentRequest {
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
		mustBuild(t, domain.NewCheckReason, "CREDIT_CHECK_INSUFFICIENT"),
		domain.ResumeByAuthorizedDisposition,
	)
	if err != nil {
		t.Fatalf("形成等处置的校验：%v", err)
	}
	spec := acceptanceDecisionSpec(t)
	spec.Checks = append(checks, control)
	waiting, err := submittedShipmentRequest(t, key, requestID).Decide(spec)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	path, present := waiting.AcceptanceDecisionTask().WaitingOn()
	if !present || path != domain.ResumeByAuthorizedDisposition {
		t.Fatalf("夹具 waitingOn = %q (present=%v)，want AUTHORIZED_DISPOSITION", path, present)
	}
	return waiting
}

func authorizedDispositionFixture(t *testing.T, choice domain.AuthorizedDispositionChoice) domain.AuthorizedDisposition {
	t.Helper()
	disposition, err := domain.NewAuthorizedDisposition(domain.AuthorizedDispositionSpec{
		Choice:     choice,
		Authority:  mustBuild(t, domain.NewDispositionAuthorityReference, "PC-DISPOSE-RULE-1/v1"),
		Disposer:   mustBuild(t, domain.NewDisposerReference, "CREDIT-OFFICER-1"),
		Reason:     mustBuild(t, domain.NewDispositionReasonReference, "CREDIT_LIMIT_NOT_EXTENDED"),
		Evidence:   mustBuild(t, domain.NewDispositionEvidenceReference, "EVID-DISPOSE-1"),
		DisposedAt: disposedAtFixture,
	})
	if err != nil {
		t.Fatalf("形成处置记录：%v", err)
	}
	return disposition
}
