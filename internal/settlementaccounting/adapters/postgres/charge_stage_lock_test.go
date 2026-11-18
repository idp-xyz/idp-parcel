package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉同一笔客户费用上「确认」与复磅那一头的「记替代」排成一列：两条路各在自己的 READ COMMITTED 事务里，
// 谁都是先查后写，不排队的话两边恰好都在对方提交之前读，结果是一笔费用既已确认又已被替代、新预估照常再被确认，同一件包裹
// 两笔都算数。本文件用真库、真登记册与真的确认编排、形成编排，让一头持锁未提交，证另一头必须等、等完读到对方提交的结果。
// 夹具全是合成串。

var stageLockAt = time.Date(2026, 10, 19, 9, 30, 0, 0, time.UTC)

type stageLockClock struct{}

func (stageLockClock) Now() time.Time { return stageLockAt }

type stageLockConditions struct {
	basis domain.ConfirmationBasisReference
}

func (double stageLockConditions) LoadConfirmationCondition(
	context.Context, domain.TenantID, domain.CustomerChargeID, domain.FeeItemReference,
) (ports.ConfirmationCondition, bool, error) {
	return ports.ConfirmationCondition{Met: true, Basis: double.basis}, true, nil
}

type stageLockFacts struct{ facts domain.ConfirmedChargeFacts }

func (double stageLockFacts) LoadConfirmedChargeFacts(
	context.Context, domain.TenantID, domain.CustomerChargeID,
) (domain.ConfirmedChargeFacts, bool, error) {
	return double.facts, true, nil
}

type stageLockHandoff struct{}

func (stageLockHandoff) HandOffChargeConfirmation(context.Context, ports.ChargeConfirmationHandoffIntent) error {
	return nil
}

type stageLockOccupation struct{}

func (stageLockOccupation) ConvertOnConfirmation(context.Context, domain.TenantID, domain.CustomerCharge) error {
	return nil
}

type stageLockMappings struct{ item domain.FeeItemReference }

func (double stageLockMappings) FindFeeItemMapping(
	_ context.Context, _ domain.TenantID, code domain.PricingChargeCode,
) (domain.FeeItemMapping, bool, error) {
	basis, err := domain.NewFeeItemMappingBasisReference("SYN-CLAUSE-" + code.String())
	if err != nil {
		return domain.FeeItemMapping{}, false, err
	}
	mapping, err := domain.NewFeeItemMapping(code, double.item, basis)
	return mapping, err == nil, err
}

type stageLockEvaluations struct{ adoption ports.SellEvaluationAdoption }

func (double stageLockEvaluations) LoadSellEvaluation(
	context.Context, domain.TenantID, domain.SellEvaluationReference,
) (ports.SellEvaluationAdoption, bool, error) {
	return double.adoption, true, nil
}

type stageLockFollowUp struct{}

func (stageLockFollowUp) AfterEstimatedChargeFormed(
	context.Context, domain.TenantID, domain.SellEvaluationRequest, domain.CustomerChargeID, domain.EstimatePricingScope,
) error {
	return nil
}

type stageLockAdoption struct{}

func (stageLockAdoption) ReweighAdjustmentAdopted(context.Context, domain.TenantID, domain.SettlementAccountID) (bool, error) {
	return true, nil
}

// stageLockFixture 把真登记册与真编排装在同一只库上：链上第一代请求已形成一笔预估费用（earlier），第二代请求已提交
// （next）、评价算出的金额比第一笔高，等复磅形成去办。
type stageLockFixture struct {
	db            *bentopg.DB
	tenant        domain.TenantID
	charges       *adapter.CustomerCharges
	supersessions *adapter.ChargeSupersessions
	adjustments   *adapter.ChargeAdjustments
	requests      *adapter.SellEvaluationRequests
	confirm       *application.ConfirmChargeHandler
	formation     *application.FormEstimatedCustomerChargeHandler
	earlier       domain.CustomerChargeID
	next          ports.SellEvaluationRequestRecord
	adoption      ports.SellEvaluationAdoption
}

func newStageLockFixture(t *testing.T) *stageLockFixture {
	t.Helper()
	db := catalogueDB(t)
	fixture := &stageLockFixture{db: db, tenant: saTenant(t, "tenant-a")}
	var err error
	if fixture.charges, err = adapter.NewCustomerCharges(db); err != nil {
		t.Fatal(err)
	}
	if fixture.supersessions, err = adapter.NewChargeSupersessions(db); err != nil {
		t.Fatal(err)
	}
	if fixture.adjustments, err = adapter.NewChargeAdjustments(db); err != nil {
		t.Fatal(err)
	}
	if fixture.requests, err = adapter.NewSellEvaluationRequests(db); err != nil {
		t.Fatal(err)
	}
	parcelLocks, err := adapter.NewParcelConfirmationLocks(db)
	if err != nil {
		t.Fatal(err)
	}
	attributionForms, err := adapter.NewChargeAttributions(db)
	if err != nil {
		t.Fatal(err)
	}
	arrangements, err := adapter.NewStatementArrangements(db)
	if err != nil {
		t.Fatal(err)
	}
	attributions, err := application.NewJudgeChargeAttributionHandler(attributionForms, arrangements)
	if err != nil {
		t.Fatal(err)
	}

	first := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-1", "SYN-PARCEL-1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	fixture.earlier = first.Request.EstimatedCustomerChargeID()
	earlierCharge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 fixture.earlier,
		FeeItem:            saValue(t, domain.NewFeeItemReference, "SYN-FEE-FREIGHT"),
		Evaluation:         saValue(t, domain.NewSellEvaluationReference, "SYN-EVAL-SELL-1"),
		OriginalCurrency:   saValue(t, domain.NewCurrencyCode, "XTS"),
		OriginalMinor:      12000,
		SettlementCurrency: saValue(t, domain.NewCurrencyCode, "XTS"),
		SettlementMinor:    12000,
		Stage:              domain.ChargeEstimated,
		FormedAt:           chargeFormedAt,
		SourceOccurredAt:   sellRequestBasisAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	formed, err := domain.NewFormedSellEvaluationRequestConclusion(fixture.earlier)
	if err != nil {
		t.Fatal(err)
	}
	basis, err := domain.NewMeasurementChangeResumeBasis("measure-1")
	if err != nil {
		t.Fatal(err)
	}
	nextRequest, err := first.Request.Resume(saValue(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-2"), basis)
	if err != nil {
		t.Fatal(err)
	}
	fixture.next = ports.SellEvaluationRequestRecord{
		Key:        ports.SellEvaluationRequestKey{TenantID: fixture.tenant, Request: nextRequest.ID()},
		Request:    nextRequest,
		RecordedAt: sellRequestRecordedAt.Add(time.Hour),
		State:      domain.SellRequestSubmitted,
	}
	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		if _, err := fixture.requests.Save(ctx, first); err != nil {
			return err
		}
		if _, err := fixture.charges.SaveEstimated(ctx, fixture.tenant, earlierCharge); err != nil {
			return err
		}
		if _, err := fixture.requests.RecordConclusion(ctx, first.Key, formed); err != nil {
			return err
		}
		_, err := fixture.requests.Save(ctx, fixture.next)
		return err
	})

	fixture.confirm, err = application.NewConfirmChargeHandler(application.ConfirmChargeDeps{
		Charges:       fixture.charges,
		Conditions:    stageLockConditions{basis: saValue(t, domain.NewConfirmationBasisReference, "DELIVERY_FINALIZED/final-1")},
		Facts:         stageLockFacts{facts: saConfirmationFacts(t)},
		Attributions:  attributions,
		Downstream:    stageLockHandoff{},
		Occupation:    stageLockOccupation{},
		Clock:         stageLockClock{},
		Supersessions: fixture.supersessions,
		ChargeLock:    fixture.charges,
	})
	if err != nil {
		t.Fatal(err)
	}

	fixture.adoption = ports.SellEvaluationAdoption{
		Evaluation:         saValue(t, domain.NewSellEvaluationReference, "SYN-EVAL-SELL-2"),
		Outcome:            ports.SellEvaluationCompleted,
		Request:            nextRequest.ID(),
		ChargeCodes:        []domain.PricingChargeCode{saValue(t, domain.NewPricingChargeCode, "BASE_FREIGHT")},
		OriginalCurrency:   saValue(t, domain.NewCurrencyCode, "XTS"),
		OriginalMinor:      15000,
		SettlementCurrency: saValue(t, domain.NewCurrencyCode, "XTS"),
		SettlementMinor:    15000,
	}
	formSell, err := application.NewFormSellCustomerChargeHandler(stageLockEvaluations{adoption: fixture.adoption}, fixture.charges)
	if err != nil {
		t.Fatal(err)
	}
	fixture.formation, err = application.NewFormEstimatedCustomerChargeHandler(application.FormEstimatedCustomerChargeDeps{
		Requests:         fixture.requests,
		Conclusions:      fixture.requests,
		Mappings:         stageLockMappings{item: saValue(t, domain.NewFeeItemReference, "SYN-FEE-FREIGHT")},
		Charges:          formSell,
		FollowUp:         stageLockFollowUp{},
		Clock:            stageLockClock{},
		PriorCharges:     fixture.charges,
		Supersessions:    fixture.supersessions,
		ReweighAdoptions: stageLockAdoption{},
		Reweigh: application.NewRecordChargeAdjustmentHandler(application.RecordChargeAdjustmentDeps{
			Charges: fixture.charges, Adjustments: fixture.adjustments, Clock: stageLockClock{},
		}),
		Successors: fixture.requests,
		Serializer: parcelLocks,
		ChargeLock: fixture.charges,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

// reweighCommand 是第二代请求的评价记下之后，形成编排收到的那一份命令。
func (fixture *stageLockFixture) reweighCommand() application.FormEstimatedCustomerChargeCommand {
	return application.FormEstimatedCustomerChargeCommand{TenantID: fixture.tenant, Evaluation: fixture.adoption}
}

func (fixture *stageLockFixture) confirmCommand() application.ConfirmChargeCommand {
	return application.ConfirmChargeCommand{TenantID: fixture.tenant, ChargeID: fixture.earlier.String()}
}

// successor 是第二代请求形成的新预估所在的费用标识。
func (fixture *stageLockFixture) successor() domain.CustomerChargeID {
	return fixture.next.Request.EstimatedCustomerChargeID()
}

// stageOf 读一笔费用此刻在库上的阶段；不在册交回 false。
func (fixture *stageLockFixture) stageOf(t *testing.T, id domain.CustomerChargeID) (domain.ChargeStage, bool) {
	t.Helper()
	charge, found, err := fixture.charges.FindByID(t.Context(), fixture.tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	return charge.Stage(), found
}

// heldTransaction 是一笔做完了 work、却还没提交的事务：它持有的行锁与未提交的写留到 release 那一刻。
type heldTransaction struct {
	held     chan struct{}
	release  func()
	done     chan error
	finished chan struct{}
}

// holdOpen 在一笔事务里跑 work，随后不提交、等 release 才放。work 做完时关 held，出错时把错误交给 done，调用方据此知道
// 没走到持锁那一步；走到了，事务的结果在提交之后从 done 取。
func holdOpen(t *testing.T, db *bentopg.DB, work func(ctx context.Context) error) *heldTransaction {
	t.Helper()
	releaseCh := make(chan struct{})
	var once sync.Once
	holder := &heldTransaction{
		held:     make(chan struct{}),
		release:  func() { once.Do(func() { close(releaseCh) }) },
		done:     make(chan error, 1),
		finished: make(chan struct{}),
	}
	// 断言失败时也放开持锁的一头：它的事务不结束，库就收不了尾，用例会挂到超时而不是报红。
	t.Cleanup(func() {
		holder.release()
		<-holder.finished
	})
	go func() {
		defer close(holder.finished)
		holder.done <- db.Transactor().WithinTransaction(context.Background(), func(ctx context.Context) error {
			if err := work(ctx); err != nil {
				return err
			}
			close(holder.held)
			<-releaseCh
			return nil
		})
	}()
	select {
	case <-holder.held:
	case err := <-holder.done:
		t.Fatalf("持锁的一头没走到持锁就结束了：%v", err)
	case <-time.After(20 * time.Second):
		t.Fatal("持锁的一头 20 秒没走到持锁")
	}
	return holder
}

// blockedFor 是断言「另一头被锁挡着」时等的时长：这段时间里对方还没答复，才算它在等。
const blockedFor = 400 * time.Millisecond

// describe 说出两头都提交之后这笔费用在库上的样子，写进「没等就答了」的失败信息：不排队的坏结果（已确认又已被替代、
// 新预估照常在册）是一眼看得出的。
func (fixture *stageLockFixture) describe(t *testing.T) string {
	t.Helper()
	stage, _ := fixture.stageOf(t, fixture.earlier)
	superseded, err := fixture.supersessions.Superseded(t.Context(), fixture.tenant, fixture.earlier)
	if err != nil {
		t.Fatal(err)
	}
	_, successorPresent := fixture.stageOf(t, fixture.successor())
	return fmt.Sprintf("原费用 %s、已被替代 %v、新预估在册 %v", stage, superseded, successorPresent)
}

type confirmAnswer struct {
	outcome application.ConfirmChargeOutcome
	err     error
}

// confirmAsync 在自己的事务里办一次确认，答复交到通道上。
func (fixture *stageLockFixture) confirmAsync() <-chan confirmAnswer {
	done := make(chan confirmAnswer, 1)
	go func() {
		var answer confirmAnswer
		answer.err = fixture.db.Transactor().WithinTransaction(context.Background(), func(ctx context.Context) error {
			result, err := fixture.confirm.Handle(ctx, fixture.confirmCommand())
			answer.outcome = result.Outcome()
			return err
		})
		done <- answer
	}()
	return done
}

type reweighAnswer struct {
	state  domain.SellEvaluationRequestState
	charge domain.CustomerChargeID
	err    error
}

// reweighAsync 在自己的事务里办一次复磅形成，答复交到通道上。
func (fixture *stageLockFixture) reweighAsync() <-chan reweighAnswer {
	done := make(chan reweighAnswer, 1)
	go func() {
		var answer reweighAnswer
		answer.err = fixture.db.Transactor().WithinTransaction(context.Background(), func(ctx context.Context) error {
			result, err := fixture.formation.Handle(ctx, fixture.reweighCommand())
			answer.state = result.State()
			answer.charge, _ = result.Charge()
			return err
		})
		done <- answer
	}()
	return done
}

// Covers: 复磅先到、确认后到——复磅那一头先形成新预估、记下 A→B，提交之前确认来办 A：确认必须等着，不能在对方提交前返回；
// 对方提交之后确认读到的是已被替代，答 CHARGE_SUPERSEDED，A 在库上仍不是已确认。复磅这一头用真的形成编排办，持着包裹锁与
// A 的行锁、未提交，正是确认能夹进去的那段窗口。
func TestAManualConfirmationWaitsForAReweighHoldingTheChargeThenAnswersSuperseded(t *testing.T) {
	fixture := newStageLockFixture(t)

	reweigh := holdOpen(t, fixture.db, func(ctx context.Context) error {
		result, err := fixture.formation.Handle(ctx, fixture.reweighCommand())
		if err != nil {
			return err
		}
		if charge, _ := result.Charge(); result.State() != domain.SellRequestFormed || charge != fixture.successor() {
			return fmt.Errorf("复磅形成 = %s / %q，want 已形成新预估 %s", result.State(), charge, fixture.successor())
		}
		return nil
	})

	confirmation := fixture.confirmAsync()
	select {
	case early := <-confirmation:
		reweigh.release()
		<-reweigh.done
		t.Fatalf("确认没等复磅提交就答了 %s；两头都提交之后：%s", early.outcome, fixture.describe(t))
	case <-time.After(blockedFor):
	}
	reweigh.release()
	if err := <-reweigh.done; err != nil {
		t.Fatalf("复磅提交：%v", err)
	}

	answer := <-confirmation
	if answer.err != nil || answer.outcome != application.ChargeSuperseded {
		t.Fatalf("确认 = %s err = %v，want CHARGE_SUPERSEDED", answer.outcome, answer.err)
	}
	if stage, found := fixture.stageOf(t, fixture.earlier); !found || stage == domain.ChargeConfirmed {
		t.Fatalf("被替代的费用阶段 = %s found=%v，want 仍未确认", stage, found)
	}
	if superseded, err := fixture.supersessions.Superseded(t.Context(), fixture.tenant, fixture.earlier); err != nil || !superseded {
		t.Fatalf("替代关系 superseded=%v err=%v，want 已记下", superseded, err)
	}
	if stage, found := fixture.stageOf(t, fixture.successor()); !found || stage != domain.ChargeEstimated {
		t.Fatalf("新预估阶段 = %s found=%v，want 在册的预估", stage, found)
	}
}

// Covers: 确认先到、复磅后到——确认先把 A 确认、提交之前复磅来办：复磅必须等着；确认提交之后复磅读到的是已确认，走 settleConfirmedReweigh——
// 只记差额，不另建费用、不记替代；这一代照记结论，指向原来那笔已确认费用。
func TestAReweighWaitsForAConfirmationHoldingTheChargeThenSettlesTheDifference(t *testing.T) {
	fixture := newStageLockFixture(t)

	confirmation := holdOpen(t, fixture.db, func(ctx context.Context) error {
		result, err := fixture.confirm.Handle(ctx, fixture.confirmCommand())
		if err != nil {
			return err
		}
		if result.Outcome() != application.ChargeConfirmedOutcome {
			return fmt.Errorf("确认 = %s，want CHARGE_CONFIRMED", result.Outcome())
		}
		return nil
	})

	reweigh := fixture.reweighAsync()
	select {
	case early := <-reweigh:
		confirmation.release()
		<-confirmation.done
		t.Fatalf("复磅没等确认提交就答了 %s / %s；两头都提交之后：%s", early.state, early.charge, fixture.describe(t))
	case <-time.After(blockedFor):
	}
	confirmation.release()
	if err := <-confirmation.done; err != nil {
		t.Fatalf("确认提交：%v", err)
	}

	answer := <-reweigh
	if answer.err != nil || answer.state != domain.SellRequestFormed || answer.charge != fixture.earlier {
		t.Fatalf("复磅 = %s / %q err = %v，want 已形成、指向原来那笔 %s", answer.state, answer.charge, answer.err, fixture.earlier)
	}
	if stage, found := fixture.stageOf(t, fixture.earlier); !found || stage != domain.ChargeConfirmed {
		t.Fatalf("原费用阶段 = %s found=%v，want 已确认", stage, found)
	}
	if _, found := fixture.stageOf(t, fixture.successor()); found {
		t.Fatal("已确认之后又建了一笔新费用")
	}
	if superseded, err := fixture.supersessions.Superseded(t.Context(), fixture.tenant, fixture.earlier); err != nil || superseded {
		t.Fatalf("已确认的费用被记了替代 superseded=%v err=%v", superseded, err)
	}
	recorded, err := fixture.adjustments.ListByCharge(t.Context(), fixture.tenant, fixture.earlier)
	if err != nil || len(recorded) != 1 {
		t.Fatalf("调整 %d 笔 err = %v，want 一笔差额", len(recorded), err)
	}
	if _, minor := recorded[0].Adjustment.SettlementAmount(); minor != 3000 || recorded[0].Adjustment.Direction() != domain.AdjustmentDebit {
		t.Fatalf("差额 = %d %s，want 借项 3000", minor, recorded[0].Adjustment.Direction())
	}
	if stored, found, err := fixture.requests.FindByID(t.Context(), fixture.next.Key); err != nil || !found ||
		stored.State != domain.SellRequestFormed || stored.FormedCharge != fixture.earlier {
		t.Fatalf("这一代的结论 = %s / %q found=%v err=%v", stored.State, stored.FormedCharge, found, err)
	}
}

// Covers: 费用行锁是事务级锁，事务外取等于没锁：按框架合同拒。
func TestTheChargeStageLockRefusesToRunOutsideATransaction(t *testing.T) {
	charges, err := adapter.NewCustomerCharges(catalogueDB(t))
	if err != nil {
		t.Fatal(err)
	}
	err = charges.LockChargeStage(t.Context(), saTenant(t, "tenant-a"), saValue(t, domain.NewCustomerChargeID, "charge-1"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外取锁应答 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 费用行锁只排同一笔费用：一笔持锁未提交时，另一笔取同一笔费用的锁要等；别的费用不等；费用不在册不报错；持锁那一笔
// 提交之后取得到。
func TestTheChargeStageLockQueuesTheSameChargeOnly(t *testing.T) {
	db := catalogueDB(t)
	charges, err := adapter.NewCustomerCharges(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := saTenant(t, "tenant-a")
	locked, other := estimatedCharge(t, "charge-lock-1"), estimatedCharge(t, "charge-lock-2")
	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		if _, err := charges.SaveEstimated(ctx, tenant, locked); err != nil {
			return err
		}
		_, err := charges.SaveEstimated(ctx, tenant, other)
		return err
	})

	holder := holdOpen(t, db, func(ctx context.Context) error {
		return charges.LockChargeStage(ctx, tenant, locked.ID())
	})
	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		if err := charges.LockChargeStage(ctx, tenant, other.ID()); err != nil {
			return fmt.Errorf("别的费用也等了：%w", err)
		}
		return charges.LockChargeStage(ctx, tenant, saValue(t, domain.NewCustomerChargeID, "charge-absent"))
	})

	waiting, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	err = db.Transactor().WithinTransaction(waiting, func(ctx context.Context) error {
		return charges.LockChargeStage(ctx, tenant, locked.ID())
	})
	cancel()
	if err == nil {
		t.Fatal("同一笔费用在对方持锁时取到了锁")
	}

	holder.release()
	if err := <-holder.done; err != nil {
		t.Fatal(err)
	}
	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		return charges.LockChargeStage(ctx, tenant, locked.ID())
	})
}

// Covers: 锁模式——费用行锁不挡对这一行的外键检查（记替代、记调整、登确认依据都对费用行取 KEY SHARE），否则记调整那一路
// 「先取余额作用域锁、再插调整行」会与确认这一路「先取费用行锁、再取余额作用域锁」互等成环；同时仍与截单出账锁
// （LockChargeBilling，FOR UPDATE）排队，两种费用行锁不会同时到手。
func TestTheChargeStageLockLetsForeignKeyWritesThroughButQueuesTheBillingLock(t *testing.T) {
	db := catalogueDB(t)
	charges, err := adapter.NewCustomerCharges(db)
	if err != nil {
		t.Fatal(err)
	}
	supersessions, err := adapter.NewChargeSupersessions(db)
	if err != nil {
		t.Fatal(err)
	}
	statements, err := adapter.NewCustomerStatements(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := saTenant(t, "tenant-a")
	locked, successor := estimatedCharge(t, "charge-lock-1"), estimatedCharge(t, "charge-lock-2")
	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		if _, err := charges.SaveEstimated(ctx, tenant, locked); err != nil {
			return err
		}
		_, err := charges.SaveEstimated(ctx, tenant, successor)
		return err
	})

	holder := holdOpen(t, db, func(ctx context.Context) error {
		return charges.LockChargeStage(ctx, tenant, locked.ID())
	})

	through, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	err = db.Transactor().WithinTransaction(through, func(ctx context.Context) error {
		return supersessions.SaveSupersession(ctx, tenant, locked.ID(), successor.ID(), stageLockAt)
	})
	cancel()
	if err != nil {
		t.Fatalf("对费用行的外键写入被费用行锁挡住了：%v", err)
	}

	waiting, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	err = db.Transactor().WithinTransaction(waiting, func(ctx context.Context) error {
		_, _, err := statements.LockChargeBilling(ctx, tenant, locked.ID())
		return err
	})
	cancel()
	if err == nil {
		t.Fatal("截单出账锁在费用行锁持有期间取到了")
	}
	holder.release()
	if err := <-holder.done; err != nil {
		t.Fatal(err)
	}
}
