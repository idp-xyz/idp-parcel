package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/adoptconsume"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/cancellationredrive"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/labelfinal"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pstf "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

// 本文件对真实 PostgreSQL 16 经生产 wireDispatcher 与真库缝证取消先后冲突解开之后的续办（ADR-0295 决定三、四）：三条被取消挡住的链——网络服务的收寄采用、网络服务履约结果
// （有效交付）的终局采用、面单渠道服务的终局判断——在运营认定「取消依据不成立」之后，被它挡住的来源重新进入原来的编排。已接受委托走 SYN-V0 那一套夹具，取消决定与冲突决定都经生产库落册。
//
// 两层证明，分开写明：
//   - 生产路由那一条（TestTheNotUpheldEnvelopeRidesTheProductionRouteAndReentersTheFinalRuleJudgment）：信封经生产路由表投到续办门，作废、回放、未决哨兵登记都是生产装配；
//     交付一路停在「终局规则未配置」这个诚实停点（PAR-COM-17 实例半边，本包一律不种），证明被挡住的来源确实重新进入了规则判断。
//   - 形成责任起点与终局的几条（TestARedriven*）：收寄资格（PAR-COM-16 硬资格取证）与终局规则（PAR-COM-17）都是租户实例半边，生产装配在本包里永远停在它们前面，所以这几条用与生产
//     同一套真库缝装续办适配器，只把这两个实例半边换成「放行」的替身，证明冲突解开之后链确实走到形成。

const (
	notUpheldRedriveConsumerName = "parcel-shipment/redrive-on-cancellation-not-upheld"
	notUpheldEventType           = "parcel-shipment.parcel-cancellation.not-upheld"
)

type establishedEligibility struct{}

func (establishedEligibility) JudgeIntakeEligibility(
	context.Context, psdomain.SourceIdentity, psdomain.ShipmentRequestID, psdomain.IntakeSource,
) (psports.IntakeEligibility, bool, error) {
	return psports.IntakeEligibility{Outcome: psports.IntakeEligibilityEstablished}, true, nil
}

type satisfiedFinalRule struct{ t *testing.T }

func (rule satisfiedFinalRule) JudgeFinalOutcome(
	context.Context, psdomain.SourceIdentity, psdomain.ResponsibilityOutcome,
) (psports.FinalRuleJudgment, bool, error) {
	return psports.FinalRuleJudgment{
		Satisfied:   true,
		Kind:        mustPS(rule.t, psdomain.NewFinalKindReference, "SYN-FINAL-KIND"),
		RuleVersion: mustPS(rule.t, psdomain.NewFinalRuleVersionReference, "SYN-FINAL-RULE-V1"),
	}, true, nil
}

// redriveRig 是与生产同一套真库缝装出来的续办适配器，只把收寄资格与终局规则两个租户实例半边换成放行的替身。
type redriveRig struct {
	redriver *cancellationredrive.Redriver
	decide   *psapplication.DecideCancellationOrderConflictHandler
	fixture  *synVerticalFixture
}

func newRedriveRig(t *testing.T, fixture *synVerticalFixture) *redriveRig {
	t.Helper()
	db := fixture.db
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	clock := systemClock{}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	adoptions, err := pspostgres.NewIntakeAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		t.Fatal(err)
	}
	conflicts, err := pspostgres.NewCancellationOrderConflicts(db)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := pspostgres.NewCancellationOrderConflictDecisions(db)
	if err != nil {
		t.Fatal(err)
	}
	locks, err := pspostgres.NewCancellationBoundaryLocks(db)
	if err != nil {
		t.Fatal(err)
	}
	commitmentIdentities, err := psidentity.NewCommitmentVersions()
	if err != nil {
		t.Fatal(err)
	}
	intakeHandoff, err := pspostgres.NewOutboxNetworkIntakeHandoff(db, store, clock)
	if err != nil {
		t.Fatal(err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		t.Fatal(err)
	}
	finalIdentities, err := psidentity.NewFinalOutcomeVersions()
	if err != nil {
		t.Fatal(err)
	}
	finalHandoff, err := pspostgres.NewOutboxFinalOutcomeHandoff(db, store, clock)
	if err != nil {
		t.Fatal(err)
	}
	transactions, err := pspostgres.NewLabelTransactions(db)
	if err != nil {
		t.Fatal(err)
	}
	registers, err := pspostgres.NewContinuedAttemptRegisters(db)
	if err != nil {
		t.Fatal(err)
	}
	replays, err := pspostgres.NewCancellationBlockedReplays(db)
	if err != nil {
		t.Fatal(err)
	}
	pickupRegistry, err := tfpostgres.NewCarrierFirstEffectivePickups(db)
	if err != nil {
		t.Fatal(err)
	}
	pickups, err := pstf.NewCarrierPickupBoundaryAdapter(pickupRegistry)
	if err != nil {
		t.Fatal(err)
	}
	handoffOfNotUpheld, err := pspostgres.NewOutboxCancellationNotUpheldHandoff(db, store, clock)
	if err != nil {
		t.Fatal(err)
	}

	intake := psapplication.NewAdoptNetworkIntakeHandler(psapplication.AdoptNetworkIntakeDeps{
		Requests: requests, Eligibility: establishedEligibility{}, Adoptions: adoptions, Identities: commitmentIdentities,
		Downstream: intakeHandoff, Clock: clock, Cancellations: cancellations, Conflicts: conflicts, BoundaryLock: locks,
		Decisions: decisions,
	})
	finalHandler := psapplication.NewFormParcelFinalHandler(psapplication.FormParcelFinalDeps{
		Requests: requests, Rules: satisfiedFinalRule{t: t}, Finals: finals, Cancellations: cancellations,
		Identities: finalIdentities, Downstream: finalHandoff, Clock: clock, BoundaryLock: locks,
	})
	judge := psapplication.NewJudgeLabelServiceFinalHandler(psapplication.JudgeLabelServiceFinalDeps{
		Transactions: transactions, Registers: registers, Cancellations: cancellations, Conflicts: conflicts, Decisions: decisions,
		Adoption: finalHandler, Clock: clock, BoundaryLock: locks,
	})
	core, err := labelfinal.NewParcelJudgmentCore(requests, judge)
	if err != nil {
		t.Fatal(err)
	}
	redriver, err := cancellationredrive.NewRedriver(replays, requests, pickups, intake, finalHandler, core, locks)
	if err != nil {
		t.Fatal(err)
	}
	return &redriveRig{
		redriver: redriver,
		decide: psapplication.NewDecideCancellationOrderConflictHandler(psapplication.DecideCancellationOrderConflictDeps{
			Conflicts: conflicts, Decisions: decisions, Voider: finals, Handoff: handoffOfNotUpheld, BoundaryLock: locks, Clock: clock,
		}),
		fixture: fixture,
	}
}

// decide 在一笔事务里对册上唯一的那份冲突下决定（与生产事务壳同形：决定、作废与续办信封同笔）。
func (rig *redriveRig) decideTheOnlyConflict(t *testing.T, kind psdomain.CancellationOrderConflictDecisionKind) psapplication.DecideCancellationOrderConflictResult {
	t.Helper()
	conflicts, err := pspostgres.NewCancellationOrderConflicts(rig.fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := conflicts.ListByParcel(t.Context(), rig.fixture.identity.TenantID(), mustPS(t, psdomain.NewDeclaredParcelID, nodeIntakeAssociation))
	if err != nil || len(entries) != 1 {
		t.Fatalf("册上的冲突 = %d 行 err=%v, want 恰一行", len(entries), err)
	}
	entry := entries[0]
	var result psapplication.DecideCancellationOrderConflictResult
	mustWithinTX(t, rig.fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var decideErr error
		result, decideErr = rig.decide.Handle(txCtx, psapplication.DecideCancellationOrderConflictCommand{
			Tenant: entry.TenantID, Parcel: entry.Parcel, BoundaryKind: entry.Boundary.Kind, BoundaryRef: entry.Boundary.Reference,
			Cancellation: entry.Cancellation, Kind: kind,
			Decider: mustPS(t, psdomain.NewCancellationOrderConflictDeciderReference, "https://id.syn.example#SYN-DECIDER-CODE76"),
			Basis:   mustPS(t, psdomain.NewCancellationOrderConflictDecisionBasis, "SYN-NODE-SCAN-CHECKED"),
		})
		return decideErr
	})
	return result
}

// redrive 把续办信封的载荷交给续办适配器（与生产消费门交给它的是同一份译码结果），在事务里跑——与消费门同形。
func (rig *redriveRig) redrive(t *testing.T) error {
	t.Helper()
	var payload struct {
		TenantID     string `json:"tenantId"`
		Parcel       string `json:"parcel"`
		Cancellation string `json:"cancellation"`
		BoundaryKind string `json:"boundaryKind"`
		BoundaryRef  string `json:"boundaryRef"`
	}
	querier, err := rig.fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := querier.QueryRow(t.Context(),
		`SELECT payload FROM bento.outbox WHERE event_type = $1`, notUpheldEventType).Scan(&raw); err != nil {
		t.Fatalf("读续办信封：%v", err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("解信封：%v", err)
	}
	return rig.fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return rig.redriver.HandleNotUpheldCancellation(txCtx, psinbox.NotUpheldCancellation{
			TenantID: payload.TenantID, Parcel: payload.Parcel, Cancellation: payload.Cancellation,
			BoundaryKind: payload.BoundaryKind, BoundaryRef: payload.BoundaryRef,
		})
	})
}

func (fixture *synVerticalFixture) countFinalizedOfParcel(t *testing.T) int {
	return fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.final_outcome WHERE finalized AND is_current`)
}

// Covers: AT-PS-173；ADR-0295 决定四——网络服务的收寄：节点收寄经生产路由停在取消先后冲突上（入账，没有责任起点）；运营认定取消依据不成立，续办把当时被挡住的来源（回放原料）
// 重新交回采用编排：目标取当前已接受委托，有效取消读已排除那份取消，采用形成责任起点与正式承诺，下游采用意图入队；已提交的取消记录原样在；续办再跑一遍答已有结果、不多一行。
func TestARedrivenIntakeFormsTheResponsibilityStartOverASetAsideCancellation(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()
	seedStandingCancellation(t, fixture, nodeIntakeAssociation, time.Now().UTC())
	eventID := recordFormedNodeIntake(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, eventID); code != "" {
		t.Fatalf("收寄信封失败码 = %q, want 空——冲突入账", code)
	}
	assertOneConflict(t, fixture, "NETWORK_INTAKE")
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.cancellation_blocked_replay WHERE chain = 'NETWORK_INTAKE_ADOPTION'`); n != 1 {
		t.Fatalf("收寄回放原料 %d 行, want 1——冲突入账时原料同笔落册", n)
	}

	rig := newRedriveRig(t, fixture)
	if result := rig.decideTheOnlyConflict(t, psdomain.CancellationNotUpheld); result.Outcome() != psapplication.CancellationOrderConflictDecided {
		t.Fatalf("决定 = %s", result.Outcome())
	}
	if n := fixture.countOutboxOfType(t, notUpheldEventType); n != 1 {
		t.Fatalf("续办信封 %d 封, want 1", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.intake_adoption`); n != 0 {
		t.Fatalf("续办之前 intake_adoption %d 行, want 0", n)
	}

	if err := rig.redrive(t); err != nil {
		t.Fatalf("续办：%v", err)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.intake_adoption WHERE adopted`); n != 1 {
		t.Fatalf("续办之后采用成立的行 %d 行, want 1——冲突解开后责任起点形成", n)
	}
	if n := fixture.countOutboxOfType(t, networkIntakeRecordedType); n != 1 {
		t.Fatalf("下游采用意图 %d 封, want 1", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.parcel_cancellation WHERE kind = 'PARCEL_CANCELLED'`); n != 1 {
		t.Fatalf("取消记录变成 %d 行——认定不成立只让它不再有效，不删它", n)
	}

	if err := rig.redrive(t); err != nil {
		t.Fatalf("续办重放：%v", err)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.intake_adoption`); n != 1 {
		t.Fatalf("续办重放之后 intake_adoption %d 行, want 仍是 1", n)
	}
}

// Covers: AT-PS-172；ADR-0295 决定四——面单渠道服务：承运商首次有效收寄经生产路由停在取消先后冲突上；认定取消依据不成立之后，续办按 TF 的首次有效收寄链尾现读，交面单终局判断：
// 有效取消已排除，判断走收寄路径形成非取消终局（当前有效终局一份）。
func TestARedrivenCarrierPickupFormsTheLabelServiceFinalOverASetAsideCancellation(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()
	cancelledAt := time.Now().UTC()
	seedStandingCancellation(t, fixture, nodeIntakeAssociation, cancelledAt)
	eventID := recordFormedCarrierPickup(t, fixture, nodeIntakeAssociation, cancelledAt.Add(-time.Hour))
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, eventID); code != "" {
		t.Fatalf("收寄信封失败码 = %q, want 空", code)
	}
	assertOneConflict(t, fixture, "CARRIER_FIRST_EFFECTIVE_PICKUP")

	rig := newRedriveRig(t, fixture)
	rig.decideTheOnlyConflict(t, psdomain.CancellationNotUpheld)
	if n := fixture.countFinalizedOfParcel(t); n != 0 {
		t.Fatalf("续办之前已有终局 %d 份", n)
	}
	if err := rig.redrive(t); err != nil {
		t.Fatalf("续办：%v", err)
	}
	if n := fixture.countFinalizedOfParcel(t); n != 1 {
		t.Fatalf("续办之后当前有效终局 %d 份, want 1——冲突解开后面单渠道服务形成非取消终局", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.final_outcome WHERE finalized AND final_kind = 'SYN-FINAL-KIND'`); n != 1 {
		t.Fatalf("终局类型没取自规则答复")
	}
}

// Covers: AT-PS-169, AT-PS-170；ADR-0295 决定三、四——网络服务履约结果（有效交付）：取消在册时到达的交付经生产路由被不采用，依据指名那份取消；运营认定那份取消依据不成立
// （要先有一份冲突可决定，这里经生产取消库摆出，冲突是收寄一侧的）：决定同笔作废依据它的不采用行，续办凭回放原料把交付重新交回终局采用，规则放行即形成终局——
// 同一版本重投只答已有结果的死局（原不采用记录原样返回）不再出现。
func TestARedrivenDeliveryFormsTheFinalOverTheVoidedRefusal(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()
	seedStandingCancellation(t, fixture, nodeIntakeAssociation, time.Now().UTC())
	intakeID := recordFormedNodeIntake(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("收寄那一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, intakeID); code != "" {
		t.Fatalf("收寄信封失败码 = %q", code)
	}
	deliveryID := recordRegisteredEffectiveDelivery(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("交付那一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, deliveryID); code != "" {
		t.Fatalf("交付信封失败码 = %q, want 空——取消在册，交付不采用并入账", code)
	}
	assertOnlyARefusalOnTheCancelledParcel(t, fixture)
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.cancellation_blocked_replay WHERE chain = 'PARCEL_FINAL_ADOPTION'`); n != 1 {
		t.Fatalf("终局回放原料 %d 行, want 1——依据取消的不采用行同笔落册", n)
	}

	rig := newRedriveRig(t, fixture)
	result := rig.decideTheOnlyConflict(t, psdomain.CancellationNotUpheld)
	if result.VoidedRefusals() != 1 {
		t.Fatalf("作废 %d 行, want 1", result.VoidedRefusals())
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.final_outcome`); n != 0 {
		t.Fatalf("作废之后 final_outcome %d 行, want 0", n)
	}
	if err := rig.redrive(t); err != nil {
		t.Fatalf("续办：%v", err)
	}
	if n := fixture.countFinalizedOfParcel(t); n != 1 {
		t.Fatalf("续办之后当前有效终局 %d 份, want 1——交付重新进入终局规则判断并形成终局", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.final_outcome WHERE refusal_basis IS NOT NULL`); n != 0 {
		t.Fatalf("续办之后仍有不采用行 %d 行", n)
	}
}

// Covers: AT-PS-173；ADR-0295 决定四、ADR-0029——生产路由那一条：认定取消依据不成立之后，续办信封经生产路由表投到续办门，作废、回放与未决哨兵登记都是生产装配。
// 被挡住的交付重新进入终局规则判断，停在「终局规则未配置」这个诚实停点（PAR-COM-17 实例半边，本包不种）——续办信封的失败码是 dispatch.consumer_undecided（哨兵登记对了，
// 不塌成 publish_failed），不采用行没有长回来；租户登记规则之后这封重投即形成终局。
func TestTheNotUpheldEnvelopeRidesTheProductionRouteAndReentersTheFinalRuleJudgment(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()
	seedStandingCancellation(t, fixture, nodeIntakeAssociation, time.Now().UTC())
	intakeID := recordFormedNodeIntake(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("收寄那一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, intakeID); code != "" {
		t.Fatalf("收寄信封失败码 = %q", code)
	}
	deliveryID := recordRegisteredEffectiveDelivery(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("交付那一拍：%v", err)
	}
	assertOnlyARefusalOnTheCancelledParcel(t, fixture)

	rig := newRedriveRig(t, fixture)
	rig.decideTheOnlyConflict(t, psdomain.CancellationNotUpheld)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("续办那一拍：%v", err)
	}

	envelopeID := fixture.outboxIDsOfType(t, notUpheldEventType)
	if len(envelopeID) != 1 {
		t.Fatalf("续办信封 %d 封, want 1", len(envelopeID))
	}
	if got := recordedFailureCode(t, fixture.db, envelopeID[0]); got != "dispatch.consumer_undecided" {
		t.Fatalf("续办信封失败码 = %q, want dispatch.consumer_undecided（终局规则未配置，哨兵登记对了）", got)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.final_outcome WHERE refusal_basis IS NOT NULL`); n != 0 {
		t.Fatalf("续办回滚之后不采用行长回来 %d 行——被取消挡住的记录不该再被写回", n)
	}
	if n := fixture.countInbox(t, notUpheldRedriveConsumerName, envelopeID[0]); n != 0 {
		t.Fatalf("续办门 inbox 有账 %d 行, want 0——未决必须回滚，不能冒充已处理", n)
	}
	if code := recordedFailureCode(t, fixture.db, deliveryID); code != "" {
		t.Fatalf("原交付信封失败码 = %q, want 空——它早已入账，续办不重投它", code)
	}
}

// Covers: AT-PS-171, AT-PS-173；ADR-0295 决定二——交来源方更正：冲突被决定之后，这一代来源在生产装配的收寄采用处不再停在冲突上，而是不采用并指名那份决定（采用账一行不采用记录，
// 依据 BOUNDARY_FACT_REFERRED/<取消>）；没有续办信封，取消照旧有效，没有责任起点。回放用的是冲突入账时落下的原料，经生产 networkIntakeAdoption 的处理方（决定面已接线）。
func TestAReferredIntakeConflictIsNotAdoptedWhenTheSourceArrivesAgain(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()
	seedStandingCancellation(t, fixture, nodeIntakeAssociation, time.Now().UTC())
	eventID := recordFormedNodeIntake(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, eventID); code != "" {
		t.Fatalf("收寄信封失败码 = %q", code)
	}

	rig := newRedriveRig(t, fixture)
	rig.decideTheOnlyConflict(t, psdomain.BoundaryFactReferred)
	if n := fixture.countOutboxOfType(t, notUpheldEventType); n != 0 {
		t.Fatalf("交来源方更正却入了 %d 封续办信封", n)
	}

	replays, err := pspostgres.NewCancellationBlockedReplays(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := replays.ListCancellationBlockedReplays(ctx, fixture.identity.TenantID(),
		mustPS(t, psdomain.NewDeclaredParcelID, nodeIntakeAssociation), mustPS(t, psdomain.NewParcelCancellationID, "SYN-PCXL-01"))
	if err != nil || len(listed) != 1 || listed[0].Intake == nil {
		t.Fatalf("回放原料 = %+v err=%v, want 一份收寄来源规格", listed, err)
	}
	graph, err := networkIntakeAdoption(fixture.db, mustOutboxStore(t, fixture), systemClock{})
	if err != nil {
		t.Fatalf("装配生产收寄采用：%v", err)
	}
	var result psapplication.AdoptNetworkIntakeResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = graph.handler.Handle(txCtx, psapplication.AdoptNetworkIntakeCommand{
			Identity: fixture.identity, ShipmentRequestID: fixture.requestID,
			SubmissionVersion: currentSubmissionVersionOf(t, fixture), Source: *listed[0].Intake,
		})
		return handleErr
	})
	if result.Outcome() != psapplication.IntakeSourceNotAdopted || result.Basis().String() != "BOUNDARY_FACT_REFERRED/SYN-PCXL-01" {
		t.Fatalf("outcome = %q basis = %q, want SOURCE_NOT_ADOPTED 指名 BOUNDARY_FACT_REFERRED/SYN-PCXL-01——决定面没接到生产收寄采用上",
			result.Outcome(), result.Basis())
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.intake_adoption WHERE adopted`); n != 0 {
		t.Fatalf("交来源方更正之后采用成立 %d 行——不该形成责任起点", n)
	}
}

func mustOutboxStore(t *testing.T, fixture *synVerticalFixture) *outbox.Store {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	return store
}

func currentSubmissionVersionOf(t *testing.T, fixture *synVerticalFixture) psdomain.SubmissionVersionID {
	t.Helper()
	request, found, err := fixture.requests.FindBySourceIdentity(t.Context(), fixture.identity)
	if err != nil || !found {
		t.Fatalf("读委托 found=%v err=%v", found, err)
	}
	return request.CurrentSubmissionVersion().VersionID()
}

// Covers: ADR-0029 / AT-PS-173——续办门的未决名单只收「等一个依赖」的几枚：目标委托还没落到已接受、收寄采用与终局采用停在自己的未决、面单终局判断的目标反查与判断编排停在依赖上；
// 采用或终局已提交而发布意图没交出去（要查 outbox 下游）、信封译不出或原料被改坏、编排结果集外一枚都不在。名单取生产切片本身，测试里重列一份会让误登记的哨兵一起消失。
func TestTheRedriveLaneRegistersOnlyResumableSentinels(t *testing.T) {
	listed := func(err error) bool {
		for _, sentinel := range cancellationNotUpheldRedriveUndecidedSentinels {
			if errors.Is(err, sentinel) {
				return true
			}
		}
		return false
	}
	for _, err := range []error{
		cancellationredrive.ErrRedriveTargetNotFound, adoptconsume.ErrAdoptionUndecided, finalconsume.ErrFinalUndecided,
		labelfinal.ErrParcelTargetNotFound, labelfinal.ErrJudgmentUndecided,
	} {
		if !listed(fmt.Errorf("%w: syn", err)) {
			t.Fatalf("续办哨兵 %v 不在名单里", err)
		}
	}
	for _, err := range []error{
		adoptconsume.ErrAdoptionHandoffPending, finalconsume.ErrFinalHandoffPending, cancellationredrive.ErrRedriveUntranslatable,
		adoptconsume.ErrUnexpectedAdoptionOutcome, finalconsume.ErrUnexpectedFinalOutcome,
	} {
		if listed(fmt.Errorf("%w: syn", err)) {
			t.Fatalf("确定性错误 %v 进了未决名单——派发会按 consumer_undecided 重投到放弃", err)
		}
	}
}
