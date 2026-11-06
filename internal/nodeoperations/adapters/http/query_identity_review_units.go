package nodeopshttp

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件是节点作业查阅页按实物列的待识别实物册（registry=identity-review-unit）：一件有过待识别那一格的作业实物一行，带它各条判断链
// 此刻的当前有效版本，行的形状与 registry=unidentified-item 答复里的 units 同一个。那边的 units 只是这一册的第一页；往后翻、按实物
// 检索走这一口。

const (
	registryIdentityReviewUnit       = "identity-review-unit"
	outcomeIdentityReviewUnitsListed = "IDENTITY_REVIEW_UNITS_LISTED"
)

func serveIdentityReviewUnits(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) {
	page, err := reader.ListIdentityReviewUnits(request.Context(), tenant, limit, query)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	writeJSON(response, http.StatusOK, identityReviewUnitListResponse{
		Outcome: outcomeIdentityReviewUnitsListed,
		Units:   identityReviewUnitBodies(page.Rows),
		Page:    page.Page(limit),
	})
}

type identityReviewUnitListResponse struct {
	Outcome string                   `json:"outcome"`
	Units   []identityReviewUnitBody `json:"units"`
	Page    cataloguepage.Page       `json:"page"`
}
