package pricinghttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// PriceCardWorkbookReader 是价卡工作簿读口转交的编排。
type PriceCardWorkbookReader interface {
	Handle(ctx context.Context, query application.ReadPriceCardWorkbookQuery) (application.ReadPriceCardWorkbookResult, error)
}

// NewQueryPriceCardWorkbookEndpoint 交回工作簿读口
// （GET /pricing-price-cards/workbook?id=<方案标识>&version=<版本>&carrier=workbook|spreadsheet）。
//
// carrier 缺省是结构化工作簿。spreadsheet 把同一份投影下载成电子表格。表达不了、查无、只有问题清单的草稿，
// 都答 200 加业务结论，不交残缺文件，也不是 500。租户只取认证信封；查询串只认 id、version 与可选的 carrier。
func NewQueryPriceCardWorkbookEndpoint(intake PricingCatalogueIntake, reader PriceCardWorkbookReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		reference, carrier, err := decodePriceCardWorkbookQuery(request)
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		result, err := reader.Handle(request.Context(), application.ReadPriceCardWorkbookQuery{
			Tenant: query.Scope.Tenant(), Plan: reference, Carrier: carrier,
		})
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if result.Outcome == application.PriceCardWorkbookReady && carrier == ports.PriceCardWorkbookSpreadsheet {
			if len(result.Body) == 0 {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			response.Header().Set("Content-Type", result.MediaType)
			response.Header().Set("Content-Disposition", `attachment; filename="`+result.FileName+`"`)
			response.WriteHeader(http.StatusOK)
			_, _ = response.Write(result.Body)
			return
		}
		if result.Outcome.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body := priceCardWorkbookResponse{
			Outcome:          result.Outcome.String(),
			PlanID:           result.PlanID,
			PlanVersion:      result.PlanVersion,
			Origin:           result.Origin.String(),
			TemplateVersion:  result.TemplateVersion,
			Canonicalization: result.Canonicalization,
			ContentDigest:    result.ContentDigest,
			Where:            result.Where,
			Conclusion:       result.Conclusion,
		}
		if result.Outcome == application.PriceCardWorkbookReady {
			body.Workbook = json.RawMessage(result.Body)
		}
		writeJSON(response, http.StatusOK, body)
	})
}

type priceCardWorkbookResponse struct {
	Outcome          string          `json:"outcome"`
	PlanID           string          `json:"planId"`
	PlanVersion      string          `json:"planVersion"`
	Origin           string          `json:"origin,omitempty"`
	TemplateVersion  string          `json:"templateVersion,omitempty"`
	Canonicalization string          `json:"canonicalization,omitempty"`
	ContentDigest    string          `json:"contentDigest,omitempty"`
	Where            string          `json:"where,omitempty"`
	Conclusion       string          `json:"conclusion,omitempty"`
	Workbook         json.RawMessage `json:"workbook,omitempty"`
}

func decodePriceCardWorkbookQuery(request *http.Request) (domain.VersionReference, ports.PriceCardWorkbookCarrier, error) {
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return domain.VersionReference{}, 0, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	for key := range values {
		if key != priceCardContentIDParameter && key != priceCardContentVersionParameter && key != priceCardWorkbookCarrierParameter {
			return domain.VersionReference{}, 0, fmt.Errorf("%w: unknown query parameter %q", ErrMalformedRequest, key)
		}
	}
	id, err := singleQueryValue(values, priceCardContentIDParameter)
	if err != nil {
		return domain.VersionReference{}, 0, err
	}
	version, err := singleQueryValue(values, priceCardContentVersionParameter)
	if err != nil {
		return domain.VersionReference{}, 0, err
	}
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, id, version)
	if err != nil {
		return domain.VersionReference{}, 0, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	carrier := ports.PriceCardWorkbookStructured
	if _, present := values[priceCardWorkbookCarrierParameter]; present {
		raw, err := singleQueryValue(values, priceCardWorkbookCarrierParameter)
		if err != nil {
			return domain.VersionReference{}, 0, err
		}
		switch raw {
		case "workbook":
			carrier = ports.PriceCardWorkbookStructured
		case "spreadsheet":
			carrier = ports.PriceCardWorkbookSpreadsheet
		default:
			return domain.VersionReference{}, 0, fmt.Errorf("%w: carrier %q", ErrMalformedRequest, raw)
		}
	}
	return reference, carrier, nil
}

const priceCardWorkbookCarrierParameter = "carrier"
