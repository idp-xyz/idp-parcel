package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证释放口外那一层 savepoint 与 settlement-accounting 释放编排合起来的落法（ADR-0122 决定三、
// ADR-0291）：组合策略下同一请求在冻结与暴露两本账上各占一笔，编排先认领冻结并落库，再读暴露账本。暴露账本读不回时，
// 事务没中止就留着冻结那本已落库的释放，事务已中止才退回。夹具不起委托与受控补充闭环：释放口、两本账、作用域锁都是生产
// 实现，读写夹具自己的库；只有暴露账本的读口能被指定「读不回」。

const (
	synHRequestID = "SYN-HALF-REQUEST/SYN-VER-01"
	synHFreeze    = 3000
	synHExposure  = 2000
)

// synHExposures 包住真实暴露账本：failures 大于零时读口先答那么多次错误，错误不来自 SQL，事务不会因此中止（读口交 nil、
// 行读回来重建不了账本之类）。
type synHExposures struct {
	inner    saports.CreditExposureLedgerRepository
	failures int
}

func (exposures *synHExposures) LoadForScope(
	ctx context.Context,
	tenant sadomain.TenantID,
	scope sadomain.SettlementScope,
) (*sadomain.CreditExposureLedger, error) {
	if exposures.failures > 0 {
		exposures.failures--
		return nil, errors.New("暴露账本读不回：重建不出账本")
	}
	return exposures.inner.LoadForScope(ctx, tenant, scope)
}

func (exposures *synHExposures) Save(
	ctx context.Context,
	tenant sadomain.TenantID,
	scope sadomain.SettlementScope,
	ledger *sadomain.CreditExposureLedger,
) error {
	return exposures.inner.Save(ctx, tenant, scope, ledger)
}

var _ saports.CreditExposureLedgerRepository = (*synHExposures)(nil)

type synHFixture struct {
	db        *bentopg.DB
	pool      *pgxpool.Pool
	exposures *synHExposures
	release   psports.PreAcceptanceControlRelease
	request   psports.ControlReleaseRequest
}

// newSynHFixture 在冻结与暴露两本账上各占一笔（同一请求身份），交回包着生产释放适配器的 savepoint 装饰器。
func newSynHFixture(t *testing.T) *synHFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	freezes, err := sapostgres.NewFreezeLedgers(db)
	if err != nil {
		t.Fatalf("构造冻结账本：%v", err)
	}
	realExposures, err := sapostgres.NewCreditExposureLedgers(db)
	if err != nil {
		t.Fatalf("构造暴露账本：%v", err)
	}
	scopeLocks, err := sapostgres.NewPreAcceptanceControlScopeLocks(db)
	if err != nil {
		t.Fatalf("构造控制作用域锁：%v", err)
	}
	exposures := &synHExposures{inner: realExposures}

	identity := synSourceIdentity(t)
	tenant := mustPS(t, sadomain.NewTenantID, identity.TenantID().String())
	scope, err := sadomain.NewSettlementScope(
		mustPS(t, sadomain.NewLegalEntityReference, "SYN-HALF-LE"),
		mustPS(t, sadomain.NewSettlementAccountID, "SYN-HALF-ACCOUNT"),
		mustPS(t, sadomain.NewCurrencyCode, "CNY"))
	if err != nil {
		t.Fatalf("结算作用域：%v", err)
	}
	at := time.Now().Add(-time.Hour)
	requestID := mustPS(t, sadomain.NewControlRequestID, synHRequestID)
	association := mustPS(t, sadomain.NewBusinessAssociationReference, "SAC-"+synHRequestID)

	balance, err := sadomain.NewOperationalBalance(scope, 10000, 0, 0, 0)
	if err != nil {
		t.Fatalf("运营余额：%v", err)
	}
	freezeRequest, err := sadomain.NewFreezeRequest(requestID, scope, synHFreeze, association, at)
	if err != nil {
		t.Fatalf("冻结请求：%v", err)
	}
	registered, err := sadomain.NewCreditStanding(scope, 0, 0, false)
	if err != nil {
		t.Fatalf("信用状况：%v", err)
	}
	standing, err := registered.WithAuthorizedLimit(8000, mustPS(t, sadomain.NewCreditPolicyReference, "SYN-CREDIT-POLICY/v1"))
	if err != nil {
		t.Fatalf("授权额度：%v", err)
	}
	exposureRequest, err := sadomain.NewExposureRequest(requestID, scope, synHExposure, association, at)
	if err != nil {
		t.Fatalf("暴露请求：%v", err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		freezeLedger := sadomain.NewFreezeLedger()
		if _, err := freezeLedger.Freeze(freezeRequest, balance); err != nil {
			return err
		}
		if err := freezes.Save(txCtx, tenant, scope, freezeLedger); err != nil {
			return err
		}
		exposureLedger := sadomain.NewCreditExposureLedger()
		if _, err := exposureLedger.Expose(exposureRequest, standing); err != nil {
			return err
		}
		return realExposures.Save(txCtx, tenant, scope, exposureLedger)
	})

	adapter := pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Release: saapplication.NewReleasePreAcceptanceControlHandler(freezes, exposures, scopeLocks, systemClock{}),
	})
	occupation, err := psdomain.NewOccupationScope("SYN-HALF-LE", "SYN-HALF-ACCOUNT", "CNY")
	if err != nil {
		t.Fatalf("占用作用域：%v", err)
	}
	return &synHFixture{
		db:        db,
		pool:      pool,
		exposures: exposures,
		release:   pspostgres.NewSavepointedControlRelease(db, adapter),
		request: psports.ControlReleaseRequest{
			Identity:          identity,
			ShipmentRequestID: mustPS(t, psdomain.NewShipmentRequestID, "SYN-HALF-SHIPMENT"),
			SubmissionVersion: mustPS(t, psdomain.NewSubmissionVersionID, "SYN-VER-01"),
			ControlResultID:   mustPS(t, psdomain.NewFinancialControlResultID, synHRequestID),
			Scope:             occupation,
		},
	}
}

type synHLedgerRow struct {
	status     int
	releasedAt *time.Time
}

func (row synHLedgerRow) released() bool { return row.status == 2 && row.releasedAt != nil }

// row 读冻结或暴露账本上这一请求那一行。读的是 SA 自己的表：释放成没成，只有提供方的账本说了算。
func (fixture *synHFixture) row(t *testing.T, table string) synHLedgerRow {
	t.Helper()
	var row synHLedgerRow
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT status, released_at FROM settlement_accounting.`+table+` WHERE control_request_id = $1`,
		synHRequestID).Scan(&row.status, &row.releasedAt); err != nil {
		t.Fatalf("读 %s 行：%v", table, err)
	}
	return row
}

// rowIn 在调用方事务里读同一行：提交之前看事务内的现状。
func (fixture *synHFixture) rowIn(ctx context.Context, table string) (synHLedgerRow, error) {
	executor, err := fixture.db.RequireExecutor(ctx)
	if err != nil {
		return synHLedgerRow{}, err
	}
	var row synHLedgerRow
	err = executor.QueryRow(ctx,
		`SELECT status, released_at FROM settlement_accounting.`+table+` WHERE control_request_id = $1`,
		synHRequestID).Scan(&row.status, &row.releasedAt)
	return row, err
}

// releaseOnce 在一笔调用方事务里释放一次，随后借一张临时表再写一行（事务若已中止，这一句答 25P02），整笔提交。
// inTx 在释放与随后的写之间看事务内的现状；断言失败交回错误，不在事务闭包里 Fatalf（Goexit 会卡住事务收尾）。
func (fixture *synHFixture) releaseOnce(
	t *testing.T,
	beforeRelease func(context.Context) error,
	inTx func(context.Context) error,
) psports.ControlReleaseAnswer {
	t.Helper()
	var answer psports.ControlReleaseAnswer
	err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if beforeRelease != nil {
			if err := beforeRelease(txCtx); err != nil {
				return err
			}
		}
		var releaseErr error
		if answer, releaseErr = fixture.release.ReleasePreAcceptanceControl(txCtx, fixture.request); releaseErr != nil {
			return releaseErr
		}
		if inTx != nil {
			if err := inTx(txCtx); err != nil {
				return err
			}
		}
		executor, err := fixture.db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		if _, err := executor.Exec(txCtx, `CREATE TEMP TABLE after_release (n int) ON COMMIT DROP`); err != nil {
			return err
		}
		_, err = executor.Exec(txCtx, `INSERT INTO after_release VALUES (1)`)
		return err
	})
	if err != nil {
		t.Fatalf("释放之后调用方事务的读写或提交失败：%v", err)
	}
	return answer
}

func synHAssertNotFormed(t *testing.T, answer psports.ControlReleaseAnswer, reason string) {
	t.Helper()
	if answer.Outcome != psports.ControlReleaseNotFormed || answer.Reason.String() != reason {
		t.Fatalf("答复 = %+v, want 可重试未形成（%s）原样交回", answer, reason)
	}
}

// Covers: AT-PS-160、ADR-0122 决定三与 ADR-0291——组合策略（预付冻结与信用校验同在）下，暴露账本读不回而语句没出错：冻结那本已落库的
// 释放保住（已释放、有释放时刻），释放仍答`可重试未形成`，调用方事务照样提交；续办重放时冻结账本按幂等交回原释放时刻，
// 暴露账本再认领一次，两本各自收口。
func TestACombinedPolicyReleaseKeepsTheFreezeReleaseWhenTheExposureLedgerCannotBeReadBack(t *testing.T) {
	fixture := newSynHFixture(t)
	fixture.exposures.failures = 1

	var inTx synHLedgerRow
	answer := fixture.releaseOnce(t, nil, func(ctx context.Context) error {
		row, err := fixture.rowIn(ctx, "funds_freeze")
		inTx = row
		return err
	})
	synHAssertNotFormed(t, answer, "SA-EXPOSURE_LEDGER_UNAVAILABLE")
	if !inTx.released() {
		t.Fatalf("提交之前事务内冻结行 = %+v, want 已释放", inTx)
	}
	freeze := fixture.row(t, "funds_freeze")
	if !freeze.released() {
		t.Fatalf("提交之后冻结行 = %+v, want 已释放且有释放时刻——事务没中止，那一半不该被撤回", freeze)
	}
	if exposure := fixture.row(t, "credit_exposure"); exposure.status != 1 || exposure.releasedAt != nil {
		t.Fatalf("暴露行 = %+v, want 仍占着——它没读回来，没有被认领", exposure)
	}

	replayed := fixture.releaseOnce(t, nil, nil)
	if replayed.Outcome != psports.ControlReleaseSettled {
		t.Fatalf("重放答复 = %+v, want 已收口", replayed)
	}
	if again := fixture.row(t, "funds_freeze"); !again.released() || !again.releasedAt.Equal(*freeze.releasedAt) {
		t.Fatalf("重放之后冻结行 = %+v, want 仍是原释放（%v）", again, freeze.releasedAt)
	}
	if exposure := fixture.row(t, "credit_exposure"); !exposure.released() {
		t.Fatalf("重放之后暴露行 = %+v, want 已释放", exposure)
	}
}

// Covers: AT-PS-160、同一格的真数据版本——暴露行的幂等指纹不是本适配器写下的形状，真实的暴露账本读口重建不了账本、没有 SQL 报错；
// 冻结那本已落库的释放同样保住。
func TestACombinedPolicyReleaseKeepsTheFreezeReleaseWhenAnExposureRowCannotBeRebuilt(t *testing.T) {
	fixture := newSynHFixture(t)
	if _, err := fixture.pool.Exec(t.Context(),
		`UPDATE settlement_accounting.credit_exposure SET request_digest = 'not-hex' WHERE control_request_id = $1`,
		synHRequestID); err != nil {
		t.Fatalf("弄坏暴露行的指纹：%v", err)
	}

	answer := fixture.releaseOnce(t, nil, nil)
	synHAssertNotFormed(t, answer, "SA-EXPOSURE_LEDGER_UNAVAILABLE")
	if freeze := fixture.row(t, "funds_freeze"); !freeze.released() {
		t.Fatalf("提交之后冻结行 = %+v, want 已释放", freeze)
	}
}

// Covers: AT-PS-160、SQL 级错误那一格如实记录（ADR-0291）——暴露账本因 SQL 错误读不回（表被挪走），事务已进中止态：装饰器退回 savepoint，
// 冻结那本刚落的释放随之撤掉（事务本就保不住它），释放仍答`可重试未形成`；退回之后同一事务照样读写与提交。
func TestACombinedPolicyReleaseStillUndoesTheFreezeReleaseWhenTheExposureReadFailsInSQL(t *testing.T) {
	fixture := newSynHFixture(t)

	var inTx synHLedgerRow
	answer := fixture.releaseOnce(t,
		func(ctx context.Context) error {
			executor, err := fixture.db.RequireExecutor(ctx)
			if err != nil {
				return err
			}
			_, err = executor.Exec(ctx,
				`ALTER TABLE settlement_accounting.credit_exposure RENAME TO credit_exposure_moved`)
			return err
		},
		func(ctx context.Context) error {
			row, err := fixture.rowIn(ctx, "funds_freeze")
			inTx = row
			return err
		})
	synHAssertNotFormed(t, answer, "SA-EXPOSURE_LEDGER_UNAVAILABLE")
	if inTx.status != 1 || inTx.releasedAt != nil {
		t.Fatalf("退回之后事务内冻结行 = %+v, want 仍占着——中止的事务保不住那一半，退回把它撤了", inTx)
	}
	if freeze := fixture.row(t, "funds_freeze"); freeze.status != 1 || freeze.releasedAt != nil {
		t.Fatalf("提交之后冻结行 = %+v, want 仍占着", freeze)
	}
}

// Covers: AT-PS-160、冻结账本读不回而语句没出错（行的指纹重建不了账本）：可重试未形成，什么也没落库，事务没中止，随后的读写与提交照常。
func TestAReleaseWhoseFreezeLedgerCannotBeRebuiltLeavesTheTransactionUsable(t *testing.T) {
	fixture := newSynHFixture(t)
	if _, err := fixture.pool.Exec(t.Context(),
		`UPDATE settlement_accounting.funds_freeze SET request_digest = 'not-hex' WHERE control_request_id = $1`,
		synHRequestID); err != nil {
		t.Fatalf("弄坏冻结行的指纹：%v", err)
	}

	answer := fixture.releaseOnce(t, nil, nil)
	synHAssertNotFormed(t, answer, "SA-FREEZE_LEDGER_UNAVAILABLE")
	if freeze := fixture.row(t, "funds_freeze"); freeze.status != 1 {
		t.Fatalf("冻结行 = %+v, want 仍占着", freeze)
	}
	if exposure := fixture.row(t, "credit_exposure"); exposure.status != 1 {
		t.Fatalf("暴露行 = %+v, want 仍占着——冻结没读回来，暴露那本没轮到", exposure)
	}
}

// Covers: AT-PS-160、`确定未受理`一格——存下的作用域是零值，提供方在读任何账本之前就拒收：答复原样交回，事务没中止，两本账都没动，
// 随后的读写与提交照常。
func TestAReleaseTheProviderRefusesLeavesTheTransactionUsableAndTheLedgersUntouched(t *testing.T) {
	fixture := newSynHFixture(t)
	fixture.request.Scope = psdomain.OccupationScope{}

	answer := fixture.releaseOnce(t, nil, nil)
	if answer.Outcome != psports.ControlReleaseNotAccepted || answer.Reason.String() != "SA-RELEASE_REQUEST_NOT_ACCEPTED" {
		t.Fatalf("答复 = %+v, want 确定未受理原样交回", answer)
	}
	if freeze := fixture.row(t, "funds_freeze"); freeze.status != 1 {
		t.Fatalf("冻结行 = %+v, want 仍占着", freeze)
	}
	if exposure := fixture.row(t, "credit_exposure"); exposure.status != 1 {
		t.Fatalf("暴露行 = %+v, want 仍占着", exposure)
	}
}

// Covers: AT-PS-160、已收口路径两本账都落库——两本账都读得回，释放答已收口，提交之后仍是已释放；重放答原释放。
func TestACombinedPolicyReleaseSettlesBothLedgers(t *testing.T) {
	fixture := newSynHFixture(t)

	answer := fixture.releaseOnce(t, nil, nil)
	if answer.Outcome != psports.ControlReleaseSettled {
		t.Fatalf("答复 = %+v, want 已收口", answer)
	}
	freeze, exposure := fixture.row(t, "funds_freeze"), fixture.row(t, "credit_exposure")
	if !freeze.released() || !exposure.released() {
		t.Fatalf("冻结行 = %+v 暴露行 = %+v, want 都已释放", freeze, exposure)
	}
	if replayed := fixture.releaseOnce(t, nil, nil); replayed.Outcome != psports.ControlReleaseSettled {
		t.Fatalf("重放答复 = %+v, want 已收口", replayed)
	}
	if again := fixture.row(t, "funds_freeze"); !again.releasedAt.Equal(*freeze.releasedAt) {
		t.Fatalf("重放之后冻结释放时刻 = %v, want 仍是 %v", again.releasedAt, freeze.releasedAt)
	}
}
