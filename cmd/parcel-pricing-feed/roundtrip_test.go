package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 走一遍受控喂价口的整条链（票 06 验证项「真库往返」）：受控目录一份
// SYN 文件 → FileConnector 抓取（摘要/时刻/定位符）→ 转录整版重述 → 登记 → 免复核声明为「是」时
// 复核记录由连接器身份写、「未声明」时不写；重跑同一文件答重放且不再写复核；第二天的文件延展成
// 新版本并闭合前一版开放末期；文件缺失/解不开 → 不登、恰一条可观察记录。全程经 execute 与真事务，
// 装配经 assemble——测的是随产品发出的那条接线。夹具全部 SYN（S 级）。

var roundTripMoment = time.Date(2026, 9, 4, 9, 15, 0, 0, time.UTC)

type steppingClock struct{ now time.Time }

func (clock *steppingClock) Now() time.Time {
	clock.now = clock.now.Add(time.Minute)
	return clock.now
}

func bindingDocumentFor(seriesID, locator string, exemption domain.ReviewExemption) []byte {
	return []byte(fmt.Sprintf(`{"tenant":"tenant-1","seriesId":"%s","bindingVersion":"b1","connectorKind":"FILE",
		"sourceIdentifier":"SYN-SOURCE/%s","sourceLocator":"%s","seriesKind":"EXCHANGE_RATE",
		"quoteBasis":{"policyId":"SYN-PRC-FX-POLICY","policyVersion":"v1","digest":"sha256:syn-fx-policy"},
		"registrant":"SYN-PRC-SERIES-REGISTRAR","reviewExemption":"%s"}`, seriesID, strings.ToLower(seriesID), locator, exemption))
}

func publication(seriesID, publishedOn, value string) string {
	return fmt.Sprintf(`{"seriesId":"%s","publishedOn":"%s","effectiveFrom":"%sT00:00:00Z","value":"%s"}`, seriesID, publishedOn, publishedOn, value)
}

func writeControlled(t *testing.T, root, relative, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("建受控目录：%v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("写来源文件：%v", err)
	}
}

func TestFeedRoundTripsThroughPostgres(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()
	root := t.TempDir()
	var observations []ports.SourceFeedObservation
	wired, err := assemble(db, root, &steppingClock{now: roundTripMoment}, func(observation ports.SourceFeedObservation) {
		observations = append(observations, observation)
	})
	if err != nil {
		t.Fatalf("装配：%v", err)
	}
	transactor := db.Transactor()
	run := func(input executeInput) (string, int) {
		return execute(ctx, input, wired.feed, wired.bindings, transactor)
	}
	feed := func(seriesID string) (string, int) {
		return run(executeInput{kind: kindFeed, tenant: "tenant-1", seriesID: seriesID})
	}
	countRows := func(table, seriesID string) int {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM parcel_pricing.`+table+` WHERE tenant_id = 'tenant-1' AND series_id = $1`, seriesID).Scan(&count); err != nil {
			t.Fatalf("数 %s：%v", table, err)
		}
		return count
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("构造租户：%v", err)
	}
	reviews, err := adapter.NewReferenceSeriesReviews(db)
	if err != nil {
		t.Fatalf("构造复核册：%v", err)
	}
	versions, err := adapter.NewReferenceSeriesVersions(db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}

	// 出厂零绑定：不登绑定就喂，答先去登绑定。
	if message, code := feed("SYN-PRC-USD-CNY"); code != exitGovernance || !strings.Contains(message, "BINDING_UNKNOWN") {
		t.Fatalf("无绑定喂价：%q %d", message, code)
	}

	// 免复核声明为「是」：登记 + 连接器身份代写复核，版本自复核时刻起在用。
	if message, code := run(executeInput{kind: kindBinding, raw: bindingDocumentFor("SYN-PRC-USD-CNY", "rates/usd-cny/latest.json", domain.ReviewExemptionGranted)}); code != exitRegistered {
		t.Fatalf("登绑定：%q %d", message, code)
	}
	writeControlled(t, root, "rates/usd-cny/latest.json", publication("SYN-PRC-USD-CNY", "2026-09-04", "7.1234"))
	message, code := feed("SYN-PRC-USD-CNY")
	if code != exitRegistered || !strings.Contains(message, "REGISTERED SYN-PRC-USD-CNY@2026-09-04") || !strings.Contains(message, "review=RECORDED") {
		t.Fatalf("首次喂价：%q %d", message, code)
	}
	var grade, reviewer, decision, basis string
	if err := pool.QueryRow(ctx,
		`SELECT v.evidence_grade, r.reviewer, r.decision, r.basis
		   FROM parcel_pricing.reference_series_version v
		   JOIN parcel_pricing.reference_series_review r
		     ON r.tenant_id = v.tenant_id AND r.series_id = v.series_id AND r.series_version = v.series_version
		  WHERE v.tenant_id = 'tenant-1' AND v.series_id = 'SYN-PRC-USD-CNY' AND v.series_version = '2026-09-04'`,
	).Scan(&grade, &reviewer, &decision, &basis); err != nil {
		t.Fatalf("读回登记与复核：%v", err)
	}
	if grade != "VERIFIABLE" || reviewer != "connector:FILE" || decision != "APPROVED" || basis != "免人工复核声明 ← 来源连接器绑定 SYN-PRC-USD-CNY@b1" {
		t.Fatalf("登记/复核走样：grade=%s reviewer=%s decision=%s basis=%q", grade, reviewer, decision, basis)
	}
	inForce, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesExchangeRate, "SYN-PRC-USD-CNY", roundTripMoment.Add(time.Hour))
	if err != nil || outcome != ports.SeriesVersionInForce || inForce.Version() != "2026-09-04" {
		t.Fatalf("免复核登记的版本没进在用：outcome=%d ref=%v err=%v", outcome, inForce, err)
	}
	reading, found, err := versions.ResolveAt(ctx, tenant, inForce, time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC))
	if err != nil || !found || reading.Value().Value().String() != "7.1234" || !reading.Verifiable() {
		t.Fatalf("在用版本解析取值失败：found=%v err=%v", found, err)
	}
	if evidence, _ := reading.Evidence(); !strings.HasPrefix(evidence, "sha256:") || !strings.Contains(evidence, "← file:rates/usd-cny/latest.json") {
		t.Fatalf("凭证不是工件引用：%q", evidence)
	}

	// 重跑同一文件：幂等重放，不再写复核。
	if message, code := feed("SYN-PRC-USD-CNY"); code != exitRegistered || !strings.Contains(message, "REPLAYED") || strings.Contains(message, "review=") {
		t.Fatalf("重跑：%q %d", message, code)
	}
	if countRows("reference_series_review", "SYN-PRC-USD-CNY") != 1 || countRows("reference_series_version", "SYN-PRC-USD-CNY") != 1 {
		t.Fatal("重跑写了第二条复核或第二版")
	}

	// 第二天的公布：延展成新版本，前一版开放末期在新起点闭合，新版本复核后在用。
	writeControlled(t, root, "rates/usd-cny/latest.json", publication("SYN-PRC-USD-CNY", "2026-09-05", "7.13"))
	if message, code := feed("SYN-PRC-USD-CNY"); code != exitRegistered || !strings.Contains(message, "REGISTERED SYN-PRC-USD-CNY@2026-09-05") {
		t.Fatalf("第二天喂价：%q %d", message, code)
	}
	latest, found, err := versions.LoadLatestVersion(ctx, tenant, "SYN-PRC-USD-CNY")
	if err != nil || !found || latest.Reference().Version() != "2026-09-05" || len(latest.Periods()) != 2 {
		t.Fatalf("延展版走样：found=%v version=%s periods=%d err=%v", found, latest.Reference().Version(), len(latest.Periods()), err)
	}
	if endsAt, bounded := latest.Periods()[0].EndsAt(); !bounded || !endsAt.Equal(time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("前一版开放末期没在新起点闭合")
	}
	if countRows("reference_series_review", "SYN-PRC-USD-CNY") != 2 {
		t.Fatal("延展版没代写复核")
	}

	// 「未声明」：登记但不代写复核，版本不进在用。
	if _, code := run(executeInput{kind: kindBinding, raw: bindingDocumentFor("SYN-PRC-EUR-CNY", "rates/eur-cny/latest.json", domain.ReviewExemptionUndeclared)}); code != exitRegistered {
		t.Fatalf("登第二条绑定 code=%d", code)
	}
	writeControlled(t, root, "rates/eur-cny/latest.json", publication("SYN-PRC-EUR-CNY", "2026-09-04", "8.3"))
	if message, code := feed("SYN-PRC-EUR-CNY"); code != exitRegistered || !strings.Contains(message, "REGISTERED") || strings.Contains(message, "review=") {
		t.Fatalf("未声明免复核的喂价：%q %d", message, code)
	}
	if countRows("reference_series_review", "SYN-PRC-EUR-CNY") != 0 {
		t.Fatal("未声明免复核却写了复核")
	}
	if _, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesExchangeRate, "SYN-PRC-EUR-CNY", roundTripMoment.Add(time.Hour)); err != nil || outcome != ports.SeriesHasNoApprovedVersion {
		t.Fatalf("未复核的版本进了在用：outcome=%d err=%v", outcome, err)
	}
	if len(observations) != 0 {
		t.Fatalf("绿路径出了可观察记录：%+v", observations)
	}

	// 文件缺失：不登，恰一条可观察记录停在 FETCH 站。
	if _, code := run(executeInput{kind: kindBinding, raw: bindingDocumentFor("SYN-PRC-JPY-CNY", "rates/jpy-cny/latest.json", domain.ReviewExemptionGranted)}); code != exitRegistered {
		t.Fatalf("登第三条绑定 code=%d", code)
	}
	if message, code := feed("SYN-PRC-JPY-CNY"); code != exitSourceFailure || !strings.Contains(message, "FETCH_FAILED") {
		t.Fatalf("文件缺失：%q %d", message, code)
	}
	if countRows("reference_series_version", "SYN-PRC-JPY-CNY") != 0 {
		t.Fatal("文件缺失却登了版本")
	}
	if len(observations) != 1 || observations[0].Stage != ports.SourceFeedStageFetch || observations[0].SeriesID != "SYN-PRC-JPY-CNY" || observations[0].Err == nil {
		t.Fatalf("可观察记录走样：%+v", observations)
	}

	// 解不开：同样不登、再一条可观察记录。
	writeControlled(t, root, "rates/jpy-cny/latest.json", "not a publication")
	if message, code := feed("SYN-PRC-JPY-CNY"); code != exitSourceFailure || !strings.Contains(message, "FETCH_FAILED") {
		t.Fatalf("解不开：%q %d", message, code)
	}
	if len(observations) != 2 || countRows("reference_series_version", "SYN-PRC-JPY-CNY") != 0 {
		t.Fatalf("解不开：observations=%d versions=%d", len(observations), countRows("reference_series_version", "SYN-PRC-JPY-CNY"))
	}

	// 顺序倒置：再放回一份更早的公布，转录拒、不登、再一条可观察记录停在 TRANSCRIBE 站。
	writeControlled(t, root, "rates/usd-cny/latest.json", publication("SYN-PRC-USD-CNY", "2026-09-01", "7.0"))
	if message, code := feed("SYN-PRC-USD-CNY"); code != exitSourceFailure || !strings.Contains(message, "TRANSCRIPTION_REFUSED") {
		t.Fatalf("顺序倒置：%q %d", message, code)
	}
	if len(observations) != 3 || observations[2].Stage != ports.SourceFeedStageTranscribe || countRows("reference_series_version", "SYN-PRC-USD-CNY") != 2 {
		t.Fatalf("顺序倒置：observations=%+v", observations)
	}
}
