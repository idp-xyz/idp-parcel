package main

import (
	"context"
	"errors"
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

	message, code := execute(t.Context(), kindReferenceSeries, seriesSnapshot(t), cards, series, passthroughTransactor{})
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
		_, code := execute(t.Context(), kindReferenceSeries, seriesSnapshot(t), &cardRegistrarDouble{}, series, passthroughTransactor{})
		if code != exitGovernance {
			t.Fatalf("%s: code = %d, 想要 %d", label, code, exitGovernance)
		}
	}
}

// TestExecuteRefusesAtTheRehydrationGate 证重建门在入库前拒：装不成登记的字节不到达
// 用例（价卡分支同规则——两条路由结构同形）。
func TestExecuteRefusesAtTheRehydrationGate(t *testing.T) {
	cards := &cardRegistrarDouble{outcome: application.PriceCardRecorded}
	message, code := execute(t.Context(), kindPriceCard, []byte(`{"not":"a-card"}`), cards, &seriesRegistrarDouble{}, passthroughTransactor{})
	if code != exitUsage {
		t.Fatalf("message=%q code=%d, 想要重建门拒绝/1", message, code)
	}
	if cards.calls != 0 {
		t.Fatal("装不成登记的字节到达了用例")
	}

	if _, code := execute(t.Context(), "moon-phase", []byte(`{}`), cards, &seriesRegistrarDouble{}, passthroughTransactor{}); code != exitUsage {
		t.Fatalf("未知种类 code = %d, 想要 %d", code, exitUsage)
	}
}

// TestExecuteSurfacesUndecided 证依赖故障译成退出码 3：登记与否未知，原因在消息里。
func TestExecuteSurfacesUndecided(t *testing.T) {
	boom := errors.New("register store is down")
	series := &seriesRegistrarDouble{err: boom}
	message, code := execute(t.Context(), kindReferenceSeries, seriesSnapshot(t), &cardRegistrarDouble{}, series, passthroughTransactor{})
	if code != exitUndecided || !strings.Contains(message, "register store is down") {
		t.Fatalf("message=%q code=%d, 想要未决/3 且带成因", message, code)
	}
}
