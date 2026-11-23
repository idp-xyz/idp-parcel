package pricinghttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// WarehousePlanIntake 把已认证的请求译成一张仓储计费方案。租户不从载荷取。
type WarehousePlanIntake interface {
	IntakeWarehousePlan(ctx context.Context, request *http.Request) (string, domain.WarehousePlan, error)
}

// WarehousePlanRegistrar 登记仓储计费方案。
type WarehousePlanRegistrar interface {
	Register(ctx context.Context, tenant string, plan domain.WarehousePlan) (application.WarehousePlanOutcome, error)
}

func NewRegisterWarehousePlanEndpoint(intake WarehousePlanIntake, registrar WarehousePlanRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		if intake == nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		tenant, plan, err := intake.IntakeWarehousePlan(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		if registrar == nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		outcome, err := registrar.Register(request.Context(), tenant, plan)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeRegistrationAnswer(response, string(outcome), outcome == application.WarehousePlanRecorded)
	})
}

func (UnconfiguredIntake) IntakeWarehousePlan(context.Context, *http.Request) (string, domain.WarehousePlan, error) {
	return "", domain.WarehousePlan{}, ErrAccessChannelNotConfigured
}

func (intake *OperatorRegistryIntake) IntakeWarehousePlan(ctx context.Context, request *http.Request) (string, domain.WarehousePlan, error) {
	operator, body, err := intake.authenticate(ctx, request)
	if err != nil {
		return "", domain.WarehousePlan{}, err
	}
	plan, err := decodeWarehousePlan(body)
	if err != nil {
		return "", domain.WarehousePlan{}, err
	}
	return operator.Tenant.String(), plan, nil
}

func decodeWarehousePlan(reader io.Reader) (domain.WarehousePlan, error) {
	var document warehousePlanDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return domain.WarehousePlan{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if document.TenantID != nil || document.ID == "" || document.Version == "" || document.Scope == "" ||
		document.Currency == "" || document.DeclaredValue == "" || document.EffectiveFrom.IsZero() {
		return domain.WarehousePlan{}, fmt.Errorf("%w: warehouse plan", ErrMalformedRequest)
	}
	plan := domain.WarehousePlan{
		ID: document.ID, Version: document.Version, Scope: document.Scope, Currency: document.Currency,
		DeclaredValue: document.DeclaredValue, EffectiveFrom: document.EffectiveFrom,
	}
	for _, group := range document.Groups {
		plan.Groups = append(plan.Groups, domain.WarehouseChargeGroup{
			Name: group.Name, Kind: group.Kind, Level: group.Level, Line: group.Line, OnMiss: group.OnMiss,
		})
	}
	for _, rule := range document.Rules {
		decoded := domain.WarehouseChargeRule{
			Code: rule.Code, Kind: rule.Kind, Level: rule.Level, Line: rule.Line, Group: rule.Group, Priority: rule.Priority,
			Method: rule.Method, Unit: rule.Unit, MeasureClass: rule.MeasureClass, UnitQuantity: rule.UnitQuantity,
			UnitPrice: rule.UnitPrice, BaseFee: rule.BaseFee, Allowance: rule.Allowance, Rounding: rule.Rounding,
		}
		for _, condition := range rule.When {
			decoded.When = append(decoded.When, domain.WarehouseCondition{
				Name: condition.Name, Equals: condition.Equals,
				MeasureUnit: condition.MeasureUnit, MeasureClass: condition.MeasureClass,
				Operator: condition.Operator, Threshold: condition.Threshold,
			})
		}
		plan.Rules = append(plan.Rules, decoded)
	}
	return plan, nil
}

type warehousePlanDocument struct {
	TenantID      *string                  `json:"tenantId"`
	ID            string                   `json:"id"`
	Version       string                   `json:"version"`
	Scope         string                   `json:"scope"`
	Currency      string                   `json:"currency"`
	DeclaredValue string                   `json:"declaredValue"`
	EffectiveFrom time.Time                `json:"effectiveFrom"`
	Groups        []warehouseGroupDocument `json:"groups"`
	Rules         []warehouseRuleDocument  `json:"rules"`
}

type warehouseGroupDocument struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Level  string `json:"level"`
	Line   string `json:"line"`
	OnMiss string `json:"onMiss"`
}

type warehouseRuleDocument struct {
	Code         string                       `json:"code"`
	Kind         string                       `json:"kind"`
	Level        string                       `json:"level"`
	Line         string                       `json:"line"`
	Group        string                       `json:"group"`
	Priority     int                          `json:"priority"`
	Method       string                       `json:"method"`
	Unit         string                       `json:"unit"`
	UnitQuantity string                       `json:"unitQuantity"`
	UnitPrice    int64                        `json:"unitPriceMinor"`
	BaseFee      int64                        `json:"baseFeeMinor"`
	Allowance    string                       `json:"allowance"`
	Rounding     string                       `json:"rounding"`
	MeasureClass string                       `json:"measureClass"`
	When         []warehouseConditionDocument `json:"when"`
}

type warehouseConditionDocument struct {
	Name         string `json:"name"`
	Equals       string `json:"equals"`
	MeasureUnit  string `json:"measureUnit"`
	MeasureClass string `json:"measureClass"`
	Operator     string `json:"operator"`
	Threshold    string `json:"threshold"`
}
