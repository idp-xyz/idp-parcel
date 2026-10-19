package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// Covers: 票 no-reception-source/03「复核目录：待识别行带来源文档里的外部标识观察（旧行如实写来源未保全）」的存储面——每一行
// 带自己的判断版本（人工重核按它指名）、来源文档在不在、在时那一格号码原样（两端空白照留）；已被接续的那一版带直接接续它的
// 判断版本，仍是当前有效版本的不带。
func TestUnidentifiedItemsCarryTheMarkAndTheirPlaceInTheChain(t *testing.T) {
	catalogue, receptions, _, transactor := newReviewCatalogue(t)
	ctx := t.Context()
	const tenant = "SYN-TENANT-NRS03"

	current := recheckablePending(t, tenant, "SYN-SCAN-NRS03-CURRENT", "SYN-INTAKEV-NRS03-CURRENT", " SYN-MARK-NRS03\t")
	sourceless := recheckablePending(t, tenant, "SYN-SCAN-NRS03-OLD", "SYN-INTAKEV-NRS03-OLD", "SYN-MARK-NRS03")
	sourceless.Source = domain.DeliverySource{}
	superseded := recheckablePending(t, tenant, "SYN-SCAN-NRS03-SUPERSEDED", "SYN-INTAKEV-NRS03-SUPERSEDED", "SYN-MARK-NRS03")
	successor := recheckablePending(t, tenant, "SYN-SCAN-NRS03-SUCCESSOR", "SYN-INTAKEV-NRS03-SUCCESSOR", "SYN-MARK-NRS03")
	successor.Predecessor = superseded.JudgmentVersion
	successor.IdentityConflict = true
	saveReceptions(t, transactor, ctx, receptions, current, sourceless, superseded, successor)

	rows, err := catalogue.ListUnidentifiedItems(ctx, ref(t, domain.NewTenantID, tenant), 10)
	if err != nil {
		t.Fatalf("上列待识别册：%v", err)
	}
	bySource := map[string]int{}
	for index, row := range rows {
		bySource[row.SourceID] = index
	}
	if len(rows) != 4 {
		t.Fatalf("待识别册 %d 行, want 4", len(rows))
	}
	got := rows[bySource["SYN-SCAN-NRS03-CURRENT"]]
	if got.JudgmentVersion != "SYN-INTAKEV-NRS03-CURRENT" || !got.SourcePreserved || got.Mark != " SYN-MARK-NRS03\t" || got.SucceededBy != "" {
		t.Errorf("当前有效的待识别 = %+v", got)
	}
	old := rows[bySource["SYN-SCAN-NRS03-OLD"]]
	if old.SourcePreserved || old.Mark != "" || old.JudgmentVersion != "SYN-INTAKEV-NRS03-OLD" {
		t.Errorf("来源未保全的旧行 = %+v", old)
	}
	if replaced := rows[bySource["SYN-SCAN-NRS03-SUPERSEDED"]]; replaced.SucceededBy != "SYN-INTAKEV-NRS03-SUCCESSOR" {
		t.Errorf("已被接续的那一版 = %+v, want 带接续它的判断版本", replaced)
	}
}
