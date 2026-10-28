package main

import (
	"net/http"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

const (
	regulatoryAcceptancePath = "/transport-fulfillment-regulatory-disposition-acceptances"
	alternateJourneyPath     = "/transport-fulfillment-alternate-journey-starts"

	// 一份合成的监管运输协作事项抄录：两件对象、从节点 A 到节点 B 的退运。全为合成（S 级）。
	acceptanceBody = `{"item":"SYN-CC-ITEM-43-01","basis":"SYN-REG-RETURN-DECISION-43-01","movementAction":"RETURN_TO_BONDED_WAREHOUSE",` +
		`"itemObjects":["SYN-UNIT-43-1","SYN-UNIT-43-2"],"origin":"SYN-NODE-43-A","destination":"SYN-NODE-43-B",` +
		`"decision":"ACCEPTED","acceptedObjects":["SYN-UNIT-43-1","SYN-UNIT-43-2"],"movementAuthority":"SYN-MOVE-AUTH-43-01",` +
		`"decidedAt":"2026-10-27T08:00:00Z"}`
)

// Covers: `AT-TF-367`、`AT-TF-370` 生产装配半边（真库）——监管运输处置承接口经操作者渠道「运营决定」能力面走到真编排：首投落一份承接决定
// （201 `DISPOSITION_DECIDED`），同一事项由另一位操作者原样再投答已有决定（200 `EXISTING_DECISION`）、库里仍是一行、决定人仍是首投的
// 那一位、回执里没有操作者；改了决定内容再投答冲突（200 `DECISION_CONFLICT`）、不顶替；「承接已记录」回执意图与决定同笔入了 Outbox，
// 重放不多入一份；承接的授予与旅程启动的授予互不相通（只持有一种的令牌对另一口答 403）。
func TestTheRegulatoryDispositionAcceptanceFaceRecordsTheDecisionAndItsOperatorAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	decisions := twoOperatorDecisions(t, db, accessidentity.DecisionRegulatoryDispositionAcceptance)
	acceptor, err := buildRegulatoryDispositionAcceptance(db)
	if err != nil {
		t.Fatalf("装配承接：%v", err)
	}
	endpoint := tfhttp.NewAcceptRegulatoryDispositionEndpoint(decisions, acceptor)

	raw := decideTwice(t, endpoint, regulatoryAcceptancePath, acceptanceBody,
		[2]any{http.StatusCreated, "DISPOSITION_DECIDED"}, [2]any{http.StatusOK, "EXISTING_DECISION"})
	if strings.Contains(raw, "continuationReference") {
		t.Fatalf("回执意图已随决定入队，重放不该带续办引用：%s", raw)
	}
	if got := recordedOperator(t, pool, `SELECT decided_by FROM transport_fulfillment.disposition_acceptance
		 WHERE tenant_id = 'SYN-TENANT-01' AND item_ref = 'SYN-CC-ITEM-43-01'`); got != firstOperatorRecorded {
		t.Fatalf("承接决定人 = %q，want %q", got, firstOperatorRecorded)
	}
	var receipts int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'transport-fulfillment.regulatory-acceptance.recorded'`).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("承接回执意图 = %d（%v），want 恰一份（重放不多入）", receipts, err)
	}

	changed := strings.Replace(acceptanceBody, `"decision":"ACCEPTED","acceptedObjects":["SYN-UNIT-43-1","SYN-UNIT-43-2"]`,
		`"decision":"PARTIALLY_ACCEPTED","acceptedObjects":["SYN-UNIT-43-1"],"declineBasis":"SYN-CAPABILITY-LIMIT-43"`, 1)
	if status, answer := postWithToken(t, endpoint, regulatoryAcceptancePath, firstOperatorToken, changed); status != http.StatusOK || answer["outcome"] != "DECISION_CONFLICT" {
		t.Fatalf("改了内容再投 = %d %v，want 200 DECISION_CONFLICT", status, answer)
	}

	// 事项本体缺起止责任：答待补充、指名缺哪一项，不落决定、不入回执。
	incomplete := strings.NewReplacer("SYN-CC-ITEM-43-01", "SYN-CC-ITEM-43-02", `"origin":"SYN-NODE-43-A"`, `"origin":""`).Replace(acceptanceBody)
	status, answer := postWithToken(t, endpoint, regulatoryAcceptancePath, firstOperatorToken, incomplete)
	if status != http.StatusOK || answer["outcome"] != "SUPPLEMENT_REQUIRED" || answer["supplementReason"] != "RESPONSIBILITY_MISSING" {
		t.Fatalf("缺起点 = %d %v，want 200 SUPPLEMENT_REQUIRED RESPONSIBILITY_MISSING", status, answer)
	}
	var rows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.disposition_acceptance WHERE tenant_id = 'SYN-TENANT-01'`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("承接册 = %d 行（%v），want 1（待补充与冲突都不落）", rows, err)
	}

	// 授予按种类互不相通：只持有承接授予的操作者开不了旅程。
	journey := buildOrFail(t, db)
	journeyEndpoint := tfhttp.NewStartAlternateJourneyEndpoint(decisions, journey)
	if status, answer := postWithToken(t, journeyEndpoint, alternateJourneyPath, firstOperatorToken, journeyBody); status != http.StatusForbidden {
		t.Fatalf("只持承接授予开旅程 = %d %v，want 403", status, answer)
	}
}

const journeyBody = `{"journey":"SYN-JOURNEY-43-RETURN","purpose":"RETURN","original":"SYN-JOURNEY-43-ORIGINAL",` +
	`"basisKind":"REGULATORY_DISPOSITION","basis":"SYN-REG-RETURN-DECISION-43-01","members":["SYN-UNIT-43-1","SYN-UNIT-43-2"],` +
	`"startedAt":"2026-10-27T09:00:00Z"}`

func buildOrFail(t *testing.T, db *bentopg.DB) tfhttp.AlternateJourneyStarter {
	t.Helper()
	starter, err := buildAlternateJourneyStart(db)
	if err != nil {
		t.Fatalf("装配旅程启动：%v", err)
	}
	return starter
}

// Covers: `AT-TF-369`、`AT-TF-370` 生产装配半边（真库）——替代/退运旅程启动口经「运营决定」能力面走到真编排：首投落一条旅程
// （201 `JOURNEY_STARTED`），监管来路同笔入处置执行与异常链两份意图；另一位操作者原样再投答已有旅程（200 `EXISTING_JOURNEY`）、库里仍是
// 一行、启动人仍是首投的那一位、两条链都不多入一份；非监管来路只入异常链一份；同一处置换了旅程身份再投答来源冲突。
func TestTheAlternateJourneyStartFaceRecordsTheJourneyAndItsOperatorAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	decisions := twoOperatorDecisions(t, db, accessidentity.DecisionAlternateJourneyStart)
	endpoint := tfhttp.NewStartAlternateJourneyEndpoint(decisions, buildOrFail(t, db))

	decideTwice(t, endpoint, alternateJourneyPath, journeyBody,
		[2]any{http.StatusCreated, "JOURNEY_STARTED"}, [2]any{http.StatusOK, "EXISTING_JOURNEY"})
	if got := recordedOperator(t, pool, `SELECT started_by FROM transport_fulfillment.alternate_journey
		 WHERE tenant_id = 'SYN-TENANT-01' AND journey_id = 'SYN-JOURNEY-43-RETURN'`); got != firstOperatorRecorded {
		t.Fatalf("旅程启动人 = %q，want %q", got, firstOperatorRecorded)
	}
	countEvents := func(eventType string) int {
		var n int
		if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`, eventType).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := countEvents("transport-fulfillment.disposition-execution.recorded"); got != 1 {
		t.Fatalf("处置执行意图 = %d，want 1", got)
	}
	if got := countEvents("transport-fulfillment.exception-journey.recorded"); got != 1 {
		t.Fatalf("异常链意图 = %d，want 1", got)
	}

	conflicting := strings.Replace(journeyBody, "SYN-JOURNEY-43-RETURN", "SYN-JOURNEY-43-OTHER", 1)
	if status, answer := postWithToken(t, endpoint, alternateJourneyPath, firstOperatorToken, conflicting); status != http.StatusOK || answer["outcome"] != "SOURCE_CONFLICT" {
		t.Fatalf("同一处置换旅程身份 = %d %v，want 200 SOURCE_CONFLICT", status, answer)
	}

	// 非监管来路（普通退运）只入异常链，不混进监管范围的处置执行。
	service := strings.NewReplacer("SYN-JOURNEY-43-RETURN", "SYN-JOURNEY-43-SERVICE", "REGULATORY_DISPOSITION", "SERVICE_DISPOSITION", "SYN-REG-RETURN-DECISION-43-01", "SYN-PS-DISPOSITION-43-01").Replace(journeyBody)
	if status, answer := postWithToken(t, endpoint, alternateJourneyPath, firstOperatorToken, service); status != http.StatusCreated || answer["outcome"] != "JOURNEY_STARTED" {
		t.Fatalf("普通退运 = %d %v", status, answer)
	}
	if got := countEvents("transport-fulfillment.disposition-execution.recorded"); got != 1 {
		t.Fatalf("普通退运混进了处置执行链：%d", got)
	}
	if got := countEvents("transport-fulfillment.exception-journey.recorded"); got != 2 {
		t.Fatalf("异常链意图 = %d，want 2", got)
	}
}
