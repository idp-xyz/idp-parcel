package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证「资料不足 → 已占用 → 补件取代」那一格（票 docs-consistency/code/20）：可达性答`证据不足`时
// 接受链照常进入接受前财务控制、在 settlement-accounting 的冻结账本上占下首版那一笔，形成决定处停在`等待受控补充`；客户
// 补件形成新版本，「新提交版本已形成」那一封驱链时先按原关联放掉首版那一笔，再判新版本——新版本照常进入控制、重新占一笔。
//
// 夹具借受控补充那条闭环（customer_supplement_resume_loop_test.go），把控制与释放两口换成生产的 PS→SA 适配器：作用域源、
// 账户目录、SA 施加与释放两只编排、两本账、采用册与留痕册都是生产实现、读写夹具自己的库；只有控制策略视图与试算口是替身
// （synRSettlementControl）。余额恰好只够冻一笔：首版那一笔不先放，新版本的冻结就占不下——「先放后判」由账本上的金额证，
// 不靠调用序。

// synRSupersededFreezeMinor 是每一版试算合计出的冻结金额，也是登记的入账余额：够冻一笔，不够冻两笔。
const synRSupersededFreezeMinor = 1000

// Covers: AT-PS-035「接受提交未成立时按原关联释放已成立各项形成的占用」被取代那一格、PS CONTEXT 词条「控制项结果」与生命周期
// 「已提交期间形成新提交版本」、ADR-0094 按恢复动作分格（释放放不掉即内部重试）、ADR-0122 决定三（释放幂等由提供方账本认领）。
func TestASupersededVersionsFreezeIsReleasedOnceBeforeTheNewVersionIsControlled(t *testing.T) {
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	control.inner, release.inner = adapter, adapter

	firstVersion := fixture.requestID.String() + "/SYN-VER-01"
	newVersion := fixture.requestID.String() + "/" + synRSupplementVersionID

	// 首版：资料不足照样进控制，冻结账本上占下首版那一笔；决定停在`等待受控补充`，本份投递入账。
	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope); err != nil {
		t.Fatalf("等待受控补充必须按处理完毕入账：%v", err)
	}
	paused := fixture.mustLoadRequest(t, ctx)
	if waiting, present := paused.AcceptanceDecisionTask().WaitingOn(); !present || waiting != psdomain.ResumeByCustomerSupplement {
		t.Fatalf("等待态 = %v（present=%v）, want CUSTOMER_SUPPLEMENT", waiting, present)
	}
	if got := fixture.freeze(t, ctx, firstVersion); !got.present || got.status != 1 || got.amount != synRSupersededFreezeMinor {
		t.Fatalf("首版冻结 = %+v, want 已冻结 %d——资料不足时控制照常占下了这一笔", got, synRSupersededFreezeMinor)
	}
	fixture.markPublished(t, submittedDelivery)

	// 客户补件：新版本落库与「新提交版本已形成」信封同一事务，首版被取代。
	supplemented := fixture.supplementOn(t, ctx, paused.CurrentSubmissionVersion().VersionID())
	if supplemented.Outcome() != psapplication.SupplementRecorded {
		t.Fatalf("受控补充 outcome = %q, want SUPPLEMENT_RECORDED（未决原因 %q）", supplemented.Outcome(), supplemented.PendingReason())
	}
	resume := fixture.claimDeliveryOfType(t, string(psinbox.SubmissionVersionFormedEventType)).Envelope

	// 第一拍：首版那一笔放不掉——链停在释放那一步，整封交回未决哨兵等重投；新版本一步都不判、一分钱都不占。
	release.failuresLeft = 1
	err := fixture.supplementGate.Consume(ctx, resume)
	if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatalf("释放放不掉应整封交回未决哨兵等重投，实际 err = %v", err)
	}
	if !strings.Contains(err.Error(), "SUPERSEDED_CONTROL_RELEASE") || !strings.Contains(err.Error(), "CONTROL_RELEASE_PENDING") {
		t.Fatalf("停在的不是被取代版本释放那一格：%v", err)
	}
	if n := fixture.countInbox(t, customerSupplementResumeConsumer, string(resume.ID)); n != 0 {
		t.Fatalf("续办门 inbox 行数 = %d, want 0——未决那一拍不该入账", n)
	}
	if fixture.reachability.assessments != 1 {
		t.Fatalf("可达性权威被问了 %d 次, want 仍为 1——首版那一笔没放，新版本却判了", fixture.reachability.assessments)
	}
	if got := fixture.freeze(t, ctx, firstVersion); got.status != 1 {
		t.Fatalf("首版冻结 = %+v, want 仍为已冻结", got)
	}
	if got := fixture.freeze(t, ctx, newVersion); got.present {
		t.Fatalf("新版本冻结 = %+v, want 没有——旧占用还在时新版本先占了", got)
	}

	// 重投：先放首版那一笔，再判新版本——余额腾出来，新版本的冻结照常占下，接受成立。
	if err := fixture.supplementGate.Consume(ctx, resume); err != nil {
		t.Fatalf("重投续办信封：%v", err)
	}
	released := fixture.freeze(t, ctx, firstVersion)
	if released.status != 2 || released.releasedAt.IsZero() {
		t.Fatalf("首版冻结 = %+v, want 已释放", released)
	}
	if got := fixture.freeze(t, ctx, newVersion); !got.present || got.status != 1 || got.amount != synRSupersededFreezeMinor {
		t.Fatalf("新版本冻结 = %+v, want 已冻结 %d——新版本照常进入接受前财务控制", got, synRSupersededFreezeMinor)
	}
	accepted := fixture.mustLoadRequest(t, ctx)
	if accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}
	if release.released != 1 {
		t.Fatalf("释放到达提供方 %d 次, want 1——接受的新版本那一笔不放", release.released)
	}

	// 已入账的投递重投：账本跳过，什么都不变。
	if err := fixture.supplementGate.Consume(ctx, resume); err != nil {
		t.Fatalf("重投已入账的续办信封：%v", err)
	}
	// 绕过 inbox 再驱一拍（另一封信封驱到同一份委托时就是这样）：释放按原关联再请求一次，提供方按幂等答原释放——
	// 首版那一笔只释放一次，释放时刻不变，新版本那一笔不动。
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		_, err := fixture.chain.Handle(txCtx, psapplication.AdvanceAcceptanceChainCommand{
			Identity:          fixture.identity,
			ShipmentRequestID: fixture.requestID,
			SubmissionVersion: mustPS(t, psdomain.NewSubmissionVersionID, synRSupplementVersionID),
			DeclaredParcelIDs: []psdomain.DeclaredParcelID{mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01")},
		})
		return err
	})
	if release.released != 2 {
		t.Fatalf("再驱一拍释放到达提供方 %d 次, want 2——每一轮都按原关联再请求", release.released)
	}
	if again := fixture.freeze(t, ctx, firstVersion); again.status != 2 || !again.releasedAt.Equal(released.releasedAt) {
		t.Fatalf("再驱之后首版冻结 = %+v, want 仍是原释放（%v）", again, released.releasedAt)
	}
	if got := fixture.freeze(t, ctx, newVersion); got.status != 1 {
		t.Fatalf("再驱之后新版本冻结 = %+v, want 仍为已冻结", got)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.funds_freeze`); n != 2 {
		t.Fatalf("冻结行数 = %d, want 2（两版各一笔）", n)
	}
}

// wireSettlement 登好 SA 那一侧并交回生产的 PS→SA 适配器：结算账户、受理前估价采用、恰好够冻一笔的入账余额。
func (fixture *customerSupplementLoopFixture) wireSettlement(t *testing.T, ctx context.Context) *pssettlement.PreAcceptanceControlAdapter {
	t.Helper()
	tenant := mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String())

	accounts, err := sapostgres.NewSettlementAccounts(fixture.db)
	if err != nil {
		t.Fatalf("构造结算账户登记册：%v", err)
	}
	adoptions, err := sapostgres.NewPreAcceptanceEstimateAdoptions(fixture.db)
	if err != nil {
		t.Fatalf("构造采用册：%v", err)
	}
	freezes, err := sapostgres.NewFreezeLedgers(fixture.db)
	if err != nil {
		t.Fatalf("构造冻结账本：%v", err)
	}
	exposures, err := sapostgres.NewCreditExposureLedgers(fixture.db)
	if err != nil {
		t.Fatalf("构造暴露账本：%v", err)
	}
	scopeLocks, err := sapostgres.NewPreAcceptanceControlScopeLocks(fixture.db)
	if err != nil {
		t.Fatalf("构造控制作用域锁：%v", err)
	}
	balances, err := sapostgres.NewOperationalBalances(fixture.db, freezes)
	if err != nil {
		t.Fatalf("构造运营余额：%v", err)
	}
	account := synRReceivableAccount(t)
	adoption, err := sadomain.NewPreAcceptanceEstimateAdoption(account.ID(), sadomain.PerParcelSellTrialSum,
		mustPS(t, sadomain.NewEstimatePricingScope, "SYN-PRICING-SCOPE-R1"))
	if err != nil {
		t.Fatalf("采用行：%v", err)
	}
	scope, err := sadomain.NewSettlementScope(
		mustPS(t, sadomain.NewLegalEntityReference, synRLegalEntity), account.ID(), mustPS(t, sadomain.NewCurrencyCode, synRCurrency))
	if err != nil {
		t.Fatalf("结算作用域：%v", err)
	}
	registeredAt := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		if _, err := accounts.Save(txCtx, tenant, account, registeredAt); err != nil {
			return err
		}
		if _, err := adoptions.SaveAdoption(txCtx, tenant, adoption, registeredAt); err != nil {
			return err
		}
		_, err := balances.RecordBalance(txCtx, tenant, scope, sapostgres.BalancePosting{
			PostedMinor: synRSupersededFreezeMinor,
			AsOf:        registeredAt,
		})
		return err
	})

	directory, err := pssettlement.NewRegisteredAccountDirectory(accounts)
	if err != nil {
		t.Fatalf("构造账户目录：%v", err)
	}
	settlement := newSynRSettlementControl(t, fixture.db)
	settlement.trials.answer = synRPricedTrials(t)
	return pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Apply:   settlement.apply,
		Release: saapplication.NewReleasePreAcceptanceControlHandler(freezes, exposures, scopeLocks, systemClock{}),
		Scopes: pssettlement.NewPolicyBackedControlScopeSource(
			&synRSettledCommercialBasis{synRSwitchableAsOfCommercialBasis: fixture.commercial}, directory),
	})
}

// synRPricedTrials 让试算口为每一版答一份带合计的试算（一件、synRSupersededFreezeMinor），控制据此占下一笔冻结。
func synRPricedTrials(t *testing.T) func(saports.PreAcceptanceTrialQuery) saports.PreAcceptanceTrialAnswer {
	t.Helper()
	return func(query saports.PreAcceptanceTrialQuery) saports.PreAcceptanceTrialAnswer {
		trial, err := sadomain.NewPreAcceptanceTrial("SYN-PARCEL-01", "SYN-TRIAL-"+query.SubmissionVersion,
			[]string{"SYN-CARD-R1/v1"}, synRSupersededFreezeMinor, query.Currency)
		if err != nil {
			t.Errorf("试算：%v", err)
		}
		return saports.PreAcceptanceTrialAnswer{
			Outcome:   saports.PreAcceptanceTrialsFormed,
			PriceCard: "SYN-CARD-R1/v1",
			Trials:    []sadomain.PreAcceptanceTrial{trial},
		}
	}
}

type synRFreezeRow struct {
	present    bool
	status     int
	amount     int64
	releasedAt time.Time
}

// freeze 按控制请求身份读冻结账本上那一行。读的是 SA 自己的表：释放成没成、放了几次，只有提供方的账本说了算。
func (fixture *customerSupplementLoopFixture) freeze(t *testing.T, ctx context.Context, controlRequest string) synRFreezeRow {
	t.Helper()
	var row synRFreezeRow
	var releasedAt *time.Time
	err := fixture.pool.QueryRow(ctx,
		`SELECT status, amount_minor, released_at FROM settlement_accounting.funds_freeze WHERE control_request_id = $1`,
		controlRequest,
	).Scan(&row.status, &row.amount, &releasedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return row
		}
		t.Fatalf("读冻结行 %s：%v", controlRequest, err)
	}
	row.present = true
	if releasedAt != nil {
		row.releasedAt = *releasedAt
	}
	return row
}

// synRFlakyRelease 把释放转给生产适配器；failuresLeft 大于零时先失败那么多次、不到提供方。released 只数到达提供方且
// 成功的那几次。
type synRFlakyRelease struct {
	inner        psports.PreAcceptanceControlRelease
	failuresLeft int
	released     int
}

func (release *synRFlakyRelease) ReleasePreAcceptanceControl(
	ctx context.Context,
	request psports.ControlReleaseRequest,
) error {
	if release.failuresLeft > 0 {
		release.failuresLeft--
		return errors.New("settlement-accounting unavailable")
	}
	if err := release.inner.ReleasePreAcceptanceControl(ctx, request); err != nil {
		return err
	}
	release.released++
	return nil
}

var _ psports.PreAcceptanceControlRelease = (*synRFlakyRelease)(nil)
