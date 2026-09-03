package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证受控登记口的路由与退出码翻译：种类路由、重建门拒绝、治理答案与未决各归
// 各的退出码。价卡登记的绿路径已在适配器与应用层证过（两条路由结构同形），这里用
// 序列走绿路径、用价卡走重建门拒绝，两个分支都被踩到。
//
// 事务替身只透传闭包：事务纪律本身由适配器真库用例证（无事务登记被拒），登记口的
// 职责只是把用例包进一个事务。

type passthroughTransactor struct{}

func (passthroughTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	return fn(ctx)
}

type cardRegistrarDouble struct {
	outcome application.RegisterPriceCardOutcome
	err     error
	calls   int
}

func (double *cardRegistrarDouble) Handle(
	_ context.Context,
	_ application.RegisterPriceCardCommand,
) (application.RegisterPriceCardOutcome, error) {
	double.calls++
	return double.outcome, double.err
}

type seriesRegistrarDouble struct {
	outcome application.RegisterReferenceSeriesOutcome
	err     error
	calls   int
}

func (double *seriesRegistrarDouble) Handle(
	_ context.Context,
	_ application.RegisterReferenceSeriesCommand,
) (application.RegisterReferenceSeriesOutcome, error) {
	double.calls++
	return double.outcome, double.err
}

type reviewerDouble struct {
	outcome application.ReviewReferenceSeriesOutcome
	err     error
	calls   int
	last    application.ReviewReferenceSeriesCommand
}

func (double *reviewerDouble) Handle(
	_ context.Context,
	command application.ReviewReferenceSeriesCommand,
) (application.ReviewReferenceSeriesOutcome, error) {
	double.calls++
	double.last = command
	return double.outcome, double.err
}

const reviewJSON = `{"tenant":"tenant-1","seriesId":"SYN-PRC-FUEL-WEEKLY","seriesVersion":"v1",
	"reviewer":"SYN-PRC-SERIES-REVIEWER","decision":"APPROVED","basis":"SYN-REVIEW/逐期核对",
	"reviewedAt":"2026-08-20T09:00:00Z"}`

// TestExecuteRoutesReviewDocumentToTheReviewer 证第三种登记：复核文档译成用例命令（时刻按
// RFC 3339 解、缺时留零给用例取时钟），`已记录`译成 0；治理答案（不在册 / 四眼不满足 / 冲突）
// 译成 2；坏文档在入库前拒成 1。
func TestExecuteRoutesReviewDocumentToTheReviewer(t *testing.T) {
	reviewer := &reviewerDouble{outcome: application.SeriesReviewRecorded}
	message, code := execute(t.Context(), kindReferenceSeriesReview, []byte(reviewJSON), &cardRegistrarDouble{}, &seriesRegistrarDouble{}, reviewer, passthroughTransactor{})
	if code != exitRegistered || !strings.Contains(message, "RECORDED") {
		t.Fatalf("message=%q code=%d, 想要 RECORDED/0", message, code)
	}
	if reviewer.calls != 1 || reviewer.last.SeriesID != "SYN-PRC-FUEL-WEEKLY" || reviewer.last.SeriesVersion != "v1" ||
		reviewer.last.Decision != domain.SeriesReviewApproved || !reviewer.last.ReviewedAt.Equal(time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("命令翻译走样：%+v", reviewer.last)
	}

	without := &reviewerDouble{outcome: application.SeriesReviewRecorded}
	execute(t.Context(), kindReferenceSeriesReview, []byte(`{"tenant":"tenant-1","seriesId":"s","seriesVersion":"v1","reviewer":"r","decision":"APPROVED","basis":"b"}`), &cardRegistrarDouble{}, &seriesRegistrarDouble{}, without, passthroughTransactor{})
	if !without.last.ReviewedAt.IsZero() {
		t.Fatal("缺 reviewedAt 时本工具自己填了时刻——那是用例取时钟的事")
	}

	for label, outcome := range map[string]application.ReviewReferenceSeriesOutcome{
		"版本不在册": application.SeriesReviewVersionUnknown,
		"四眼不满足": application.SeriesReviewNeedsAnotherReviewer,
		"同键异内容": application.SeriesReviewConflict,
	} {
		_, code := execute(t.Context(), kindReferenceSeriesReview, []byte(reviewJSON), &cardRegistrarDouble{}, &seriesRegistrarDouble{}, &reviewerDouble{outcome: outcome}, passthroughTransactor{})
		if code != exitGovernance {
			t.Fatalf("%s: code = %d, 想要 %d", label, code, exitGovernance)
		}
	}

	untouched := &reviewerDouble{outcome: application.SeriesReviewRecorded}
	if _, code := execute(t.Context(), kindReferenceSeriesReview, []byte(`{"tenant":"tenant-1","reviewedAt":"yesterday"}`), &cardRegistrarDouble{}, &seriesRegistrarDouble{}, untouched, passthroughTransactor{}); code != exitUsage || untouched.calls != 0 {
		t.Fatalf("坏时刻 code = %d calls = %d, 想要 1 且不到达用例", code, untouched.calls)
	}
	if _, code := execute(t.Context(), kindReferenceSeriesReview, []byte(`not json`), &cardRegistrarDouble{}, &seriesRegistrarDouble{}, untouched, passthroughTransactor{}); code != exitUsage || untouched.calls != 0 {
		t.Fatalf("坏 JSON code = %d calls = %d", code, untouched.calls)
	}
}

// TestSeedReviewDocumentsParse 把 seedgen 产出的两份复核种子过一遍本工具的文档翻译：字段名
// 由两处各写一遍（seedgen 的 map 键与这里的 reviewDocument 标签），没有类型把它们钉在一起，
// 这条用例就是那根钉。复核责任方 ≠ 登记责任方在这里也顺手核一次——种子若同人登记又复核，
// 跑 seed.sh 时才会在四眼门上红。
func TestSeedReviewDocumentsParse(t *testing.T) {
	for _, name := range []string{"reference-series-fuel-review.json", "reference-series-fx-cny-sgd-review.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing", name))
		if err != nil {
			t.Fatalf("读种子 %s：%v", name, err)
		}
		command, err := parseReviewDocument(raw)
		if err != nil {
			t.Fatalf("种子 %s 过不了文档翻译：%v", name, err)
		}
		if command.SeriesID == "" || command.SeriesVersion == "" || command.Reviewer == "" || command.Basis == "" ||
			command.Decision != domain.SeriesReviewApproved || command.ReviewedAt.IsZero() {
			t.Fatalf("种子 %s 字段不齐：%+v", name, command)
		}
		if command.Reviewer == "SYN-PRICING-OPS-01" {
			t.Fatalf("种子 %s 的复核责任方就是登记责任方，四眼门会拒", name)
		}
	}
}

func seriesSnapshot(t *testing.T) []byte {
	t.Helper()
	reference, err := domain.NewVersionReference(
		domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v1", "sha256:syn-fuel")
	if err != nil {
		t.Fatalf("构造序列引用：%v", err)
	}
	value, err := domain.ParseDecimal("0.22")
	if err != nil {
		t.Fatalf("构造取值：%v", err)
	}
	period, err := domain.NewSeriesPeriodValue(
		time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		value,
		"SYN-EVIDENCE/fuel-2026-W32",
	)
	if err != nil {
		t.Fatalf("构造期次：%v", err)
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("构造租户：%v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           tenant,
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        reference,
		SourceIdentifier: "SYN-CARRIER/fuel-weekly-bulletin",
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		Periods:          []domain.SeriesPeriodValue{period},
	})
	if err != nil {
		t.Fatalf("构造序列登记：%v", err)
	}
	raw, err := domain.MarshalReferenceSeriesRegistration(registration)
	if err != nil {
		t.Fatalf("折装序列快照：%v", err)
	}
	return raw
}

// TestExecuteRoutesReferenceSeriesToItsRegistrar 证绿路径：合法序列快照过重建门、
// 进登记用例，`已登记`译成退出码 0。
func TestExecuteRoutesReferenceSeriesToItsRegistrar(t *testing.T) {
	series := &seriesRegistrarDouble{outcome: application.ReferenceSeriesRecorded}
	cards := &cardRegistrarDouble{}

	message, code := execute(t.Context(), kindReferenceSeries, seriesSnapshot(t), cards, series, &reviewerDouble{}, passthroughTransactor{})
	if code != exitRegistered || !strings.Contains(message, "RECORDED") {
		t.Fatalf("message=%q code=%d, 想要 RECORDED/0", message, code)
	}
	if series.calls != 1 || cards.calls != 0 {
		t.Fatalf("路由错位：series=%d cards=%d", series.calls, cards.calls)
	}
}

// TestExecuteTranslatesGovernanceAnswers 证治理答案的退出码：冲突与形状不同都是
// 登记册答案（原行未被顶替），译成退出码 2，不冒充成功也不冒充故障。
func TestExecuteTranslatesGovernanceAnswers(t *testing.T) {
	for label, outcome := range map[string]application.RegisterReferenceSeriesOutcome{
		"版本内容冲突": application.ReferenceSeriesRegistrationConflict,
		"形状不可比":  application.ReferenceSeriesRegistrationIncomparable,
	} {
		series := &seriesRegistrarDouble{outcome: outcome}
		_, code := execute(t.Context(), kindReferenceSeries, seriesSnapshot(t), &cardRegistrarDouble{}, series, &reviewerDouble{}, passthroughTransactor{})
		if code != exitGovernance {
			t.Fatalf("%s: code = %d, 想要 %d", label, code, exitGovernance)
		}
	}
}

// TestExecuteRefusesAtTheRehydrationGate 证重建门在入库前拒：装不成登记的字节不到达
// 用例（价卡分支同规则——两条路由结构同形）。
func TestExecuteRefusesAtTheRehydrationGate(t *testing.T) {
	cards := &cardRegistrarDouble{outcome: application.PriceCardRecorded}
	message, code := execute(t.Context(), kindPriceCard, []byte(`{"not":"a-card"}`), cards, &seriesRegistrarDouble{}, &reviewerDouble{}, passthroughTransactor{})
	if code != exitUsage {
		t.Fatalf("message=%q code=%d, 想要重建门拒绝/1", message, code)
	}
	if cards.calls != 0 {
		t.Fatal("装不成登记的字节到达了用例")
	}

	if _, code := execute(t.Context(), "moon-phase", []byte(`{}`), cards, &seriesRegistrarDouble{}, &reviewerDouble{}, passthroughTransactor{}); code != exitUsage {
		t.Fatalf("未知种类 code = %d, 想要 %d", code, exitUsage)
	}
}

// TestExecuteSurfacesUndecided 证依赖故障译成退出码 3：登记与否未知，原因在消息里。
func TestExecuteSurfacesUndecided(t *testing.T) {
	boom := errors.New("register store is down")
	series := &seriesRegistrarDouble{err: boom}
	message, code := execute(t.Context(), kindReferenceSeries, seriesSnapshot(t), &cardRegistrarDouble{}, series, &reviewerDouble{}, passthroughTransactor{})
	if code != exitUndecided || !strings.Contains(message, "register store is down") {
		t.Fatalf("message=%q code=%d, 想要未决/3 且带成因", message, code)
	}
}
