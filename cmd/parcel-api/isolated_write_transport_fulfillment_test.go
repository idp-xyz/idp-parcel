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
	// 交接判断首登：结论已交接、两侧证据与规则齐、不带依据（依据只属拒收与待确认）、不指名段——落一版交接，
	// 201 HANDOVER_REGISTERED。
	"/transport-fulfillment/handovers": {
		endpoint: func(t *testing.T, db *bentopg.DB, intake *tfhttp.IsolatedCommandIntake) http.Handler {
			controlFacts, err := buildControlFactOrchestrations(db)
			if err != nil {
				t.Fatalf("装配控制事实编排：%v", err)
			}
			return tfhttp.NewRegisterTransportHandoverEndpoint(intake, controlFacts.handover)
		},
		body: `{"object":"SYN-PARCEL-08-05","scope":"SYN-SCOPE/hub-dock-05","releasedBy":"SYN-PARTY/hub-08","receivedBy":"SYN-PARTY/linehaul-08",` +
			`"verdict":"HANDED_OVER","releasingEvidence":"SYN-EVIDENCE/release-05","receivingEvidence":"SYN-EVIDENCE/receive-05",` +
			`"rule":"SYN-RULE/handover@v1","version":"v1","judgedAt":"2026-09-24T13:00:00+08:00"}`,
		status:  http.StatusCreated,
		outcome: "HANDOVER_REGISTERED",
	},
	// 自营执行方的到达事实：到达不受门禁管（门禁只约束装载出发），不带门禁两格——落一条移动事实，201 MOVEMENT_FACT_RECORDED。
	// 来源格是装配点注入的合成来源，不在载荷里。
	"/transport-fulfillment/movement-facts": {
		endpoint: func(t *testing.T, db *bentopg.DB, intake *tfhttp.IsolatedCommandIntake) http.Handler {
			movement, err := buildMovementFactOrchestration(db)
			if err != nil {
				t.Fatalf("装配移动事实编排：%v", err)
			}
			return tfhttp.NewRecordMovementFactEndpoint(intake, movement)
		},
		body: `{"fact":"SYN-MOVE-08-06","schedule":"SYN-SCHEDULE/linehaul-06","kind":"ARRIVAL","location":"SYN-NODE-SIN-HUB",` +
			`"version":"v1","occurredAt":"2026-09-24T18:00:00+08:00"}`,
		status:  http.StatusCreated,
		outcome: "MOVEMENT_FACT_RECORDED",
	},
	// 授权角色建立派送任务：工作范围七件齐——立一个任务，201 DISPATCH_TASK_OPENED。
	"/transport-fulfillment-dispatch-task-registrations": {
		endpoint: func(t *testing.T, db *bentopg.DB, intake *tfhttp.IsolatedCommandIntake) http.Handler {
			segmentOps, err := buildSegmentOperations(db)
			if err != nil {
				t.Fatalf("装配段运营编排：%v", err)
			}
			return tfhttp.NewOpenDispatchTaskEndpoint(intake, segmentOps.opener)
		},
		body: `{"task":"SYN-DISPATCH-08-07","kind":"DELIVERY","objects":["SYN-PARCEL-08-07"],"place":"SYN-PLACE/consignee-07",` +
			`"windowFrom":"2026-09-25T09:00:00+08:00","windowTo":"2026-09-25T12:00:00+08:00","conditions":"SYN-CONDITION/signature-required",` +
			`"openedAt":"2026-09-24T20:00:00+08:00"}`,
		status:  http.StatusCreated,
		outcome: "DISPATCH_TASK_OPENED",
	},
	// 派送发起的一拍：段不在册——执行器如实答`不是触发事实`（SEGMENT_NOT_FOUND），形成了的业务答案，200。
	"/transport-fulfillment-delivery-dispatch-triggers": {
		endpoint: func(t *testing.T, db *bentopg.DB, intake *tfhttp.IsolatedCommandIntake) http.Handler {
			trigger, err := buildDeliveryDispatchTrigger(db)
			if err != nil {
				t.Fatalf("装配派送发起执行器：%v", err)
			}
			return tfhttp.NewTriggerDeliveryDispatchEndpoint(intake, trigger)
		},
		body:    `{"segment":"SYN-SEGMENT-08-09","object":"SYN-PARCEL-08-09","occurredAt":"2026-09-25T07:00:00+08:00"}`,
		status:  http.StatusOK,
		outcome: "NOT_A_DELIVERY_TRIGGER",
	},
	// 交付生效首登：五格齐。交付只能落在已登记的派送尝试结果上（tfpostgres.DeliveryAttempts 只读，「一个入口同时造尝试和
	// 造交付，就没有东西拦得住先声称到过场再声称交付成功」），而派送尝试登记册今天没有生产写入方——编排如实答`未受理`，
	// 200。停点从「渠道未配置」挪到「派送尝试无写入方」，这一格照实写回 psb/05。
	"/transport-fulfillment/deliveries": {
		endpoint: func(t *testing.T, db *bentopg.DB, intake *tfhttp.IsolatedCommandIntake) http.Handler {
			delivery, err := buildDeliveryOrchestration(db)
			if err != nil {
				t.Fatalf("装配交付编排：%v", err)
			}
			return tfhttp.NewRegisterEffectiveDeliveryEndpoint(intake, delivery)
		},
		body: `{"attempt":"SYN-ATTEMPT-08-10","object":"SYN-PARCEL-08-10","method":"HANDED_TO_RECIPIENT",` +
			`"recipient":"SYN-RECIPIENT/consignee-10","proof":"SYN-POD/signature-10"}`,
		status:  http.StatusOK,
		outcome: "SOURCE_NOT_ACCEPTED",
	},
	// 关段声明：段不在册——编排如实答 SEGMENT_NOT_FOUND，形成了的业务答案，200。
	"/transport-fulfillment-segment-closures": {
		endpoint: func(t *testing.T, db *bentopg.DB, intake *tfhttp.IsolatedCommandIntake) http.Handler {
			segmentOps, err := buildSegmentOperations(db)
			if err != nil {
				t.Fatalf("装配段运营编排：%v", err)
			}
			return tfhttp.NewCloseFulfillmentSegmentEndpoint(intake, segmentOps.closer)
		},
		body:    `{"segment":"SYN-SEGMENT-08-11","closedAt":"2026-09-25T20:00:00+08:00"}`,
		status:  http.StatusOK,
		outcome: "SEGMENT_NOT_FOUND",
	},
	// 有效时间显式判断：指名的轨迹事实不在册（外部轨迹只经 TrackingSource 入站口进，隔离形态不开那条路）——编排如实答
	// `未受理`，形成了的业务答案，200。
	"/transport-fulfillment-effective-time-judgments": {
		endpoint: func(t *testing.T, db *bentopg.DB, intake *tfhttp.IsolatedCommandIntake) http.Handler {
			judge, err := buildEffectiveTimeJudgment(db)
			if err != nil {
				t.Fatalf("装配有效时间判断编排：%v", err)
			}
			return tfhttp.NewJudgeEffectiveTimeEndpoint(intake, judge)
		},
		body:    `{"fact":"SYN-TRACKING-FACT-08-12","effectiveAt":"2026-09-24T16:00:00+08:00"}`,
		status:  http.StatusOK,
		outcome: "INPUT_NOT_ACCEPTED",
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
