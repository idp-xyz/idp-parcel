package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfparcelshipment "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/parcelshipment"
)

// 本文件对真实 PostgreSQL 证收寄后服务处置决定到待人工处理项的整条进程（ADR-0341；票 launch-wiring/05），夹具全为合成（S 级）。
// 决定编排与登记编排是生产那一套真库缝（决定链、请求册、采用链、终局库、取消册、信封交接、取消边界锁、待人工处理项），派发一拍走生产 wireDispatcher；
// HTTP 面的授权与响应形状由 cmd/parcel-api 的用例钉。

type postIntakeManualFixture struct {
	*synVerticalFixture
	decide   *psapplication.DecidePostIntakeDispositionHandler
	register *psapplication.RegisterPostIntakeManualItemResultHandler
	items    *pspostgres.PostIntakeManualItems
}

func newPostIntakeManualFixture(t *testing.T) *postIntakeManualFixture {
	t.Helper()
	base := newSYNVerticalFixture(t)
	db := base.db
	dispositions, err := pspostgres.NewPostIntakeDispositions(db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := pspostgres.NewPostIntakeDispositionRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	adoptions, err := pspostgres.NewIntakeAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		t.Fatal(err)
	}
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		t.Fatal(err)
	}
	locks, err := pspostgres.NewCancellationBoundaryLocks(db)
	if err != nil {
		t.Fatal(err)
	}
	items, err := pspostgres.NewPostIntakeManualItems(db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := pspostgres.NewOutboxPostIntakeDispositionHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	return &postIntakeManualFixture{
		synVerticalFixture: base,
		items:              items,
		decide: psapplication.NewDecidePostIntakeDispositionHandler(psapplication.DecidePostIntakeDispositionDeps{
			Dispositions: dispositions, Requests: requests, Adoptions: adoptions, Finals: finals, Cancellations: cancellations,
			Handoff: handoff, ManualItems: items, BoundaryLock: locks, Clock: systemClock{},
		}),
		register: psapplication.NewRegisterPostIntakeManualItemResultHandler(psapplication.RegisterPostIntakeManualItemResultDeps{
			ManualItems: items, Clock: systemClock{},
		}),
	}
}

// arrangeIntake 摆出一件已有责任起点（越过取消边界）的包裹：收寄采用在册。
func (fixture *postIntakeManualFixture) arrangeIntake(t *testing.T, parcel string) {
	t.Helper()
	adoptions, err := pspostgres.NewIntakeAdoptions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	parcelID := mustPS(t, psdomain.NewDeclaredParcelID, parcel)
	version := mustPS(t, psdomain.NewSourceResultVersion, "intake-result/v1")
	intakeAt := time.Now().UTC().Add(-2 * time.Hour)
	source, err := psdomain.NewIntakeSource(psdomain.IntakeSourceSpec{
		Kind:       psdomain.NodeIntakeSource,
		Object:     mustPS(t, psdomain.NewSourceObjectReference, "SYN-UNIT-"+parcel),
		Parcel:     parcelID,
		Place:      mustPS(t, psdomain.NewIntakePlaceReference, "SYN-NODE-ORIGIN"),
		Control:    mustPS(t, psdomain.NewIntakeControlReference, "SYN-CONTROL-"+parcel),
		Version:    version,
		OccurredAt: intakeAt,
	})
	if err != nil {
		t.Fatalf("收寄来源：%v", err)
	}
	intake, err := psdomain.AdoptNetworkIntake(source, mustPS(t, psdomain.NewSubmissionVersionID, "SYN-VERSION-1"))
	if err != nil {
		t.Fatalf("采用收寄：%v", err)
	}
	commitment, err := psdomain.FormFormalCommitment(
		mustPS(t, psdomain.NewCommitmentVersionID, "SYN-CMT-"+parcel), intake,
		mustPS(t, psdomain.NewExpectedCommitmentReference, "SYN-RES-1@2026-10-01T00:00:00Z"))
	if err != nil {
		t.Fatalf("正式承诺：%v", err)
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		_, err := adoptions.Save(txCtx, psports.IntakeAdoptionRecord{
			Key:               psports.IntakeAdoptionKey{TenantID: fixture.identity.TenantID(), Parcel: parcelID, Kind: psdomain.NodeIntakeSource, Version: version},
			CustomerAccountID: mustPS(t, psdomain.NewCustomerAccountID, "SYN-ACCOUNT-1"),
			ShipmentRequestID: mustPS(t, psdomain.NewShipmentRequestID, "SYN-REQUEST-"+parcel),
			ContentDigest:     "digest-adoption-" + parcel,
			Adopted:           true,
			Intake:            intake,
			Commitment:        commitment,
			AdoptedAt:         intakeAt.Add(time.Minute),
		})
		return err
	})
}

func (fixture *postIntakeManualFixture) decideOperatorInitiated(t *testing.T, parcel string, direction psdomain.PostIntakeDispositionDirection) {
	t.Helper()
	command := psapplication.DecidePostIntakeDispositionCommand{
		Tenant:    fixture.identity.TenantID(),
		Parcel:    mustPS(t, psdomain.NewDeclaredParcelID, parcel),
		Direction: direction,
		Decider:   mustPS(t, psdomain.NewPostIntakeDispositionDeciderReference, "https://id.syn.example#decider"),
		Basis:     mustPS(t, psdomain.NewPostIntakeDispositionBasis, "SYN-BASIS-"+parcel),
	}
	var result psapplication.DecidePostIntakeDispositionResult
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.decide.Handle(txCtx, command)
		return err
	})
	if result.Outcome() != psapplication.PostIntakeDispositionDecided {
		t.Fatalf("决定 %s 的结果 %s, want DECIDED", parcel, result.Outcome())
	}
}

func (fixture *postIntakeManualFixture) registerResult(
	t *testing.T,
	parcel string,
	owner psdomain.DispositionOwner,
	action psdomain.DispositionOwnerAction,
	outcome psdomain.ManualItemOutcome,
	note string,
) psapplication.PostIntakeManualRegistrationOutcome {
	t.Helper()
	command := psapplication.RegisterPostIntakeManualItemResultCommand{
		Key: psdomain.ManualItemKey{
			Tenant: fixture.identity.TenantID(), Parcel: mustPS(t, psdomain.NewDeclaredParcelID, parcel),
			DecisionVersion: 1, Owner: owner, Action: action,
		},
		Outcome: outcome,
		Note:    mustPS(t, psdomain.NewManualItemNote, note),
		Handler: mustPS(t, psdomain.NewManualItemHandlerReference, "https://id.syn.example#handler"),
	}
	var result psapplication.RegisterPostIntakeManualItemResultResult
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.register.Handle(txCtx, command)
		return err
	})
	return result.Outcome()
}

// Covers: AT-PS-083（退运决定形成并转 UC-TF-007：退运旅程开出，不表示退运完成）、AT-PS-088（处置请求的承接结果由 PS 读回）；ADR-0341。
// 演示租户同形的合成租户上：「退运」决定 → 派发后 TF 退运旅程开出，NR 的规划退运路径与 NO 的按退运装载进待人工处理清单（TF 的项不进）；「拦截」决定 → 不开旅程，
// NO 扣住与 TF 扣住两项进清单；运营逐项登记处理结果，PS 按包裹读回各项结果，清单清空。
func TestAReturnAndAnInterceptDecisionLeaveManualItemsThatOperatorsRegisterAndPSReadsBack(t *testing.T) {
	fixture := newPostIntakeManualFixture(t)
	const returned, intercepted = "SYN-PARCEL-MAN-RET", "SYN-PARCEL-MAN-INT"
	fixture.arrangeIntake(t, returned)
	fixture.arrangeIntake(t, intercepted)

	fixture.decideOperatorInitiated(t, returned, psdomain.PostIntakeReturn)
	fixture.dispatchUntilQuiet(t)
	if n := fixture.countSQL(t, `SELECT count(*) FROM transport_fulfillment.alternate_journey WHERE journey_id = 'JRN/`+returned+`/PID-1' AND purpose = 'RETURN'`); n != 1 {
		t.Fatalf("退运旅程 %d 条, want 1", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM bento.outbox WHERE event_type = 'parcel-shipment.post-intake-disposition.decided' AND failure_code IS NOT NULL`); n != 0 {
		t.Fatalf("带失败码的处置信封 %d 封, want 0", n)
	}
	pending, err := fixture.items.ListPendingPostIntakeManualItems(t.Context(), fixture.identity.TenantID(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != 2 || len(pending.Items) != 2 {
		t.Fatalf("退运之后待人工处理项 %d（列出 %d）, want 2", pending.Count, len(pending.Items))
	}
	for _, item := range pending.Items {
		if item.Key.Owner == psdomain.DispositionOwnerTransportFulfillment || item.HasResult {
			t.Fatalf("清单里出现不该有的项 %+v", item)
		}
	}

	fixture.decideOperatorInitiated(t, intercepted, psdomain.PostIntakeIntercept)
	fixture.dispatchUntilQuiet(t)
	if n := fixture.countSQL(t, `SELECT count(*) FROM transport_fulfillment.alternate_journey WHERE journey_id = 'JRN/`+intercepted+`/PID-1'`); n != 0 {
		t.Fatalf("拦截开出了旅程 %d 条, want 0", n)
	}
	if pending, err = fixture.items.ListPendingPostIntakeManualItems(t.Context(), fixture.identity.TenantID(), 50); err != nil || pending.Count != 4 {
		t.Fatalf("拦截之后待人工处理项 %d（%v）, want 4——退运两项加拦截两项", pending.Count, err)
	}

	const nr, no, tf = psdomain.DispositionOwnerNetworkRouting, psdomain.DispositionOwnerNodeOperations, psdomain.DispositionOwnerTransportFulfillment
	handled, unable := psdomain.ManualItemHandled, psdomain.ManualItemUnable
	if got := fixture.registerResult(t, returned, nr, psdomain.DispositionActionPlanReturnRoute, handled, "已在系统外重新规划退运路径"); got != psapplication.PostIntakeManualOutcomeRegistered {
		t.Fatalf("首次登记 = %s", got)
	}
	if got := fixture.registerResult(t, returned, nr, psdomain.DispositionActionPlanReturnRoute, handled, "已在系统外重新规划退运路径"); got != psapplication.PostIntakeManualOutcomeAlreadyRegistered {
		t.Fatalf("同内容重放 = %s, want ALREADY_REGISTERED", got)
	}
	if got := fixture.registerResult(t, returned, nr, psdomain.DispositionActionPlanReturnRoute, unable, "改口"); got != psapplication.PostIntakeManualOutcomeConflict {
		t.Fatalf("同项异内容 = %s, want RESULT_CONFLICT", got)
	}
	fixture.registerResult(t, returned, no, psdomain.DispositionActionLoadForReturn, handled, "已按退运装载并交出")
	fixture.registerResult(t, intercepted, no, psdomain.DispositionActionHoldAtControl, handled, "已在控制处扣住")
	fixture.registerResult(t, intercepted, tf, psdomain.DispositionActionHoldOnwardMovement, unable, "承运商已提货，无法截停")

	if pending, err = fixture.items.ListPendingPostIntakeManualItems(t.Context(), fixture.identity.TenantID(), 50); err != nil || pending.Count != 0 {
		t.Fatalf("全部登记后待人工处理项 %d（%v）, want 0", pending.Count, err)
	}
	read, err := fixture.items.ListPostIntakeManualItemsOfParcel(t.Context(), fixture.identity.TenantID(), mustPS(t, psdomain.NewDeclaredParcelID, intercepted))
	if err != nil || len(read) != 2 {
		t.Fatalf("PS 读回拦截包裹的项 %d（%v）, want 2", len(read), err)
	}
	for _, item := range read {
		if !item.HasResult {
			t.Fatalf("读回的项没有结果 %+v", item)
		}
		if item.Key.Owner == tf && (item.Result.Outcome != unable || item.Result.Note.String() != "承运商已提货，无法截停") {
			t.Fatalf("TF 扣住的读回结果 %+v", item.Result)
		}
	}
	// 冲突没有覆盖先前的结果。
	read, _ = fixture.items.ListPostIntakeManualItemsOfParcel(t.Context(), fixture.identity.TenantID(), mustPS(t, psdomain.NewDeclaredParcelID, returned))
	for _, item := range read {
		if item.Key.Owner == nr && item.Result.Outcome != handled {
			t.Fatalf("异内容的登记覆盖了先前的结果 %+v", item.Result)
		}
	}
}

// Covers: ADR-0341 决定一的护栏——PS 请求表里「系统执行」为真的（所有者，动作）恰好等于运输履约适配器真正执行的集合。多了，是有动作被当成系统执行而无人执行，
// 既不开旅程也不落待人工处理项，静默成功；少了，是适配器执行的动作又落了一份人工项，重复。
func TestTheActionsMarkedSystemExecutedAreExactlyTheOnesTheTransportAdapterExecutes(t *testing.T) {
	marked := map[[2]string]bool{}
	for _, direction := range []psdomain.PostIntakeDispositionDirection{
		psdomain.PostIntakeContinue, psdomain.PostIntakeIntercept, psdomain.PostIntakeRedirect, psdomain.PostIntakeReturn, psdomain.PostIntakeTerminateService,
	} {
		for _, request := range direction.OwnerRequests() {
			if request.Action.ExecutedBySystem() {
				marked[[2]string{request.Owner.String(), request.Action.String()}] = true
			}
		}
	}
	handled := map[[2]string]bool{}
	for _, pair := range tfparcelshipment.HandledPostIntakeActions() {
		handled[pair] = true
	}
	if len(marked) != len(handled) {
		t.Fatalf("系统执行的集合 %v 与适配器执行的集合 %v 不等", marked, handled)
	}
	for pair := range marked {
		if !handled[pair] {
			t.Fatalf("%v 标为系统执行，适配器不执行它", pair)
		}
	}
	// 全集里每个动作要么系统执行，要么出现在某个去向的人工项里：没有第三种。
	manual := map[string]bool{}
	for _, direction := range []psdomain.PostIntakeDispositionDirection{psdomain.PostIntakeIntercept, psdomain.PostIntakeRedirect, psdomain.PostIntakeReturn, psdomain.PostIntakeTerminateService} {
		for _, request := range direction.ManualOwnerRequests() {
			manual[request.Action.String()] = true
		}
	}
	for _, action := range psdomain.DispositionOwnerActions() {
		if !action.ExecutedBySystem() && !manual[action.String()] {
			t.Errorf("动作 %s 既非系统执行、也不在任何去向的人工项里", action)
		}
	}
}
