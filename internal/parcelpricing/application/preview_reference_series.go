package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// PreviewReferenceSeriesOutcome 是登记前预览的应用处理结果。
type PreviewReferenceSeriesOutcome uint8

const (
	PreviewReferenceSeriesOutcomeInvalid PreviewReferenceSeriesOutcome = iota
	// ReferenceSeriesPreviewed：拟登版本过了领域构造门，等级、摘要与（若要求）逐期差异已算出。
	ReferenceSeriesPreviewed
	// ReferenceSeriesPreviewNotAccepted：请求不合法（零值登记），什么也没算。
	ReferenceSeriesPreviewNotAccepted
	// ReferenceSeriesPreviewUndecided：取对照版本时依赖故障，原因随错误交回。
	ReferenceSeriesPreviewUndecided
)

func (outcome PreviewReferenceSeriesOutcome) String() string {
	switch outcome {
	case ReferenceSeriesPreviewed:
		return "PREVIEWED"
	case ReferenceSeriesPreviewNotAccepted:
		return "NOT_ACCEPTED"
	case ReferenceSeriesPreviewUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// SeriesComparisonOutcome 说对照版本这一格的下场。三种「没有比出来」按续办动作分格：没要求
// 比就不比；要求了但那一版不在册，去查版本号；在册但不是同一条序列（种类不同），去查绑错了
// 哪条——三格都不是预览失败，预览的等级与摘要照样成立。
type SeriesComparisonOutcome uint8

const (
	// SeriesComparisonNotRequested：命令没指名对照版本，拟登版本也不是更正版本。
	SeriesComparisonNotRequested SeriesComparisonOutcome = iota
	// SeriesComparisonCompared：对照版本取回并逐期比对，差异随预览交回。
	SeriesComparisonCompared
	// SeriesComparisonBaseUnknown：指名（或回指）的对照版本不在册。
	SeriesComparisonBaseUnknown
	// SeriesComparisonBaseIncomparable：对照版本在册，但与拟登版本不是同一条序列——版本读口
	// 按（序列标识、版本号）取回，种类不合只有领域比对能看出来。
	SeriesComparisonBaseIncomparable
)

func (outcome SeriesComparisonOutcome) String() string {
	switch outcome {
	case SeriesComparisonNotRequested:
		return "NOT_REQUESTED"
	case SeriesComparisonCompared:
		return "COMPARED"
	case SeriesComparisonBaseUnknown:
		return "BASE_UNKNOWN"
	case SeriesComparisonBaseIncomparable:
		return "BASE_INCOMPARABLE"
	default:
		return ""
	}
}

// PreviewReferenceSeriesCommand 携带一次登记前预览：拟登的登记本体（已过领域构造门），以及
// 可选的对照版本号。对照版本按序取：指名的优先；没指名而拟登是更正版本，取它声明更正的那
// 一版；两样都没有就不比——延展版本没有更正关系可回指，要看与上一版差在哪只能指名。
type PreviewReferenceSeriesCommand struct {
	Registration       domain.ReferenceSeriesRegistration
	CompareWithVersion string
}

// ReferenceSeriesPreview 是预览的答复。等级、规范化版本与内容摘要都取自领域登记对象本身
// ——与登记册 Register 写下的是同一个方法的返回值（ADR-0101 决定四：校验与发布共用一份
// 摘要），这里不另算、不转写。Changes 只在 Comparison 为 Compared 时非空。
type ReferenceSeriesPreview struct {
	Outcome          PreviewReferenceSeriesOutcome
	EvidenceGrade    domain.SeriesEvidenceGrade
	Canonicalization string
	ContentDigest    string
	Comparison       SeriesComparisonOutcome
	BaseVersion      string
	Changes          []domain.SeriesPeriodChange
}

type PreviewReferenceSeriesDeps struct {
	Versions ports.ReferenceSeriesVersionLoader
}

// PreviewReferenceSeriesHandler 编排登记前预览（票 pricing-reference-series-operations/08 件②）。
// 依赖里**只有版本读口，没有登记册写口**：预览不写库、不进版本清单，任何评价读不到它——这一
// 点由依赖形状而不是由一句约定守着。
type PreviewReferenceSeriesHandler struct {
	deps PreviewReferenceSeriesDeps
}

func NewPreviewReferenceSeriesHandler(deps PreviewReferenceSeriesDeps) *PreviewReferenceSeriesHandler {
	return &PreviewReferenceSeriesHandler{deps: deps}
}

// Handle 把一次预览请求推进到答复。依赖故障不吞：预览是登记这个治理动作的前一步，操作者要
// 拿到原因才知道是自己的载荷有问题还是读口坏了。
func (handler *PreviewReferenceSeriesHandler) Handle(
	ctx context.Context,
	command PreviewReferenceSeriesCommand,
) (ReferenceSeriesPreview, error) {
	registration := command.Registration
	// 零值登记连租户都指不出来。构造器保证非零登记整体立得住，所以受理门只需分辨
	// 「根本没构造过」（判据同登记用例）。
	if registration.Tenant().String() == "" {
		return ReferenceSeriesPreview{Outcome: ReferenceSeriesPreviewNotAccepted}, nil
	}

	preview := ReferenceSeriesPreview{
		Outcome:          ReferenceSeriesPreviewed,
		EvidenceGrade:    registration.EvidenceGrade(),
		Canonicalization: registration.Canonicalization(),
		ContentDigest:    registration.ContentDigest(),
		Comparison:       SeriesComparisonNotRequested,
	}

	baseVersion := command.CompareWithVersion
	if baseVersion == "" {
		if prior, _, corrected := registration.Correction(); corrected {
			baseVersion = prior.Version()
		}
	}
	if baseVersion == "" {
		return preview, nil
	}
	preview.BaseVersion = baseVersion

	base, found, err := handler.deps.Versions.LoadVersion(ctx, registration.Tenant(), registration.Reference().ID(), baseVersion)
	if err != nil {
		return ReferenceSeriesPreview{Outcome: ReferenceSeriesPreviewUndecided}, fmt.Errorf("preview reference series: %w", err)
	}
	if !found {
		preview.Comparison = SeriesComparisonBaseUnknown
		return preview, nil
	}

	changes, err := domain.DiffSeriesPeriods(base, registration)
	switch {
	case errors.Is(err, domain.ErrSeriesPeriodDiffAcrossSeries):
		preview.Comparison = SeriesComparisonBaseIncomparable
		return preview, nil
	case err != nil:
		// 两版都过了构造门（对照那版还过了重建门），比对本不该拒；拒了是要人看的结构失败。
		return ReferenceSeriesPreview{Outcome: ReferenceSeriesPreviewUndecided}, fmt.Errorf("preview reference series: %w", err)
	}
	preview.Comparison = SeriesComparisonCompared
	preview.Changes = changes
	return preview, nil
}
