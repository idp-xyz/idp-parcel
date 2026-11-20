package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 经生产装配提交一件已经称过的包裹，版本里带着那次毛重与来源身份；再登记一条复测并采用形态后，提交记录的此刻答 CHANGED。
func TestASubmittedDeclarationKeepsTheMeasurementAndShowsWhenItChanges(t *testing.T) {
	fixture := newCustomsDecisionFixture(t)
	saveAcceptedLabelChannelRequest(t, fixture.db)
	fixture.registerCaseRequirement(t)
	fixture.registerReadiness(t, customsDecisionUnit)
	associateCustomsParcel(t, fixture, customsDecisionParcel)
	firstAt := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Hour)
	recordCustomsMeasurement(t, fixture, "SYN-HU-CMW-01", "SYN-WEIGH-CMW-1", "2.1", firstAt)

	status, established := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBody)
	caseID, _ := established["caseId"].(string)
	if status != http.StatusCreated || caseID == "" {
		t.Fatalf("建案：%d %v", status, established)
	}
	if status, confirmed := fixture.post(t, "/customs-submission-authorizations", customsApprover, `{"unitId":"`+customsDecisionUnit+`"}`); status != http.StatusCreated || confirmed["outcome"] != "AUTHORIZATION_CONFIRMED" {
		t.Fatalf("授权：%d %v", status, confirmed)
	}
	status, submitted := fixture.post(t, "/customs-declaration-submissions", customsSubmitter, customsSubmissionBody(caseID))
	versionID, _ := submitted["versionId"].(string)
	if status != http.StatusCreated || submitted["outcome"] != "DECLARATION_SUBMITTED" || versionID == "" {
		t.Fatalf("提交：%d %v", status, submitted)
	}
	var raw []byte
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT member_weights FROM customs_compliance.declaration_submission WHERE tenant_id = $1 AND version_id = $2`,
		customsDecisionTenant, versionID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var stored []map[string]string
	if err := json.Unmarshal(raw, &stored); err != nil || len(stored) != 1 || stored[0]["grossWeight"] != "2.1" ||
		stored[0]["grossWeightUnit"] != "KG" || stored[0]["measurementSource"] != "SYN-WEIGH-CMW-1" || stored[0]["parcel"] != customsDecisionParcel {
		t.Fatalf("版本里的毛重 = %s err=%v", raw, err)
	}

	recordCustomsMeasurement(t, fixture, "SYN-HU-CMW-01", "SYN-WEIGH-CMW-2", "3", secondAt)
	adoptLatestMeasurement(t, fixture)
	records := mountCustomsSubmissionRecords(t, fixture)
	request := httptest.NewRequest(http.MethodGet, "/customs-submission-records", nil)
	request.Header.Set("Authorization", "Bearer "+registerCustomsReader(t, fixture))
	response := httptest.NewRecorder()
	records.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("提交记录：%d %s", response.Code, response.Body.String())
	}
	var listed struct {
		Submissions []struct {
			Version       string `json:"version"`
			IsCurrent     bool   `json:"isCurrent"`
			MemberWeights []struct {
				MeasurementSource string `json:"measurementSource"`
			} `json:"memberWeights"`
			MeasurementsNow []struct {
				State             string `json:"state"`
				MeasurementSource string `json:"measurementSource"`
				GrossWeight       string `json:"grossWeight"`
			} `json:"measurementsNow"`
		} `json:"submissions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatalf("记录不是 JSON：%s", response.Body.String())
	}
	var current *struct {
		Version       string `json:"version"`
		IsCurrent     bool   `json:"isCurrent"`
		MemberWeights []struct {
			MeasurementSource string `json:"measurementSource"`
		} `json:"memberWeights"`
		MeasurementsNow []struct {
			State             string `json:"state"`
			MeasurementSource string `json:"measurementSource"`
			GrossWeight       string `json:"grossWeight"`
		} `json:"measurementsNow"`
	}
	for i := range listed.Submissions {
		if listed.Submissions[i].Version == versionID {
			current = &listed.Submissions[i]
		}
	}
	if current == nil || !current.IsCurrent || len(current.MemberWeights) != 1 || current.MemberWeights[0].MeasurementSource != "SYN-WEIGH-CMW-1" ||
		len(current.MeasurementsNow) != 1 || current.MeasurementsNow[0].State != "CHANGED" ||
		current.MeasurementsNow[0].MeasurementSource != "SYN-WEIGH-CMW-2" || current.MeasurementsNow[0].GrossWeight != "3" {
		t.Fatalf("此刻 = %+v", listed.Submissions)
	}
}

func associateCustomsParcel(t *testing.T, fixture *customsDecisionFixture, parcel string) {
	t.Helper()
	intake := fmt.Sprintf(`{"unit":"SYN-HU-CMW-01","association":%q,"version":"v1"}`, parcel)
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO node_operations.reception
			(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict,
			 service_markers, recorded_at, judgment_version, predecessor_version)
		 VALUES ($1, 'SYN-CMW-RECEPTION-01', 'digest', 'INTAKE_FORMED', $2::jsonb, '{}'::jsonb, '[]'::jsonb, false,
		         '[]'::jsonb, now(), 'v1', NULL)`,
		customsDecisionTenant, intake); err != nil {
		t.Fatalf("登记包裹关联：%v", err)
	}
}

func recordCustomsMeasurement(t *testing.T, fixture *customsDecisionFixture, unit, sourceID, kilograms string, occurred time.Time) {
	t.Helper()
	source, err := nodomain.NewWorkFactSource(
		sourceID,
		mustValue(t, nodomain.NewPerformingPartyReference, "SYN-SCALE-CMW"),
		mustValue(t, nodomain.NewExecutionEvidenceReference, "WEIGH/"+sourceID),
		occurred,
	)
	if err != nil {
		t.Fatal(err)
	}
	gross, err := nodomain.NewGrossWeight(kilograms, "KG")
	if err != nil {
		t.Fatal(err)
	}
	outer, err := nodomain.NewOuterDimensions("30", "20", "10", "CM")
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := nodomain.NewActualMeasurement(nodomain.ActualMeasurementSpec{
		Source: source, ObjectKind: nodomain.HandlingUnitObject, ObjectID: unit,
		Node: mustValue(t, nodomain.NewNodeReference, "node-origin"), Location: mustValue(t, nodomain.NewWorkLocationReference, "bench-2"),
		Basis:  mustValue(t, nodomain.NewWorkBasisReference, "intake-1"),
		Weight: gross, Dimensions: outer,
	})
	if err != nil {
		t.Fatal(err)
	}
	measurements, err := nopostgres.NewActualMeasurements(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	record := noports.ActualMeasurementRecord{
		Key:           noports.ActualMeasurementKey{TenantID: mustValue(t, nodomain.NewTenantID, customsDecisionTenant), SourceID: sourceID},
		Measurement:   measurement,
		ContentDigest: measurement.ContentDigest(),
		RecordedAt:    occurred.Add(time.Minute),
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		outcome, err := measurements.SaveMeasurement(ctx, record)
		if err != nil {
			return err
		}
		if outcome != noports.ActualMeasurementSaved {
			return fmt.Errorf("登记实测 %s", outcome)
		}
		return nil
	}); err != nil {
		t.Fatalf("登记实测：%v", err)
	}
}

func adoptLatestMeasurement(t *testing.T, fixture *customsDecisionFixture) {
	t.Helper()
	adoptions, err := nopostgres.NewMeasurementSelectionAdoptions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := adoptions.SaveAdoption(ctx, noports.MeasurementSelectionAdoption{
			TenantID: mustValue(t, nodomain.NewTenantID, customsDecisionTenant),
			Form:     nodomain.LatestStillValidByOccurredAt, Basis: "SYN-BASIS-CMW",
			RecordedAt: time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC),
		})
		return err
	}); err != nil {
		t.Fatalf("采用形态：%v", err)
	}
}

func registerCustomsReader(t *testing.T, fixture *customsDecisionFixture) string {
	t.Helper()
	const subject = "SYN-READER-CMW"
	operator, err := accessidentity.NewOperatorSubject(customsDecisionIssuer, subject)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewOperatorBinding(operator, customsDecisionTenant, "SYN-BIND-BASIS-CMW")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewOperatorGrant(customsDecisionTenant, "SYN-GRANT-CMW-READ", operator, accessidentity.CapabilityMasterDataAndOperationsRead, interval, "SYN-GRANT-BASIS-CMW")
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if _, err := fixture.operators.RegisterOperator(ctx, binding); err != nil {
			return err
		}
		_, err := fixture.operators.RegisterGrant(ctx, grant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return subject
}

func mountCustomsSubmissionRecords(t *testing.T, fixture *customsDecisionFixture) http.Handler {
	t.Helper()
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, fixture.operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	reads, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*customsRecordsWiring)(nil)) {
			continue
		}
		mounted, err := feature.production(endpointSources{db: fixture.db}, endpointChannels{operatorRegistries: reads})
		if err != nil {
			t.Fatal(err)
		}
		for _, endpoint := range mounted {
			if endpoint.Pattern == "/customs-submission-records" {
				return endpoint.Handler
			}
		}
	}
	t.Fatal("没挂上提交记录口")
	return nil
}
