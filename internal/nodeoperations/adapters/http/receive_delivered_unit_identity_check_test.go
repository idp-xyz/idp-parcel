package nodeopshttp_test

import (
	"errors"
	"net/http"
	"testing"
)

// Covers: `AT-NO-051` 与 ADR-0199 决定五「核对态是信息格：设备答复……带它，线上字段名 `identityCheck`」的 HTTP 面——零候选与
// 身份核对读不动两条待识别的 outcome、状态码与候选逐字相同（都是 200 PENDING_IDENTIFICATION、没有候选），只有 identityCheck
// 分得开 CHECKED 与 UNCHECKED；首答与重放都带，重放的 outcome 是 EXISTING_RESULT。关联收寄不带这一格。
func TestTheIdentityCheckTravelsWithEveryPendingUnit(t *testing.T) {
	for name, testCase := range map[string]struct {
		err  error
		want string
	}{
		"查无此号": {want: "CHECKED"},
		"读不动":  {err: errors.New("parcel shipment unreachable"), want: "UNCHECKED"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newAdapterFixture(t)
			fixture.identity.candidates = nil
			fixture.identity.err = testCase.err

			first := fixture.post(t, defaultBody())
			replay := fixture.post(t, defaultBody())

			for answer, recorder := range map[string]struct {
				status  int
				outcome string
				view    receptionView
			}{
				"首答": {http.StatusOK, "PENDING_IDENTIFICATION", decodeReception(t, first)},
				"重放": {http.StatusOK, "EXISTING_RESULT", decodeReception(t, replay)},
			} {
				view := recorder.view
				if view.Outcome != recorder.outcome || view.IdentityCheck == nil || *view.IdentityCheck != testCase.want {
					t.Fatalf("%s：outcome %q identityCheck %v, want %s / %q", answer, view.Outcome, view.IdentityCheck, recorder.outcome, testCase.want)
				}
				if len(view.Candidates) != 0 || view.ControlBasis == "" || view.Association != "" {
					t.Fatalf("%s：候选 %v 控制依据 %q 关联 %q, want 没有候选、控制照建、不带关联", answer, view.Candidates, view.ControlBasis, view.Association)
				}
			}
			if first.Code != http.StatusOK || replay.Code != http.StatusOK {
				t.Fatalf("首答 %d 重放 %d, want 都是 200——未核对与已核对对设备是同一种处置", first.Code, replay.Code)
			}
		})
	}

	t.Run("关联收寄不带", func(t *testing.T) {
		fixture := newAdapterFixture(t)
		view := decodeReception(t, fixture.post(t, defaultBody()))
		if view.Outcome != "INTAKE_FORMED" || view.IdentityCheck != nil {
			t.Fatalf("outcome %q identityCheck %v, want INTAKE_FORMED 且不带", view.Outcome, view.IdentityCheck)
		}
	})
}
