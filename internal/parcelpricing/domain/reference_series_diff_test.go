package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证「逐期差异」纯函数（票 pricing-reference-series-operations/08 件②的领域半边）：
// 每一版都整版重述该序列自起点以来的全部期次（CONTEXT），所以两版之间的差异按期次起点
// 对齐——同起点比止点、取值与凭证，只在一侧出现的期次是新增或移除。它不裁哪一版在用、
// 不算摘要，只把两张期次表并排说清楚，供登记前预览呈现。

func diffFixtureBase(t *testing.T) domain.ReferenceSeriesRegistration {
	t.Helper()
	spec := fuelSeriesSpec(t)
	spec.Periods = []domain.SeriesPeriodValue{
		seriesPeriod(t, seriesWeekOne, seriesWeekTwo, "0.22", "SYN-EVIDENCE/fuel-2026-W32"),
		seriesPeriod(t, seriesWeekTwo, time.Time{}, "0.24", "SYN-EVIDENCE/fuel-2026-W33"),
	}
	registration, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		t.Fatalf("构造基准版本：%v", err)
	}
	return registration
}

func changesByStart(t *testing.T, changes []domain.SeriesPeriodChange) map[time.Time]domain.SeriesPeriodChange {
	t.Helper()
	indexed := make(map[time.Time]domain.SeriesPeriodChange, len(changes))
	for _, change := range changes {
		if _, dup := indexed[change.StartsAt()]; dup {
			t.Fatalf("同一起点出现两次：%v", change.StartsAt())
		}
		indexed[change.StartsAt()] = change
	}
	return indexed
}

// TestDiffSeriesPeriodsReportsAnIdenticalRestatementAsUnchanged 证同表两版逐期全为未变，
// 且结果按起点升序、一期一条。
func TestDiffSeriesPeriodsReportsAnIdenticalRestatementAsUnchanged(t *testing.T) {
	base := diffFixtureBase(t)
	spec := fuelSeriesSpec(t)
	spec.Reference = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v2")
	spec.Periods = base.Periods()
	proposed, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		t.Fatalf("构造重述版本：%v", err)
	}

	changes, err := domain.DiffSeriesPeriods(base, proposed)
	if err != nil {
		t.Fatalf("同表两版比对报错：%v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("期次数 = %d，想要 2", len(changes))
	}
	if !changes[0].StartsAt().Equal(seriesWeekOne) || !changes[1].StartsAt().Equal(seriesWeekTwo) {
		t.Fatalf("结果没按起点升序：%v / %v", changes[0].StartsAt(), changes[1].StartsAt())
	}
	for _, change := range changes {
		if change.Kind() != domain.SeriesPeriodUnchanged {
			t.Fatalf("%v 的种类 = %s，想要 UNCHANGED", change.StartsAt(), change.Kind())
		}
		if change.ValueChanged() || change.EndChanged() || change.EvidenceChanged() {
			t.Fatalf("%v 未变却报了细项变化：%+v", change.StartsAt(), change)
		}
		if _, ok := change.Base(); !ok {
			t.Fatalf("%v 未变却缺基准期次", change.StartsAt())
		}
		if _, ok := change.Proposed(); !ok {
			t.Fatalf("%v 未变却缺拟登期次", change.StartsAt())
		}
	}
}

// TestDiffSeriesPeriodsTellsWhichFieldOfAPeriodMoved 证更正版本只改一期取值时，恰好那一期
// 报 CHANGED 且细项只有取值；换凭证与改止点各自单独可见——续办动作不同（抄错的数 vs.
// 补凭证 vs. 期界挪动），合成一个布尔会让人分不出改的是什么。
func TestDiffSeriesPeriodsTellsWhichFieldOfAPeriodMoved(t *testing.T) {
	base := diffFixtureBase(t)
	spec := fuelSeriesSpec(t)
	spec.Reference = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v2")
	spec.PriorVersion = base.Reference()
	spec.CorrectionBasis = "SYN-CORRECTION/fuel-w32-transcription"
	spec.Periods = []domain.SeriesPeriodValue{
		// 首期只改取值。
		seriesPeriod(t, seriesWeekOne, seriesWeekTwo, "0.23", "SYN-EVIDENCE/fuel-2026-W32"),
		// 末期改凭证并封上界。
		seriesPeriod(t, seriesWeekTwo, seriesWeekThree, "0.24", "SYN-EVIDENCE/fuel-2026-W33-reissued"),
	}
	proposed, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		t.Fatalf("构造更正版本：%v", err)
	}

	changes, err := domain.DiffSeriesPeriods(base, proposed)
	if err != nil {
		t.Fatalf("比对报错：%v", err)
	}
	indexed := changesByStart(t, changes)

	first := indexed[seriesWeekOne]
	if first.Kind() != domain.SeriesPeriodChanged || !first.ValueChanged() || first.EndChanged() || first.EvidenceChanged() {
		t.Fatalf("首期应只报取值变化：%s value=%v end=%v evidence=%v",
			first.Kind(), first.ValueChanged(), first.EndChanged(), first.EvidenceChanged())
	}
	basePeriod, ok := first.Base()
	if !ok || basePeriod.Value().String() != "0.22" {
		t.Fatalf("首期基准取值 = %v/%v，想要 0.22", basePeriod.Value(), ok)
	}
	proposedPeriod, ok := first.Proposed()
	if !ok || proposedPeriod.Value().String() != "0.23" {
		t.Fatalf("首期拟登取值 = %v/%v，想要 0.23", proposedPeriod.Value(), ok)
	}

	second := indexed[seriesWeekTwo]
	if second.Kind() != domain.SeriesPeriodChanged || second.ValueChanged() || !second.EndChanged() || !second.EvidenceChanged() {
		t.Fatalf("末期应报止点与凭证变化而非取值：%s value=%v end=%v evidence=%v",
			second.Kind(), second.ValueChanged(), second.EndChanged(), second.EvidenceChanged())
	}
}

// TestDiffSeriesPeriodsSeesExtensionsAndRemovals 证延展版本新增的期次报 ADDED（基准侧缺席），
// 基准有而拟登没有的期次报 REMOVED（拟登侧缺席）；两种缺席各只在一侧有期次可取。
func TestDiffSeriesPeriodsSeesExtensionsAndRemovals(t *testing.T) {
	base := diffFixtureBase(t)
	spec := fuelSeriesSpec(t)
	spec.Reference = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v2")
	spec.Periods = []domain.SeriesPeriodValue{
		// 基准的第二期（起点 seriesWeekTwo）被拿掉，改从第三周起一期新的。
		seriesPeriod(t, seriesWeekOne, seriesWeekTwo, "0.22", "SYN-EVIDENCE/fuel-2026-W32"),
		seriesPeriod(t, seriesWeekThree, time.Time{}, "0.26", "SYN-EVIDENCE/fuel-2026-W34"),
	}
	proposed, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		t.Fatalf("构造延展版本：%v", err)
	}

	changes, err := domain.DiffSeriesPeriods(base, proposed)
	if err != nil {
		t.Fatalf("比对报错：%v", err)
	}
	if len(changes) != 3 {
		t.Fatalf("期次数 = %d，想要 3（未变、移除、新增）", len(changes))
	}
	indexed := changesByStart(t, changes)

	removed := indexed[seriesWeekTwo]
	if removed.Kind() != domain.SeriesPeriodRemoved {
		t.Fatalf("第二周应报 REMOVED，得到 %s", removed.Kind())
	}
	if _, ok := removed.Proposed(); ok {
		t.Fatal("被移除的期次不该有拟登侧")
	}
	if basePeriod, ok := removed.Base(); !ok || basePeriod.Value().String() != "0.24" {
		t.Fatalf("被移除期次的基准取值 = %v/%v", basePeriod.Value(), ok)
	}

	added := indexed[seriesWeekThree]
	if added.Kind() != domain.SeriesPeriodAdded {
		t.Fatalf("第三周应报 ADDED，得到 %s", added.Kind())
	}
	if _, ok := added.Base(); ok {
		t.Fatal("新增的期次不该有基准侧")
	}
	if proposedPeriod, ok := added.Proposed(); !ok || proposedPeriod.Value().String() != "0.26" {
		t.Fatalf("新增期次的拟登取值 = %v/%v", proposedPeriod.Value(), ok)
	}
	// 排序覆盖两侧：移除的那一期（只在基准）也要落在按起点排好的位置上。
	if !changes[0].StartsAt().Equal(seriesWeekOne) || !changes[1].StartsAt().Equal(seriesWeekTwo) || !changes[2].StartsAt().Equal(seriesWeekThree) {
		t.Fatalf("结果没按起点升序合并两侧：%v %v %v", changes[0].StartsAt(), changes[1].StartsAt(), changes[2].StartsAt())
	}
}

// TestDiffSeriesPeriodsRefusesToCompareAcrossSeries 证跨序列、跨种类或跨租户的两版不比：
// 那不是「差异很大」，是问错了对象；零值登记同样拒——它连期次都没有。
func TestDiffSeriesPeriodsRefusesToCompareAcrossSeries(t *testing.T) {
	base := diffFixtureBase(t)

	other := fuelSeriesSpec(t)
	other.Reference = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-OTHER-SERIES", "v1")
	otherSeries, err := domain.NewReferenceSeriesRegistration(other)
	if err != nil {
		t.Fatalf("构造另一条序列：%v", err)
	}
	if _, err := domain.DiffSeriesPeriods(base, otherSeries); !errors.Is(err, domain.ErrSeriesPeriodDiffAcrossSeries) {
		t.Fatalf("跨序列比对：err = %v", err)
	}

	otherTenant := fuelSeriesSpec(t)
	otherTenant.Tenant = mustValue(t, domain.NewTenantID, "tenant-2")
	otherTenantSeries, err := domain.NewReferenceSeriesRegistration(otherTenant)
	if err != nil {
		t.Fatalf("构造他租序列：%v", err)
	}
	if _, err := domain.DiffSeriesPeriods(base, otherTenantSeries); !errors.Is(err, domain.ErrSeriesPeriodDiffAcrossSeries) {
		t.Fatalf("跨租户比对：err = %v", err)
	}

	if _, err := domain.DiffSeriesPeriods(domain.ReferenceSeriesRegistration{}, base); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("零值基准：err = %v", err)
	}
	if _, err := domain.DiffSeriesPeriods(base, domain.ReferenceSeriesRegistration{}); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("零值拟登：err = %v", err)
	}
}
