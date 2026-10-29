package main

import (
	"fmt"
	"strings"
	"testing"
)

// 本文件对真实 PostgreSQL 16 经受控登记口证外部更正碰到已有核销的那一格（ADR-0285，隔离合成 S）：更正低于未撤销核销合计时
// 照采、答复说「累计超核 X」（与映射、核销答复同一句话），同一事务里把它落到的作用域按新金额重算；更正与核销在事实身份行上
// 排队，先后哪一种都落到一个自洽的终态。作用域是 SYN-ACCT-RECV-1（对账单 C、已确认应收 10000）。

// Covers: 票 code/92 评审手算例走受控登记口——收款 5000 映到对账单 C 并核满 5000，更正到 3000：答复说「累计超核 2000」，
// 入账余额同事务跟上（P 封顶 3000，减 W 5000，-2000）；重放同一次更正仍说超核 2000；再更正到 8000 高于合计，答复不说超核，
// 入账余额随金额重算（P 8000 减 W 5000，3000）。
func TestACorrectionBelowTheAppliedTotalSaysOverAppliedThroughTheCommand(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedReceivableScope(t)
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-COR", "5000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	rig.run(t, commandFundsMapping, claimMappingDocument("SYN-MAPPING-COR", "SYN-FACT-COR", "STATEMENT", scopeStatement),
		exitRegistered, "FUNDS_MAPPED")
	rig.run(t, commandFundsApplication,
		statementApplicationDocument("SYN-APPLICATION-COR", "SYN-FACT-COR", "SYN-MAPPING-COR", "5000"),
		exitRegistered, "SETTLEMENT_APPLIED")
	rig.assertPostedOf(t, scopeAccount, 0, 5000)

	corrected := rig.run(t, commandExternalFundsFactCorrection,
		correctionDocumentFor("SYN-FACT-COR", "SYN-FACT-COR/v1", "SYN-FACT-COR/v2", "3000"),
		exitRegistered, "FUNDS_FACT_ADOPTED")
	if !strings.Contains(corrected, "事实 SYN-FACT-COR 累计超核 2000") {
		t.Fatalf("更正答复 %q 要说累计超核 2000", corrected)
	}
	if strings.Contains(corrected, "累计未分配") {
		t.Fatalf("更正答复 %q 不该把超核说成累计未分配", corrected)
	}
	rig.assertPostedOf(t, scopeAccount, -2000, 5000)

	replay := rig.run(t, commandExternalFundsFactCorrection,
		correctionDocumentFor("SYN-FACT-COR", "SYN-FACT-COR/v1", "SYN-FACT-COR/v2", "3000"),
		exitRegistered, "EXISTING_FUNDS_FACT")
	if !strings.Contains(replay, "累计超核 2000") {
		t.Fatalf("重放更正的答复 %q 要仍说累计超核 2000", replay)
	}

	raised := rig.run(t, commandExternalFundsFactCorrection,
		correctionDocumentFor("SYN-FACT-COR", "SYN-FACT-COR/v2", "SYN-FACT-COR/v3", "8000"),
		exitRegistered, "FUNDS_FACT_ADOPTED")
	if strings.Contains(raised, "超核") {
		t.Fatalf("改到合计以上的更正答复 %q 不该说超核", raised)
	}
	rig.assertPostedOf(t, scopeAccount, 3000, 5000)
}

// appliedOf 读一条事实上未撤销核销的合计。
func (rig ceilingRig) appliedOf(t *testing.T, fact string) int64 {
	t.Helper()
	var total int64
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT COALESCE(SUM(applied_minor), 0) FROM settlement_accounting.settlement_application
		  WHERE tenant_id = $1 AND fact_id = $2 AND reversal_basis IS NULL`, claimTenant, fact).Scan(&total); err != nil {
		t.Fatalf("合计已核销：%v", err)
	}
	return total
}

// Covers: 票 code/92 (c)——更正与核销同一条事实并发：两笔都在事实身份行上排队，谁先进锁有两种终态，都得自洽。收款 5000 已核
// 3000，更正到 2000 与新核销 1000 同时到：更正先进，核销读到更正后的金额答分配失衡，已核销停在 3000、更正说超核 1000；核销
// 先进，已核销 4000，更正数进它说超核 2000。两种都满足：更正说的超核 = 终态的已核销合计 - 2000，入账余额 = 2000 - 已核销
// 合计（P 封顶 2000，W 是已核销），已确认未结 = 10000 - 已核销合计。不取事实锁的更正读不到先到的核销，这几个数就对不上。
func TestAConcurrentCorrectionAndApplicationOfOneFactEndInAConsistentState(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedReceivableScope(t)
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-CR", "5000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	rig.run(t, commandFundsMapping, claimMappingDocument("SYN-MAPPING-CR", "SYN-FACT-CR", "STATEMENT", scopeStatement),
		exitRegistered, "FUNDS_MAPPED")
	rig.run(t, commandFundsApplication,
		statementApplicationDocument("SYN-APPLICATION-CR-1", "SYN-FACT-CR", "SYN-MAPPING-CR", "3000"),
		exitRegistered, "SETTLEMENT_APPLIED")

	got := rig.raceUnder(t, lockFactRow("SYN-FACT-CR"), [2]raceCommand{
		{commandExternalFundsFactCorrection, correctionDocumentFor("SYN-FACT-CR", "SYN-FACT-CR/v1", "SYN-FACT-CR/v2", "2000")},
		{commandFundsApplication, statementApplicationDocument("SYN-APPLICATION-CR-2", "SYN-FACT-CR", "SYN-MAPPING-CR", "1000")},
	})
	correction, application := got[0], got[1]
	if correction.code != exitRegistered || !strings.Contains(correction.message, "FUNDS_FACT_ADOPTED") {
		t.Fatalf("并发的更正没被采用：%d %q", correction.code, correction.message)
	}

	applied := rig.appliedOf(t, "SYN-FACT-CR")
	switch {
	case application.code == exitRegistered && strings.Contains(application.message, "SETTLEMENT_APPLIED"):
		if applied != 4000 {
			t.Fatalf("核销落下了，已核销合计 = %d，要 4000", applied)
		}
	case application.code == exitUsage && strings.Contains(application.message, "APPLICATION_IMBALANCE"):
		if applied != 3000 {
			t.Fatalf("核销被拒了，已核销合计 = %d，要停在 3000", applied)
		}
	default:
		t.Fatalf("并发的核销答复意外：%d %q", application.code, application.message)
	}

	if want := fmt.Sprintf("事实 SYN-FACT-CR 累计超核 %d", applied-2000); !strings.Contains(correction.message, want) {
		t.Fatalf("更正答复 %q 要说 %q——超核该等于终态的已核销合计减事实金额", correction.message, want)
	}
	rig.assertPostedOf(t, scopeAccount, 2000-applied, 10000-applied)
}

// Covers: 票 code/92 评审 N1 经受控登记口——收款 5000 核满 5000 到对账单 C，再插一条确认后的价格纠错贷记 6000（已确认应收降到
// 4000 < 已核销 5000，作用域账已不平）；此时更正到 4500：更正照采、退出 0，答复点名这个作用域（法人、账户、币种）说入账余额
// 没有推进，同时说累计超核 500；运营余额留在原值，版本行落了两行。
func TestACorrectionIntoAnUnbalancedScopeIsAdoptedAndNamesTheScopeThroughTheCommand(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedReceivableScope(t)
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-UNB", "5000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	rig.run(t, commandFundsMapping, claimMappingDocument("SYN-MAPPING-UNB", "SYN-FACT-UNB", "STATEMENT", scopeStatement),
		exitRegistered, "FUNDS_MAPPED")
	rig.run(t, commandFundsApplication,
		statementApplicationDocument("SYN-APPLICATION-UNB", "SYN-FACT-UNB", "SYN-MAPPING-UNB", "5000"),
		exitRegistered, "SETTLEMENT_APPLIED")
	rig.assertPostedOf(t, scopeAccount, 0, 5000)
	if _, err := rig.pool.Exec(t.Context(),
		`INSERT INTO settlement_accounting.charge_adjustment
			(tenant_id, adjustment_id, charge_id, kind, direction,
			 evaluation_ref, authorization_ref, original_currency, original_minor,
			 settlement_currency, settlement_minor, conversion_ref, formed_at, recorded_at)
		 VALUES ('SYN-T1', 'SYN-ADJUSTMENT-UNB', 'SYN-CHARGE-C-1', 'PRICING_CORRECTION', 'CREDIT',
		         'SYN-EVAL-CORRECTION-UNB', NULL, 'CNY', 6000, 'CNY', 6000, NULL, now(), now())`); err != nil {
		t.Fatalf("登价格纠错贷记：%v", err)
	}

	corrected := rig.run(t, commandExternalFundsFactCorrection,
		correctionDocumentFor("SYN-FACT-UNB", "SYN-FACT-UNB/v1", "SYN-FACT-UNB/v2", "4500"),
		exitRegistered, "FUNDS_FACT_ADOPTED")
	if !strings.Contains(corrected, "作用域 法人 "+scopeLegalEntity+"、账户 "+scopeAccount+"、币种 "+scopeCurrency+" 账已不平，入账余额没有跟着新金额推进") {
		t.Fatalf("更正答复 %q 没点名账不平、没推进的作用域", corrected)
	}
	if !strings.Contains(corrected, "累计超核 500") {
		t.Fatalf("更正答复 %q 要同时说累计超核 500", corrected)
	}
	rig.assertPostedOf(t, scopeAccount, 0, 5000)
	if versions := countRows(t, rig.pool,
		`SELECT count(*) FROM settlement_accounting.external_funds_fact_version WHERE tenant_id = $1 AND fact_id = $2`,
		claimTenant, "SYN-FACT-UNB"); versions != 2 {
		t.Fatalf("更正之后版本行 = %d，要 2", versions)
	}
}
