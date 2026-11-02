package docnumber

import (
	"testing"
	"time"
)

func newReader(t *testing.T, book *memoryBook, secrets SecretTable) *Reader {
	t.Helper()
	reader, err := NewReader(ReaderDeps{Catalogue: testCatalogue(), Book: book, Secrets: secrets, Clock: fixedClock{at: utcEvening}})
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

// Covers: ADR-0355 决定六——读口列出每类单据此刻生效的规则（租户版或产品默认）、重置周期与历次版本；单据编号时区
// 未登时如实答未配置、不渲染带日期的样例，登了之后样例按它出日期。
func TestReadListsEveryTypeWithItsEffectiveRule(t *testing.T) {
	book := &memoryBook{}
	view, err := newReader(t, book, SecretTable{}).Read(t.Context(), testTenant)
	if err != nil {
		t.Fatal(err)
	}
	if view.NumberingTimeZone != nil || len(view.Types) != len(testDeclarations) {
		t.Fatalf("empty register: %+v", view)
	}
	for _, typeView := range view.Types {
		if typeView.Source != SourceProductDefault || typeView.Template != typeView.Declaration.DefaultTemplate || typeView.Sample != "" || typeView.EffectiveVersion != nil {
			t.Fatalf("%s should be the product default without a sample: %+v", typeView.Declaration.Code, typeView)
		}
	}

	book.zones = append(book.zones, TimeZoneVersion{Tenant: testTenant, TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN", RegisteredAt: utcEvening})
	book.rules = append(book.rules,
		RuleVersion{Tenant: testTenant, Type: "SHIPMENT", Template: "OLD{yyyy}{mm}{dd}{seq:6}", EffectiveFrom: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN", RegisteredAt: utcEvening},
		RuleVersion{Tenant: testTenant, Type: "SHIPMENT", Template: "SYN{yyyy}{mm}-{seq:4}", EffectiveFrom: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN", RegisteredAt: utcEvening},
		RuleVersion{Tenant: testTenant, Type: "SHIPMENT", Template: "NEXT{yyyy}{seq:6}", EffectiveFrom: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN", RegisteredAt: utcEvening},
		RuleVersion{Tenant: "SYN-TENANT-02", Type: "SHIPMENT", Template: "OTHER{seq:6}", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN", RegisteredAt: utcEvening},
	)
	view, err = newReader(t, book, SecretTable{}).Read(t.Context(), testTenant)
	if err != nil {
		t.Fatal(err)
	}
	if view.NumberingTimeZone == nil || view.NumberingTimeZone.TimeZone != "Asia/Shanghai" || len(view.NumberingTimeZoneVersions) != 1 {
		t.Fatalf("numbering time zone: %+v", view)
	}
	shipment := view.Types[0]
	if shipment.Source != SourceTenantRule || shipment.Template != "SYN{yyyy}{mm}-{seq:4}" || shipment.ResetPeriod != ResetMonthly {
		t.Fatalf("shipment effective rule: %+v", shipment)
	}
	if shipment.EffectiveVersion == nil || !shipment.EffectiveVersion.EffectiveFrom.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("effective version: %+v", shipment.EffectiveVersion)
	}
	if len(shipment.Versions) != 3 || shipment.Sample != "SYN202611-0001" {
		t.Fatalf("history or sample: %d %q", len(shipment.Versions), shipment.Sample)
	}
	if statement := view.Types[2]; statement.Source != SourceProductDefault || statement.Sample != "STMT-202611-00001" || statement.ResetPeriod != ResetMonthly {
		t.Fatalf("statement stays on the default: %+v", statement)
	}
}

// Covers: ADR-0355 决定六、八——预览按样例流水渲染、不读也不占序列（Reader 没有序列依赖），拒因与登记同一组；没有
// 时区答未配置；可给样例时刻与时区；新模板能渲染出与此前模板相同的号时提示可能重叠，逐字相同的模板不报。
func TestPreview(t *testing.T) {
	book := &memoryBook{}
	reader := newReader(t, book, SecretTable{})

	rejected := []struct {
		request PreviewRequest
		reason  error
	}{
		{PreviewRequest{Tenant: testTenant, Type: "CLAIM", Template: "C{seq}"}, ErrTypeNotDeclared},
		{PreviewRequest{Tenant: testTenant, Type: "SHIPMENT", Template: "P{yyyy}"}, ErrTemplateWithoutSequence},
		{PreviewRequest{Tenant: testTenant, Type: "SHIPMENT", Template: "P{yyyy}{seqx:6}"}, ErrScrambleSecretNotConfigured},
		{PreviewRequest{Tenant: testTenant, Type: "SHIPMENT", Template: "P{yyyy}{seq:6}", TimeZone: "Local"}, ErrTimeZoneNotRecognized},
	}
	for _, testCase := range rejected {
		preview, err := reader.Preview(t.Context(), testCase.request)
		if err != nil || preview.Outcome != PreviewRejected || preview.Rejection != testCase.reason {
			t.Errorf("%+v: %+v %v", testCase.request, preview, err)
		}
	}

	notConfigured, err := reader.Preview(t.Context(), PreviewRequest{Tenant: testTenant, Type: "SHIPMENT", Template: "P{yyyy}{mm}{dd}{seq:6}"})
	if err != nil || notConfigured.Outcome != PreviewTimeZoneNotConfigured || notConfigured.Sample != "" || notConfigured.ResetPeriod != ResetDaily {
		t.Fatalf("no time zone: %+v %v", notConfigured, err)
	}

	at := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	rendered, err := reader.Preview(t.Context(), PreviewRequest{Tenant: testTenant, Type: "SHIPMENT", Template: "SYN-{yyyy}{mm}{dd}-{seq:4}", TimeZone: "Asia/Shanghai", At: at})
	if err != nil || rendered.Outcome != PreviewRendered || rendered.Sample != "SYN-20261201-0001" || rendered.TimeZone != "Asia/Shanghai" || len(rendered.Overlaps) != 0 {
		t.Fatalf("rendered: %+v %v", rendered, err)
	}

	// 产品默认按日 `P{yyyy}{mm}{dd}{seq:6}`；改成按月的 `P{yyyy}{mm}{seq:8}`，月序列走到 1000001 时渲染出
	// `P20261201000001`，与默认模板 12 月 1 日的第一个号逐字相同。
	overlapping, err := reader.Preview(t.Context(), PreviewRequest{Tenant: testTenant, Type: "SHIPMENT", Template: "P{yyyy}{mm}{seq:8}", TimeZone: "Asia/Shanghai", At: at})
	if err != nil || len(overlapping.Overlaps) != 1 || overlapping.Overlaps[0].Source != SourceProductDefault {
		t.Fatalf("overlap with the default not hinted: %+v %v", overlapping, err)
	}

	same, err := reader.Preview(t.Context(), PreviewRequest{Tenant: testTenant, Type: "SHIPMENT", Template: "P{yyyy}{mm}{dd}{seq:6}", TimeZone: "Asia/Shanghai", At: at})
	if err != nil || len(same.Overlaps) != 0 {
		t.Fatalf("the identical template shares the sequence and cannot overlap: %+v %v", same, err)
	}
	// 同为按日、只改流水最少位数：两份模板共用一条序列，六位及以上的流水两边渲染一样，同号必同流水；不足六位时新模板
	// 更短。所以不报——这是最常见的一种改法，误报会让提示失去分量。
	narrower, err := reader.Preview(t.Context(), PreviewRequest{Tenant: testTenant, Type: "SHIPMENT", Template: "P{yyyy}{mm}{dd}{seq:4}", TimeZone: "Asia/Shanghai", At: at})
	if err != nil || len(narrower.Overlaps) != 0 {
		t.Fatalf("a narrower sequence on the same reset cannot overlap: %+v %v", narrower, err)
	}

	// 租户先前按日 `Q{yyyy}{mm}{dd}{seq:4}`，改按月 `Q{yyyy}{mm}{seq:6}`：月序列走到 20001 就渲染出 `Q202611020001`，
	// 与旧模板 11 月 2 日的第一个号相同——逐个流水去碰要渲染两万个才碰得到，按位比形状一眼就见。
	book.zones = append(book.zones, TimeZoneVersion{Tenant: testTenant, TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN", RegisteredAt: utcEvening})
	book.rules = append(book.rules, RuleVersion{Tenant: testTenant, Type: "SHIPMENT", Template: "Q{yyyy}{mm}{dd}{seq:4}", EffectiveFrom: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN", RegisteredAt: utcEvening})
	fromTenantZone, err := reader.Preview(t.Context(), PreviewRequest{Tenant: testTenant, Type: "SHIPMENT", Template: "Q{yyyy}{mm}{seq:6}"})
	if err != nil || fromTenantZone.TimeZone != "Asia/Shanghai" || fromTenantZone.Sample != "Q202611000001" {
		t.Fatalf("numbering time zone not used: %+v %v", fromTenantZone, err)
	}
	if len(fromTenantZone.Overlaps) != 1 || fromTenantZone.Overlaps[0].Source != SourceTenantRule || fromTenantZone.Overlaps[0].EffectiveFrom.IsZero() {
		t.Fatalf("overlap with the tenant's earlier version not hinted: %+v", fromTenantZone.Overlaps)
	}
}
