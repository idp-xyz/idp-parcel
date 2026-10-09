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

// PreviewPriceCardImportCommand 是一次预览：租户取自操作者信封，文件名与字节取自上传。
type PreviewPriceCardImportCommand struct {
	Tenant   domain.TenantID
	FileName string
	Raw      []byte
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

// PreviewPriceCardImportHandler 只依赖模板读口：预览不写库，依赖结构上就没有仓储。
type PreviewPriceCardImportHandler struct {
	reader ports.PriceCardTemplateReader
}

func NewPreviewPriceCardImportHandler(reader ports.PriceCardTemplateReader) (*PreviewPriceCardImportHandler, error) {
	if reader == nil {
		return nil, errors.New("parcel pricing: price card import preview needs a template reader")
	}
	return &PreviewPriceCardImportHandler{reader: reader}, nil
}

func (handler *PreviewPriceCardImportHandler) Handle(_ context.Context, command PreviewPriceCardImportCommand) (PriceCardImportPreview, error) {
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
	reading := reader.ReadPriceCardTemplate(raw)
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
