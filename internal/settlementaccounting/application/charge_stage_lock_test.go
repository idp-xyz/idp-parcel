package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉费用行锁（ports.ChargeStageLock）在编排层的先后：同一笔费用上的「确认」与复磅那一头的「记替代」要排成一列，靠的是
// 谁都先锁这一行、再读阶段与替代关系。替身只记调用先后，排队本身由真库用例证（settlementaccounting/adapters/postgres 的
// charge_stage_lock_test.go）。夹具全是合成串。

// callOrder 把几只替身各自被调用的次序记在同一条线上。
type callOrder struct{ calls []string }

func (order *callOrder) add(call string) { order.calls = append(order.calls, call) }

// assertBefore 断言 first 与 second 都出现过，且 first 先于 second。
func (order *callOrder) assertBefore(t *testing.T, first, second string) {
	t.Helper()
	earlier, later := slices.Index(order.calls, first), slices.Index(order.calls, second)
	if earlier < 0 || later < 0 || earlier > later {
		t.Fatalf("调用次序 = %v，want %q 先于 %q", order.calls, first, second)
	}
}

type orderedChargeStore struct {
	*chargeStoreDouble
	order *callOrder
}

func (store orderedChargeStore) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CustomerChargeID,
) (domain.CustomerCharge, bool, error) {
	store.order.add("读费用 " + id.String())
	return store.chargeStoreDouble.FindByID(ctx, tenant, id)
}

func (store orderedChargeStore) SaveConfirmed(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerCharge,
) (ports.ChargeSaveOutcome, error) {
	store.order.add("存确认 " + charge.ID().String())
	return store.chargeStoreDouble.SaveConfirmed(ctx, tenant, charge)
}

type orderedChargeLock struct {
	order *callOrder
	err   error
}

func (lock orderedChargeLock) LockChargeStage(_ context.Context, _ domain.TenantID, charge domain.CustomerChargeID) error {
	lock.order.add("锁费用行 " + charge.String())
	return lock.err
}

type orderedParcelLock struct{ order *callOrder }

func (lock orderedParcelLock) SerializeParcelConfirmation(_ context.Context, _ domain.TenantID, parcel domain.ParcelReference) error {
	lock.order.add("包裹锁 " + parcel.String())
	return nil
}

type orderedSupersessions struct {
	order      *callOrder
	superseded bool
}

func (store *orderedSupersessions) SaveSupersession(
	_ context.Context,
	_ domain.TenantID,
	charge domain.CustomerChargeID,
	_ domain.CustomerChargeID,
	_ time.Time,
) error {
	store.order.add("记替代 " + charge.String())
	return nil
}

func (store *orderedSupersessions) SaveSupersessionWithoutSuccessor(
	_ context.Context,
	_ domain.TenantID,
	charge domain.CustomerChargeID,
	_ time.Time,
) error {
	store.order.add("记无后继替代 " + charge.String())
	return nil
}

func (store *orderedSupersessions) Superseded(_ context.Context, _ domain.TenantID, charge domain.CustomerChargeID) (bool, error) {
	store.order.add("读替代 " + charge.String())
	return store.superseded, nil
}

// orderedConfirmHandler 按记先后的替身装一只确认编排，交回它与那条先后线。
func orderedConfirmHandler(
	t *testing.T,
	fixture *confirmFixture,
	superseded bool,
	lockErr error,
) (*application.ConfirmChargeHandler, *callOrder) {
	t.Helper()
	order := &callOrder{}
	deps := fixture.deps(t)
	deps.Charges = orderedChargeStore{chargeStoreDouble: fixture.store, order: order}
	deps.ChargeLock = orderedChargeLock{order: order, err: lockErr}
	deps.Supersessions = &orderedSupersessions{order: order, superseded: superseded}
	handler, err := application.NewConfirmChargeHandler(deps)
	if err != nil {
		t.Fatal(err)
	}
	return handler, order
}

// Covers: 同一笔费用上「确认」与「记替代」排成一列——确认先锁费用行，锁到手之后才读阶段与替代关系，最后才落确认；顺序反了
// （先读后锁），读到的就是对方提交之前的样子。
func TestConfirmingLocksTheChargeRowBeforeReadingItsStageAndSupersession(t *testing.T) {
	fixture := newConfirmFixture(t)
	handler, order := orderedConfirmHandler(t, fixture, false, nil)

	result, err := handler.Handle(t.Context(), confirmCommand(t))
	if err != nil || result.Outcome() != application.ChargeConfirmedOutcome {
		t.Fatalf("结果 = %s err = %v，want CHARGE_CONFIRMED", result.Outcome(), err)
	}
	want := []string{"锁费用行 charge-1", "读费用 charge-1", "读替代 charge-1", "存确认 charge-1"}
	if !slices.Equal(order.calls, want) {
		t.Fatalf("调用次序 = %v，want %v", order.calls, want)
	}
}

// Covers: 替代先提交 → 确认答 CHARGE_SUPERSEDED，不落确认：锁到手之后读到的替代关系已在，费用不转已确认、不发确认意图、
// 不收口占用。
func TestAChargeSupersededBeforeTheLockWasWonIsNotConfirmed(t *testing.T) {
	fixture := newConfirmFixture(t)
	handler, order := orderedConfirmHandler(t, fixture, true, nil)

	result, err := handler.Handle(t.Context(), confirmCommand(t))
	if err != nil || result.Outcome() != application.ChargeSuperseded {
		t.Fatalf("结果 = %s err = %v，want CHARGE_SUPERSEDED", result.Outcome(), err)
	}
	if fixture.store.saves != 0 || fixture.store.charges["charge-1"].Stage() == domain.ChargeConfirmed {
		t.Fatalf("被替代的费用落了确认：写入 %d 次", fixture.store.saves)
	}
	if len(fixture.handoff.intents) != 0 || len(fixture.occupation.charges) != 0 {
		t.Fatalf("被替代的费用发了确认意图 %d 份、收口占用 %d 次", len(fixture.handoff.intents), len(fixture.occupation.charges))
	}
	order.assertBefore(t, "锁费用行 charge-1", "读替代 charge-1")
}

// Covers: 费用行锁取不到是依赖读写不动：答未决、续办引用指名费用库，不读阶段也不落确认——不能在没排上队的情况下照旧确认。
func TestAConfirmationThatCannotLockTheChargeRowIsUndecidedAndReadsNothing(t *testing.T) {
	fixture := newConfirmFixture(t)
	handler, order := orderedConfirmHandler(t, fixture, false, errors.New("synthetic lock down"))

	result, err := handler.Handle(t.Context(), confirmCommand(t))
	if err != nil || result.Outcome() != application.ConfirmUndecided || result.UndecidedReason() != application.ChargeStoreUnavailable {
		t.Fatalf("结果 = %s / %s err = %v，want 未决、费用库不可用", result.Outcome(), result.UndecidedReason(), err)
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决没有续办引用")
	}
	if !slices.Equal(order.calls, []string{"锁费用行 charge-1"}) || fixture.store.saves != 0 {
		t.Fatalf("取锁失败之后还动了费用库：%v，写入 %d 次", order.calls, fixture.store.saves)
	}
}

// Covers: 费用行锁是确认编排的必需依赖：装配时核齐，缺了在这里报，不等到并发那一刻才发现没排队。
func TestTheConfirmationRefusesToBeBuiltWithoutTheChargeLock(t *testing.T) {
	fixture := newConfirmFixture(t)
	deps := fixture.deps(t)
	deps.ChargeLock = nil
	if _, err := application.NewConfirmChargeHandler(deps); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("err = %v，want 缺依赖", err)
	}
}

// Covers: 替代册是确认编排的必需依赖：「被替代的不再确认」靠它守，漏装就无声地不守，所以装配时核齐，缺了在这里报。
func TestTheConfirmationRefusesToBeBuiltWithoutTheSupersessionRegister(t *testing.T) {
	fixture := newConfirmFixture(t)
	deps := fixture.deps(t)
	deps.Supersessions = nil
	if _, err := application.NewConfirmChargeHandler(deps); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("err = %v，want 缺依赖", err)
	}
}

// Covers: 确认的各条入口都被覆盖——管理台手工确认直接调确认编排；到点自动确认（按包裹与按费用两种进法）与形成后的跟进确认
// 都经同一只确认编排，所以都在读阶段之前取到这笔费用的行锁。有包裹可锁的入口先取包裹锁、再取费用行锁。
func TestEveryConfirmationEntryTakesTheChargeRowLock(t *testing.T) {
	tenant := confirmCommand(t).TenantID
	charge := billValue(t, domain.NewCustomerChargeID, "charge-1")
	parcel := billValue(t, domain.NewParcelReference, "SYN-PARCEL-1")

	t.Run("管理台手工确认", func(t *testing.T) {
		fixture := newConfirmFixture(t)
		if _, err := fixture.handler.Handle(t.Context(), confirmCommand(t)); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(fixture.lock.locked, []string{"charge-1"}) {
			t.Fatalf("费用行锁 = %v，want [charge-1]", fixture.lock.locked)
		}
	})

	t.Run("到点自动确认", func(t *testing.T) {
		base := newConfirmFixture(t)
		confirm, order := orderedConfirmHandler(t, base, false, nil)
		requests := &parcelRequestDouble{found: true}
		requests.record.FormedCharge = charge
		handler, err := application.NewConfirmChargeOnBasisHandler(application.ConfirmChargeOnBasisDeps{
			Serializer: orderedParcelLock{order: order},
			Requests:   requests,
			Charges:    base.store,
			Bases:      &basisRecorderDouble{},
			Moments: newMomentHandler(t, momentViewRef{
				&settlementMomentViewDouble{found: true, moment: domain.SettlementMomentConfirm}}),
			Confirm: confirm,
			Clock:   confirmClock{at: chargeConfirmedAt},
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := handler.OnParcel(t.Context(), tenant, parcel, finalBasis(t))
		if err != nil || result.Outcome() != application.ConfirmOnBasisConfirmAttempted {
			t.Fatalf("按包裹 = %s err = %v，want CONFIRM_ATTEMPTED", result.Outcome(), err)
		}
		order.assertBefore(t, "包裹锁 SYN-PARCEL-1", "锁费用行 charge-1")
		order.assertBefore(t, "锁费用行 charge-1", "读替代 charge-1")

		fixture := newConfirmOnBasisFixture(t, true)
		if _, err := fixture.handler.OnCharge(t.Context(), tenant, charge, finalBasis(t)); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(fixture.confirm.lock.locked, []string{"charge-1"}) {
			t.Fatalf("按费用进的费用行锁 = %v，want [charge-1]", fixture.confirm.lock.locked)
		}
	})

	t.Run("形成后的跟进确认", func(t *testing.T) {
		fixture := newFollowUpFixture(t)
		fixture.finals.found = true
		fixture.finals.basis = finalBasis(t).Reference
		if err := fixture.followUp.AfterEstimatedChargeFormed(t.Context(), tenant, followUpRequest(t), charge,
			domain.EstimatePricingScope{}); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(fixture.basis.confirm.lock.locked, []string{"charge-1"}) {
			t.Fatalf("费用行锁 = %v，want [charge-1]", fixture.basis.confirm.lock.locked)
		}
	})
}

// reweighOrdered 按记先后的替身装一份复磅形成夹具：包裹锁、费用行锁、读链上前一笔费用都记到同一条线上。
func reweighOrdered(t *testing.T, stage domain.ChargeStage, lockErr error) (*reweighFormation, *callOrder) {
	t.Helper()
	order := &callOrder{}
	fixture := newReweighFormationWith(t, stage, 15000, true, func(deps *application.FormEstimatedCustomerChargeDeps) {
		deps.Serializer = orderedParcelLock{order: order}
		deps.ChargeLock = orderedChargeLock{order: order, err: lockErr}
		deps.PriorCharges = orderedChargeStore{chargeStoreDouble: deps.PriorCharges.(*chargeStoreDouble), order: order}
	})
	return fixture, order
}

// Covers: 复磅形成那一头判前一笔费用的阶段之前先锁它的行：先取包裹锁，再锁链上前一笔费用的行，锁到手之后才读阶段——前一笔
// 此刻是预估还是已确认，次序都一样。确认与记替代排成一列，这一头的先后是前提。
func TestReweighLocksTheEarlierChargeRowAfterTheParcelAndBeforeJudgingItsStage(t *testing.T) {
	for name, stage := range map[string]domain.ChargeStage{"前一笔仍是预估": domain.ChargeEstimated, "前一笔已确认": domain.ChargeConfirmed} {
		t.Run(name, func(t *testing.T) {
			fixture, order := reweighOrdered(t, stage, nil)
			result, err := fixture.handle(t)
			if err != nil || result.State() != domain.SellRequestFormed {
				t.Fatalf("结果 = %s err = %v，want 已形成", result.State(), err)
			}
			earlier := "SELLCC-SELLREQ-SYN-1"
			order.assertBefore(t, "包裹锁 SYN-PARCEL-1", "锁费用行 "+earlier)
			order.assertBefore(t, "锁费用行 "+earlier, "读费用 "+earlier)
		})
	}
}

// Covers: 复磅这一头取不到费用行锁是依赖读写不动：整笔答未决等重投，不建新费用、不记替代、不记调整，请求册上这一代仍是已提交。
func TestReweighAnswersUndecidedWhenTheEarlierChargeRowCannotBeLocked(t *testing.T) {
	fixture, order := reweighOrdered(t, domain.ChargeEstimated, errors.New("synthetic lock down"))
	result, err := fixture.handle(t)
	if err != nil || result.Outcome() != application.EstimatedCustomerChargeUndecided {
		t.Fatalf("结果 = %s err = %v，want 未决", result.Outcome(), err)
	}
	if len(fixture.charges.saved) != 0 || len(fixture.supersessions.saved) != 0 || fixture.adjustments.saves != 0 {
		t.Fatalf("取锁失败之后还写了：预估 %v，替代 %v，调整 %d 笔", fixture.charges.saved, fixture.supersessions.saved, fixture.adjustments.saves)
	}
	if slices.Contains(order.calls, "读费用 SELLCC-SELLREQ-SYN-1") {
		t.Fatalf("取锁失败之后还读了前一笔费用的阶段：%v", order.calls)
	}
	if stored := fixture.requests.records[fixture.tenant.String()+"/SELLREQ-SYN-2"]; stored.State != domain.SellRequestSubmitted {
		t.Fatalf("这一代的状态 = %s，want 仍是已提交", stored.State)
	}
}

// Covers: 处理链上已有费用的那一组依赖含费用行锁，一起装才构造得起来：其余都装、只缺这一把，构造时即拒。
func TestTheEarlierChargeDependenciesAreRefusedWithoutTheChargeLock(t *testing.T) {
	base := newEstimatedChargeFixture(t, nil)
	charges, err := application.NewFormSellCustomerChargeHandler(&formationEvaluationDouble{adoption: base.adoption}, base.charges)
	if err != nil {
		t.Fatalf("形成本体：%v", err)
	}
	_, err = application.NewFormEstimatedCustomerChargeHandler(application.FormEstimatedCustomerChargeDeps{
		Requests: base.requests, Conclusions: base.requests, Mappings: base.mappings,
		Charges: charges, FollowUp: base.followUp, Clock: fixedClock{at: estimatedChargeAt},
		PriorCharges: newChargeStore(), Supersessions: &supersessionDouble{},
		ReweighAdoptions: reweighAdoptionDouble{adopted: true},
		Reweigh: application.NewRecordChargeAdjustmentHandler(application.RecordChargeAdjustmentDeps{
			Charges: newChargeStore(), Adjustments: newChargeAdjustmentStore(), Clock: fixedClock{at: estimatedChargeAt},
		}),
		Successors: base.requests, Serializer: &serializerDouble{},
	})
	if !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("err = %v，want 缺依赖", err)
	}
}
