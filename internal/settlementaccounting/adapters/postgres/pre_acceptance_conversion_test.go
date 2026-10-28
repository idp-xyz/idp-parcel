package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// conversionRig 是占用收口用例的底座：一个应收结算账户、两本占用账、转出册与收口编排，全在真库上。
// 租户与作用域沿用客户收款那组用例的 tenant-receipt / le-1 / acct-1 / CNY，对账单 stmt-1 同在。
type conversionRig struct {
	t         *testing.T
	db        *bentopg.DB
	pool      *pgxpool.Pool
	tenant    domain.TenantID
	scope     domain.SettlementScope
	clock     *movingClock
	postings  *adapter.CustomerReceiptPostings
	freezes   *adapter.FreezeLedgers
	exposures *adapter.CreditExposureLedgers
	balances  *adapter.OperationalBalances
	standings *adapter.CreditStandings
	charges   *adapter.CustomerCharges
	locks     *adapter.PreAcceptanceControlScopeLocks
	handler   *application.ConvertOccupationOnChargeConfirmationHandler
}

func newConversionRig(t *testing.T) *conversionRig {
	t.Helper()
	db, pool := receiptPostingDB(t)
	seedCustomerReceiptScope(t, pool)
	rig := &conversionRig{
		t: t, db: db, pool: pool, tenant: receiptTenant(t), scope: receiptScope(t),
		clock:    &movingClock{at: time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)},
		postings: mustReceiptPostings(t, db),
	}
	var err error
	if rig.freezes, err = adapter.NewFreezeLedgers(db); err != nil {
		t.Fatal(err)
	}
	if rig.exposures, err = adapter.NewCreditExposureLedgers(db); err != nil {
		t.Fatal(err)
	}
	if rig.balances, err = adapter.NewOperationalBalances(db, rig.freezes); err != nil {
		t.Fatal(err)
	}
	if rig.standings, err = adapter.NewCreditStandings(db, rig.exposures); err != nil {
		t.Fatal(err)
	}
	if rig.charges, err = adapter.NewCustomerCharges(db); err != nil {
		t.Fatal(err)
	}
	if rig.locks, err = adapter.NewPreAcceptanceControlScopeLocks(db); err != nil {
		t.Fatal(err)
	}
	book, err := adapter.NewPreAcceptanceConversions(db)
	if err != nil {
		t.Fatal(err)
	}
	if rig.handler, err = application.NewConvertOccupationOnChargeConfirmationHandler(application.ConvertOccupationDeps{
		Book: book, Freezes: rig.freezes, Exposures: rig.exposures, ScopeLock: rig.locks,
		Balances: rig.postings, Books: rig.postings, Clock: rig.clock,
	}); err != nil {
		t.Fatal(err)
	}
	return rig
}

func (rig *conversionRig) within(run func(context.Context) error) {
	rig.t.Helper()
	rig.clock.at = rig.clock.at.Add(time.Hour)
	saWithin(rig.t, rig.db.Transactor(), rig.t.Context(), run)
}

// seedTrace 登一份受理前估价留痕：控制请求 request 下一件包裹 parcel 的试算合计。
func (rig *conversionRig) seedTrace(request, parcel string, totalMinor int64) {
	rig.t.Helper()
	trials := fmt.Sprintf(`[{"parcel":%q,"evaluation":"eval-%s","versions":[],"totalMinor":%d,"currency":"CNY"}]`,
		parcel, parcel, totalMinor)
	if _, err := rig.pool.Exec(rig.t.Context(),
		`INSERT INTO settlement_accounting.pre_acceptance_estimate_trace
			(tenant_id, legal_entity, account_id, currency, control_request_id,
			 shipment_request_ref, submission_version_ref, trial_at, form, pricing_scope_id,
			 price_card, total_minor, trials, recorded_at)
		 VALUES ('tenant-receipt', 'le-1', 'acct-1', 'CNY', $1, $2, 'v1', now(),
		         'PER_PARCEL_SELL_TRIAL_SUM', 'scope-1', 'card-1', $3, $4::jsonb, now())`,
		request, "ship-"+parcel, totalMinor, trials); err != nil {
		rig.t.Fatalf("估价留痕：%v", err)
	}
}

// freeze 按预付走一遍接受前冻结：读余额、占用、落账。返回冻结状态。
func (rig *conversionRig) freeze(request, parcel string, amount int64) domain.FreezeStatus {
	rig.t.Helper()
	rig.seedTrace(request, parcel, amount)
	var status domain.FreezeStatus
	rig.within(func(ctx context.Context) error {
		if err := rig.locks.LockControlScope(ctx, rig.tenant, rig.scope); err != nil {
			return err
		}
		balance, err := rig.balances.LoadBalance(ctx, rig.tenant, rig.scope)
		if err != nil {
			return err
		}
		ledger, err := rig.freezes.LoadForScope(ctx, rig.tenant, rig.scope)
		if err != nil {
			return err
		}
		frozen, err := ledger.Freeze(domainFreezeRequest(rig.t, request, rig.scope, amount, rig.clock.at), balance)
		if err != nil {
			return err
		}
		status = frozen.Status()
		return rig.freezes.Save(ctx, rig.tenant, rig.scope, ledger)
	})
	return status
}

// expose 按账期走一遍接受前信用校验，额度取授权额度 limit（生产上取信用依据）。
func (rig *conversionRig) expose(request, parcel string, amount, limit int64) domain.ExposureStatus {
	rig.t.Helper()
	rig.seedTrace(request, parcel, amount)
	var status domain.ExposureStatus
	rig.within(func(ctx context.Context) error {
		if err := rig.locks.LockControlScope(ctx, rig.tenant, rig.scope); err != nil {
			return err
		}
		standing, err := rig.standings.LoadCreditStanding(ctx, rig.tenant, rig.scope)
		if err != nil {
			return err
		}
		standing, err = standing.WithAuthorizedLimit(limit, mustValue(rig.t, domain.NewCreditPolicyReference, "credit-1/v1"))
		if err != nil {
			return err
		}
		ledger, err := rig.exposures.LoadForScope(ctx, rig.tenant, rig.scope)
		if err != nil {
			return err
		}
		exposed, err := ledger.Expose(domainExposureRequest(rig.t, request, rig.scope, amount, rig.clock.at), standing)
		if err != nil {
			return err
		}
		status = exposed.Status()
		return rig.exposures.Save(ctx, rig.tenant, rig.scope, ledger)
	})
	return status
}

// confirm 落一笔已确认应收并把它记在包裹 parcel 的 SELL 评价请求上，然后跑占用收口（生产上两步同在确认那一笔事务里）。
func (rig *conversionRig) confirm(chargeID, parcel string, amount int64) application.ConvertOccupationResult {
	rig.t.Helper()
	var result application.ConvertOccupationResult
	rig.within(func(ctx context.Context) error {
		charge := confirmedReceivable(rig.t, chargeID, amount, rig.clock.at)
		if _, err := rig.charges.SaveConfirmed(ctx, rig.tenant, charge); err != nil {
			return err
		}
		return nil
	})
	if _, err := rig.pool.Exec(rig.t.Context(),
		`INSERT INTO settlement_accounting.sell_evaluation_request
			(tenant_id, request_id, parcel_id, account_id, settlement_currency, pricing_scope_id,
			 billing_moment, source_fact_ref, pricing_basis_at, recorded_at, state, customer_charge_id, generation)
		 VALUES ('tenant-receipt', $1, $2, 'acct-1', 'CNY', 'scope-1',
		         'EFFECTIVE_DELIVERY_FINALIZED', 'final-1', now(), now(), 'FORMED', $3, 1)
		 ON CONFLICT DO NOTHING`, "req-"+chargeID, parcel, chargeID); err != nil {
		rig.t.Fatalf("SELL 评价请求：%v", err)
	}
	rig.within(func(ctx context.Context) error {
		stored, found, err := rig.charges.FindByID(ctx, rig.tenant, mustValue(rig.t, domain.NewCustomerChargeID, chargeID))
		if err != nil || !found {
			return fmt.Errorf("读回已确认费用 %v / %v", found, err)
		}
		result, err = rig.handler.Handle(ctx, rig.tenant, stored)
		return err
	})
	return result
}

func (rig *conversionRig) available() int64 {
	rig.t.Helper()
	balance, err := rig.balances.LoadBalance(rig.t.Context(), rig.tenant, rig.scope)
	if err != nil {
		rig.t.Fatalf("读余额：%v", err)
	}
	return balance.Available()
}

func (rig *conversionRig) headroom(limit int64) int64 {
	rig.t.Helper()
	standing, err := rig.standings.LoadCreditStanding(rig.t.Context(), rig.tenant, rig.scope)
	if err != nil {
		rig.t.Fatalf("读信用状况：%v", err)
	}
	standing, err = standing.WithAuthorizedLimit(limit, mustValue(rig.t, domain.NewCreditPolicyReference, "credit-1/v1"))
	if err != nil {
		rig.t.Fatal(err)
	}
	return standing.Headroom()
}

func (rig *conversionRig) occupation(table string, request string) (status int, converted int64) {
	rig.t.Helper()
	if err := rig.pool.QueryRow(rig.t.Context(),
		`SELECT status, converted_minor FROM settlement_accounting.`+table+`
		  WHERE tenant_id = 'tenant-receipt' AND control_request_id = $1`, request).Scan(&status, &converted); err != nil {
		rig.t.Fatalf("读占用行 %s/%s：%v", table, request, err)
	}
	return status, converted
}

// Covers: SA CONTEXT「费用确认后将适用冻结转为正式扣费」「最终确认费用高于冻结金额时必须完整入账」与 ADR-0343——预付：
// 确认把这件包裹的冻结整份转出，实际金额进已确认未结，可用余额 = 入账余额 − 剩余冻结 − 已确认未结；估价与确认额的差
// 不另立科目，重放不二转、不二算；其余委托的冻结原样占着；既有释放路径照旧（已转满的答无可释放，没转的照放）。
func TestAConfirmedPrepaidChargeConvertsItsParcelsFreezeIntoConfirmedUnsettled(t *testing.T) {
	rig := newConversionRig(t)
	// 预付的钱经普通路径进来：采用一笔 20000 的收款并映射到对账单，入账余额由映射事务按账重算（不直写表）。
	receipts := mustFundsHandler(t, mustFundsFacts(t, rig.db), mustFundsMappings(t, rig.db), mustApplications(t, rig.db),
		rig.postings, rig.clock)
	rig.within(func(ctx context.Context) error {
		_, err := receipts.AdoptFact(ctx, receiptFact(rig.tenant, "receipt-1", domain.FundsReceiptConfirmed, 20_000, "v1"))
		return err
	})
	rig.within(func(ctx context.Context) error {
		_, err := receipts.Map(ctx, receiptMap(rig.tenant, "map-receipt", "receipt-1", domain.TargetStatement, "stmt-1"))
		return err
	})
	assertPosted(t, rig.pool, 20_000, 0, 0)

	if got := rig.freeze("ship-1/v1", "P1", 9_600); got != domain.FreezeHeld {
		t.Fatalf("第一件冻结 = %s", got)
	}
	if got := rig.freeze("ship-2/v1", "P2", 9_600); got != domain.FreezeHeld {
		t.Fatalf("第二件冻结 = %s", got)
	}
	if got := rig.available(); got != 800 {
		t.Fatalf("两件冻结后可用 = %d，want 800", got)
	}
	if got := rig.freeze("ship-3/v1", "P3", 9_600); got != domain.FreezeRestricted {
		t.Fatalf("第三件 = %s，want RESTRICTED（钱不够）", got)
	}

	// 第一件终局确认：实际 9000，比估价 9600 少 600。
	result := rig.confirm("charge-p1", "P1", 9_000)
	if result.FreezeConvertedMinor() != 9_600 || result.ExposureConvertedMinor() != 0 || !result.Recomputed() {
		t.Fatalf("收口 = 冻结 %d / 暴露 %d / 重算 %v，want 9600 / 0 / true",
			result.FreezeConvertedMinor(), result.ExposureConvertedMinor(), result.Recomputed())
	}
	if status, converted := rig.occupation("funds_freeze", "ship-1/v1"); status != int(domain.FreezeConverted) || converted != 9_600 {
		t.Fatalf("ship-1 冻结 = 状态 %d 转出 %d，want CONVERTED 9600", status, converted)
	}
	if status, converted := rig.occupation("funds_freeze", "ship-2/v1"); status != int(domain.FreezeHeld) || converted != 0 {
		t.Fatalf("ship-2 冻结被动了：状态 %d 转出 %d", status, converted)
	}
	assertPosted(t, rig.pool, 20_000, 0, 9_000)
	if got := rig.available(); got != 20_000-9_600-9_000 {
		t.Fatalf("确认后可用 = %d，want %d（入账 − 剩余冻结 − 已确认未结）", got, 20_000-9_600-9_000)
	}

	// 重放同一确认：不二转、不二算。
	again := rig.confirm("charge-p1", "P1", 9_000)
	if again.FreezeConvertedMinor() != 0 {
		t.Fatalf("重放又转出 %d", again.FreezeConvertedMinor())
	}
	assertPosted(t, rig.pool, 20_000, 0, 9_000)
	var conversions int
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM settlement_accounting.pre_acceptance_control_conversion`).Scan(&conversions); err != nil || conversions != 1 {
		t.Fatalf("转出册 %d 行（%v），want 1", conversions, err)
	}

	// 既有释放路径：已转满的冻结无可释放；没转的照放，放后钱回来。
	release := mustReleaseHandler(t, rig)
	released := release("ship-1/v1")
	if released != application.NothingToRelease {
		t.Fatalf("释放已转满的冻结 = %s，want NOTHING_TO_RELEASE", released)
	}
	if released := release("ship-2/v1"); released != application.ControlReleased {
		t.Fatalf("释放没转的冻结 = %s，want CONTROL_RELEASED", released)
	}
	if got := rig.available(); got != 20_000-9_000 {
		t.Fatalf("放掉第二件后可用 = %d，want 11000", got)
	}
	if got := rig.freeze("ship-4/v1", "P4", 9_600); got != domain.FreezeHeld {
		t.Fatalf("放掉之后再来一件 = %s，want HELD", got)
	}
}

// Covers: 同上，授信：确认把暴露转入已确认未结，额度仍被占着（钱真用了），核销让已确认未结下降、额度恢复——
// 付清一件，下一件就能再占。
func TestAConfirmedCreditChargeMovesTheExposureIntoUnsettledAndSettlementRestoresTheLimit(t *testing.T) {
	const limit = 20_000
	rig := newConversionRig(t)
	if _, err := rig.pool.Exec(t.Context(),
		`INSERT INTO settlement_accounting.credit_standing
			(tenant_id, legal_entity, account_id, currency, limit_minor, overdue, as_of)
		 VALUES ('tenant-receipt', 'le-1', 'acct-1', 'CNY', 0, false, now() - interval '1 day')`); err != nil {
		t.Fatalf("信用状况：%v", err)
	}

	for index, parcel := range []string{"P1", "P2"} {
		request := fmt.Sprintf("ship-%d/v1", index+1)
		if got := rig.expose(request, parcel, 9_600, limit); got != domain.ExposureRecorded {
			t.Fatalf("%s 暴露 = %s", parcel, got)
		}
	}
	if got := rig.expose("ship-3/v1", "P3", 9_600, limit); got != domain.ExposureRestricted {
		t.Fatalf("第三件 = %s，want RESTRICTED（额度 %d ÷ 估价 9600 = 2 件）", got, limit)
	}

	for _, confirmation := range []struct{ charge, parcel string }{{"charge-p1", "P1"}, {"charge-p2", "P2"}} {
		result := rig.confirm(confirmation.charge, confirmation.parcel, 9_600)
		if result.ExposureConvertedMinor() != 9_600 || result.FreezeConvertedMinor() != 0 {
			t.Fatalf("%s 收口 = 暴露 %d / 冻结 %d，want 9600 / 0", confirmation.parcel,
				result.ExposureConvertedMinor(), result.FreezeConvertedMinor())
		}
	}
	for _, request := range []string{"ship-1/v1", "ship-2/v1"} {
		if status, converted := rig.occupation("credit_exposure", request); status != int(domain.ExposureConverted) || converted != 9_600 {
			t.Fatalf("%s 暴露 = 状态 %d 转出 %d，want CONVERTED 9600", request, status, converted)
		}
	}
	assertPosted(t, rig.pool, 0, 0, 19_200)
	if got := rig.headroom(limit); got != limit-19_200 {
		t.Fatalf("确认后额度余量 = %d，want %d（暴露已转，改由已确认未结占额度）", got, limit-19_200)
	}
	if got := rig.expose("ship-5/v1", "P5", 9_600, limit); got != domain.ExposureRestricted {
		t.Fatalf("钱还没付清就再占 = %s，want RESTRICTED", got)
	}

	// 付清第一件：收款进来、映射到对账单、核销 9600 → 已确认未结降到 9600，额度恢复一件。
	handler := mustFundsHandler(t, mustFundsFacts(t, rig.db), mustFundsMappings(t, rig.db), mustApplications(t, rig.db),
		rig.postings, rig.clock)
	for _, step := range []struct {
		what string
		run  func(context.Context) (application.FundsResult, error)
	}{
		{"采用", func(ctx context.Context) (application.FundsResult, error) {
			return handler.AdoptFact(ctx, receiptFact(rig.tenant, "receipt-1", domain.FundsReceiptConfirmed, 9_600, "v1"))
		}},
		{"映射", func(ctx context.Context) (application.FundsResult, error) {
			return handler.Map(ctx, receiptMap(rig.tenant, "map-receipt", "receipt-1", domain.TargetStatement, "stmt-1"))
		}},
		{"核销", func(ctx context.Context) (application.FundsResult, error) {
			return handler.Apply(ctx, statementApplication(rig.tenant, "app-1", 9_600))
		}},
	} {
		rig.within(func(ctx context.Context) error {
			result, err := step.run(ctx)
			if err != nil {
				return err
			}
			t.Logf("%s = %s", step.what, result.Outcome())
			return nil
		})
	}
	assertPosted(t, rig.pool, 0, 0, 9_600)
	if got := rig.headroom(limit); got != limit-9_600 {
		t.Fatalf("核销后额度余量 = %d，want %d", got, limit-9_600)
	}
	if got := rig.expose("ship-6/v1", "P6", 9_600, limit); got != domain.ExposureRecorded {
		t.Fatalf("核销之后再占 = %s，want RECORDED——付清一件，下一件恢复", got)
	}
}

// 没有占用可转的费用（不是接受前控制那一路形成的）照常重算已确认未结，不因找不到占用而失败。
func TestAChargeWithoutAnyOccupationStillRecomputesTheUnsettled(t *testing.T) {
	rig := newConversionRig(t)
	result := rig.confirm("charge-orphan", "P-NO-TRACE", 5_000)
	if result.FreezeConvertedMinor() != 0 || result.ExposureConvertedMinor() != 0 || !result.Recomputed() {
		t.Fatalf("收口 = %d / %d / %v", result.FreezeConvertedMinor(), result.ExposureConvertedMinor(), result.Recomputed())
	}
	assertPosted(t, rig.pool, 0, 0, 5_000)
}

func domainFreezeRequest(t *testing.T, request string, scope domain.SettlementScope, amount int64, at time.Time) domain.FreezeRequest {
	t.Helper()
	built, err := domain.NewFreezeRequest(
		mustValue(t, domain.NewControlRequestID, request), scope, amount,
		mustValue(t, domain.NewBusinessAssociationReference, request), at)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func domainExposureRequest(t *testing.T, request string, scope domain.SettlementScope, amount int64, at time.Time) domain.ExposureRequest {
	t.Helper()
	built, err := domain.NewExposureRequest(
		mustValue(t, domain.NewControlRequestID, request), scope, amount,
		mustValue(t, domain.NewBusinessAssociationReference, request), at)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// confirmedReceivable 造一笔确认在 tenant-receipt / le-1 / acct-1 / CNY 上的应收费用。
func confirmedReceivable(t *testing.T, id string, amount int64, at time.Time) domain.CustomerCharge {
	t.Helper()
	formed, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 mustValue(t, domain.NewCustomerChargeID, id),
		FeeItem:            mustValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:         mustValue(t, domain.NewSellEvaluationReference, "eval-"+id),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:      amount,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    amount,
		Stage:              domain.ChargeEstimated,
		FormedAt:           at.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("预估费用：%v", err)
	}
	confirmed, err := formed.Confirm(domain.ConfirmedChargeFacts{
		ResponsibleEntity:    mustValue(t, domain.NewLegalEntityReference, "le-1"),
		Counterparty:         mustValue(t, domain.NewSettlementCounterpartyReference, "shipper-1"),
		Direction:            domain.ChargeReceivable,
		SettlementAccount:    mustValue(t, domain.NewSettlementAccountID, "acct-1"),
		ContractBasis:        mustValue(t, domain.NewContractBasisReference, "contract-1"),
		PrimaryChargingScope: mustValue(t, domain.NewChargingScopeReference, "scope-1"),
		SourceFact:           mustValue(t, domain.NewSourceFactReference, "final-1"),
	}, mustValue(t, domain.NewConfirmationBasisReference, "basis-"+id), at)
	if err != nil {
		t.Fatalf("确认费用：%v", err)
	}
	return confirmed
}

// mustReleaseHandler 交回一个按原控制请求在事务里释放的函数，走生产的释放编排。
func mustReleaseHandler(t *testing.T, rig *conversionRig) func(request string) application.ReleaseOutcome {
	t.Helper()
	handler := application.NewReleasePreAcceptanceControlHandler(rig.freezes, rig.exposures, rig.locks, rig.clock)
	return func(request string) application.ReleaseOutcome {
		t.Helper()
		var outcome application.ReleaseOutcome
		rig.within(func(ctx context.Context) error {
			result, err := handler.Handle(ctx, application.ReleasePreAcceptanceControlCommand{
				TenantID: rig.tenant, RequestID: mustValue(t, domain.NewControlRequestID, request), Scope: rig.scope,
			})
			outcome = result.Outcome()
			return err
		})
		return outcome
	}
}

// 转出册的写口与其余写口同一条纪律：无环境事务即被 RequireExecutor 拒，转出不能落在事务之外。
func TestPreAcceptanceConversionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	rig := newConversionRig(t)
	book, err := adapter.NewPreAcceptanceConversions(rig.db)
	if err != nil {
		t.Fatal(err)
	}
	_, err = book.RecordConversion(t.Context(), rig.tenant, rig.scope,
		mustValue(t, domain.NewControlRequestID, "ship-1/v1"), "P1", ports.OccupationFreeze, 100,
		mustValue(t, domain.NewCustomerChargeID, "charge-1"), rig.clock.at)
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务记转出应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: ADR-0343 决定三——同一控制请求含两件包裹：P1 确认只转 P1 份额，请求仍占着 P2 份额；P2 确认后转满。
func TestAMultiParcelRequestKeepsTheOtherParcelsShareUntilItsOwnChargeIsConfirmed(t *testing.T) {
	rig := newConversionRig(t)
	receipts := mustFundsHandler(t, mustFundsFacts(t, rig.db), mustFundsMappings(t, rig.db), mustApplications(t, rig.db),
		rig.postings, rig.clock)
	rig.within(func(ctx context.Context) error {
		_, err := receipts.AdoptFact(ctx, receiptFact(rig.tenant, "receipt-1", domain.FundsReceiptConfirmed, 20_000, "v1"))
		return err
	})
	rig.within(func(ctx context.Context) error {
		_, err := receipts.Map(ctx, receiptMap(rig.tenant, "map-receipt", "receipt-1", domain.TargetStatement, "stmt-1"))
		return err
	})
	trials := `[{"parcel":"P1","evaluation":"e1","versions":[],"totalMinor":4000,"currency":"CNY"},` +
		`{"parcel":"P2","evaluation":"e2","versions":[],"totalMinor":6000,"currency":"CNY"}]`
	if _, err := rig.pool.Exec(t.Context(),
		`INSERT INTO settlement_accounting.pre_acceptance_estimate_trace
			(tenant_id, legal_entity, account_id, currency, control_request_id,
			 shipment_request_ref, submission_version_ref, trial_at, form, pricing_scope_id,
			 price_card, total_minor, trials, recorded_at)
		 VALUES ('tenant-receipt', 'le-1', 'acct-1', 'CNY', 'ship-1/v1', 'ship-1', 'v1', now(),
		         'PER_PARCEL_SELL_TRIAL_SUM', 'scope-1', 'card-1', 10000, $1::jsonb, now())`, trials); err != nil {
		t.Fatalf("估价留痕：%v", err)
	}
	rig.within(func(ctx context.Context) error {
		balance, err := rig.balances.LoadBalance(ctx, rig.tenant, rig.scope)
		if err != nil {
			return err
		}
		ledger, err := rig.freezes.LoadForScope(ctx, rig.tenant, rig.scope)
		if err != nil {
			return err
		}
		if _, err := ledger.Freeze(domainFreezeRequest(t, "ship-1/v1", rig.scope, 10_000, rig.clock.at), balance); err != nil {
			return err
		}
		return rig.freezes.Save(ctx, rig.tenant, rig.scope, ledger)
	})

	if got := rig.confirm("charge-p1", "P1", 4_000); got.FreezeConvertedMinor() != 4_000 {
		t.Fatalf("P1 确认转出 %d，want 4000", got.FreezeConvertedMinor())
	}
	if status, converted := rig.occupation("funds_freeze", "ship-1/v1"); status != int(domain.FreezeHeld) || converted != 4_000 {
		t.Fatalf("P1 之后冻结 = 状态 %d 转出 %d，want HELD 4000（P2 份额仍占着）", status, converted)
	}
	if got := rig.available(); got != 20_000-6_000-4_000 {
		t.Fatalf("P1 之后可用 = %d，want 10000", got)
	}
	if got := rig.confirm("charge-p2", "P2", 6_000); got.FreezeConvertedMinor() != 6_000 {
		t.Fatalf("P2 确认转出 %d，want 6000", got.FreezeConvertedMinor())
	}
	if status, converted := rig.occupation("funds_freeze", "ship-1/v1"); status != int(domain.FreezeConverted) || converted != 10_000 {
		t.Fatalf("P2 之后冻结 = 状态 %d 转出 %d，want CONVERTED 10000", status, converted)
	}
}
