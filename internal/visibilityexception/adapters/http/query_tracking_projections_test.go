package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

type operationsIntakeDouble struct {
	query visibilityhttp.OperationsTrackingQuery
	err   error
}

func (double operationsIntakeDouble) IntakeOperationsQuery(
	_ context.Context,
	_ *http.Request,
) (visibilityhttp.OperationsTrackingQuery, error) {
	if double.err != nil {
		return visibilityhttp.OperationsTrackingQuery{}, double.err
	}
	return double.query, nil
}

type projectionReaderDouble struct {
	listed    []domain.TrackingProjection
	byParcel  map[domain.TrackedParcelReference]domain.TrackingProjection
	byVersion map[domain.ProjectionVersionID]domain.TrackingProjection
	tenant    domain.TenantID
	err       error
}

func (double *projectionReaderDouble) ListCurrent(
	_ context.Context,
	tenant domain.TenantID,
	_ int,
) ([]domain.TrackingProjection, error) {
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.listed, nil
}

func (double *projectionReaderDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
) (domain.TrackingProjection, bool, error) {
	if double.err != nil {
		return domain.TrackingProjection{}, false, double.err
	}
	if tenant != double.tenant {
		return domain.TrackingProjection{}, false, nil
	}
	projection, found := double.byParcel[parcel]
	return projection, found, nil
}

func (double *projectionReaderDouble) FindByVersion(
	_ context.Context,
	tenant domain.TenantID,
	version domain.ProjectionVersionID,
) (domain.TrackingProjection, bool, error) {
	if double.err != nil {
		return domain.TrackingProjection{}, false, double.err
	}
	if tenant != double.tenant {
		return domain.TrackingProjection{}, false, nil
	}
	projection, found := double.byVersion[version]
	return projection, found, nil
}

var operationsBaseAt = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

func operationsScope(t *testing.T) visibilityhttp.OperationsTrackingQuery {
	t.Helper()
	scope, err := domain.NewOperationsQueryScope(
		value(t, domain.NewOperationsScopeReference, "ops-scope-1"),
		value(t, domain.NewTenantID, "tenant-1"),
	)
	if err != nil {
		t.Fatalf("构造运营作用域：%v", err)
	}
	return visibilityhttp.OperationsTrackingQuery{Scope: scope, Limit: 50}
}

// operationsProjection 造一份两条目的投影:一条已归类且带替代关系,一条如实未归类。
func operationsProjection(t *testing.T) domain.TrackingProjection {
	t.Helper()
	parcel := value(t, domain.NewTrackedParcelReference, "parcel-1")
	mapping := value(t, domain.NewMappingVersionReference, "milestone-map/v2")
	corrected, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
		Source:      domain.SourceTransportFulfillment,
		Parcel:      parcel,
		Fact:        value(t, domain.NewSourceFactReference, "handover/leg-1"),
		Kind:        value(t, domain.NewSourceFactKind, "transport-handover"),
		Version:     value(t, domain.NewSourceFactVersion, "v2"),
		Supersedes:  value(t, domain.NewSourceFactVersion, "v1"),
		OccurredAt:  operationsBaseAt,
		EffectiveAt: operationsBaseAt.Add(time.Hour),
		ReceivedAt:  operationsBaseAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("构造更正事实：%v", err)
	}
	classified, err := domain.ClassifyMilestone(corrected,
		value(t, domain.NewMilestoneReference, "IN_TRANSIT"), mapping)
	if err != nil {
		t.Fatalf("归类：%v", err)
	}
	opaque, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
		Source:      domain.SourceCustomsCompliance,
		Parcel:      parcel,
		Fact:        value(t, domain.NewSourceFactReference, "customs/case-9"),
		Kind:        value(t, domain.NewSourceFactKind, "external-result"),
		Version:     value(t, domain.NewSourceFactVersion, "v1"),
		OccurredAt:  operationsBaseAt.Add(3 * time.Hour),
		EffectiveAt: operationsBaseAt.Add(3 * time.Hour),
		ReceivedAt:  operationsBaseAt.Add(4 * time.Hour),
	})
	if err != nil {
		t.Fatalf("构造关务事实：%v", err)
	}
	unclassified, err := domain.LeaveUnclassified(opaque, mapping)
	if err != nil {
		t.Fatalf("未归类：%v", err)
	}
	first, err := domain.DeriveTrackingProjection(
		value(t, domain.NewProjectionVersionID, "projection-1"),
		parcel,
		[]domain.MilestoneClassification{classified},
		operationsBaseAt.Add(5*time.Hour),
	)
	if err != nil {
		t.Fatalf("派生首版：%v", err)
	}
	current, err := first.Rederive(
		value(t, domain.NewProjectionVersionID, "projection-2"),
		[]domain.MilestoneClassification{classified, unclassified},
		operationsBaseAt.Add(6*time.Hour),
	)
	if err != nil {
		t.Fatalf("重派生：%v", err)
	}
	return current
}

func opsServe(
	t *testing.T,
	intake visibilityhttp.OperationsTrackingIntake,
	reader visibilityhttp.OperationsProjectionReader,
	method, target string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := visibilityhttp.NewQueryTrackingProjectionsEndpoint(intake, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

func opsReader(t *testing.T, projection domain.TrackingProjection) *projectionReaderDouble {
	t.Helper()
	return &projectionReaderDouble{
		tenant:   value(t, domain.NewTenantID, "tenant-1"),
		listed:   []domain.TrackingProjection{projection},
		byParcel: map[domain.TrackedParcelReference]domain.TrackingProjection{projection.Parcel(): projection},
		byVersion: map[domain.ProjectionVersionID]domain.TrackingProjection{
			projection.Version(): projection,
		},
	}
}

// Covers: ADR-0076 决定一、四与 CONTEXT「运营追踪查阅」 — 列表按租户作答,投影逐字段
// 透出:来源、事实类型、三个时间、替代关系与未归类态都不经披露删减;空列表照答。
func TestOperationsListReturnsProjectionsVerbatim(t *testing.T) {
	projection := operationsProjection(t)
	recorder := opsServe(t, operationsIntakeDouble{query: operationsScope(t)},
		opsReader(t, projection), http.MethodGet, "/tracking-projections")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		Outcome     string `json:"outcome"`
		Projections []struct {
			Version      string `json:"version"`
			Parcel       string `json:"parcel"`
			PriorVersion string `json:"priorVersion"`
			Entries      []struct {
				Source         string `json:"source"`
				Kind           string `json:"kind"`
				FactVersion    string `json:"factVersion"`
				Supersedes     string `json:"supersedes"`
				OccurredAt     string `json:"occurredAt"`
				MappingVersion string `json:"mappingVersion"`
				Milestone      string `json:"milestone"`
			} `json:"entries"`
		} `json:"projections"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "PROJECTIONS_LISTED" || len(body.Projections) != 1 {
		t.Fatalf("outcome = %q projections = %d", body.Outcome, len(body.Projections))
	}
	got := body.Projections[0]
	if got.Version != "projection-2" || got.Parcel != "parcel-1" || got.PriorVersion != "projection-1" {
		t.Fatalf("projection = %+v; 版本、包裹与指回必须逐字段在场", got)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(got.Entries))
	}
	classified := got.Entries[0]
	if classified.Source != "TRANSPORT_FULFILLMENT" || classified.Kind != "transport-handover" ||
		classified.FactVersion != "v2" || classified.Supersedes != "v1" ||
		classified.Milestone != "IN_TRANSIT" || classified.MappingVersion != "milestone-map/v2" {
		t.Fatalf("已归类条目走样：%+v", classified)
	}
	if classified.OccurredAt != operationsBaseAt.Format(time.RFC3339Nano) {
		t.Fatalf("occurredAt = %q", classified.OccurredAt)
	}
	unclassified := got.Entries[1]
	if unclassified.Source != "CUSTOMS_COMPLIANCE" || unclassified.Milestone != "" ||
		unclassified.Supersedes != "" || unclassified.MappingVersion != "milestone-map/v2" {
		t.Fatalf("未归类条目走样：%+v（milestone 缺席才是真话,映射版本仍必备）", unclassified)
	}

	empty := opsServe(t, operationsIntakeDouble{query: operationsScope(t)},
		&projectionReaderDouble{tenant: value(t, domain.NewTenantID, "tenant-1")},
		http.MethodGet, "/tracking-projections")
	var emptyBody struct {
		Outcome     string            `json:"outcome"`
		Projections []json.RawMessage `json:"projections"`
	}
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if empty.Code != http.StatusOK || emptyBody.Outcome != "PROJECTIONS_LISTED" || emptyBody.Projections == nil {
		t.Fatalf("空列表 = %d %s; want 200 PROJECTIONS_LISTED 空数组", empty.Code, empty.Body.String())
	}
}

// Covers: ADR-0076 决定四 — 「投影未形成」对租户内已授权查阅如实作答(2xx 业务答案,
// 不承袭 VIEW_NOT_FOUND 三义合并);命中时按 parcel 答当前版。
func TestOperationsParcelQueryAnswersHonestly(t *testing.T) {
	projection := operationsProjection(t)
	reader := opsReader(t, projection)

	hit := opsServe(t, operationsIntakeDouble{query: operationsScope(t)}, reader,
		http.MethodGet, "/tracking-projections?parcel=parcel-1")
	var hitBody struct {
		Outcome    string `json:"outcome"`
		Projection struct {
			Version string `json:"version"`
		} `json:"projection"`
	}
	if err := json.Unmarshal(hit.Body.Bytes(), &hitBody); err != nil {
		t.Fatalf("decode hit: %v", err)
	}
	if hit.Code != http.StatusOK || hitBody.Outcome != "CURRENT_PROJECTION" ||
		hitBody.Projection.Version != "projection-2" {
		t.Fatalf("hit = %d %s", hit.Code, hit.Body.String())
	}

	missing := opsServe(t, operationsIntakeDouble{query: operationsScope(t)}, reader,
		http.MethodGet, "/tracking-projections?parcel=parcel-404")
	var missingBody struct {
		Outcome    string          `json:"outcome"`
		Projection json.RawMessage `json:"projection"`
	}
	if err := json.Unmarshal(missing.Body.Bytes(), &missingBody); err != nil {
		t.Fatalf("decode missing: %v", err)
	}
	if missing.Code != http.StatusOK || missingBody.Outcome != "PROJECTION_NOT_FORMED" ||
		missingBody.Projection != nil {
		t.Fatalf("missing = %d %s; 未形成是业务答案且不带投影体", missing.Code, missing.Body.String())
	}
}

// Covers: ADR-0065 审计口 — 按版本读回留存版本;未留存的版本如实答 VERSION_NOT_FOUND。
func TestOperationsVersionQueryReadsBackStoredVersions(t *testing.T) {
	projection := operationsProjection(t)
	reader := opsReader(t, projection)

	hit := opsServe(t, operationsIntakeDouble{query: operationsScope(t)}, reader,
		http.MethodGet, "/tracking-projections?version=projection-2")
	var hitBody struct {
		Outcome    string `json:"outcome"`
		Projection struct {
			Version string `json:"version"`
		} `json:"projection"`
	}
	if err := json.Unmarshal(hit.Body.Bytes(), &hitBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if hit.Code != http.StatusOK || hitBody.Outcome != "PROJECTION_VERSION" ||
		hitBody.Projection.Version != "projection-2" {
		t.Fatalf("hit = %d %s", hit.Code, hit.Body.String())
	}

	missing := opsServe(t, operationsIntakeDouble{query: operationsScope(t)}, reader,
		http.MethodGet, "/tracking-projections?version=projection-404")
	var missingBody struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(missing.Body.Bytes(), &missingBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if missing.Code != http.StatusOK || missingBody.Outcome != "VERSION_NOT_FOUND" {
		t.Fatalf("missing = %d %s", missing.Code, missing.Body.String())
	}
}

// Covers: 传输形状 — 两个定位参数同时在场按坏请求拒,且拒在 Intake 之前(intake 双身
// 若被调用会答 INTAKE_FAILED,而这里必须是 MALFORMED_REQUEST)。
func TestOperationsQueryRejectsAmbiguousLocators(t *testing.T) {
	recorder := opsServe(t, operationsIntakeDouble{err: errors.New("must not be reached")},
		&projectionReaderDouble{}, http.MethodGet,
		"/tracking-projections?parcel=parcel-1&version=projection-2")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "MALFORMED_REQUEST" {
		t.Fatalf("code = %q, want MALFORMED_REQUEST", body.Error.Code)
	}
}

// Covers: ADR-0076 决定三沿 ADR-0055 — 未配置渠道对全部分支同答 403,无 outcome;
// 空参数值构造不出查询键即 400。
func TestOperationsTransportGridsStayClosed(t *testing.T) {
	unconfiguredList := opsServe(t, visibilityhttp.UnconfiguredIntake{}, &projectionReaderDouble{},
		http.MethodGet, "/tracking-projections")
	unconfiguredParcel := opsServe(t, visibilityhttp.UnconfiguredIntake{}, &projectionReaderDouble{},
		http.MethodGet, "/tracking-projections?parcel=parcel-1")
	if unconfiguredList.Code != http.StatusForbidden || unconfiguredParcel.Code != http.StatusForbidden {
		t.Fatalf("unconfigured = %d/%d, want 403/403", unconfiguredList.Code, unconfiguredParcel.Code)
	}
	if unconfiguredList.Body.String() != unconfiguredParcel.Body.String() {
		t.Fatal("未配置答复随分支变化:分支选择泄露了东西")
	}
	var unconfiguredBody struct {
		Outcome string `json:"outcome"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(unconfiguredList.Body.Bytes(), &unconfiguredBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if unconfiguredBody.Outcome != "" || unconfiguredBody.Error.Code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("body = %s", unconfiguredList.Body.String())
	}

	wrongMethod := opsServe(t, operationsIntakeDouble{query: operationsScope(t)},
		&projectionReaderDouble{}, http.MethodPost, "/tracking-projections")
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d allow = %q, want 405/GET", wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}

	blankParcel := opsServe(t, operationsIntakeDouble{query: operationsScope(t)},
		&projectionReaderDouble{}, http.MethodGet, "/tracking-projections?parcel=%20")
	if blankParcel.Code != http.StatusBadRequest {
		t.Fatalf("blank parcel status = %d, want 400", blankParcel.Code)
	}

	readerDown := opsServe(t, operationsIntakeDouble{query: operationsScope(t)},
		&projectionReaderDouble{err: errors.New("store down")}, http.MethodGet, "/tracking-projections")
	var downBody struct {
		Outcome string `json:"outcome"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(readerDown.Body.Bytes(), &downBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if readerDown.Code != http.StatusInternalServerError || downBody.Outcome != "" ||
		downBody.Error.Code != "NO_ANSWER_FORMED" {
		t.Fatalf("reader down = %d %s; 故障不得伪装成业务答案", readerDown.Code, readerDown.Body.String())
	}
}
