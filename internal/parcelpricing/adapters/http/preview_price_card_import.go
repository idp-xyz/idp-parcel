package pricinghttp

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 价卡导入预览口（ADR-0101 决定四；票 price-card-import/02）：上传一份模板文件，答已校验（带内容
// 摘要与方案概要）、带问题（逐格坐标）或未受理，不写库。租户来自操作者信封，与录入口等的是同一样
// 东西，所以同挂字面量 UnconfiguredIntake{}；隔离读放行装不进它（编译期）。

// PriceCardPreviewIntake 把一次已认证的接入请求翻译成预览命令：上传的文件 + 信封里的租户。
type PriceCardPreviewIntake interface {
	IntakePriceCardPreview(ctx context.Context, request *http.Request) (application.PreviewPriceCardImportCommand, error)
}

// PriceCardImportPreviewer 是本端点转交的预览编排。
type PriceCardImportPreviewer interface {
	Handle(ctx context.Context, command application.PreviewPriceCardImportCommand) (application.PriceCardImportPreview, error)
}

func NewPreviewPriceCardImportEndpoint(intake PriceCardPreviewIntake, previewer PriceCardImportPreviewer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakePriceCardPreview(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		preview, err := previewer.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if preview.Outcome.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		writeJSON(response, http.StatusOK, priceCardPreviewResponseOf(preview))
	})
}

type priceCardPreviewResponse struct {
	Outcome         string                 `json:"outcome"`
	TemplateVersion string                 `json:"templateVersion,omitempty"`
	SourceFile      *sourceFileBody        `json:"sourceFile,omitempty"`
	Plan            *planReferenceBody     `json:"plan,omitempty"`
	Content         *priceCardContentBody  `json:"content,omitempty"`
	Problems        []priceCardProblemBody `json:"problems"`
}

type sourceFileBody struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// priceCardContentBody 是已校验方案的概要：给人核对「读成了哪一张卡」，不是方案全文——全文在
// 发布后经价卡目录读口查。
type priceCardContentBody struct {
	Canonicalization       string              `json:"canonicalization"`
	ContentDigest          string              `json:"contentDigest"`
	Scope                  string              `json:"scope"`
	Direction              string              `json:"direction"`
	Purpose                string              `json:"purpose"`
	Aggregation            string              `json:"aggregation"`
	BaseChargeCode         string              `json:"baseChargeCode"`
	StartsAt               string              `json:"startsAt"`
	EndsAt                 string              `json:"endsAt,omitempty"`
	RateTable              priceCardTableBody  `json:"rateTable"`
	WeightPolicy           priceCardWeightBody `json:"weightPolicy"`
	FixedCharges           int                 `json:"fixedCharges"`
	Surcharges             int                 `json:"surcharges"`
	Manifest               []manifestEntryBody `json:"manifest"`
	DirectionAuthorization planReferenceBody   `json:"directionAuthorization"`
}

type priceCardTableBody struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	Family     string `json:"family"`
	Currency   string `json:"currency"`
	WeightUnit string `json:"weightUnit"`
	Rows       int    `json:"rows"`
}

type priceCardWeightBody struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Method  string `json:"method"`
}

type priceCardProblemBody struct {
	Sheet   string `json:"sheet"`
	Row     int    `json:"row"`
	Column  string `json:"column"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func priceCardPreviewResponseOf(preview application.PriceCardImportPreview) priceCardPreviewResponse {
	body := priceCardPreviewResponse{
		Outcome:         preview.Outcome.String(),
		TemplateVersion: preview.TemplateVersion,
		Problems:        make([]priceCardProblemBody, 0, len(preview.Problems)),
	}
	if preview.SourceFile.Name() != "" {
		body.SourceFile = &sourceFileBody{Name: preview.SourceFile.Name(), SHA256: preview.SourceFile.SHA256()}
	}
	if preview.Plan.ID() != "" {
		plan := planReferenceBodyOf(preview.Plan)
		body.Plan = &plan
	}
	for _, problem := range preview.Problems {
		body.Problems = append(body.Problems, priceCardProblemBody{
			Sheet: problem.Sheet, Row: problem.Row, Column: problem.Column, Code: string(problem.Code), Message: problem.Message,
		})
	}
	if preview.Content != nil {
		content := priceCardContentBodyOf(preview.Content.Plan)
		content.DirectionAuthorization = planReferenceBodyOf(preview.Content.DirectionAuthorization)
		body.Content = &content
	}
	return body
}

func priceCardContentBodyOf(plan domain.PricingPlanVersion) priceCardContentBody {
	table, weight := plan.RateTable(), plan.WeightPolicy()
	body := priceCardContentBody{
		Canonicalization: plan.CanonicalizationVersion(),
		ContentDigest:    plan.ContentDigest(),
		Scope:            plan.Scope().String(),
		Direction:        string(plan.Direction()),
		Purpose:          string(plan.Purpose()),
		Aggregation:      string(plan.Aggregation()),
		BaseChargeCode:   plan.BaseChargeCode().String(),
		StartsAt:         rfc3339(plan.EffectivePeriod().StartsAt()),
		RateTable: priceCardTableBody{
			ID: table.Reference().ID(), Version: table.Reference().Version(), Family: string(table.Family()),
			Currency: table.Currency().String(), WeightUnit: string(table.WeightUnit()),
			Rows: len(table.Entries()) + len(table.FirstContinueRates()) + len(table.UnitPriceRates()),
		},
		WeightPolicy: priceCardWeightBody{
			ID: weight.Reference().ID(), Version: weight.Reference().Version(), Method: string(weight.Method()),
		},
		FixedCharges: len(plan.Rules()),
		Surcharges:   len(plan.Structures().SurchargeRules()),
		Manifest:     []manifestEntryBody{},
	}
	if endsAt := plan.EffectivePeriod().EndsAt(); !endsAt.IsZero() {
		body.EndsAt = rfc3339(endsAt)
	}
	for _, reference := range plan.Manifest().References() {
		body.Manifest = append(body.Manifest, manifestEntryBody{Kind: string(reference.Kind()), ID: reference.ID(), Version: reference.Version()})
	}
	return body
}

// PriceCardUploadField 是上传表单里文件那一格的字段名。
const PriceCardUploadField = "file"

// maxPriceCardUploadBytes 比模板读口的文件上限（pricecardtemplate.MaxFileBytes）多留 1 MiB 给表单封套：
// 文件本身超限由读口如实答「文件过大」，这里截断请求体只为不无界读入。
const maxPriceCardUploadBytes = 11 << 20

// PriceCardUpload 是解出来的一次上传：文件名与原始字节。
type PriceCardUpload struct {
	FileName string
	Raw      []byte
}

// DecodePriceCardUpload 解 multipart/form-data 上传：表单里恰有一个文件格、没有别的格。多出来的格
// 一律不收——身份不从载荷里来，按未知键拒同 JSON 载荷。错误一律包 ErrMalformedRequest。
func DecodePriceCardUpload(request *http.Request) (PriceCardUpload, error) {
	request.Body = http.MaxBytesReader(nil, request.Body, maxPriceCardUploadBytes)
	if err := request.ParseMultipartForm(maxPriceCardUploadBytes); err != nil {
		return PriceCardUpload{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	form := request.MultipartForm
	defer func() { _ = form.RemoveAll() }()
	if len(form.Value) != 0 || len(form.File) != 1 || len(form.File[PriceCardUploadField]) != 1 {
		return PriceCardUpload{}, fmt.Errorf("%w: the form carries exactly one %q file and nothing else", ErrMalformedRequest, PriceCardUploadField)
	}
	header := form.File[PriceCardUploadField][0]
	file, err := header.Open()
	if err != nil {
		return PriceCardUpload{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		return PriceCardUpload{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return PriceCardUpload{FileName: header.Filename, Raw: raw}, nil
}

// PreviewCommand 把上传与信封里的租户折成预览命令。
func (upload PriceCardUpload) PreviewCommand(tenant domain.TenantID) application.PreviewPriceCardImportCommand {
	return application.PreviewPriceCardImportCommand{Tenant: tenant, FileName: upload.FileName, Raw: upload.Raw}
}
