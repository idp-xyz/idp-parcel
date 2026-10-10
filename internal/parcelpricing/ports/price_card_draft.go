package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 价卡草稿册的端口（ADR-0101 决定三；票 price-card-import/03）。单立一个文件而不进 price_card_catalog.go：草稿不是
// 价卡版本，任何评价读不到它；版本仓储只增不改，草稿册却在`已批准`之前换内容时替换那一行——理由同 ADR-0126
// 决定三：权威记录是发布后的价卡版本，那一册照旧只插不改。

// PriceCardDraftSubmitOutcome 是录入落点的封闭代数（ADR-0031：重放与修订都不是 error）。
type PriceCardDraftSubmitOutcome uint8

const (
	PriceCardDraftSubmitOutcomeInvalid PriceCardDraftSubmitOutcome = iota
	// PriceCardDraftSaved：新的一行。
	PriceCardDraftSaved
	// PriceCardDraftReplayed：同版同内容再录，行一字不动。
	PriceCardDraftReplayed
	// PriceCardDraftRevised：`已批准`之前换了内容，行被替换。
	PriceCardDraftRevised
	// PriceCardDraftContentFixed：草稿已`已批准`或`已发布`，内容固定，行一字不动。
	PriceCardDraftContentFixed
)

func (outcome PriceCardDraftSubmitOutcome) String() string {
	switch outcome {
	case PriceCardDraftSaved:
		return "SAVED"
	case PriceCardDraftReplayed:
		return "REPLAYED"
	case PriceCardDraftRevised:
		return "REVISED"
	case PriceCardDraftContentFixed:
		return "CONTENT_FIXED"
	default:
		return ""
	}
}

// PriceCardDraftRegister 是草稿册的写口，一版一行。落点由领域 PriceCardDraft.ResubmissionOf 判，写口只管落库与并发：
// 读册上那一行与替换它在同一笔事务里、且锁住那一行。按框架合同无事务即拒（RequireExecutor）。
type PriceCardDraftRegister interface {
	SubmitDraft(ctx context.Context, draft domain.PriceCardDraft) (PriceCardDraftSubmitOutcome, error)
}

// PriceCardDraftRead 是草稿册的查阅读口：按租户列出，status 为零值即四格都列，否则只列那一格。读回经领域重建门整图
// 重验；limit 必须为正，每页多大由接入面定（判据同目录读口）。
type PriceCardDraftRead interface {
	ListPriceCardDrafts(ctx context.Context, tenant domain.TenantID, status domain.PriceCardDraftStatus, limit int) ([]domain.PriceCardDraft, error)
}

// PriceCardDraftAdvanceOutcome 是推进落点的封闭代数（票 price-card-import/04）。
type PriceCardDraftAdvanceOutcome uint8

const (
	PriceCardDraftAdvanceOutcomeInvalid PriceCardDraftAdvanceOutcome = iota
	// PriceCardDraftAdvanced：册上那一行还是读到时的样子，已推进到交来的那一格。
	PriceCardDraftAdvanced
	// PriceCardDraftAdvanceNotFound：册上没有这一版。
	PriceCardDraftAdvanceNotFound
	// PriceCardDraftAdvanceSuperseded：读与写之间那一行被别的操作者动过（修订、或已被推进），行一字不动。
	PriceCardDraftAdvanceSuperseded
)

func (outcome PriceCardDraftAdvanceOutcome) String() string {
	switch outcome {
	case PriceCardDraftAdvanced:
		return "ADVANCED"
	case PriceCardDraftAdvanceNotFound:
		return "NOT_FOUND"
	case PriceCardDraftAdvanceSuperseded:
		return "SUPERSEDED"
	default:
		return ""
	}
}

// PriceCardDraftProgress 是批准与发布推进草稿的读写口：按键读一版，把推进后的那一份写回。写回只改状态与批准、发布的
// 痕迹，不重写内容文档；「还是读到时的那一行」按交来那一份的前一格与录入痕迹判，判不上即`已被替换`。按框架合同无事务
// 即拒（RequireExecutor）。
type PriceCardDraftProgress interface {
	LoadPriceCardDraft(ctx context.Context, tenant domain.TenantID, plan domain.VersionReference) (domain.PriceCardDraft, bool, error)
	AdvancePriceCardDraft(ctx context.Context, draft domain.PriceCardDraft) (PriceCardDraftAdvanceOutcome, error)
}
