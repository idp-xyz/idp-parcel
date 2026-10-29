package pricinghttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 价卡导入模板规格只读口（GET /pricing-price-card-template/spec；ADR-0349）：答模板版本、表清单与每一列的定义，编辑器照它画
// 表单，而不是在前端抄一份列表。内容由读取模板的同一张列定义生成（ports.PriceCardTemplateSpecSource），租户无关，但仍挂
// 查阅行的 Intake——认证与授予照常，租户取认证信封，不因内容静态就免掉。

// PriceCardTemplateSpecReader 是本端点消费的读口。真实现是进程内的静态规格、不会失败；读口仍带错误返回，是为了和端点表里
// 每一条读行同形——装配测试的未接线占位如实答没接线，而不是让这一行从「接线是否到位」的核对里缺席。
type PriceCardTemplateSpecReader interface {
	ReadPriceCardTemplateSpec(ctx context.Context) (ports.PriceCardTemplateSpec, error)
}

// NewQueryPriceCardTemplateSpecEndpoint 交回规格口。先认证、后解查询串：令牌不过的请求不泄露本端点认哪些键；查询串封闭，
// 本口不收任何键，带一个 tenant 来就是以为自己能指定租户。
func NewQueryPriceCardTemplateSpecEndpoint(intake PricingCatalogueIntake, reader PriceCardTemplateSpecReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		if _, err := intake.IntakeCatalogueQuery(request.Context(), request); err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		if request.URL.RawQuery != "" {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		spec, err := reader.ReadPriceCardTemplateSpec(request.Context())
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, priceCardTemplateSpecResponseOf(spec))
	})
}

type priceCardTemplateSpecResponse struct {
	TemplateVersion          string                           `json:"templateVersion"`
	AcceptedTemplateVersions []string                         `json:"acceptedTemplateVersions"`
	Sheets                   []priceCardTemplateSheetSpecBody `json:"sheets"`
}

type priceCardTemplateSheetSpecBody struct {
	Name       string                            `json:"name"`
	Title      string                            `json:"title"`
	Purpose    string                            `json:"purpose"`
	Layout     string                            `json:"layout"`
	Repeatable bool                              `json:"repeatable"`
	Since      string                            `json:"since,omitempty"`
	Columns    []priceCardTemplateColumnSpecBody `json:"columns"`
}

type priceCardTemplateColumnSpecBody struct {
	Key           string   `json:"key"`
	Title         string   `json:"title"`
	Requirement   string   `json:"requirement"`
	Description   string   `json:"description"`
	AllowedValues []string `json:"allowedValues,omitempty"`
	MultiValued   bool     `json:"multiValued,omitempty"`
	// AllowedValuesWhen 只在「平时开放、满足条件才封闭」的列上有：同一行的 column 格等于 equals 时，本列只收 allowedValues。
	AllowedValuesWhen []priceCardTemplateValuesWhenBody `json:"allowedValuesWhen,omitempty"`
}

type priceCardTemplateValuesWhenBody struct {
	Column        string   `json:"column"`
	Equals        string   `json:"equals"`
	AllowedValues []string `json:"allowedValues"`
}

func priceCardTemplateSpecResponseOf(spec ports.PriceCardTemplateSpec) priceCardTemplateSpecResponse {
	body := priceCardTemplateSpecResponse{
		TemplateVersion:          spec.TemplateVersion,
		AcceptedTemplateVersions: append([]string{}, spec.AcceptedTemplateVersions...),
		Sheets:                   make([]priceCardTemplateSheetSpecBody, 0, len(spec.Sheets)),
	}
	for _, sheet := range spec.Sheets {
		out := priceCardTemplateSheetSpecBody{
			Name: sheet.Name, Title: sheet.Title, Purpose: sheet.Purpose, Layout: sheet.Layout,
			Repeatable: sheet.Repeatable, Since: sheet.Since,
			Columns: make([]priceCardTemplateColumnSpecBody, 0, len(sheet.Columns)),
		}
		for _, column := range sheet.Columns {
			body := priceCardTemplateColumnSpecBody{
				Key: column.Key, Title: column.Title, Requirement: column.Requirement, Description: column.Description,
				AllowedValues: column.AllowedValues, MultiValued: column.MultiValued,
			}
			for _, when := range column.AllowedValuesWhen {
				body.AllowedValuesWhen = append(body.AllowedValuesWhen, priceCardTemplateValuesWhenBody{Column: when.Column, Equals: when.Equals, AllowedValues: when.Values})
			}
			out.Columns = append(out.Columns, body)
		}
		body.Sheets = append(body.Sheets, out)
	}
	return body
}
