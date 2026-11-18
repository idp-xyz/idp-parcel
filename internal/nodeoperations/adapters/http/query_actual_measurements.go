package nodeopshttp

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件是节点作业查阅页的实际测量册（registry=measurement）。册上是全部测量，含已作废的：
// 作废是另一条事实指回来源身份，不改这一行的数字。voided 只说有没有作废指过来。

const outcomeActualMeasurementsListed = "MEASUREMENTS_LISTED"

func serveActualMeasurements(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) {
	page, err := reader.ListActualMeasurements(request.Context(), tenant, limit, query)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]actualMeasurementBody, 0, len(page.Rows))
	for _, row := range page.Rows {
		bodies = append(bodies, actualMeasurementBody{
			SourceID:         row.SourceID,
			ObjectKind:       row.ObjectKind,
			ObjectID:         row.ObjectID,
			WeightValue:      row.WeightValue,
			WeightUnit:       row.WeightUnit,
			Length:           row.Length,
			Width:            row.Width,
			Height:           row.Height,
			DimensionUnit:    row.DimensionUnit,
			OccurredAt:       rfc3339(row.OccurredAt),
			RecordedAt:       rfc3339(row.RecordedAt),
			Location:         row.Location,
			Basis:            row.Basis,
			PerformedBy:      row.PerformedBy,
			Voided:           row.Voided,
			EnteredBy:        row.EnteredBy,
			OriginalDocument: row.OriginalDocument,
		})
	}
	writeJSON(response, http.StatusOK, actualMeasurementListResponse{
		Outcome:      outcomeActualMeasurementsListed,
		Measurements: bodies,
		Page:         page.Page(limit),
	})
}

type actualMeasurementListResponse struct {
	Outcome      string                  `json:"outcome"`
	Measurements []actualMeasurementBody `json:"measurements"`
	Page         cataloguepage.Page      `json:"page"`
}

// actualMeasurementBody 逐字段透出一行测量。objectKind 是封闭词原样透出。重量与三边是登记时的原数，各带
// 自己的单位，恒在场，不另算。voided 恒在场：false 是仍有效，不是「没问到」。录入人与原件只在手工录入时出现。
type actualMeasurementBody struct {
	SourceID         string `json:"sourceId"`
	ObjectKind       string `json:"objectKind"`
	ObjectID         string `json:"objectId"`
	WeightValue      string `json:"weightValue"`
	WeightUnit       string `json:"weightUnit"`
	Length           string `json:"length"`
	Width            string `json:"width"`
	Height           string `json:"height"`
	DimensionUnit    string `json:"dimensionUnit"`
	OccurredAt       string `json:"occurredAt"`
	RecordedAt       string `json:"recordedAt"`
	Location         string `json:"location"`
	Basis            string `json:"basis"`
	PerformedBy      string `json:"performedBy"`
	Voided           bool   `json:"voided"`
	EnteredBy        string `json:"enteredBy,omitempty"`
	OriginalDocument string `json:"originalDocument,omitempty"`
}
