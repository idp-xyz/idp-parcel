package postgres_test

import (
	"net/url"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// Covers: ETA 预测查阅——一条预测线一行、只列当前版、新近预测的在前、他租户读不到、q 按字面收窄；人工录入的当前版带出留痕
// （谁、原件、录入时刻），预测来源送来的不带；人录的旧版被新版取代后，留痕不跟到新版上。
func TestListingCurrentETAPredictionsTellsManualVersionsFromSourceOnes(t *testing.T) {
	fixture := newETAHandoffFixture(t)
	ctx := t.Context()
	base := time.Date(2026, 11, 13, 6, 0, 0, 0, time.UTC)
	version := func(tenant, parcel, source, versionID string, predictedAt time.Time, prior any, current bool) {
		t.Helper()
		if _, err := fixture.pool.Exec(ctx,
			`INSERT INTO visibility_exception.eta_prediction_version
			     (tenant_id, parcel_ref, milestone_ref, source, version_id, inputs_ref, model_ref,
			      range_from, range_to, confidence_ref, predicted_at, prior_version)
			 VALUES ($1, $2, 'DELIVERED', $3, $4, 'inputs-'||$4, 'model/v1', $5, $6, 'HIGH', $7, $8)`,
			tenant, parcel, source, versionID, predictedAt.Add(24*time.Hour), predictedAt.Add(30*time.Hour), predictedAt, prior); err != nil {
			t.Fatalf("落预测版本 %s：%v", versionID, err)
		}
		if !current {
			return
		}
		if _, err := fixture.pool.Exec(ctx,
			`INSERT INTO visibility_exception.eta_prediction_current (tenant_id, parcel_ref, milestone_ref, source, version_id)
			 VALUES ($1, $2, 'DELIVERED', $3, $4)`, tenant, parcel, source, versionID); err != nil {
			t.Fatalf("落当前版指针 %s：%v", versionID, err)
		}
	}
	entry := func(versionID string, enteredAt time.Time) {
		t.Helper()
		if _, err := fixture.pool.Exec(ctx,
			`INSERT INTO visibility_exception.eta_manual_entry (tenant_id, version_id, entered_by, original_ref, entered_at)
			 VALUES ('tenant-eta', $1, 'SYN-OP-6', '承运商到港通知 '||$1, $2)`, versionID, enteredAt); err != nil {
			t.Fatalf("落人工录入留痕 %s：%v", versionID, err)
		}
	}
	// PCL-MANUAL：当前版是人录的。
	version("tenant-eta", "PCL-MANUAL", "CARRIER_PROVIDED", "ETAV-M1", base.Add(3*time.Hour), nil, true)
	entry("ETAV-M1", base.Add(3*time.Hour))
	// PCL-SUPERSEDED：人录过首版，之后预测来源出了新版——当前版不带留痕。
	version("tenant-eta", "PCL-SUPERSEDED", "CARRIER_PROVIDED", "ETAV-S1", base.Add(time.Hour), nil, false)
	entry("ETAV-S1", base.Add(time.Hour))
	version("tenant-eta", "PCL-SUPERSEDED", "CARRIER_PROVIDED", "ETAV-S2", base.Add(2*time.Hour), "ETAV-S1", true)
	version("tenant-other", "PCL-OTHER", "CARRIER_PROVIDED", "ETAV-O1", base, nil, true)

	tenant, err := domain.NewTenantID("tenant-eta")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	rows, err := rowsOf(fixture.etas.ListCurrentETAPredictions(ctx, tenant, 10, firstPage(t, ports.ETAPredictionPages)))
	if err != nil {
		t.Fatalf("上列预测：%v", err)
	}
	if len(rows) != 2 || rows[0].Version != "ETAV-M1" || rows[1].Version != "ETAV-S2" {
		t.Fatalf("应只列本租户两条线的当前版、新近在前：%+v", rows)
	}
	manual := rows[0]
	if manual.Parcel != "PCL-MANUAL" || manual.Milestone != "DELIVERED" || manual.Source != "CARRIER_PROVIDED" ||
		manual.Inputs != "inputs-ETAV-M1" || manual.Model != "model/v1" || manual.Confidence != "HIGH" ||
		!manual.RangeFrom.Equal(base.Add(27*time.Hour)) || !manual.RangeTo.Equal(base.Add(33*time.Hour)) ||
		!manual.PredictedAt.Equal(base.Add(3*time.Hour)) || manual.PriorVersion != "" {
		t.Fatalf("当前版各格转写走样：%+v", manual)
	}
	if manual.ManualEntry == nil || manual.ManualEntry.EnteredBy != "SYN-OP-6" ||
		manual.ManualEntry.Original != "承运商到港通知 ETAV-M1" || !manual.ManualEntry.EnteredAt.Equal(base.Add(3*time.Hour)) {
		t.Fatalf("人工录入的当前版没带出留痕：%+v", manual.ManualEntry)
	}
	if rows[1].ManualEntry != nil || rows[1].PriorVersion != "ETAV-S1" {
		t.Fatalf("新版不是人录的，不得带旧版的留痕；前版应透出：%+v", rows[1])
	}

	keyword, err := ports.ETAPredictionPages.Decode(url.Values{"q": {"pcl-superseded"}})
	if err != nil {
		t.Fatalf("解 q：%v", err)
	}
	if hit, err := rowsOf(fixture.etas.ListCurrentETAPredictions(ctx, tenant, 10, keyword)); err != nil || len(hit) != 1 || hit[0].Version != "ETAV-S2" {
		t.Fatalf("q 收窄走样：%v %+v", err, hit)
	}
	if _, err := fixture.etas.ListCurrentETAPredictions(ctx, tenant, 0, firstPage(t, ports.ETAPredictionPages)); err == nil {
		t.Fatal("页大小 0 没被拒")
	}
}
