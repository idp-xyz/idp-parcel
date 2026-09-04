package sourcefeed_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/sourcefeed"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outbound"
)

// 本文件证以受控目录文件为源的连接器（票 06 裁决一）：Fetch 只读受控目录之内、字节在手那一刻算
// 摘要、记读取时刻与文件定位符、读出来源声明的公布日期；文件缺失或解不开一律拒——不补数不沿用
// 旧值；Transcribe 只从原文读观测，整版重述交给领域。仓内不得出现任何出网代码（本票红线），
// 由同包 TestNoOutboundNetworkCodeShipsWithTheFileConnector 把守。

var fetchClockMoment = time.Date(2026, 9, 4, 1, 2, 3, 0, time.UTC)

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

const usdCnyPublication = `{"seriesId":"SYN-PRC-USD-CNY","publishedOn":"2026-09-04","effectiveFrom":"2026-09-04T00:00:00Z","value":"7.1234"}`

func writeSource(t *testing.T, root, relative, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("建受控目录：%v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("写来源文件：%v", err)
	}
}

func fileConnector(t *testing.T, root string) *sourcefeed.FileConnector {
	t.Helper()
	connector, err := sourcefeed.NewFileConnector(root, fixedClock{now: fetchClockMoment})
	if err != nil {
		t.Fatalf("构造 FileConnector：%v", err)
	}
	return connector
}

func fileBinding(t *testing.T, seriesID string) domain.SourceConnectorBinding {
	t.Helper()
	policy, err := domain.NewVersionReferenceWithFingerprint(domain.ArtifactCommercialPolicy, "SYN-PRC-FX-POLICY", "v1", "sha256:syn-fx-policy")
	if err != nil {
		t.Fatalf("构造口径引用：%v", err)
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("构造租户：%v", err)
	}
	binding, err := domain.NewSourceConnectorBinding(domain.SourceConnectorBindingSpec{
		Tenant:           tenant,
		SeriesID:         seriesID,
		Version:          "b1",
		ConnectorKind:    sourcefeed.FileConnectorKind,
		SourceIdentifier: "SYN-SOURCE/usd-cny-daily",
		SourceLocator:    "rates/usd-cny/2026-09-04.json",
		SeriesKind:       domain.ReferenceSeriesExchangeRate,
		QuoteBasis:       policy,
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		ReviewExemption:  domain.ReviewExemptionGranted,
	})
	if err != nil {
		t.Fatalf("构造绑定：%v", err)
	}
	return binding
}

func fetchSpec(binding domain.SourceConnectorBinding) ports.FetchSpec {
	return ports.FetchSpec{Tenant: binding.Tenant(), SeriesID: binding.SeriesID(), Locator: binding.SourceLocator()}
}

// TestFileConnectorFetchReadsTheControlledDirectory 证抓取三件：摘要是文件字节的 SHA-256、时刻取
// 时钟当下、定位符是受控目录内的相对路径（file: 前缀）；公布日期按文件自己声明的读。
func TestFileConnectorFetchReadsTheControlledDirectory(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "rates/usd-cny/2026-09-04.json", usdCnyPublication)
	connector := fileConnector(t, root)
	binding := fileBinding(t, "SYN-PRC-USD-CNY")

	record, err := connector.Fetch(t.Context(), fetchSpec(binding))
	if err != nil {
		t.Fatalf("抓取：%v", err)
	}
	sum := sha256.Sum256([]byte(usdCnyPublication))
	if record.ContentDigest() != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatalf("摘要 = %q，不是文件字节的 SHA-256", record.ContentDigest())
	}
	if !record.FetchedAt().Equal(fetchClockMoment) {
		t.Fatalf("抓取时刻 = %v，想要时钟当下", record.FetchedAt())
	}
	if record.SourceLocator() != "file:rates/usd-cny/2026-09-04.json" {
		t.Fatalf("定位符 = %q", record.SourceLocator())
	}
	if !record.PublishedOn().Equal(time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("公布日期 = %v", record.PublishedOn())
	}
	if connector.Kind() != sourcefeed.FileConnectorKind || connector.Kind() != "FILE" {
		t.Fatalf("连接器种类 = %q", connector.Kind())
	}
}

// TestFileConnectorFetchRefusesWhatItCannotRead 证抓取失败的每一格都是拒而不是猜：文件不在、
// 不是 JSON、没声明公布日期、定位符逃出受控目录（相对上溯与绝对路径）。
func TestFileConnectorFetchRefusesWhatItCannotRead(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "rates/not-json.txt", "hello")
	writeSource(t, root, "rates/no-date.json", `{"seriesId":"SYN-PRC-USD-CNY","value":"7.1"}`)
	writeSource(t, root, "rates/bad-date.json", `{"seriesId":"SYN-PRC-USD-CNY","publishedOn":"yesterday","value":"7.1"}`)
	writeSource(t, root, "rates/empty.json", "")
	connector := fileConnector(t, root)
	tenant, _ := domain.NewTenantID("tenant-1")

	fetch := func(locator string) error {
		_, err := connector.Fetch(t.Context(), ports.FetchSpec{Tenant: tenant, SeriesID: "SYN-PRC-USD-CNY", Locator: locator})
		return err
	}
	if err := fetch("rates/missing.json"); !errors.Is(err, sourcefeed.ErrSourceUnavailable) {
		t.Fatalf("文件缺失：err=%v，想要 ErrSourceUnavailable", err)
	}
	for _, locator := range []string{"rates/not-json.txt", "rates/no-date.json", "rates/bad-date.json", "rates/empty.json"} {
		if err := fetch(locator); !errors.Is(err, sourcefeed.ErrPublicationUnreadable) {
			t.Fatalf("%s：err=%v，想要 ErrPublicationUnreadable", locator, err)
		}
	}
	for _, locator := range []string{"../outside.json", filepath.Join(root, "..", "outside.json"), "/etc/passwd", ""} {
		if err := fetch(locator); !errors.Is(err, sourcefeed.ErrLocatorOutsideRoot) {
			t.Fatalf("%q：err=%v，想要 ErrLocatorOutsideRoot", locator, err)
		}
	}
	if _, err := sourcefeed.NewFileConnector("", fixedClock{now: fetchClockMoment}); err == nil {
		t.Fatal("空受控目录被接受了——连接器不猜读取根")
	}
	if _, err := sourcefeed.NewFileConnector(root, nil); err == nil {
		t.Fatal("没有时钟被接受了——抓取时刻不能凭空")
	}
}

// TestFileConnectorTranscribesThroughTheDomain 证转录只从原文读观测（生效起点与取值），其余交给
// domain.TranscribeSourceFeed：首版一期开放、凭证为工件引用且带存放定位符（本体另有存放处时）；
// 文件里的序列标识与绑定不符如实拒——放错目录的文件不得登进别的序列。
func TestFileConnectorTranscribesThroughTheDomain(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "rates/usd-cny/2026-09-04.json", usdCnyPublication)
	writeSource(t, root, "rates/usd-cny/bad-value.json", `{"seriesId":"SYN-PRC-USD-CNY","publishedOn":"2026-09-04","effectiveFrom":"2026-09-04T00:00:00Z","value":"seven"}`)
	writeSource(t, root, "rates/usd-cny/no-start.json", `{"seriesId":"SYN-PRC-USD-CNY","publishedOn":"2026-09-04","value":"7.1"}`)
	connector := fileConnector(t, root)
	binding := fileBinding(t, "SYN-PRC-USD-CNY")

	record, err := connector.Fetch(t.Context(), fetchSpec(binding))
	if err != nil {
		t.Fatalf("抓取：%v", err)
	}
	spec, err := connector.Transcribe(ports.TranscriptionInput{
		Record:    record,
		Placement: ports.ArtifactPlacement{Outcome: outbound.Accept(), Locator: "s3://syn-artifacts/usd-cny/2026-09-04.json"},
		Binding:   binding,
	})
	if err != nil {
		t.Fatalf("转录：%v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		t.Fatalf("转录出的 spec 立不住：%v", err)
	}
	if registration.Reference().Version() != "2026-09-04" || registration.Reference().Fingerprint() != record.ContentDigest() {
		t.Fatalf("版本引用走样：%+v", registration.Reference())
	}
	periods := registration.Periods()
	if len(periods) != 1 || periods[0].Value().String() != "7.1234" || !periods[0].StartsAt().Equal(time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("观测走样：%+v", periods)
	}
	if evidence, _ := periods[0].Evidence(); evidence != record.EvidenceReference("s3://syn-artifacts/usd-cny/2026-09-04.json") {
		t.Fatalf("凭证 = %q", evidence)
	}
	if registration.EvidenceGrade() != domain.SeriesEvidenceVerifiable {
		t.Fatal("连接器登记的期次该天生 VERIFIABLE")
	}

	unconfigured, err := connector.Transcribe(ports.TranscriptionInput{Record: record, Placement: ports.ArtifactPlacement{Outcome: outbound.Unconfigured()}, Binding: binding})
	if err != nil {
		t.Fatalf("未配置存放时转录：%v", err)
	}
	if evidence, _ := unconfigured.Periods[0].Evidence(); evidence != record.EvidenceReference("") {
		t.Fatalf("未配置存放的凭证带了定位符：%q", evidence)
	}

	if _, err := connector.Transcribe(ports.TranscriptionInput{Record: record, Binding: fileBinding(t, "SYN-PRC-EUR-CNY")}); !errors.Is(err, sourcefeed.ErrPublicationSeriesMismatch) {
		t.Fatalf("序列不符：err=%v，想要 ErrPublicationSeriesMismatch", err)
	}
	for _, locator := range []string{"rates/usd-cny/bad-value.json", "rates/usd-cny/no-start.json"} {
		fetched, err := connector.Fetch(t.Context(), ports.FetchSpec{Tenant: binding.Tenant(), SeriesID: binding.SeriesID(), Locator: locator})
		if err != nil {
			t.Fatalf("%s 抓取：%v", locator, err)
		}
		if _, err := connector.Transcribe(ports.TranscriptionInput{Record: fetched, Binding: binding}); !errors.Is(err, sourcefeed.ErrPublicationUnreadable) {
			t.Fatalf("%s：err=%v，想要 ErrPublicationUnreadable", locator, err)
		}
	}
}

// TestInPlaceArtifactStoreAnswersTheSourceLocator 证原地引用：本体已经躺在受控目录里，存放答
// Accepted、定位符即来源地址——不复制、不引对象存储。
func TestInPlaceArtifactStoreAnswersTheSourceLocator(t *testing.T) {
	record, err := domain.NewPublishedRecord([]byte("x"), fetchClockMoment, "file:rates/usd-cny/2026-09-04.json", fetchClockMoment)
	if err != nil {
		t.Fatalf("构造记录：%v", err)
	}
	placement, err := sourcefeed.NewInPlaceArtifactStore().Store(t.Context(), record)
	if err != nil {
		t.Fatalf("原地存放：%v", err)
	}
	locator, stored := placement.StoredLocator()
	if placement.Outcome.Disposition() != outbound.Accepted || !stored || locator != record.SourceLocator() {
		t.Fatalf("原地引用答复走样：%+v", placement)
	}
	if _, stored := (ports.ArtifactPlacement{Outcome: outbound.Unconfigured()}).StoredLocator(); stored {
		t.Fatal("未配置存放却报有定位符")
	}
}

// TestConnectorRegistryResolvesByKind 证按绑定声明的连接器种类找实现：装了的找得到，没装的（CFETS
// 段留 draft）答 false 不答 error；同种类装两个是装配错误，构造期拒。
func TestConnectorRegistryResolvesByKind(t *testing.T) {
	connector := fileConnector(t, t.TempDir())
	registry, err := sourcefeed.NewConnectorRegistry(connector)
	if err != nil {
		t.Fatalf("构造登记表：%v", err)
	}
	if resolved, found := registry.ConnectorFor("FILE"); !found || resolved.Kind() != "FILE" {
		t.Fatalf("FILE 找不到：found=%v", found)
	}
	if _, found := registry.ConnectorFor("CFETS"); found {
		t.Fatal("没装配的连接器种类被找到了")
	}
	if _, err := sourcefeed.NewConnectorRegistry(connector, connector); err == nil {
		t.Fatal("同种类装两个被接受了")
	}
}
