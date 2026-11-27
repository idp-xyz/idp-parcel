package commercialhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 范围叫法与服务产品名称。不进发布摘要。同内容再写答已登记，换叫法不覆盖。

type LabelNameStore interface {
	ListScopeNames(ctx context.Context, tenant domain.TenantID) ([]ports.CommercialScopeName, error)
	SaveScopeName(ctx context.Context, tenant domain.TenantID, scope, name string) (ports.LabelSaveOutcome, error)
	SaveDisplayName(ctx context.Context, tenant domain.TenantID, objectID, version, name string) (ports.LabelSaveOutcome, error)
}

func NewQueryScopeNamesEndpoint(intake CommercialCatalogueIntake, store LabelNameStore) http.Handler {
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
		names, err := store.ListScopeNames(request.Context(), query.Scope.Tenant())
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body := make([]scopeNameBody, 0, len(names))
		for _, name := range names {
			body = append(body, scopeNameBody{Scope: name.Scope, Name: name.Name})
		}
		writeJSON(response, http.StatusOK, scopeNameListResponse{Outcome: "SCOPE_NAMES_LISTED", Scopes: body})
	})
}

func NewRegisterScopeNameEndpoint(intake CommercialCatalogueIntake, store LabelNameStore) http.Handler {
	return newLabelEndpoint(intake, store, func(ctx context.Context, store LabelNameStore, tenant domain.TenantID, raw json.RawMessage) (ports.LabelSaveOutcome, error) {
		var body scopeNameBody
		if err := json.Unmarshal(raw, &body); err != nil {
			return "", errMalformed{detail: "不是一份对象"}
		}
		if strings.TrimSpace(body.Scope) == "" || strings.TrimSpace(body.Name) == "" {
			return "", errMalformed{detail: "范围编号和叫法都不能留空"}
		}
		return store.SaveScopeName(ctx, tenant, strings.TrimSpace(body.Scope), strings.TrimSpace(body.Name))
	})
}

func NewRegisterDisplayNameEndpoint(intake CommercialCatalogueIntake, store LabelNameStore) http.Handler {
	return newLabelEndpoint(intake, store, func(ctx context.Context, store LabelNameStore, tenant domain.TenantID, raw json.RawMessage) (ports.LabelSaveOutcome, error) {
		var body displayNameBody
		if err := json.Unmarshal(raw, &body); err != nil {
			return "", errMalformed{detail: "不是一份对象"}
		}
		if strings.TrimSpace(body.ObjectID) == "" || strings.TrimSpace(body.Version) == "" || strings.TrimSpace(body.Name) == "" {
			return "", errMalformed{detail: "产品、版本和名称都不能留空"}
		}
		return store.SaveDisplayName(ctx, tenant, strings.TrimSpace(body.ObjectID), strings.TrimSpace(body.Version), strings.TrimSpace(body.Name))
	})
}

type scopeNameBody struct {
	Scope string `json:"scope"`
	Name  string `json:"name"`
}

type scopeNameListResponse struct {
	Outcome string          `json:"outcome"`
	Scopes  []scopeNameBody `json:"scopes"`
}

type displayNameBody struct {
	ObjectID string `json:"objectId"`
	Version  string `json:"version"`
	Name     string `json:"name"`
}

type errMalformed struct{ detail string }

func (err errMalformed) Error() string { return err.detail }

type labelSave func(ctx context.Context, store LabelNameStore, tenant domain.TenantID, raw json.RawMessage) (ports.LabelSaveOutcome, error)

func newLabelEndpoint(intake CommercialCatalogueIntake, store LabelNameStore, save labelSave) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		var raw json.RawMessage
		if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
			writeProblemWithDetail(response, http.StatusBadRequest, codeMalformedRequest, "不是合法 JSON")
			return
		}
		outcome, err := save(request.Context(), store, query.Scope.Tenant(), raw)
		if malformed, ok := err.(errMalformed); ok {
			writeProblemWithDetail(response, http.StatusBadRequest, codeMalformedRequest, malformed.detail)
			return
		}
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, map[string]string{"outcome": string(outcome)})
	})
}
