package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 价卡导入预览（ADR-0101 决定三、四；票 price-card-import/02）：按上传字节算出源文件身份，
// 交模板读口读成三种读法之一，照实交回，不写库。录入口（票 03）复用同一个读口与同一段算法，
// 同一份字节过预览与过录入逐字节同摘要——预览若自己另读一遍，就会长出「预览通过、录入却不同」。

// PreviewPriceCardImportOutcome 是预览的答复。
type PreviewPriceCardImportOutcome uint8

const (
	PreviewPriceCardImportOutcomeInvalid PreviewPriceCardImportOutcome = iota
	// PriceCardImportValidated：没有任何问题，方案已过构造门、内容摘要已算出，即生命周期的「已校验」。
	PriceCardImportValidated
	// PriceCardImportHasProblems：方案身份读得出来，其余某些格有问题；逐格问题随答复交回。
	PriceCardImportHasProblems
	// PriceCardImportNotAccepted：命令立不住（缺租户、文件名不合格），或整份文件不收（带那一条问题）。
	PriceCardImportNotAccepted
)

func (outcome PreviewPriceCardImportOutcome) String() string {
	switch outcome {
	case PriceCardImportValidated:
		return "VALIDATED"
	case PriceCardImportHasProblems:
		return "HAS_PROBLEMS"
	case PriceCardImportNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// PreviewPriceCardImportCommand 是一次预览：租户取自操作者信封，文件名与字节取自上传。FromForm 为真时 Raw 是页面表单
// 提交的结构化工作簿（JSON），不是电子表格字节，FileName 不用：来源标识由规范化字节算出（ADR-0349）。
type PreviewPriceCardImportCommand struct {
	Tenant   domain.TenantID
	FileName string
	Raw      []byte
	FromForm bool
}

// PriceCardImportPreview 是预览的结果。SourceFile 在命令立得住时才有；Plan 在方案身份读得出来时
// 才有；Content 只在已校验时有。
type PriceCardImportPreview struct {
	Outcome         PreviewPriceCardImportOutcome
	TemplateVersion string
	SourceFile      domain.SourceFileIdentity
	Plan            domain.VersionReference
	Content         *ports.PriceCardTemplateContent
	Problems        []ports.PriceCardTemplateProblem
}

// ErrWorkbookCarrierNotWired：收到页面表单提交的工作簿，而本编排没有装配工作簿读口。是装配错误，不是业务答案。
var ErrWorkbookCarrierNotWired = errors.New("parcel pricing: price card import was handed a form workbook but no workbook reader is wired")

// PriceCardImportOption 给导入的两个编排（预览、录入）装可选件。
type PriceCardImportOption func(*priceCardImportOptions)

type priceCardImportOptions struct {
	workbook ports.PriceCardWorkbookReader
}

// WithWorkbookReader 装上页面表单提交的工作簿读口。预览与录入要装同一个，同一份内容过两口才得同一个来源标识与同一份读法。
func WithWorkbookReader(reader ports.PriceCardWorkbookReader) PriceCardImportOption {
	return func(options *priceCardImportOptions) { options.workbook = reader }
}

func importOptionsOf(options []PriceCardImportOption) priceCardImportOptions {
	var resolved priceCardImportOptions
	for _, option := range options {
		option(&resolved)
	}
	return resolved
}

// PreviewPriceCardImportHandler 只依赖模板读口：预览不写库，依赖结构上就没有仓储。
type PreviewPriceCardImportHandler struct {
	reader   ports.PriceCardTemplateReader
	workbook ports.PriceCardWorkbookReader
}

func NewPreviewPriceCardImportHandler(reader ports.PriceCardTemplateReader, options ...PriceCardImportOption) (*PreviewPriceCardImportHandler, error) {
	if reader == nil {
		return nil, errors.New("parcel pricing: price card import preview needs a template reader")
	}
	return &PreviewPriceCardImportHandler{reader: reader, workbook: importOptionsOf(options).workbook}, nil
}

func (handler *PreviewPriceCardImportHandler) Handle(_ context.Context, command PreviewPriceCardImportCommand) (PriceCardImportPreview, error) {
	if command.FromForm {
		if handler.workbook == nil {
			return PriceCardImportPreview{}, ErrWorkbookCarrierNotWired
		}
		return readPriceCardWorkbookImport(handler.workbook, command.Tenant, command.Raw), nil
	}
	return readPriceCardImport(handler.reader, command.Tenant, command.FileName, command.Raw), nil
}

// readPriceCardImport 是预览与录入共用的那一段：命令立不住（缺租户、文件名不合格）答未受理、不读文件；否则按上传
// 字节算源文件身份，交模板读口读成三种读法之一。
func readPriceCardImport(reader ports.PriceCardTemplateReader, tenant domain.TenantID, fileName string, raw []byte) PriceCardImportPreview {
	if tenant.String() == "" {
		return PriceCardImportPreview{Outcome: PriceCardImportNotAccepted}
	}
	source, err := sourceFileOf(fileName, raw)
	if err != nil {
		return PriceCardImportPreview{Outcome: PriceCardImportNotAccepted}
	}
	return previewOf(source, reader.ReadPriceCardTemplate(raw))
}

// readPriceCardWorkbookImport 是页面表单提交的那一条路：先把工作簿 JSON 规范化，来源标识取规范化字节的散列（名称 form:<散列前
// 12 位>），再读规范化字节。读法交给与读文件同一个 previewOf，三种形状与问题坐标因此一模一样；形状都没过的整份不收，
// 没有规范化字节也就没有来源标识。
func readPriceCardWorkbookImport(reader ports.PriceCardWorkbookReader, tenant domain.TenantID, raw []byte) PriceCardImportPreview {
	if tenant.String() == "" {
		return PriceCardImportPreview{Outcome: PriceCardImportNotAccepted}
	}
	canonical, err := reader.CanonicalizePriceCardWorkbook(raw)
	if err != nil {
		code, detail := ports.TemplateFileNotWorkbook, err.Error()
		var rejection ports.PriceCardWorkbookRejection
		if errors.As(err, &rejection) {
			detail = rejection.Detail
			if rejection.TooLarge {
				code = ports.TemplateFileTooLarge
			}
		}
		return PriceCardImportPreview{
			Outcome:  PriceCardImportNotAccepted,
			Problems: []ports.PriceCardTemplateProblem{{Code: code, Message: "读不出这份工作簿：" + detail}},
		}
	}
	digest := sha256.Sum256(canonical)
	source, err := domain.NewFormSourceFileIdentity(hex.EncodeToString(digest[:]))
	if err != nil {
		return PriceCardImportPreview{Outcome: PriceCardImportNotAccepted}
	}
	return previewOf(source, reader.ReadPriceCardWorkbook(canonical))
}

// previewOf 把读口交出的读法折成预览的答复，读文件与读表单工作簿共用。
func previewOf(source domain.SourceFileIdentity, reading ports.PriceCardTemplateReading) PriceCardImportPreview {
	preview := PriceCardImportPreview{
		TemplateVersion: reading.TemplateVersion,
		SourceFile:      source,
		Problems:        reading.Problems,
	}
	switch {
	case !reading.Accepted:
		preview.Outcome = PriceCardImportNotAccepted
	case reading.Content != nil:
		preview.Outcome, preview.Plan, preview.Content = PriceCardImportValidated, reading.Plan, reading.Content
	default:
		preview.Outcome, preview.Plan = PriceCardImportHasProblems, reading.Plan
	}
	return preview
}

// sourceFileOf 按上传字节算源文件身份：SHA-256 由服务端算，不采信调用方声明的哈希。
func sourceFileOf(name string, raw []byte) (domain.SourceFileIdentity, error) {
	digest := sha256.Sum256(raw)
	return domain.NewSourceFileIdentity(name, hex.EncodeToString(digest[:]))
}
