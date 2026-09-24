package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// isolatedTransportLine 是运输履约主链一口的真库取证：怎么用生产装配拼出这一口的端点、一份合成载荷、该答的状态与 outcome。
// 每放一口在下面的表里加一行。
type isolatedTransportLine struct {
	endpoint func(t *testing.T, db *bentopg.DB, intake *tfhttp.IsolatedCommandIntake) http.Handler
	body     string
	status   int
	outcome  string
}

var isolatedTransportLines = map[string]isolatedTransportLine{
	// 单对象场外揽收登记：控制证据齐、不指名段——落一版揽收，201 PICKUP_REGISTERED。
	"/transport-fulfillment/offsite-pickups": {
		endpoint: func(t *testing.T, db *bentopg.DB, intake *tfhttp.IsolatedCommandIntake) http.Handler {
			controlFacts, err := buildControlFactOrchestrations(db)
			if err != nil {
				t.Fatalf("装配控制事实编排：%v", err)
			}
			return tfhttp.NewRegisterOffsitePickupEndpoint(intake, controlFacts.pickupRegistration)
		},
		body: `{"object":"SYN-PARCEL-08-01","task":"SYN-TASK-08-01","attempt":"SYN-ATTEMPT-08-01","place":"SYN-PLACE/shipper-dock-01",` +
			`"control":"SYN-CONTROL/signed-pickup-01","executedBy":"SYN-COURIER-08","occurredAt":"2026-09-24T08:30:00+08:00"}`,
		status:  http.StatusCreated,
		outcome: "PICKUP_REGISTERED",
	},
	// 一次到访多对象揽收执行：一件揽到手带控制依据、不指名段——落一次到访结果，201 ATTEMPT_RECORDED。
	"/transport-fulfillment/offsite-pickup-attempts": {
		endpoint: func(t *testing.T, db *bentopg.DB, intake *tfhttp.IsolatedCommandIntake) http.Handler {
			controlFacts, err := buildControlFactOrchestrations(db)
			if err != nil {
				t.Fatalf("装配控制事实编排：%v", err)
			}
			return tfhttp.NewPerformOffsitePickupEndpoint(intake, controlFacts.pickupAttempt)
		},
		body: `{"sourceId":"SYN-DEVICE-08/attempt-01","task":"SYN-TASK-08-02","attempt":"SYN-ATTEMPT-08-02","executedBy":"SYN-COURIER-08",` +
			`"place":"SYN-PLACE/shipper-dock-02","plannedFrom":"2026-09-24T08:00:00+08:00","plannedTo":"2026-09-24T10:00:00+08:00",` +
			`"arrivedAt":"2026-09-24T08:40:00+08:00","evidence":"SYN-EVIDENCE/visit-02","objects":[{"object":"SYN-PARCEL-08-02",` +
			`"outcome":"PICKED_UP","control":"SYN-CONTROL/signed-02","occurredAt":"2026-09-24T08:45:00+08:00"}]}`,
		status:  http.StatusCreated,
		outcome: "ATTEMPT_RECORDED",
	},
	// 承运商首次有效收寄的显式判断：承运商揽收扫描一条、表达取得控制、承运主体给引用。生产装配的承运主体目录读 PC 的真
	// 身份登记册，合成承运方没在册，判断如实落成`待确认`（IDENTITY_NOT_REGISTERED）——形成了的业务答案，200。
	"/transport-fulfillment-carrier-first-effective-pickup-judgments": {
		endpoint: func(t *testing.T, db *bentopg.DB, intake *tfhttp.IsolatedCommandIntake) http.Handler {
			judge, _, err := buildCarrierPickupJudgment(db)
			if err != nil {
				t.Fatalf("装配承运商收寄判断编排：%v", err)
			}
			return tfhttp.NewJudgeCarrierFirstEffectivePickupEndpoint(intake, judge)
		},
		body: `{"object":"SYN-PARCEL-08-04","source":"CARRIER_PICKUP_SCAN","evidenceReference":"SYN-SCAN-08-04","evidenceVersion":"v1",` +
			`"expressesControl":true,"occurredAt":"2026-09-24T11:00:00+08:00","carrierKind":"EXTERNAL_PARTY","carrierReference":"SYN-PARTY/carrier-08"}`,
		status:  http.StatusOK,
		outcome: "PICKUP_PENDING",
	},
}

// Covers: 票 operator-channel/08 完成判据「隔离环境里上列各口对合成写答业务结果而不是 ACCESS_CHANNEL_NOT_CONFIGURED」——
// 运输履约主链各口。Intake 走进程自己那道门（buildIsolatedWriteAdmission），编排走生产装配，中间是生产的端点构造函数：
// 三件都是 main 交给路由的那一份。每口另投一份带 tenantId 的载荷，Intake 就拒、不带 outcome。测试输入是隔离合成，只记 `S`。
func TestIsolatedTransportFulfillmentLinesAnswerBusinessOutcomesAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	intake := isolatedWriteAdmissionForTest(t).transportFulfillmentIntake()

	for pattern, line := range isolatedTransportLines {
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
