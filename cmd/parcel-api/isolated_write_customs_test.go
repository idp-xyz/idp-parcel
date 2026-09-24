package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// isolatedCustomsLine 是关务主链一口的真库取证，形同 isolatedTransportLine。每放一口在下面的表里加一行。
type isolatedCustomsLine struct {
	endpoint func(t *testing.T, db *bentopg.DB, intake *customshttp.IsolatedCommandIntake) http.Handler
	body     string
	status   int
	outcome  string
}

var isolatedCustomsLines = map[string]isolatedCustomsLine{
	// 外部结果：声称的申报版本不在提交索引里——编排留存原始响应不猜（CONTEXT「不得据此猜测提交、补造缺失层次」），
	// 如实答`归属不上`，形成了的业务答案，200。
	"/customs/external-results": {
		endpoint: func(t *testing.T, db *bentopg.DB, intake *customshttp.IsolatedCommandIntake) http.Handler {
			results, err := buildExternalResultsOrchestration(db)
			if err != nil {
				t.Fatalf("装配外部结果编排：%v", err)
			}
			return customshttp.NewReceiveExternalResultEndpoint(intake, results)
		},
		body: `{"sourceId":"SYN-CUSTOMS-RESP/0801","layer":"REGULATORY_RECEIPT","role":"CUSTOMS_AUTHORITY","rawSemantics":"SYN 受理回执",` +
			`"claimedVersion":"SYN-DECL-V-0801","attempt":1,"scope":"SYN-DECL-UNIT-0801","occurredAt":"2026-09-24T10:30:00+08:00"}`,
		status:  http.StatusOK,
		outcome: "UNATTRIBUTABLE",
	},
}

// Covers: 票 operator-channel/08 完成判据「隔离环境里上列各口对合成写答业务结果而不是 ACCESS_CHANNEL_NOT_CONFIGURED」——
// 关务主链各口。三件都是 main 交给路由的那一份（进程自己那道门的 Intake、生产装配的编排、生产的端点构造函数）；每口另投
// 一份带 tenantId 的载荷，Intake 就拒、不带 outcome。测试输入是隔离合成，只记 `S`。
func TestIsolatedCustomsLinesAnswerBusinessOutcomesAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	intake := isolatedWriteAdmissionForTest(t).customsIntake()

	for pattern, line := range isolatedCustomsLines {
		t.Run(pattern, func(t *testing.T) {
			endpoint := line.endpoint(t, db, intake)
			post := func(body string) *httptest.ResponseRecorder {
				recorder := httptest.NewRecorder()
				endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(body)))
				return recorder
			}

			answered := post(line.body)
			if answered.Code != line.status || registrationOutcome(t, answered) != line.outcome {
				t.Fatalf("合成载荷答 %d %s，want %d %s", answered.Code, answered.Body.String(), line.status, line.outcome)
			}

			selfReported := post(`{"tenantId":"SYN-TENANT-01",` + strings.TrimPrefix(line.body, "{"))
			if selfReported.Code != http.StatusBadRequest || problemCode(t, selfReported) != "MALFORMED_REQUEST" {
				t.Fatalf("带 tenantId 的载荷答 %d %s，want 400 MALFORMED_REQUEST", selfReported.Code, selfReported.Body.String())
			}
			assertNoOutcome(t, selfReported, pattern)
		})
	}
}
