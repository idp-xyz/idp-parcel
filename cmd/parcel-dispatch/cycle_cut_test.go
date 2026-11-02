package main

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件钉到点截单在生产装配上（ADR-0339 决定三）：演示租户登了确认与截单两个时点、对账安排（上海时区）、按确认日归属的形态与
// 日截周期。一笔费用照 charge_confirmation_on_final_outcome_test.go 那条线确认之后，清空它的归属日、把确认时刻拨到前天——真实时间里
// 刚确认的费用要等下一个截单时刻才关上，用例不等；清空归属日同时走一遍「确认时判不出、截单时当场补判」那一格。一拍之后：前天那一期发出一张单、收这一笔、合计是它的结算金额；昨天那一期没有费用，只记截止日；
// 再一拍什么都不多出。真库、真路由表，证据只记 S。

// Covers: AT-SA-224——真库装配。单号由截单编排在发布事务里按编号规则取（ADR-0355 决定九），不再从账户与截止日拼；账期 CYCLE/<截止日>
// 与截止版本 v1 照旧，同一个截止日就是同一个发布意图。
func TestTheDispatchBeatCutsADueCycleIntoAPublishedStatement(t *testing.T) {
	beat, db, dayBeforeYesterday := dueCycleCut(t)
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("截单那一拍：%v", err)
	}
	if got := cycleChargeAttribution(t, db); got != dayBeforeYesterday {
		t.Fatalf("补判的归属日 = %s，want %s", got, dayBeforeYesterday)
	}
	var number, total, period, version string
	if err := querier.QueryRow(t.Context(),
		`SELECT statement_number, total_minor::text, period_ref, draft_version FROM settlement_accounting.customer_statement WHERE tenant_id = $1`, demoTenant,
	).Scan(&number, &total, &period, &version); err != nil {
		t.Fatalf("没出单：%v", err)
	}
	// 产品默认模板 STMT-{yyyy}{mm}-{seq:5}：年月是服务端落单此刻在账户业务时区里的那个月，比形状不比年月（月末那一秒用例与服务端会各说一个月）。
	if !regexp.MustCompile(`^STMT-\d{6}-00001$`).MatchString(number) || total != "10500" ||
		period != "CYCLE/"+dayBeforeYesterday || version != "v1" {
		t.Fatalf("对账单 = %s / %s / %s / %s，want STMT-<年月>-00001 / 10500 / CYCLE/%s / v1", number, total, period, version, dayBeforeYesterday)
	}
	if n := countRows(t, db, `SELECT count(*) FROM settlement_accounting.statement_cycle_cut WHERE outcome = 'STATEMENT_PUBLISHED' AND statement_number = $1`, number); n != 1 {
		t.Fatalf("截止记录里发了 %d 张", n)
	}
	// 前天之后已关上的截止日都只记无费用：上海 18 点前是昨天一天，18 点后今天也关上了。
	if n := countRows(t, db, `SELECT count(*) FROM settlement_accounting.statement_cycle_cut WHERE closing_date > $1::date AND outcome <> 'NO_CHARGE'`, dayBeforeYesterday); n != 0 {
		t.Fatalf("前天之后的截止日出了 %d 张单", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM settlement_accounting.statement_cycle_cut WHERE closing_date = $1::date + 1`, dayBeforeYesterday); n != 1 {
		t.Fatalf("昨天那一期的截止记录 = %d 行，want 1", n)
	}

	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("再一拍：%v", err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM settlement_accounting.customer_statement`); n != 1 {
		t.Fatalf("再一拍之后有 %d 张单，want 仍是 1", n)
	}
}

// Covers: AT-SA-224（撞号那一格）——租户换了一份编号格式，新格式下的前二十个号都已被换格式前的单占用：到点截单答编号规则撞号，这一户整笔
// 回滚——不出单、不记截止日、不落补判的归属日，取过的流水随之释放；日志分类是撞号。腾出第一个号之后下一拍照常截出那一期，单号正是它。
func TestADispatchCycleCutStopsOnANumberingCollisionAndReleasesTheSequence(t *testing.T) {
	beat, db, dayBeforeYesterday := dueCycleCut(t)
	numbers, err := docnumbering.Build(db, docnumber.SecretTable{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := numbers.Registrations.RegisterRule(t.Context(), docnumber.RegisterRuleCommand{
		Tenant: demoTenant, Type: sadomain.CustomerStatementDocumentNumberType.Code, Template: "OLDSTMT{seq:2}",
		EffectiveFrom: time.Now().Add(-time.Hour), RegisteredBy: "SYN-TEST",
	}); err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		executor, err := db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(txCtx, `INSERT INTO settlement_accounting.customer_statement
			(tenant_id, statement_number, account_id, period_ref, currency, total_minor, content_digest, lines, adjustment_lines,
			 published_at, recorded_at)
			SELECT $1, 'OLDSTMT' || lpad(n::text, 2, '0'), 'SYN-OLD-ACCOUNT', 'OLD-' || n, 'CNY', 1, 'digest-old-' || n,
			       jsonb_build_array(jsonb_build_object('charge', 'SYN-OLD-CHARGE-' || n, 'amount_minor', 1)), '[]'::jsonb, now(), now()
			  FROM generate_series(1, $2::int) AS n`, demoTenant, docnumber.MaxSkippedNumbers)
		return err
	})

	_, err = beat.DispatchOnce(t.Context())
	if !errors.Is(err, saapplication.ErrCycleCutNumberingCollision) || !strings.Contains(err.Error(), "document numbering rule collision") {
		t.Fatalf("撞号那一拍 = %v，want 按撞号分类", err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM settlement_accounting.customer_statement WHERE account_id = 'SYN-SA-SHIPPER-01-RECEIVABLE'`); n != 0 {
		t.Fatalf("撞号还出了 %d 张单", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM settlement_accounting.statement_cycle_cut WHERE closing_date >= $1::date`, dayBeforeYesterday); n != 0 {
		t.Fatalf("撞号还记了 %d 个截止日：照记就得提交，取过的流水全成空号", n)
	}
	if got := cycleChargeAttribution(t, db); got != "" {
		t.Fatalf("撞号那一户没整笔回滚，补判的归属日 %s 落了库", got)
	}

	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		executor, err := db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(txCtx, `DELETE FROM settlement_accounting.customer_statement WHERE tenant_id = $1 AND statement_number = 'OLDSTMT01'`, demoTenant)
		return err
	})
	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("腾出第一个号之后那一拍：%v", err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM settlement_accounting.customer_statement WHERE statement_number = 'OLDSTMT01' AND account_id = 'SYN-SA-SHIPPER-01-RECEIVABLE'`); n != 1 {
		t.Fatalf("腾出第一个号之后没截出用它的那一张：%d", n)
	}
}

// dueCycleCut 备好一户到点该截的一期：登确认与截单时点、对账安排、形态与日截周期，确认一笔费用，再把它的确认时刻拨到前天、清空
// 归属日、撤掉前天起的截止记录——下一拍到点截单就要当场补判并把它收进前天那一期。交回派发器、库与前天那一天。
func dueCycleCut(t *testing.T) (Beat, *bentopg.DB, string) {
	t.Helper()
	beat, db, store := wiredBeat(t)
	tenant := saValue(t, sadomain.NewTenantID, demoTenant)
	seedConfirmingTenant(t, db, tenant)

	shanghai := time.FixedZone("CST", 8*3600)
	today := time.Now().In(shanghai)
	// 确认时刻拨回两天之后，按上海 18:00 截单落成的归属日：18 点前是前天，18 点后是昨天。
	backdated := today.AddDate(0, 0, -2)
	if backdated.Hour() >= 18 {
		backdated = backdated.AddDate(0, 0, 1)
	}
	dayBeforeYesterday := backdated.Format("2006-01-02")
	since := today.AddDate(0, 0, -3).Format("2006-01-02")
	seedCycleCut(t, db, tenant, since)

	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		t.Fatal(err)
	}
	finalRecord := effectiveDeliveryFinalRecord(t, effectiveDeliveryFinalKey(t, "SYN-PARCEL-01"))
	evaluations, err := pppostgres.NewEvaluations(db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := pppostgres.NewOutboxEvaluationHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	evaluation := sellChargeEvaluation(t, "SYN-EVAL-SELL-CONFIRM", "SELLREQ-SYN-CONFIRM", "SYN-PARCEL-01",
		sellChargeCard(t, "REMOTE_AREA_SURCHARGE"))
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		if _, err := finals.Save(txCtx, finalRecord); err != nil {
			return err
		}
		if _, err := evaluations.Save(txCtx, evaluation); err != nil {
			return err
		}
		return handoff.HandOffEvaluation(txCtx, ppports.EvaluationHandoffIntent{Evaluation: evaluation})
	})
	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("确认那一拍：%v", err)
	}
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var stage string
	var attributed *string
	if err := querier.QueryRow(t.Context(),
		`SELECT stage, to_char(attribution_date, 'YYYY-MM-DD') FROM settlement_accounting.customer_charge WHERE charge_id = 'SELLCC-SELLREQ-SYN-CONFIRM'`,
	).Scan(&stage, &attributed); err != nil {
		t.Fatal(err)
	}
	if stage != "CONFIRMED" || attributed == nil {
		t.Fatalf("费用 = %s / %v，want 已确认且形成归属日", stage, attributed)
	}
	// 刚确认的这一笔归属日还没关上：这一拍到点截单只截了前几天，都没有费用。
	if n := countRows(t, db, `SELECT count(*) FROM settlement_accounting.customer_statement`); n != 0 {
		t.Fatalf("归属日没关上就出了 %d 张单", n)
	}

	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		executor, err := db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		// 归属日清空、确认时刻拨到前天：等同于确认时对账安排还没登、判不出归属日。到点截单要先当场补判（按确认日落成前天），
		// 再把它收进前天那一期。
		if _, err := executor.Exec(txCtx, `UPDATE settlement_accounting.customer_charge
			   SET attribution_date = NULL, confirmed_at = confirmed_at - interval '2 days', formed_at = formed_at - interval '2 days'
			 WHERE charge_id = 'SELLCC-SELLREQ-SYN-CONFIRM'`); err != nil {
			return err
		}
		// 拨回之前那一拍已把前天、昨天记成无费用：撤掉这两行，好让这一笔重新落进前天那一期。
		_, err = executor.Exec(txCtx, `DELETE FROM settlement_accounting.statement_cycle_cut WHERE closing_date >= $1::date`, dayBeforeYesterday)
		return err
	})
	return beat, db, dayBeforeYesterday
}

// seedCycleCut 登到点截单要的几本：对账安排、运费项目按确认日归属、截单时点、日截周期。值在事务外造好。
func seedCycleCut(t *testing.T, db *bentopg.DB, tenant sadomain.TenantID, since string) {
	t.Helper()
	arrangements, err := sapostgres.NewStatementArrangements(db)
	if err != nil {
		t.Fatal(err)
	}
	attributions, err := sapostgres.NewChargeAttributions(db)
	if err != nil {
		t.Fatal(err)
	}
	moments, err := sapostgres.NewSettlementMoments(db)
	if err != nil {
		t.Fatal(err)
	}
	cycles, err := sapostgres.NewStatementCycles(db)
	if err != nil {
		t.Fatal(err)
	}
	_, arrangement, err := registrationjson.StatementArrangementFromJSON([]byte(
		`{"tenantId":"` + demoTenant + `","accountId":"SYN-SA-SHIPPER-01-RECEIVABLE","timeZone":"Asia/Shanghai","cutoffMinute":1080}`))
	if err != nil {
		t.Fatal(err)
	}
	_, form, err := registrationjson.ChargeAttributionFromJSON([]byte(
		`{"tenantId":"` + demoTenant + `","feeItem":"SYN-FEE-ITEM-FREIGHT","form":"CHARGE_CONFIRMED"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, cycle, err := registrationjson.StatementCycleFromJSON([]byte(
		`{"tenantId":"` + demoTenant + `","accountId":"SYN-SA-SHIPPER-01-RECEIVABLE","kind":"DAILY","since":"` + since + `"}`))
	if err != nil {
		t.Fatal(err)
	}
	at := beatInstant()
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		if _, err := arrangements.SaveStatementArrangement(txCtx, tenant, arrangement, at); err != nil {
			return err
		}
		if _, err := attributions.SaveChargeAttribution(txCtx, tenant, form, at); err != nil {
			return err
		}
		if _, err := moments.SaveSettlementMoment(txCtx, tenant, sadomain.SettlementMomentCutOff, at); err != nil {
			return err
		}
		_, err := cycles.SaveStatementCycle(txCtx, tenant, cycle, at)
		return err
	})
}

func cycleChargeAttribution(t *testing.T, db *bentopg.DB) string {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var date *string
	if err := querier.QueryRow(t.Context(), `SELECT to_char(attribution_date, 'YYYY-MM-DD') FROM settlement_accounting.customer_charge
		 WHERE charge_id = 'SELLCC-SELLREQ-SYN-CONFIRM'`).Scan(&date); err != nil {
		t.Fatal(err)
	}
	if date == nil {
		return ""
	}
	return *date
}
