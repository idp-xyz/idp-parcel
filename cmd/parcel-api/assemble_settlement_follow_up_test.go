package main

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
)

// 本文件对真实 PostgreSQL 证对账单发布之后四个结算运营决定口的生产装配（UC-SA-003 步 6–7）：按特性登记处那一份 build 与
// mount 装上，进的是 buildSettlementOrchestrations 装的调整编排与截单编排。对账单经确认口与发布口真发一张。证据只记 `S`。

const (
	followUpAdjuster = "SYN-FOLLOWUP-ADJUSTER"
	followUpVoider   = "SYN-FOLLOWUP-VOIDER"
	followUpIncluder = "SYN-FOLLOWUP-INCLUDER"

	chargeAdjustmentFace    = "/settlement-charge-adjustments"
	statementVoidFace       = "/settlement-statement-voids"
	lateChargeInclusionFace = "/settlement-late-charge-inclusions"
	adjustmentInclusionFace = "/settlement-adjustment-inclusions"
)

func newFollowUpFixture(t *testing.T) *settlementDecisionsFixture {
	t.Helper()
	fixture, _ := newPublishedStatementFixture(t)
	operators, err := aipg.NewOperatorRegistry(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	registerOperatorIn(t, fixture.db, operators, decisionsTenant, followUpAdjuster, interval, accessidentity.DecisionChargeAdjustmentRegistration)
	registerOperatorIn(t, fixture.db, operators, decisionsTenant, followUpVoider, interval, accessidentity.DecisionStatementVoid)
	registerOperatorIn(t, fixture.db, operators, decisionsTenant, followUpIncluder, interval,
		accessidentity.DecisionLateChargeInclusion, accessidentity.DecisionAdjustmentInclusion)
	for _, pattern := range []string{chargeAdjustmentFace, statementVoidFace, lateChargeInclusionFace, adjustmentInclusionFace} {
		if fixture.endpoints[pattern] == nil {
			t.Fatalf("%s 没按登记处挂上", pattern)
		}
	}
	return fixture
}

func concessionBody(id, minor string) string {
	return `{"adjustment":"` + id + `","charge":"` + decisionsCharge + `","kind":"COMMERCIAL_CONCESSION","direction":"CREDIT",` +
		`"authorization":"SYN-AUTH-CONCESSION-1","originalCurrency":"CNY","originalMinor":"` + minor + `","settlementCurrency":"CNY","settlementMinor":"` + minor + `"}`
}

// Covers: 调整登记口——持调整授予的操作者对已确认费用登记一笔商业让利：答 201 并交回调整，形成时刻由服务端给；同一标识
// 重投答已登记并交回原来那一笔；纠错不带新评价、让利不带授权、费用不在册答未受理；载荷多一个键、金额不是整数串答 400；
// 持别的授予的人答未授予。
func TestTheChargeAdjustmentFaceRecordsAnAdjustmentOnceUnderItsOwnGrant(t *testing.T) {
	fixture := newFollowUpFixture(t)

	status, answer := fixture.post(t, chargeAdjustmentFace, followUpAdjuster, concessionBody("SYN-ADJ-1", "600"))
	if status != http.StatusCreated || answer["outcome"] != "ADJUSTMENT_RECORDED" || answer["settlementMinor"] != "600" ||
		answer["authorization"] != "SYN-AUTH-CONCESSION-1" || answer["formedAt"] == nil {
		t.Fatalf("登记让利 = %d %v", status, answer)
	}
	if status, answer = fixture.post(t, chargeAdjustmentFace, followUpAdjuster, concessionBody("SYN-ADJ-1", "999")); status != http.StatusOK ||
		answer["outcome"] != "ALREADY_RECORDED" || answer["settlementMinor"] != "600" {
		t.Fatalf("同标识重投 = %d %v，想要交回原来那一笔", status, answer)
	}
	for name, body := range map[string]string{
		"纠错不带新评价": `{"adjustment":"SYN-ADJ-2","charge":"` + decisionsCharge + `","kind":"PRICING_CORRECTION","direction":"DEBIT","originalCurrency":"CNY","originalMinor":"100","settlementCurrency":"CNY","settlementMinor":"100"}`,
		"让利不带授权":  `{"adjustment":"SYN-ADJ-3","charge":"` + decisionsCharge + `","kind":"COMMERCIAL_CONCESSION","direction":"CREDIT","originalCurrency":"CNY","originalMinor":"100","settlementCurrency":"CNY","settlementMinor":"100"}`,
		"费用不在册":   `{"adjustment":"SYN-ADJ-4","charge":"SYN-CHARGE-NOWHERE","kind":"COMMERCIAL_CONCESSION","direction":"CREDIT","authorization":"SYN-AUTH-1","originalCurrency":"CNY","originalMinor":"100","settlementCurrency":"CNY","settlementMinor":"100"}`,
	} {
		if status, answer = fixture.post(t, chargeAdjustmentFace, followUpAdjuster, body); status != http.StatusOK || answer["outcome"] != "SOURCE_NOT_ACCEPTED" {
			t.Fatalf("%s = %d %v", name, status, answer)
		}
	}
	for name, body := range map[string]string{
		"多一个键":    `{"adjustment":"SYN-ADJ-5","charge":"` + decisionsCharge + `","kind":"COMMERCIAL_CONCESSION","direction":"CREDIT","authorization":"SYN-AUTH-1","originalCurrency":"CNY","originalMinor":"100","settlementCurrency":"CNY","settlementMinor":"100","formedAt":"2020-01-01T00:00:00Z"}`,
		"金额不是整数串": `{"adjustment":"SYN-ADJ-5","charge":"` + decisionsCharge + `","kind":"COMMERCIAL_CONCESSION","direction":"CREDIT","authorization":"SYN-AUTH-1","originalCurrency":"CNY","originalMinor":"1.5","settlementCurrency":"CNY","settlementMinor":"100"}`,
		"种类不在词表":  `{"adjustment":"SYN-ADJ-5","charge":"` + decisionsCharge + `","kind":"GOODWILL","direction":"CREDIT","originalCurrency":"CNY","originalMinor":"100","settlementCurrency":"CNY","settlementMinor":"100"}`,
	} {
		if status, answer = fixture.post(t, chargeAdjustmentFace, followUpAdjuster, body); status != http.StatusBadRequest {
			t.Fatalf("%s = %d %v，想要 400", name, status, answer)
		}
	}
	if status, answer = fixture.post(t, chargeAdjustmentFace, followUpVoider, concessionBody("SYN-ADJ-6", "100")); status != http.StatusForbidden {
		t.Fatalf("持作废授予的人登记调整 = %d %v，想要 403", status, answer)
	}
	if count := fixture.text(t, `SELECT count(*)::text FROM settlement_accounting.charge_adjustment WHERE tenant_id = $1`, decisionsTenant); count != "1" {
		t.Fatalf("调整册 %s 行，想要 1", count)
	}
}

// Covers: 两个纳入口——调整纳入后续账期答 201，纳入时刻由服务端给；同一纳入标识重投答已存在（时刻取册里那一份，不因
// 时刻不同读成冲突），换了账期答冲突；换一个纳入标识再纳同一笔调整答已纳入过，册里仍只一行；纳回原账期答回填；调整不
// 在册答未受理。后到费用口：已在原账单上的费用不是后到费用，答未受理。
func TestTheInclusionFacesIncludeASubjectOnceAndRecogniseReplays(t *testing.T) {
	fixture := newFollowUpFixture(t)
	if status, answer := fixture.post(t, chargeAdjustmentFace, followUpAdjuster, concessionBody("SYN-ADJ-1", "600")); status != http.StatusCreated {
		t.Fatalf("先登记调整 = %d %v", status, answer)
	}
	include := func(inclusion, adjustment, period string) (int, map[string]any) {
		return fixture.post(t, adjustmentInclusionFace, followUpIncluder,
			`{"inclusion":"`+inclusion+`","statement":"`+fixture.published+`","adjustment":"`+adjustment+`","subsequentPeriod":"`+period+`"}`)
	}

	if status, answer := include("SYN-INC-0", "SYN-ADJ-1", "SYN-PERIOD-2026-11"); status != http.StatusOK || answer["outcome"] != "INCLUSION_BACKFILLS_PERIOD" {
		t.Fatalf("纳回原账期 = %d %v", status, answer)
	}
	status, answer := include("SYN-INC-1", "SYN-ADJ-1", "SYN-PERIOD-2026-12")
	if status != http.StatusCreated || answer["outcome"] != "ADJUSTMENT_INCLUDED" || answer["adjustment"] != "SYN-ADJ-1" ||
		answer["originalPeriod"] != "SYN-PERIOD-2026-11" || answer["subsequentPeriod"] != "SYN-PERIOD-2026-12" || answer["includedAt"] == nil {
		t.Fatalf("纳入调整 = %d %v", status, answer)
	}
	includedAt := answer["includedAt"]
	time.Sleep(5 * time.Millisecond)
	if status, answer = include("SYN-INC-1", "SYN-ADJ-1", "SYN-PERIOD-2026-12"); status != http.StatusOK || answer["outcome"] != "EXISTING_INCLUSION" || answer["includedAt"] != includedAt {
		t.Fatalf("同标识重投 = %d %v，想要已存在且时刻不变", status, answer)
	}
	if status, answer = include("SYN-INC-1", "SYN-ADJ-1", "SYN-PERIOD-2027-01"); status != http.StatusOK || answer["outcome"] != "INCLUSION_CONFLICT" {
		t.Fatalf("同标识换账期 = %d %v", status, answer)
	}
	if status, answer = include("SYN-INC-2", "SYN-ADJ-1", "SYN-PERIOD-2027-01"); status != http.StatusOK || answer["outcome"] != "SUBJECT_ALREADY_INCLUDED" {
		t.Fatalf("换标识再纳同一笔调整 = %d %v", status, answer)
	}
	if status, answer = include("SYN-INC-3", "SYN-ADJ-NOWHERE", "SYN-PERIOD-2026-12"); status != http.StatusOK || answer["outcome"] != "SOURCE_NOT_ACCEPTED" {
		t.Fatalf("调整不在册 = %d %v", status, answer)
	}
	if count := fixture.text(t, `SELECT count(*)::text FROM settlement_accounting.subsequent_inclusion WHERE tenant_id = $1`, decisionsTenant); count != "1" {
		t.Fatalf("纳入册 %s 行，想要 1", count)
	}

	late := `{"inclusion":"SYN-INC-LATE-1","statement":"` + fixture.published + `","charge":"` + decisionsCharge + `","subsequentPeriod":"SYN-PERIOD-2026-12"}`
	if status, answer = fixture.post(t, lateChargeInclusionFace, followUpIncluder, late); status != http.StatusOK || answer["outcome"] != "SOURCE_NOT_ACCEPTED" {
		t.Fatalf("把原账单上的费用当后到费用纳入 = %d %v", status, answer)
	}
	if status, answer = fixture.post(t, lateChargeInclusionFace, followUpAdjuster, late); status != http.StatusForbidden {
		t.Fatalf("持调整授予的人纳入 = %d %v，想要 403", status, answer)
	}
}

// Covers: 作废口——依据必备；两个人同时作废同一张单，恰有一人答已作废成立（201）、另一人答已作废过（200），都交回同一份
// 作废依据与时刻；单号不在册答未受理；作废时刻由服务端给。
func TestTheVoidFaceVoidsAStatementOnceEvenWhenTwoOperatorsRace(t *testing.T) {
	fixture := newFollowUpFixture(t)
	if status, answer := fixture.post(t, statementVoidFace, followUpVoider, `{"statement":"`+fixture.published+`","basis":""}`); status != http.StatusOK || answer["outcome"] != "SOURCE_NOT_ACCEPTED" {
		t.Fatalf("没有依据 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, statementVoidFace, followUpVoider, `{"statement":"STMT-209912-00001","basis":"SYN-VOID-BASIS-1"}`); status != http.StatusOK || answer["outcome"] != "SOURCE_NOT_ACCEPTED" {
		t.Fatalf("单号不在册 = %d %v", status, answer)
	}

	type reply struct {
		status int
		answer map[string]any
	}
	replies := make([]reply, 2)
	var group sync.WaitGroup
	start := make(chan struct{})
	for index := range replies {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			status, answer := fixture.post(t, statementVoidFace, followUpVoider, `{"statement":"`+fixture.published+`","basis":"SYN-VOID-BASIS-1"}`)
			replies[index] = reply{status, answer}
		}()
	}
	close(start)
	group.Wait()
	outcomes := map[string]int{}
	for _, got := range replies {
		outcomes[got.answer["outcome"].(string)]++
		if got.answer["voidBasis"] != "SYN-VOID-BASIS-1" || got.answer["voidedAt"] == nil || got.answer["voidedAt"] != replies[0].answer["voidedAt"] {
			t.Fatalf("作废答复 = %d %v", got.status, got.answer)
		}
	}
	if outcomes["STATEMENT_VOIDED"] != 1 || outcomes["ALREADY_VOIDED"] != 1 {
		t.Fatalf("两人同时作废 = %v，想要一人作废成立、一人已作废过", outcomes)
	}
	if status, answer := fixture.post(t, statementVoidFace, followUpVoider, `{"statement":"`+fixture.published+`","basis":"SYN-VOID-BASIS-2"}`); status != http.StatusOK ||
		answer["outcome"] != "ALREADY_VOIDED" || answer["voidBasis"] != "SYN-VOID-BASIS-1" {
		t.Fatalf("再作废 = %d %v，想要交回原作废", status, answer)
	}
}
