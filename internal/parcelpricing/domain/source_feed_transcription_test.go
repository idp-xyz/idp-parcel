package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证来源连接器转录的领域半边（ADR-0099 决定五、六）：每一版都是整版重述——前一版全部
// 期次 + 新期次，前一版开放的末期在新期次起点闭合；凭证 = 工件引用，连接器登记的期次因此天生
// VERIFIABLE；同一份工件再来一次转录出与前一版逐字相同的 spec（登记册据以答幂等重放）；公布
// 顺序倒置不补数不改数，如实拒绝——缺口留给评价去挂起。

var (
	feedDayOne   = time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	feedDayTwo   = time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	feedDayThree = time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
)

func observation(t *testing.T, effectiveFrom time.Time, value string) domain.SeriesObservation {
	t.Helper()
	observed, err := domain.NewSeriesObservation(effectiveFrom, decimal(t, value))
	if err != nil {
		t.Fatalf("构造观测：%v", err)
	}
	return observed
}

func fetchedRecord(t *testing.T, body string, fetchedAt, publishedOn time.Time) domain.PublishedRecord {
	t.Helper()
	record, err := domain.NewPublishedRecord([]byte(body), fetchedAt, "file:rates/usd-cny/"+publishedOn.Format("2006-01-02")+".json", publishedOn)
	if err != nil {
		t.Fatalf("构造公布记录：%v", err)
	}
	return record
}

func transcribed(t *testing.T, input domain.SourceFeedTranscription) domain.ReferenceSeriesRegistration {
	t.Helper()
	spec, err := domain.TranscribeSourceFeed(input)
	if err != nil {
		t.Fatalf("转录：%v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		t.Fatalf("转录出的 spec 立不住：%v", err)
	}
	return registration
}

// TestTranscribeFirstVersionFromAPublication 证没有前一版时的首版：一期开放期次，取值与生效
// 起点按观测原样转录（连接器不算数、不改数），口径与来源标识、登记责任方来自绑定（不在连接器里
// 写死），版本号取来源声明的公布日期，引用 digest 取工件摘要，凭证为工件引用——整版 VERIFIABLE。
func TestTranscribeFirstVersionFromAPublication(t *testing.T) {
	binding := fxBinding(t)
	record := fetchedRecord(t, `{"value":"7.1234"}`, fetchMoment, feedDayOne)

	registration := transcribed(t, domain.SourceFeedTranscription{
		Binding:     binding,
		Record:      record,
		Prior:       nil,
		Observation: observation(t, feedDayOne, "7.1234"),
	})

	if registration.Tenant() != binding.Tenant() || registration.Kind() != domain.ReferenceSeriesExchangeRate ||
		registration.SourceIdentifier() != binding.SourceIdentifier() || registration.Registrant() != binding.Registrant() {
		t.Fatalf("登记的身份四件不来自绑定：%+v", registration)
	}
	reference := registration.Reference()
	if reference.Kind() != domain.ArtifactReferenceSeries || reference.ID() != "SYN-PRC-USD-CNY" ||
		reference.Version() != "2026-09-04" || reference.Digest() != record.ContentDigest() {
		t.Fatalf("版本引用走样：%+v", reference)
	}
	basis, declared := registration.QuoteBasis()
	if !declared || basis.ID() != "SYN-PRC-FX-POLICY" {
		t.Fatalf("口径没从绑定带过来：declared=%v", declared)
	}
	periods := registration.Periods()
	if len(periods) != 1 || !periods[0].StartsAt().Equal(feedDayOne) || periods[0].Value().String() != "7.1234" {
		t.Fatalf("首版期次走样：%+v", periods)
	}
	if _, bounded := periods[0].EndsAt(); bounded {
		t.Fatal("首版唯一一期该是开放末期")
	}
	if evidence, _ := periods[0].Evidence(); evidence != record.EvidenceReference("") {
		t.Fatalf("凭证 = %q，想要工件引用 %q", evidence, record.EvidenceReference(""))
	}
	if registration.EvidenceGrade() != domain.SeriesEvidenceVerifiable {
		t.Fatalf("连接器登记的期次该天生 VERIFIABLE，得 %s", registration.EvidenceGrade())
	}
	if _, _, corrected := registration.Correction(); corrected {
		t.Fatal("延展不带更正关系")
	}
}

// TestTranscribeRestatesTheWholeSeriesAndClosesTheOpenTail 证延展是整版重述：前一版的全部期次
// 原样在（含各自的凭证），前一版开放末期在新期次起点闭合，新期次接上并开放；本体另有存放处时
// 凭证带定位符。
func TestTranscribeRestatesTheWholeSeriesAndClosesTheOpenTail(t *testing.T) {
	binding := fxBinding(t)
	first := transcribed(t, domain.SourceFeedTranscription{
		Binding:     binding,
		Record:      fetchedRecord(t, `{"value":"7.1234"}`, fetchMoment, feedDayOne),
		Observation: observation(t, feedDayOne, "7.1234"),
	})

	second := fetchedRecord(t, `{"value":"7.13"}`, fetchMoment.Add(24*time.Hour), feedDayTwo)
	extended := transcribed(t, domain.SourceFeedTranscription{
		Binding:        binding,
		Record:         second,
		StorageLocator: "s3://syn-artifacts/usd-cny/2026-09-05.json",
		Prior:          &first,
		Observation:    observation(t, feedDayTwo, "7.13"),
	})

	if extended.Reference().Version() != "2026-09-05" || extended.Reference().Digest() != second.ContentDigest() {
		t.Fatalf("延展版引用走样：%+v", extended.Reference())
	}
	periods := extended.Periods()
	if len(periods) != 2 {
		t.Fatalf("整版重述该有两期，得 %d", len(periods))
	}
	endsAt, bounded := periods[0].EndsAt()
	if !bounded || !endsAt.Equal(feedDayTwo) || periods[0].Value().String() != "7.1234" {
		t.Fatalf("前一版开放末期没在新期次起点闭合：%+v", periods[0])
	}
	if firstEvidence, _ := first.Periods()[0].Evidence(); firstEvidence == "" {
		t.Fatal("夹具首版没有凭证")
	} else if evidence, _ := periods[0].Evidence(); evidence != firstEvidence {
		t.Fatalf("前一版期次的凭证被改写：%q vs %q", evidence, firstEvidence)
	}
	if !periods[1].StartsAt().Equal(feedDayTwo) || periods[1].Value().String() != "7.13" {
		t.Fatalf("新期次走样：%+v", periods[1])
	}
	if _, bounded := periods[1].EndsAt(); bounded {
		t.Fatal("新期次该是开放末期")
	}
	if evidence, _ := periods[1].Evidence(); evidence != second.EvidenceReference("s3://syn-artifacts/usd-cny/2026-09-05.json") {
		t.Fatalf("新期次凭证 = %q", evidence)
	}
	if extended.EvidenceGrade() != domain.SeriesEvidenceVerifiable {
		t.Fatalf("延展版该整版 VERIFIABLE，得 %s", extended.EvidenceGrade())
	}
}

// TestTranscribeTheSameArtifactRestatesThePriorVerbatim 证同一份工件再抓一次（抓取时刻不同）
// 转录出与前一版逐字相同的 spec：内容摘要相等，登记册据以答幂等重放，而不是因为凭证里的时刻
// 变了就报同版本内容冲突。
func TestTranscribeTheSameArtifactRestatesThePriorVerbatim(t *testing.T) {
	binding := fxBinding(t)
	first := transcribed(t, domain.SourceFeedTranscription{
		Binding:     binding,
		Record:      fetchedRecord(t, `{"value":"7.1234"}`, fetchMoment, feedDayOne),
		Observation: observation(t, feedDayOne, "7.1234"),
	})

	again := transcribed(t, domain.SourceFeedTranscription{
		Binding:     binding,
		Record:      fetchedRecord(t, `{"value":"7.1234"}`, fetchMoment.Add(time.Hour), feedDayOne),
		Prior:       &first,
		Observation: observation(t, feedDayOne, "7.1234"),
	})

	if again.ContentDigest() != first.ContentDigest() || again.Reference() != first.Reference() {
		t.Fatalf("同一工件重转录不等于前一版：%s vs %s", again.ContentDigest(), first.ContentDigest())
	}
}

// TestTranscribeRefusesOutOfOrderPublications 证顺序倒置如实拒绝：新期次起点不晚于前一版末期
// 起点、或落进前一版有界末期之内，都不登——不补数、不改数、不沿用旧值，缺口留着。缺口本身
// （新起点晚于有界末期的终点）是允许的。
func TestTranscribeRefusesOutOfOrderPublications(t *testing.T) {
	binding := fxBinding(t)
	first := transcribed(t, domain.SourceFeedTranscription{
		Binding:     binding,
		Record:      fetchedRecord(t, `{"value":"7.1234"}`, fetchMoment, feedDayTwo),
		Observation: observation(t, feedDayTwo, "7.1234"),
	})

	for label, effectiveFrom := range map[string]time.Time{
		"同一起点": feedDayTwo,
		"更早起点": feedDayOne,
	} {
		_, err := domain.TranscribeSourceFeed(domain.SourceFeedTranscription{
			Binding:     binding,
			Record:      fetchedRecord(t, `{"value":"7.2"}`, fetchMoment, feedDayThree),
			Prior:       &first,
			Observation: observation(t, effectiveFrom, "7.2"),
		})
		if !errors.Is(err, domain.ErrSourceFeedOutOfOrder) {
			t.Fatalf("%s 被接受了：err=%v", label, err)
		}
	}

	bounded, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           binding.Tenant(),
		Kind:             domain.ReferenceSeriesExchangeRate,
		Reference:        versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-USD-CNY", "manual-1"),
		SourceIdentifier: binding.SourceIdentifier(),
		Registrant:       binding.Registrant(),
		QuoteBasis:       versionReference(t, domain.ArtifactCommercialPolicy, "SYN-PRC-FX-POLICY", "v1"),
		Periods:          []domain.SeriesPeriodValue{seriesPeriod(t, feedDayOne, feedDayThree, "7.0", "SYN-EVIDENCE/manual")},
	})
	if err != nil {
		t.Fatalf("构造有界前一版：%v", err)
	}
	if _, err := domain.TranscribeSourceFeed(domain.SourceFeedTranscription{
		Binding:     binding,
		Record:      fetchedRecord(t, `{"value":"7.2"}`, fetchMoment, feedDayThree),
		Prior:       &bounded,
		Observation: observation(t, feedDayTwo, "7.2"),
	}); !errors.Is(err, domain.ErrSourceFeedOutOfOrder) {
		t.Fatalf("落进有界末期之内的起点被接受了：err=%v", err)
	}

	gap := transcribed(t, domain.SourceFeedTranscription{
		Binding:     binding,
		Record:      fetchedRecord(t, `{"value":"7.2"}`, fetchMoment, feedDayThree.Add(24*time.Hour)),
		Prior:       &bounded,
		Observation: observation(t, feedDayThree.Add(24*time.Hour), "7.2"),
	})
	if periods := gap.Periods(); len(periods) != 2 || !periods[1].StartsAt().Equal(feedDayThree.Add(24*time.Hour)) {
		t.Fatalf("缺口之后的延展走样：%+v", periods)
	}
	if endsAt, isBounded := gap.Periods()[0].EndsAt(); !isBounded || !endsAt.Equal(feedDayThree) {
		t.Fatal("有界末期被改动了——闭合只对开放末期做")
	}
}

// TestTranscribeRefusesAPriorOfAnotherSeries 证前一版必须是同租户同序列的登记；零值绑定或记录
// 同样进不了门。
func TestTranscribeRefusesAPriorOfAnotherSeries(t *testing.T) {
	binding := fxBinding(t)
	other := fuelSeries(t)
	if _, err := domain.TranscribeSourceFeed(domain.SourceFeedTranscription{
		Binding:     binding,
		Record:      fetchedRecord(t, `{"value":"7.2"}`, fetchMoment, feedDayThree),
		Prior:       &other,
		Observation: observation(t, feedDayThree, "7.2"),
	}); !errors.Is(err, domain.ErrInvalidSourceFeedTranscription) {
		t.Fatalf("别的序列作前一版被接受了：err=%v", err)
	}
	if _, err := domain.TranscribeSourceFeed(domain.SourceFeedTranscription{
		Record:      fetchedRecord(t, `{"value":"7.2"}`, fetchMoment, feedDayThree),
		Observation: observation(t, feedDayThree, "7.2"),
	}); !errors.Is(err, domain.ErrInvalidSourceFeedTranscription) {
		t.Fatalf("零值绑定被接受了：err=%v", err)
	}
	if _, err := domain.TranscribeSourceFeed(domain.SourceFeedTranscription{
		Binding:     binding,
		Observation: observation(t, feedDayThree, "7.2"),
	}); !errors.Is(err, domain.ErrInvalidSourceFeedTranscription) {
		t.Fatalf("零值记录被接受了：err=%v", err)
	}
	if _, err := domain.NewSeriesObservation(time.Time{}, decimal(t, "1")); !errors.Is(err, domain.ErrInvalidSourceFeedTranscription) {
		t.Fatalf("零起点观测被接受了：err=%v", err)
	}
}

// TestRegistrationSpecRoundTrips 证 Spec 是构造器的逆：从一版登记取回 spec 再构造，内容摘要与
// 引用逐字相同，更正关系一并带回——整版重述与幂等重放都靠这条。
func TestRegistrationSpecRoundTrips(t *testing.T) {
	original := fuelSeries(t)
	rebuilt, err := domain.NewReferenceSeriesRegistration(original.Spec())
	if err != nil {
		t.Fatalf("按 Spec 重建：%v", err)
	}
	if rebuilt.ContentDigest() != original.ContentDigest() || rebuilt.Reference() != original.Reference() {
		t.Fatal("Spec 往返改变了登记内容")
	}

	spec := fuelSeriesSpec(t)
	spec.Reference = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v2")
	spec.PriorVersion = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v1")
	spec.CorrectionBasis = "SYN-CORRECTION/transcription"
	corrected, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		t.Fatalf("构造更正版：%v", err)
	}
	roundTripped := corrected.Spec()
	if roundTripped.PriorVersion != spec.PriorVersion || roundTripped.CorrectionBasis != spec.CorrectionBasis {
		t.Fatalf("更正关系没随 Spec 带回：%+v", roundTripped)
	}
}
