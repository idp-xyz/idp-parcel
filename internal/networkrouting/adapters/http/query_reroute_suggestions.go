package networkhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// RerouteSuggestionCatalogueReader 是改路建议读面消费的读口（ADR-0284 决定六）。
type RerouteSuggestionCatalogueReader interface {
	ListRerouteSuggestions(ctx context.Context, tenant domain.TenantID, limit int) ([]ports.RerouteSuggestionCatalogueRow, error)
}

var _ RerouteSuggestionCatalogueReader = ports.RerouteSuggestionCatalogueRead(nil)

// outcomeRerouteSuggestionsListed 是读面唯一的业务结果：空册如实答空列表。
const outcomeRerouteSuggestionsListed = "REROUTE_SUGGESTIONS_LISTED"

// NewQueryRerouteSuggestionsEndpoint 交回改路建议读面（GET /route-reroute-suggestions）。复用本包目录查阅的 Intake：建议同属运营
// 查阅，授权边界同是租户。状态只答此刻的大概，决定成不成立仍由决定口在判断键锁里判。
func NewQueryRerouteSuggestionsEndpoint(intake CatalogueQueryIntake, reader RerouteSuggestionCatalogueReader) http.Handler {
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
		rows, err := reader.ListRerouteSuggestions(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			// 读不回是答案未形成（5xx），不伪装成空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]rerouteSuggestionBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, rerouteSuggestionBodyOf(row))
		}
		writeJSON(response, http.StatusOK, rerouteSuggestionListResponse{
			Outcome:     outcomeRerouteSuggestionsListed,
			Suggestions: bodies,
		})
	})
}

type rerouteSuggestionListResponse struct {
	Outcome     string                  `json:"outcome"`
	Suggestions []rerouteSuggestionBody `json:"suggestions"`
}

// rerouteSuggestionBody 逐字段透出一份建议。候选的结局与依据是建议形成那一刻的评估；`state` 是 PENDING / DECIDED / SUPERSEDED
// 原词；`decision` 只随已决定在场。
type rerouteSuggestionBody struct {
	CorrelationID      string                       `json:"correlationId"`
	CustomerAccountID  string                       `json:"customerAccountId"`
	ShipmentRequestID  string                       `json:"shipmentRequestId"`
	AcceptanceBaseline string                       `json:"acceptanceBaseline"`
	DeclaredParcelID   string                       `json:"declaredParcelId"`
	ServicePurpose     string                       `json:"servicePurpose"`
	ReviewedPlan       string                       `json:"reviewedPlan,omitempty"`
	LapseBasis         string                       `json:"lapseBasis,omitempty"`
	Blockers           []string                     `json:"blockers"`
	Candidates         []rerouteSuggestionCandidate `json:"candidates"`
	SuggestedAt        string                       `json:"suggestedAt"`
	TriggerOccurredAt  string                       `json:"triggerOccurredAt,omitempty"`
	State              string                       `json:"state"`
	Decision           *rerouteSuggestionDecision   `json:"decision,omitempty"`
}

type rerouteSuggestionCandidate struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

type rerouteSuggestionDecision struct {
	Candidate      string `json:"candidate"`
	NewPlanVersion string `json:"newPlanVersion"`
	DecidedBy      string `json:"decidedBy"`
	Basis          string `json:"basis"`
	DecidedAt      string `json:"decidedAt"`
}

func rerouteSuggestionBodyOf(row ports.RerouteSuggestionCatalogueRow) rerouteSuggestionBody {
	body := rerouteSuggestionBody{
		CorrelationID:      row.CorrelationID,
		CustomerAccountID:  row.CustomerAccountID,
		ShipmentRequestID:  row.ShipmentRequestID,
		AcceptanceBaseline: row.AcceptanceBaseline,
		DeclaredParcelID:   row.DeclaredParcelID,
		ServicePurpose:     row.ServicePurpose,
		ReviewedPlan:       row.ReviewedPlan,
		LapseBasis:         row.LapseBasis,
		Blockers:           append([]string{}, row.Blockers...),
		Candidates:         make([]rerouteSuggestionCandidate, 0, len(row.Candidates)),
		SuggestedAt:        utcText(row.SuggestedAt),
		State:              string(row.State),
	}
	if !row.TriggerOccurredAt.IsZero() {
		body.TriggerOccurredAt = utcText(row.TriggerOccurredAt)
	}
	for _, candidate := range row.Candidates {
		body.Candidates = append(body.Candidates, rerouteSuggestionCandidate(candidate))
	}
	if row.HasDecision {
		body.Decision = &rerouteSuggestionDecision{
			Candidate:      row.Decision.Candidate,
			NewPlanVersion: row.Decision.NewPlanVersion,
			DecidedBy:      row.Decision.DecidedBy,
			Basis:          row.Decision.Basis,
			DecidedAt:      utcText(row.Decision.DecidedAt),
		}
	}
	return body
}
