package pricinghttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 本文件证价卡收尾的两个在线口（ADR-0234）：租户取认证结果，载荷里的租户、截止点与退役时刻按未知键拒；没有登记册配置写
// 授予在读载荷之前就拒、编排一次也不被调到；答复回显落下的那一次收尾。

const closureBody = `{"planId": "SYN-PLAN-CLOSE", "version": "v1", "successorVersion": "v2", "reference": "SYN-CUT-01"}`

func closureRequest(token, target, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

// Covers: AT-PP-010 在线口那一半的形状门——两口的命令取认证出的租户，载荷四格照搬；自报租户、自报截止点、自报退役时刻、
// 尾随内容都答请求畸形；不带令牌答凭证被拒。
func TestPriceCardClosureIntakesTakeTheAuthenticatedTenant(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	cut, err := intake.IntakePriceCardCut(context.Background(), closureRequest("t", "/pricing-price-card-successor-cuts", closureBody))
	if err != nil {
		t.Fatalf("cut: %v", err)
	}
	if cut.Tenant.String() != "SYN-TENANT-01" || cut.PlanID != "SYN-PLAN-CLOSE" || cut.Version != "v1" ||
		cut.Successor != "v2" || cut.Reference != "SYN-CUT-01" {
		t.Fatalf("cut command = %+v", cut)
	}
	retire, err := intake.IntakePriceCardRetirement(context.Background(), closureRequest("t", "/pricing-price-card-retirements", closureBody))
	if err != nil || retire.Tenant.String() != "SYN-TENANT-01" || retire.Successor != "v2" {
		t.Fatalf("retirement command = %+v err=%v", retire, err)
	}
	for name, body := range map[string]string{
		"自报租户":   strings.Replace(closureBody, `"planId"`, `"tenant": "SYN-TENANT-02", "planId"`, 1),
		"自报截止点":  strings.Replace(closureBody, `"reference"`, `"selectionEndsAt": "2026-06-01T00:00:00Z", "reference"`, 1),
		"自报退役时刻": strings.Replace(closureBody, `"reference"`, `"retiredAt": "2026-06-01T00:00:00Z", "reference"`, 1),
		"尾随内容":   closureBody + `{}`,
	} {
		if _, err := intake.IntakePriceCardCut(context.Background(), closureRequest("t", "/pricing-price-card-successor-cuts", body)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v，want ErrMalformedRequest", name, err)
		}
	}
	if _, err := intake.IntakePriceCardRetirement(context.Background(), closureRequest("", "/pricing-price-card-retirements", closureBody)); !errors.Is(err, pricinghttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v，want ErrOperatorCredentialRejected", err)
	}
}

type closureRegistrarDouble struct{ calls int }

func (double *closureRegistrarDouble) Cut(context.Context, application.CutPriceCardVersionCommand) (application.PriceCardClosureResult, error) {
	double.calls++
	return application.PriceCardClosureResult{}, errors.New("unexpected call")
}

func (double *closureRegistrarDouble) RetireAtCoveringSuccessor(context.Context, application.RetirePriceCardVersionCommand) (application.PriceCardClosureResult, error) {
	double.calls++
	return application.PriceCardClosureResult{}, errors.New("unexpected call")
}

// Covers: AT-PP-010 越权那一格——令牌有效却没有登记册配置写授予，两口都答 403 OPERATOR_NOT_GRANTED，载荷一个字节不读，
// 编排一次也不被调到。
func TestPriceCardClosureEndpointsRefuseAnOperatorWithoutTheRegistryGrant(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{err: pricinghttp.ErrOperatorNotGranted})
	if err != nil {
		t.Fatal(err)
	}
	registrar := &closureRegistrarDouble{}
	for target, endpoint := range map[string]http.Handler{
		"/pricing-price-card-successor-cuts": pricinghttp.NewCutPriceCardVersionEndpoint(intake, registrar),
		"/pricing-price-card-retirements":    pricinghttp.NewRetirePriceCardVersionEndpoint(intake, registrar),
	} {
		probe := &bodyReadProbe{}
		request := httptest.NewRequest(http.MethodPost, target, probe)
		request.Header.Set("Authorization", "Bearer t")
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden || errorCode(t, recorder) != "OPERATOR_NOT_GRANTED" {
			t.Fatalf("%s：答 %d %s，want 403 OPERATOR_NOT_GRANTED", target, recorder.Code, recorder.Body)
		}
		if probe.read {
			t.Fatalf("%s：没有授予却读了载荷", target)
		}
	}
	if registrar.calls != 0 {
		t.Fatal("收尾编排被调到了")
	}
}

// closureStandings 是内存里的收尾册：一版一个此刻的样子，写入一律落下。
type closureStandings struct {
	standings map[string]domain.PriceCardStanding
}

func (registry closureStandings) LockPriceCardVersionForClosure(_ context.Context, _ domain.TenantID, plan domain.VersionReference) (bool, error) {
	_, found := registry.standings[plan.Version()]
	return found, nil
}

func (registry closureStandings) LoadPriceCardStanding(_ context.Context, _ domain.TenantID, plan domain.VersionReference) (domain.PriceCardStanding, bool, error) {
	standing, found := registry.standings[plan.Version()]
	return standing, found, nil
}

func (closureStandings) SavePriceCardCut(context.Context, domain.PriceCardCut) (ports.PriceCardClosureSaveOutcome, error) {
	return ports.PriceCardClosureSaved, nil
}

func (closureStandings) SavePriceCardRetirement(context.Context, domain.PriceCardRetirement) (ports.PriceCardClosureSaveOutcome, error) {
	return ports.PriceCardClosureSaved, nil
}

type closureHandoffDouble struct{}

func (closureHandoffDouble) HandOffPricingParameterRegistered(context.Context, ports.PricingParameterRegisteredIntent) error {
	return nil
}

type closureClock struct{}

func (closureClock) Now() time.Time { return time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC) }

func closureStanding(t *testing.T, version string, starts time.Time) domain.PriceCardStanding {
	t.Helper()
	plan := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-CLOSE", version),
		TableReference:        pptest.IdentityReference(t, domain.ArtifactRateTable, "SYN-TABLE-CLOSE", version),
		WeightPolicyReference: pptest.IdentityReference(t, domain.ArtifactWeightPolicy, "SYN-WEIGHT-CLOSE", version),
		Scope:                 "SYN-SCOPE-CLOSE",
		Direction:             domain.PricingDirectionBuy,
		Purpose:               domain.PricingPurposeSupplierCost,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period:                pptest.Period{StartsAt: starts, EndsAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		RateEntryID:           "SYN-CLOSE-Z1-" + version,
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "10",
		WeightRounding:        domain.RoundingNone,
		WeightStepKilograms:   "1",
	})
	standing, err := domain.NewPriceCardStanding(pptest.Value(t, domain.NewTenantID, "SYN-TENANT-01"), plan, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return standing
}

// Covers: 两口的答复代数——`已登记` 201 并回显截止点与后继、或后继与退役时刻；`未受理` 200 带原因、不回显。编排是真用例，
// 册是内存里的一份。
func TestPriceCardClosureEndpointsEchoTheClosure(t *testing.T) {
	june := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	handler, err := application.NewClosePriceCardVersionHandler(application.ClosePriceCardVersionDeps{
		Registry: closureStandings{standings: map[string]domain.PriceCardStanding{
			"v1": closureStanding(t, "v1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
			"v2": closureStanding(t, "v2", june),
			"v3": closureStanding(t, "v3", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		}},
		Registered: closureHandoffDouble{}, Clock: closureClock{},
	})
	if err != nil {
		t.Fatal(err)
	}
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	serve := func(endpoint http.Handler, target, body string) (int, map[string]any) {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, closureRequest("t", target, body))
		return recorder.Code, decodeBody(t, recorder)
	}
	cutEndpoint := pricinghttp.NewCutPriceCardVersionEndpoint(intake, handler)
	retireEndpoint := pricinghttp.NewRetirePriceCardVersionEndpoint(intake, handler)

	if code, answer := serve(cutEndpoint, "/pricing-price-card-successor-cuts", closureBody); code != http.StatusCreated ||
		answer["outcome"] != "REGISTERED" || answer["selectionEndsAt"] != "2026-06-01T00:00:00Z" || answer["successorVersion"] != "v2" {
		t.Fatalf("截止答复 = %d %v", code, answer)
	}
	retireBody := strings.Replace(closureBody, `"successorVersion": "v2"`, `"successorVersion": "v3"`, 1)
	if code, answer := serve(retireEndpoint, "/pricing-price-card-retirements", retireBody); code != http.StatusCreated ||
		answer["retiredAt"] != "2026-10-21T08:00:00Z" || answer["successorVersion"] != "v3" {
		t.Fatalf("退役答复 = %d %v", code, answer)
	}
	// 用退役口收尾晚起的后继：那一格走截止，退役口答未受理、带原因、不回显。
	if code, answer := serve(retireEndpoint, "/pricing-price-card-retirements", closureBody); code != http.StatusOK ||
		answer["outcome"] != "NOT_ACCEPTED" || answer["cause"] == nil || answer["retiredAt"] != nil {
		t.Fatalf("不成立的退役答复 = %d %v", code, answer)
	}
}
