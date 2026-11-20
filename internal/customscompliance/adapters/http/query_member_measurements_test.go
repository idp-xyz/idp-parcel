package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

func TestSubmissionRecordsEchoMemberWeightsAndTheFiveMeasurementStates(t *testing.T) {
	weighedAt := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	weights := &scriptedMemberWeights{byParcel: map[string]scriptedReading{
		"parcel-same":    {reading: oneReading("2.1", "KG", "SYN-WEIGH-A", weighedAt)},
		"parcel-changed": {reading: oneReading("3", "KG", "SYN-WEIGH-B", weighedAt.Add(time.Hour))},
		"parcel-voided":  {reading: ports.MeasuredWeightReading{Answer: ports.MeasuredWeightNone}},
		"parcel-tied":    {reading: ports.MeasuredWeightReading{Answer: ports.MeasuredWeightUndetermined, Cause: "SAME_OCCURRED_AT"}},
		"parcel-down":    {err: errors.New("down")},
		"parcel-empty":   {reading: ports.MeasuredWeightReading{Answer: ports.MeasuredWeightNone}},
	}}
	current := ports.SubmissionVersionRecord{
		Version: "SUBV-NOW", Unit: "UNIT-1", Procedure: "proc-1", ContentDigest: "dg", IsCurrent: true,
		Members: []string{"parcel-changed", "parcel-same", "parcel-voided", "parcel-tied", "parcel-down", "parcel-empty"},
		FixedAt: weighedAt,
		MemberWeights: []ports.MemberMeasuredWeightRecord{
			{Parcel: "parcel-same", GrossWeight: "2.1", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-A", MeasuredAt: weighedAt},
			{Parcel: "parcel-changed", GrossWeight: "2.1", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-A", MeasuredAt: weighedAt},
			{Parcel: "parcel-voided", GrossWeight: "2.1", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-A", MeasuredAt: weighedAt},
		},
	}
	empty := ports.SubmissionVersionRecord{
		Version: "SUBV-EMPTY", Unit: "UNIT-2", Procedure: "proc-1", IsCurrent: true, Members: []string{"parcel-empty"}, FixedAt: weighedAt,
	}
	handler := customshttp.NewQuerySubmissionVersionsEndpoint(
		grantedCatalogueIntake{tenant: "tenant-1", limit: 20},
		submissionVersions{current, empty},
		weights,
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customs-submission-records", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Submissions []map[string]json.RawMessage `json:"submissions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Submissions) != 2 {
		t.Fatalf("submissions = %d", len(payload.Submissions))
	}
	var memberWeights []memberWeightJSON
	if err := json.Unmarshal(payload.Submissions[0]["memberWeights"], &memberWeights); err != nil || len(memberWeights) != 3 ||
		memberWeights[0].Parcel != "parcel-same" || memberWeights[0].GrossWeight != "2.1" || memberWeights[0].MeasurementSource != "SYN-WEIGH-A" {
		t.Fatalf("memberWeights = %s err=%v", payload.Submissions[0]["memberWeights"], err)
	}
	var now []measurementNowJSON
	if err := json.Unmarshal(payload.Submissions[0]["measurementsNow"], &now); err != nil {
		t.Fatal(err)
	}
	got := map[string]measurementNowJSON{}
	for _, row := range now {
		got[row.Parcel] = row
	}
	if got["parcel-same"].State != "SAME" || got["parcel-same"].GrossWeight != "2.1" || got["parcel-same"].MeasurementSource != "SYN-WEIGH-A" ||
		got["parcel-changed"].State != "CHANGED" || got["parcel-changed"].GrossWeight != "3" || got["parcel-changed"].MeasurementSource != "SYN-WEIGH-B" ||
		got["parcel-voided"].State != "NONE_NOW" || got["parcel-voided"].GrossWeight != "" ||
		got["parcel-tied"].State != "UNDETERMINED" || got["parcel-down"].State != "UNAVAILABLE" ||
		got["parcel-empty"].State != "" {
		t.Fatalf("此刻 = %+v", got)
	}
	var emptyWeights []memberWeightJSON
	var emptyNow []measurementNowJSON
	if err := json.Unmarshal(payload.Submissions[1]["memberWeights"], &emptyWeights); err != nil || len(emptyWeights) != 0 {
		t.Fatalf("空毛重 = %s", payload.Submissions[1]["memberWeights"])
	}
	if err := json.Unmarshal(payload.Submissions[1]["measurementsNow"], &emptyNow); err != nil || len(emptyNow) != 0 {
		t.Fatalf("两边都没有仍列出了：%s", payload.Submissions[1]["measurementsNow"])
	}
}

func TestHistoricalSubmissionRecordsDoNotAskForMeasurementsNow(t *testing.T) {
	weights := &scriptedMemberWeights{byParcel: map[string]scriptedReading{}}
	historical := ports.SubmissionVersionRecord{
		Version: "SUBV-OLD", Unit: "UNIT-1", Procedure: "proc-1", IsCurrent: false,
		Members: []string{"parcel-old"}, FixedAt: time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC),
		MemberWeights: []ports.MemberMeasuredWeightRecord{
			{Parcel: "parcel-old", GrossWeight: "2.1", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-A", MeasuredAt: time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)},
		},
	}
	handler := customshttp.NewQuerySubmissionVersionsEndpoint(
		grantedCatalogueIntake{tenant: "tenant-1", limit: 20},
		submissionVersions{historical},
		weights,
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customs-submission-records", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if len(weights.asked) != 0 {
		t.Fatalf("非当前版问了读口：%v", weights.asked)
	}
	var payload struct {
		Submissions []map[string]json.RawMessage `json:"submissions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, present := payload.Submissions[0]["measurementsNow"]; present {
		t.Fatalf("非当前版带了此刻：%s", payload.Submissions[0]["measurementsNow"])
	}
	if string(payload.Submissions[0]["memberWeights"]) == "" || string(payload.Submissions[0]["memberWeights"]) == "null" {
		t.Fatal("历史版的毛重不该丢")
	}
}

type submissionVersions []ports.SubmissionVersionRecord

func (rows submissionVersions) ListSubmissionVersions(
	context.Context, domain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[ports.SubmissionVersionRecord], error) {
	return cataloguepage.Result[ports.SubmissionVersionRecord]{Rows: rows, Total: int64(len(rows))}, nil
}

type scriptedReading struct {
	reading ports.MeasuredWeightReading
	err     error
}

type scriptedMemberWeights struct {
	byParcel map[string]scriptedReading
	asked    []string
}

func (weights *scriptedMemberWeights) LoadMemberMeasuredWeight(
	_ context.Context, _ domain.TenantID, parcel domain.DeclaredParcelReference,
) (ports.MeasuredWeightReading, error) {
	weights.asked = append(weights.asked, parcel.String())
	scripted := weights.byParcel[parcel.String()]
	return scripted.reading, scripted.err
}

func oneReading(gross, unit, source string, at time.Time) ports.MeasuredWeightReading {
	return ports.MeasuredWeightReading{
		Answer: ports.MeasuredWeightOne, GrossWeight: gross, GrossWeightUnit: unit, MeasurementSource: source, MeasuredAt: at,
	}
}

type memberWeightJSON struct {
	Parcel            string `json:"parcel"`
	GrossWeight       string `json:"grossWeight"`
	MeasurementSource string `json:"measurementSource"`
}

type measurementNowJSON struct {
	Parcel            string `json:"parcel"`
	State             string `json:"state"`
	GrossWeight       string `json:"grossWeight"`
	MeasurementSource string `json:"measurementSource"`
}
