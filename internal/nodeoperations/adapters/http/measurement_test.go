package nodeopshttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证称重量方登记口的载荷：重量与长宽高都是必填字段，缺哪一格都在口上拒成载荷不成形；复测是带新来源标识
// 再报一条完整的。编排是替身，只记下译出的命令。夹具全为合成（S 级）。

type measurementDecisionAuthenticator struct{}

func (measurementDecisionAuthenticator) AuthenticateOperatorDecision(
	_ context.Context, _ string, decision nodeopshttp.OperatorDecision,
) (nodeopshttp.OperatorIdentity, error) {
	if decision != nodeopshttp.OperatorDecisionManualMeasurementEntry {
		return nodeopshttp.OperatorIdentity{}, errors.New("wrong decision kind")
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	return nodeopshttp.OperatorIdentity{Tenant: tenant, Operator: "https://id.syn.example#SYN-OPERATOR"}, err
}

type recordingMeasurements struct {
	recorded []application.RecordActualMeasurementCommand
}

func (handler *recordingMeasurements) Handle(_ context.Context, command application.RecordActualMeasurementCommand) (application.RecordActualMeasurementResult, error) {
	handler.recorded = append(handler.recorded, command)
	return application.RecordActualMeasurementResult{}, nil
}

func (handler *recordingMeasurements) Invalidate(context.Context, application.InvalidateActualMeasurementCommand) (application.InvalidateActualMeasurementResult, error) {
	return application.InvalidateActualMeasurementResult{}, nil
}

const validMeasurement = `{"sourceId":"SYN-WEIGH-1","objectKind":"HANDLING_UNIT","objectId":"SYN-HU-1","node":"SYN-NODE-SHA-HUB",` +
	`"location":"SYN-BENCH-01","basis":"SYN-PARCEL-1","performedBy":"SYN-SCALE-01","evidenceRef":"WEIGH/1",` +
	`"occurredAt":"2026-11-17T10:00:00+08:00","weightValue":"2.10","weightUnit":"KG",` +
	`"length":"30","width":"25","height":"20","dimensionUnit":"CM","originalDocument":"SYN-DOC-1"}`

// Covers: 重量与长宽高都是必填字段。只称重不量方、只量方不称重、缺一边、缺单位，都拒成 ErrMalformedRequest，
// 错误文本里点名缺的那一格——点名只在服务端，这一族的响应形状不带 detail；
// 旧的「测量项」一格不再是载荷的一部分，带了就拒。
func TestTheMeasurementPayloadRequiresBothTheWeightAndTheDimensions(t *testing.T) {
	intake, err := nodeopshttp.NewOperatorDecisionIntake(measurementDecisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct{ body, names string }{
		"只称重不量方": {
			strings.Replace(validMeasurement, `"length":"30","width":"25","height":"20","dimensionUnit":"CM",`, ``, 1), "length",
		},
		"只量方不称重": {strings.Replace(validMeasurement, `"weightValue":"2.10","weightUnit":"KG",`, ``, 1), "weightValue"},
		"缺一边":    {strings.Replace(validMeasurement, `"height":"20",`, ``, 1), "height"},
		"宽度空白":   {strings.Replace(validMeasurement, `"width":"25"`, `"width":" "`, 1), "width"},
		"缺重量单位":  {strings.Replace(validMeasurement, `"weightUnit":"KG",`, ``, 1), "weightUnit"},
		"缺尺寸单位":  {strings.Replace(validMeasurement, `"dimensionUnit":"CM",`, ``, 1), "dimensionUnit"},
		"带了测量项":  {strings.Replace(validMeasurement, `"weightValue"`, `"item":"GROSS_WEIGHT","weightValue"`, 1), "item"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
		_, err := intake.IntakeManualMeasurement(request.Context(), request)
		if !errors.Is(err, nodeopshttp.ErrMalformedRequest) || !strings.Contains(err.Error(), test.names) {
			t.Fatalf("%s：err = %v，want 坏请求并点名 %s", name, err, test.names)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(validMeasurement))
	command, err := intake.IntakeManualMeasurement(request.Context(), request)
	if err != nil {
		t.Fatalf("完整的一条：%v", err)
	}
	if command.TenantID != "SYN-TENANT-01" || command.EnteredBy != "https://id.syn.example#SYN-OPERATOR" || command.OriginalDocument != "SYN-DOC-1" {
		t.Fatalf("租户与录入人 = %+v，want 取自认证", command)
	}
	if command.WeightValue != "2.10" || command.WeightUnit != "KG" ||
		command.Length != "30" || command.Width != "25" || command.Height != "20" || command.DimensionUnit != "CM" {
		t.Fatalf("重量与尺寸 = %+v，want 一条命令里两样都在", command)
	}
}

// Covers: 缺必填字段的一条到不了编排：口上答 400，没有命令被译出来。
func TestAMeasurementMissingItsDimensionsNeverReachesTheOrchestration(t *testing.T) {
	intake, err := nodeopshttp.NewOperatorDecisionIntake(measurementDecisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	handler := &recordingMeasurements{}
	endpoint := nodeopshttp.NewRecordManualMeasurementEndpoint(intake, handler)
	weightOnly := strings.Replace(validMeasurement, `"length":"30","width":"25","height":"20","dimensionUnit":"CM",`, ``, 1)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(weightOnly)))
	if response.Code != http.StatusBadRequest || len(handler.recorded) != 0 {
		t.Fatalf("status = %d，译出 %d 条命令，want 400 且不到编排。body = %s", response.Code, len(handler.recorded), response.Body.String())
	}
}
