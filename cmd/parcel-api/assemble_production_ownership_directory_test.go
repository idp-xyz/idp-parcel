package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 与演示种子 data/access/17、data/governance/52 同值：词来自 accessidentity.ShipmentRequestSubmissionAdmission，对象范围沿用
// 委托受理那一个试点范围。
const productionShipmentIntakeObjectScope = "SYN-PILOT-SCOPE/shipment-intake@v1"

// registerShipmentRequestAdmissionMapping 登演示租户的委托提交对照（接入身份册）。
func registerShipmentRequestAdmissionMapping(t *testing.T, db *bentopg.DB, tenant string) {
	t.Helper()
	mappings, err := aipg.NewAdmissionScopeMappings(db)
	if err != nil {
		t.Fatal(err)
	}
	validity, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := accessidentity.NewAdmissionScopeMapping(tenant, "SYN-MAP-CC-SHIPMENT-REQUEST-SUBMISSION-01",
		accessidentity.ShipmentRequestSubmissionAdmission(), productionShipmentIntakeObjectScope, validity, "SYN-MAP-BASIS-03")
	if err != nil {
		t.Fatal(err)
	}
	var outcome accessidentity.AdmissionScopeMappingOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		var registerErr error
		outcome, registerErr = mappings.RegisterAdmissionScopeMapping(ctx, mapping)
		return registerErr
	}); err != nil {
		t.Fatalf("登委托提交对照：%v", err)
	}
	if outcome != accessidentity.AdmissionScopeMappingRecorded {
		t.Fatalf("登对照 = %s", outcome)
	}
}

// appendShipmentRequestAuthorityInterval 登本产品在委托提交那一对词下的权威区间（治理登记册）。
func appendShipmentRequestAuthorityInterval(t *testing.T, db *bentopg.DB) {
	t.Helper()
	intervals, err := pgpostgres.NewAuthorityIntervals(db)
	if err != nil {
		t.Fatal(err)
	}
	requirement := accessidentity.ShipmentRequestSubmissionAdmission()
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		return intervals.Append(ctx, pgdomain.AuthorityInterval{
			ObjectScope: productionShipmentIntakeObjectScope,
			Capability:  requirement.Capability,
			FactKind:    requirement.FactKind,
			Authority:   isolatedSelfAuthority,
			From:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		})
	}); err != nil {
		t.Fatalf("登委托提交权威区间：%v", err)
	}
}

// submitThroughProductionForm 按生产形态（不设写开关，isolated 为 nil）装首方口，以 tenant 的客户端提交一单，交回答复。
func submitThroughProductionForm(t *testing.T, db *bentopg.DB, tenant string) *httptest.ResponseRecorder {
	t.Helper()
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	issuer := newCustomerIssuerDouble(t)
	verifier, _, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildCustomerClientMinter(verifier, registry, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	subject, err := accessidentity.NewCustomerClientSubject(syntheticCustomerIssuer, syntheticCustomerSubject)
	if err != nil {
		t.Fatal(err)
	}
	registerCustomerClient(t, db, registry, subject, tenant, syntheticCustomerAccount, "SYN-CUSTOMER-GRANT-PRODUCTION-01")
	ownership, err := buildProductionOwnership(db, nil, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := buildCustomerSubmissionIntake(minter, ownership)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := buildSubmissionOrchestration(db, nil, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := shipmenthttp.NewSubmitShipmentRequestEndpoint(intake, submission)
	request := httptest.NewRequest(http.MethodPost, "/v1/shipment-requests", strings.NewReader(customerDraftBody("SYN-ACCESS-REQUEST-PROD-0001")))
	request.Header.Set("Authorization", "Bearer "+issuer.token(t, syntheticCustomerAudience, syntheticCustomerSubject))
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

func productionDB(t *testing.T) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db
}

// Covers: AT-PS-118（票 first-party-api/04 完成判据，ADR-0308）——生产形态（不设写开关）下，按种子同款登了委托提交对照与
// 本产品的权威区间，演示租户经首方口提交一单答 201 SUBMITTED。库里没有任何 SYN-CAP / SYN-FACT 合成坐标的区间：能放行
// 只因为生产目录读对照换了坐标。
func TestWithoutTheWriteSwitchARegisteredTenantSubmitsThroughTheProductionDirectory(t *testing.T) {
	db := productionDB(t)
	registerShipmentRequestAdmissionMapping(t, db, "SYN-TENANT-01")
	appendShipmentRequestAuthorityInterval(t, db)

	response := submitThroughProductionForm(t, db, "SYN-TENANT-01")
	if response.Code != http.StatusCreated || outcomeOf(t, response) != "SUBMITTED" {
		t.Fatalf("生产形态提交 = %d %s, want 201 SUBMITTED", response.Code, response.Body.String())
	}
}

// Covers: AT-PS-119（票 first-party-api/04 完成判据，ADR-0308）——不代拟：对照没登（区间登了）、区间没登（对照登了）、
// 对照登给了别的租户，三种都答 OWNERSHIP_UNRESOLVED、原因规则不可用，不因另一半在册就放行。
func TestWithoutTheWriteSwitchAMissingMappingOrIntervalLeavesOwnershipUnresolved(t *testing.T) {
	for name, setup := range map[string]func(*testing.T, *bentopg.DB){
		"interval only": func(t *testing.T, db *bentopg.DB) { appendShipmentRequestAuthorityInterval(t, db) },
		"mapping only": func(t *testing.T, db *bentopg.DB) {
			registerShipmentRequestAdmissionMapping(t, db, "SYN-TENANT-01")
		},
		"mapping of another tenant": func(t *testing.T, db *bentopg.DB) {
			registerShipmentRequestAdmissionMapping(t, db, "SYN-TENANT-02")
			appendShipmentRequestAuthorityInterval(t, db)
		},
	} {
		t.Run(name, func(t *testing.T) {
			db := productionDB(t)
			setup(t, db)
			response := submitThroughProductionForm(t, db, "SYN-TENANT-01")
			if response.Code != http.StatusOK || outcomeOf(t, response) != "OWNERSHIP_UNRESOLVED" {
				t.Fatalf("提交 = %d %s, want 200 OWNERSHIP_UNRESOLVED", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "RULE_UNAVAILABLE") {
				t.Fatalf("答复 %s 缺 RULE_UNAVAILABLE", response.Body.String())
			}
		})
	}
}

// Covers: AT-PS-118、AT-PS-119——写开关设与不设两种形态并存（写开关随 operator-channel/53 退场，见 ADR-0150 决定三）：
// 设了开关、种子同款的生产登记也在册时，开关租户仍凭合成坐标 SUBMITTED（TestTheWriteSwitchLendsItsCoordinatesOnlyToItsOwnTenant），
// 这里钉另一头：不设开关时，只有合成坐标的区间（没有生产对照）不放行——生产形态不借合成坐标。
func TestWithoutTheWriteSwitchSyntheticCoordinatesAloneDoNotAdmit(t *testing.T) {
	db := productionDB(t)
	appendIsolatedAuthorityInterval(t, db)

	response := submitThroughProductionForm(t, db, "SYN-TENANT-01")
	if response.Code != http.StatusOK || outcomeOf(t, response) != "OWNERSHIP_UNRESOLVED" {
		t.Fatalf("提交 = %d %s, want 200 OWNERSHIP_UNRESOLVED", response.Code, response.Body.String())
	}
}
