package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证计价参考序列登记册的领域半边（票 08）：登记一期取值是一次来源事实断言，
// 必须携带来源标识、取值凭证、登记责任方与生效区间（CONTEXT 硬句）；缺凭证的期次
// 只有断言强度；汇率不接受未声明口径的裸值；取值更正形成新序列版本并声明与原版本
// 的更正关系；解析按计价基准时点（ADR-0013）。夹具全部为 SYN-PRC 合成序列（S 级）。

var (
	seriesWeekOne   = time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	seriesWeekTwo   = time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	seriesWeekThree = time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
)

func seriesPeriod(t *testing.T, from, to time.Time, value, evidence string) domain.SeriesPeriodValue {
	t.Helper()
	period, err := domain.NewSeriesPeriodValue(from, to, decimal(t, value), evidence)
	if err != nil {
		t.Fatalf("构造期次 %s：%v", value, err)
	}
	return period
}

func fuelSeriesSpec(t *testing.T) domain.ReferenceSeriesRegistrationSpec {
	t.Helper()
	return domain.ReferenceSeriesRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v1"),
		SourceIdentifier: "SYN-CARRIER/fuel-weekly-bulletin",
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		Periods: []domain.SeriesPeriodValue{
			seriesPeriod(t, seriesWeekOne, seriesWeekTwo, "0.22", "SYN-EVIDENCE/fuel-2026-W32"),
			seriesPeriod(t, seriesWeekTwo, seriesWeekThree, "0.24", ""),
		},
	}
}

func fuelSeries(t *testing.T) domain.ReferenceSeriesRegistration {
	t.Helper()
	registration, err := domain.NewReferenceSeriesRegistration(fuelSeriesSpec(t))
	if err != nil {
		t.Fatalf("构造燃油序列登记：%v", err)
	}
	return registration
}

// TestReferenceSeriesResolvesByPricingBasisTime 证解析按计价基准时点取期次：区间按
// [起, 止) 判，命中期次给出可冻结进评价输入的序列取值；断言强度随答案带出——缺凭证
// 的期次解析得到但只有断言强度，生产金额的门在消费侧。
func TestReferenceSeriesResolvesByPricingBasisTime(t *testing.T) {
	registration := fuelSeries(t)

	if !registration.EffectiveFrom().Equal(seriesWeekOne) {
		t.Fatalf("生效起点漂移：%v", registration.EffectiveFrom())
	}
	if endsAt, bounded := registration.EffectiveTo(); !bounded || !endsAt.Equal(seriesWeekThree) {
		t.Fatalf("生效终点漂移：%v bounded=%v", endsAt, bounded)
	}
	if registration.Verifiable() {
		t.Fatal("第二期缺凭证，登记不该整体可复核")
	}

	reading, found := registration.ResolveAt(seriesWeekOne.Add(72 * time.Hour))
	if !found {
		t.Fatal("首期内时点没解析到取值")
	}
	if reading.Value().Kind() != domain.ReferenceSeriesFuelRate ||
		reading.Value().Value().String() != "0.22" {
		t.Fatalf("首期取值变形：%s %s", reading.Value().Kind(), reading.Value().Value())
	}
	if evidence, ok := reading.Evidence(); !ok || evidence != "SYN-EVIDENCE/fuel-2026-W32" || !reading.Verifiable() {
		t.Fatalf("首期凭证漂移：%q ok=%v verifiable=%v", evidence, ok, reading.Verifiable())
	}

	boundary, found := registration.ResolveAt(seriesWeekTwo)
	if !found || boundary.Value().Value().String() != "0.24" {
		t.Fatalf("期界时点该落进第二期：found=%v", found)
	}
	if boundary.Verifiable() {
		t.Fatal("缺凭证期次只有断言强度，不该报可复核")
	}
	if _, ok := boundary.Evidence(); ok {
		t.Fatal("缺凭证期次凭空带了凭证")
	}

	if _, found := registration.ResolveAt(seriesWeekThree); found {
		t.Fatal("终点右开：终点时点不该命中")
	}
	if _, found := registration.ResolveAt(seriesWeekOne.Add(-time.Hour)); found {
		t.Fatal("起点之前不该命中")
	}
}

// TestReferenceSeriesGapsAndOpenTail 证期次允许留缺口（承运商可能停发一周，缺口内
// 解析不到即评价挂起，不编数值），末期可以无上界；无上界期次只许在最后。
func TestReferenceSeriesGapsAndOpenTail(t *testing.T) {
	spec := fuelSeriesSpec(t)
	spec.Periods = []domain.SeriesPeriodValue{
		seriesPeriod(t, seriesWeekOne, seriesWeekTwo, "0.22", "SYN-EVIDENCE/fuel-2026-W32"),
		seriesPeriod(t, seriesWeekThree, time.Time{}, "0.26", "SYN-EVIDENCE/fuel-2026-W34"),
	}
	registration, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		t.Fatalf("带缺口的登记立不住：%v", err)
	}
	if _, bounded := registration.EffectiveTo(); bounded {
		t.Fatal("末期无上界，生效终点不该有界")
	}
	if _, found := registration.ResolveAt(seriesWeekTwo.Add(time.Hour)); found {
		t.Fatal("缺口内不该解析到取值")
	}
	late, found := registration.ResolveAt(seriesWeekThree.AddDate(1, 0, 0))
	if !found || late.Value().Value().String() != "0.26" {
		t.Fatalf("无上界末期该罩住远期时点：found=%v", found)
	}

	disordered := fuelSeriesSpec(t)
	disordered.Periods = []domain.SeriesPeriodValue{
		seriesPeriod(t, seriesWeekOne, time.Time{}, "0.22", "SYN-EVIDENCE/fuel-2026-W32"),
		seriesPeriod(t, seriesWeekThree, time.Time{}, "0.26", "SYN-EVIDENCE/fuel-2026-W34"),
	}
	if _, err := domain.NewReferenceSeriesRegistration(disordered); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("非末期无上界：err = %v, 想要 ErrInvalidReferenceSeriesRegistration", err)
	}
}

// TestReferenceSeriesPeriodDiscipline 证期次纪律：至少一期、按起点严格递增、不重叠；
// 期次本身要求起点非零、止点晚于起点、取值非负、凭证引用不带边空白。
func TestReferenceSeriesPeriodDiscipline(t *testing.T) {
	empty := fuelSeriesSpec(t)
	empty.Periods = nil
	if _, err := domain.NewReferenceSeriesRegistration(empty); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("零期登记：err = %v", err)
	}

	unordered := fuelSeriesSpec(t)
	unordered.Periods = []domain.SeriesPeriodValue{
		seriesPeriod(t, seriesWeekTwo, seriesWeekThree, "0.24", ""),
		seriesPeriod(t, seriesWeekOne, seriesWeekTwo, "0.22", ""),
	}
	if _, err := domain.NewReferenceSeriesRegistration(unordered); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("乱序期次：err = %v", err)
	}

	overlapping := fuelSeriesSpec(t)
	overlapping.Periods = []domain.SeriesPeriodValue{
		seriesPeriod(t, seriesWeekOne, seriesWeekThree, "0.22", ""),
		seriesPeriod(t, seriesWeekTwo, seriesWeekThree, "0.24", ""),
	}
	if _, err := domain.NewReferenceSeriesRegistration(overlapping); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("重叠期次：err = %v", err)
	}

	if _, err := domain.NewSeriesPeriodValue(time.Time{}, seriesWeekTwo, decimal(t, "0.22"), ""); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("零值起点：err = %v", err)
	}
	if _, err := domain.NewSeriesPeriodValue(seriesWeekTwo, seriesWeekOne, decimal(t, "0.22"), ""); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("止点早于起点：err = %v", err)
	}
	if _, err := domain.NewSeriesPeriodValue(seriesWeekOne, seriesWeekTwo, decimal(t, "-0.1"), ""); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("负取值：err = %v", err)
	}
	if _, err := domain.NewSeriesPeriodValue(seriesWeekOne, seriesWeekTwo, decimal(t, "0.22"), " padded "); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("凭证带边空白：err = %v", err)
	}
}

// TestReferenceSeriesRegistrationDemandsProvenance 证登记门的来源硬件：来源标识与
// 登记责任方缺一不可（转抄与登记错误由登记责任方承担——没有责任方就没有承担者）。
func TestReferenceSeriesRegistrationDemandsProvenance(t *testing.T) {
	unsourced := fuelSeriesSpec(t)
	unsourced.SourceIdentifier = "  "
	if _, err := domain.NewReferenceSeriesRegistration(unsourced); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("空来源标识：err = %v", err)
	}

	unowned := fuelSeriesSpec(t)
	unowned.Registrant = ""
	if _, err := domain.NewReferenceSeriesRegistration(unowned); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("空登记责任方：err = %v", err)
	}

	wrongArtifact := fuelSeriesSpec(t)
	wrongArtifact.Reference = versionReference(t, domain.ArtifactRateTable, "not-a-series", "v1")
	if _, err := domain.NewReferenceSeriesRegistration(wrongArtifact); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("错种类序列引用：err = %v", err)
	}
}

// TestExchangeRateSeriesDemandsAQuoteBasis 证 CONTEXT「不接受未声明口径的裸汇率」：
// 汇率序列必须携带声明其口径的商业价格政策版本引用；解析出的取值带着口径走。
func TestExchangeRateSeriesDemandsAQuoteBasis(t *testing.T) {
	bare := fuelSeriesSpec(t)
	bare.Kind = domain.ReferenceSeriesExchangeRate
	bare.Reference = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-USD-CNY", "v1")
	if _, err := domain.NewReferenceSeriesRegistration(bare); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("裸汇率：err = %v, 想要 ErrInvalidReferenceSeriesRegistration", err)
	}

	wrongBasis := bare
	wrongBasis.QuoteBasis = versionReference(t, domain.ArtifactReferenceSeries, "not-a-policy", "v1")
	if _, err := domain.NewReferenceSeriesRegistration(wrongBasis); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("错种类口径：err = %v", err)
	}

	quoted := bare
	quoted.QuoteBasis = versionReference(t, domain.ArtifactCommercialPolicy, "SYN-PRC-FX-POLICY", "v1")
	registration, err := domain.NewReferenceSeriesRegistration(quoted)
	if err != nil {
		t.Fatalf("带口径的汇率登记立不住：%v", err)
	}
	reading, found := registration.ResolveAt(seriesWeekOne.Add(time.Hour))
	if !found {
		t.Fatal("汇率首期没解析到")
	}
	basis, ok := reading.Value().QuoteBasis()
	if !ok || basis.ID() != "SYN-PRC-FX-POLICY" {
		t.Fatalf("解析取值丢了口径：ok=%v basis=%s", ok, basis.ID())
	}
}

// TestSeriesCorrectionDeclaresItsRelation 证 CONTEXT「取值更正形成新序列版本并声明
// 与原版本的更正关系」：更正两件成对、不自指、不换序列身份——换身份就是另一条序列，
// 只能另行登记。
func TestSeriesCorrectionDeclaresItsRelation(t *testing.T) {
	correction := fuelSeriesSpec(t)
	correction.Reference = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v2")
	correction.PriorVersion = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v1")
	correction.CorrectionBasis = "SYN-CORRECTION/fuel-2026-W32-transcription"
	registration, err := domain.NewReferenceSeriesRegistration(correction)
	if err != nil {
		t.Fatalf("更正登记立不住：%v", err)
	}
	prior, basis, corrected := registration.Correction()
	if !corrected || prior.Version() != "v1" || basis != "SYN-CORRECTION/fuel-2026-W32-transcription" {
		t.Fatalf("更正关系漂移：%v %q %v", prior, basis, corrected)
	}
	if _, _, corrected := fuelSeries(t).Correction(); corrected {
		t.Fatal("首版凭空带了更正关系")
	}

	orphanBasis := fuelSeriesSpec(t)
	orphanBasis.CorrectionBasis = "SYN-CORRECTION/loose"
	if _, err := domain.NewReferenceSeriesRegistration(orphanBasis); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("只带依据不带回指：err = %v", err)
	}

	orphanPrior := correction
	orphanPrior.CorrectionBasis = ""
	if _, err := domain.NewReferenceSeriesRegistration(orphanPrior); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("只带回指不带依据：err = %v", err)
	}

	selfPointing := correction
	selfPointing.PriorVersion = correction.Reference
	if _, err := domain.NewReferenceSeriesRegistration(selfPointing); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("自指更正：err = %v", err)
	}

	identitySwap := correction
	identitySwap.PriorVersion = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-OTHER-SERIES", "v1")
	if _, err := domain.NewReferenceSeriesRegistration(identitySwap); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("换序列身份的更正：err = %v", err)
	}
}

// TestReferenceSeriesSnapshotRoundTrips 证登记快照折装重建逐字节同答，内容摘要在
// 往返两侧一致，且只随内容变化。
func TestReferenceSeriesSnapshotRoundTrips(t *testing.T) {
	registration := fuelSeries(t)

	raw, err := domain.MarshalReferenceSeriesRegistration(registration)
	if err != nil {
		t.Fatalf("折装序列快照：%v", err)
	}
	rebuilt, err := domain.RehydrateReferenceSeriesRegistration(raw)
	if err != nil {
		t.Fatalf("重建序列登记：%v", err)
	}
	again, err := domain.MarshalReferenceSeriesRegistration(rebuilt)
	if err != nil {
		t.Fatalf("重建后再折装：%v", err)
	}
	if !bytes.Equal(raw, again) {
		t.Fatalf("序列快照往返不同答\n首次=%s\n再次=%s", raw, again)
	}
	if rebuilt.ContentDigest() != registration.ContentDigest() {
		t.Fatal("内容摘要在往返两侧不一致")
	}

	revalued := fuelSeriesSpec(t)
	revalued.Periods = []domain.SeriesPeriodValue{
		seriesPeriod(t, seriesWeekOne, seriesWeekTwo, "0.23", "SYN-EVIDENCE/fuel-2026-W32"),
		seriesPeriod(t, seriesWeekTwo, seriesWeekThree, "0.24", ""),
	}
	other, err := domain.NewReferenceSeriesRegistration(revalued)
	if err != nil {
		t.Fatalf("构造对照登记：%v", err)
	}
	if other.ContentDigest() == registration.ContentDigest() {
		t.Fatal("取值不同的两版摘要相同——摘要没罩住期次取值")
	}
}

// TestReferenceSeriesSnapshotExposesTampering 证快照携带自己的内容摘要并在重建门上
// 自校：登记后被改过的取值读不回来，而不是变成一个看起来合法的序列。
func TestReferenceSeriesSnapshotExposesTampering(t *testing.T) {
	raw, err := domain.MarshalReferenceSeriesRegistration(fuelSeries(t))
	if err != nil {
		t.Fatalf("折装序列快照：%v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("解开序列快照：%v", err)
	}
	periods, ok := document["periods"].([]any)
	if !ok || len(periods) == 0 {
		t.Fatalf("快照缺期次")
	}
	first, ok := periods[0].(map[string]any)
	if !ok {
		t.Fatalf("期次形状不对")
	}
	value, ok := first["value"].(map[string]any)
	if !ok {
		t.Fatalf("取值形状不对")
	}
	value["coefficient"] = "99"
	tampered, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("重封序列快照：%v", err)
	}

	if _, err := domain.RehydrateReferenceSeriesRegistration(tampered); !errors.Is(err, domain.ErrReferenceSeriesRegistrationSnapshotInvalid) {
		t.Fatalf("err = %v, 想要 ErrReferenceSeriesRegistrationSnapshotInvalid", err)
	}
}

// TestReferenceSeriesSnapshotRefusesForeignCanonicalization 证摘要只在同一规范化
// 版本内可比（ADR-0014 的同一条纪律）：按别的形状记录的快照拒绝重建，不硬算。
func TestReferenceSeriesSnapshotRefusesForeignCanonicalization(t *testing.T) {
	raw, err := domain.MarshalReferenceSeriesRegistration(fuelSeries(t))
	if err != nil {
		t.Fatalf("折装序列快照：%v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("解开序列快照：%v", err)
	}
	document["canonicalization"] = "PRS-0"
	tampered, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("重封序列快照：%v", err)
	}

	if _, err := domain.RehydrateReferenceSeriesRegistration(tampered); !errors.Is(err, domain.ErrCanonicalizationVersionUnsupported) {
		t.Fatalf("err = %v, 想要 ErrCanonicalizationVersionUnsupported", err)
	}
}
