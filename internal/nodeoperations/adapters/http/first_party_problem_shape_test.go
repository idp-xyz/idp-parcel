package nodeopshttp_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
)

// Covers: ADR-0140 决定三在本包的分界——首方客户族各口（送站预报的提交与撤回、交接单的客户结论）写 `application/problem+json`，作业事实与操作者族的口照旧（迁移另立
// 后续票）；同一个外壳、同一份映射，只是形状随口定。
func TestOnlyTheFirstPartyLinesWriteProblemDetails(t *testing.T) {
	unconfigured := nodeopshttp.UnconfiguredIntake{}
	preAdvices, handovers := &preAdviceHandlerDouble{}, &handoverHandlerDouble{}
	cases := map[string]struct {
		endpoint http.Handler
		want     string
	}{
		"预报提交":   {nodeopshttp.NewSubmitDeliveryPreAdviceEndpoint(unconfigured, preAdvices), "application/problem+json"},
		"预报撤回":   {nodeopshttp.NewWithdrawDeliveryPreAdviceEndpoint(unconfigured, preAdvices), "application/problem+json"},
		"交接客户结论": {nodeopshttp.NewConcludeDeliveryHandoverEndpoint(unconfigured, handovers), "application/problem+json"},
		"交接单开立":  {nodeopshttp.NewOpenDeliveryHandoverNoteEndpoint(unconfigured, handovers), "application/json"},
		"交接运营确认": {nodeopshttp.NewConfirmDeliveryHandoverEndpoint(unconfigured, handovers), "application/json"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			testCase.endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
			if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("answer = %d %s", response.Code, response.Body)
			}
			if got := response.Header().Get("Content-Type"); got != testCase.want {
				t.Fatalf("Content-Type = %q, want %q", got, testCase.want)
			}
		})
	}
}
