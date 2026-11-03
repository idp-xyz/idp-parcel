package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证票 document-numbering/16：写口的 JSON 请求体里，字符串含 NUL、原始字节不是合法 UTF-8、或 \u 转义出孤代理，各口照
// 自己「请求体读不出」那一格答 400，处理器之后的写一笔都不发生、库里无行；合法多字节、代理对、转义的反斜杠照收、原样落库。
// 测试输入是隔离合成，只记 `S`。
//
// 修之前（实测于 b29cf8216）：NUL 进了写的口答 500 NO_ANSWER_FORMED；非法 UTF-8 与孤代理被换成 U+FFFD 照常受理。运输履约有的口
// NUL 答 200 INPUT_NOT_ACCEPTED，首方提交的客户包裹件号带 NUL 答 400 并点名那一格——那些判断仍在各自的构造门里，钉它们的用例直挂
// 处理器（TestLoadAssignmentUnstorableValuesAreNotAcceptedOnTheProductionChannel、
// TestTheWiredSubmissionRefusesACustomerParcelReferenceTheRegisterCannotStore），生产路由上请求体检查先答 400。

// unstorableBodyValues 是放进 JSON 字符串里的三种坏处，照原样拼进载荷：NUL 转义、原始 FF 字节、孤高位转义。
var unstorableBodyValues = map[string]string{
	"NUL 转义": `\u0000`,
	"原始 FF":  "\xff",
	"孤高位转义":  `\ud800`,
}

// haltingBody 头一次 Read 交出 first：一段停在坏处之后、串还没收尾的载荷；再被读就记下 readPast。请求体检查判到坏处即停，不会
// 再读它；没判这种坏处的口要读完这个串才解得出值，一定会再读——读没读过这一段，就是这一行判没判它。
type haltingBody struct {
	first    string
	reads    int
	readPast bool
}

func (body *haltingBody) Read(p []byte) (int, error) {
	body.reads++
	if body.reads == 1 {
		return copy(p, body.first), nil
	}
	body.readPast = true
	return copy(p, `X"}`), io.EOF
}

const (
	haltAfterNUL = `{"probe":"SYN\u0000`
	haltAfterFF  = "{\"probe\":\"SYN\xff"
)

// assertBodyCheck 发一份停在坏处之后的请求体，断言这一行读了请求体、读到坏处即停、答 400。
func assertBodyCheck(t *testing.T, router http.Handler, pattern string, line endpointLine, probe businessEndpointProbe, first string) {
	t.Helper()
	body := &haltingBody{first: first}
	request := httptest.NewRequest(probe.method, probe.target, body)
	// 价卡两行按这个头走结构化工作簿那一支，其余各行不看它。
	request.Header.Set("Content-Type", "application/json")
	stampWorkScope(request, line)
	request.Header.Set("Authorization", "Bearer presented.token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if body.reads == 0 || body.readPast || response.Code != http.StatusBadRequest {
		t.Errorf("%s（%s，自建 Intake=%t）带 %q：读了 %d 次、读过坏处 %t，答 %d %s；want 读到坏处即停、答 400",
			pattern, line.face, line.ownIntake, first, body.reads, body.readPast, response.Code, response.Body.String())
	}
}

// assertEveryBadnessIsJudged 对一行发两份：停在 NUL 转义之后的，停在原始 FF 之后的。
func assertEveryBadnessIsJudged(t *testing.T, router http.Handler, pattern string, line endpointLine, probe businessEndpointProbe) {
	t.Helper()
	assertBodyCheck(t, router, pattern, line, probe, haltAfterNUL)
	assertBodyCheck(t, router, pattern, line, probe, haltAfterFF)
}

// admitEverything 是放行一切的准入范围：本文件只看请求体检查，准入那一格另有用例钉。
type admitEverything struct{}

func (admitEverything) Admits(context.Context, string, accessidentity.AdmissionRequirement, time.Time) (bool, error) {
	return true, nil
}

// everyOperatorGrant 是操作者册的替身：主体绑在 SYN-TENANT-01 上，持查阅读、登记写与 kinds 里每一种运营决定的授予。
type everyOperatorGrant struct{ kinds []accessidentity.DecisionKind }

func (register everyOperatorGrant) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	var grants []accessidentity.RecordedGrant
	record := func(grant accessidentity.OperatorGrant, err error) error {
		if err != nil {
			return err
		}
		recorded, err := accessidentity.NewRecordedGrant(grant, nil)
		grants = append(grants, recorded)
		return err
	}
	for index, capability := range []accessidentity.CapabilityFace{accessidentity.CapabilityMasterDataAndOperationsRead, accessidentity.CapabilityRegistryConfigurationWrite} {
		if err := record(accessidentity.NewOperatorGrant("SYN-TENANT-01", fmt.Sprintf("SYN-GRANT-FACE-%d", index), subject, capability, interval, "SYN-BASIS-GRANT")); err != nil {
			return accessidentity.OperatorStanding{}, false, err
		}
	}
	for index, kind := range register.kinds {
		if err := record(accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", fmt.Sprintf("SYN-GRANT-DECISION-%d", index), subject, kind, interval, "SYN-BASIS-GRANT")); err != nil {
			return accessidentity.OperatorStanding{}, false, err
		}
	}
	standing, err := accessidentity.NewOperatorStanding(binding, grants)
	return standing, err == nil, err
}

// everyCustomerCapability 是客户渠道册的替身：合成客户端绑在 SYN-TENANT-01 的合成账户上，持登记处首方各行声明的每一项能力。
type everyCustomerCapability struct{}

func (everyCustomerCapability) FindCustomerClient(_ context.Context, subject accessidentity.CustomerClientSubject) (accessidentity.CustomerClientStanding, bool, error) {
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-REF/customer-01")
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	binding, err := accessidentity.NewCustomerClientBinding(subject, "SYN-TENANT-01", syntheticCustomerAccount, reference, false, "SYN-BASIS")
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	var grants []accessidentity.RecordedCustomerClientGrant
	for _, capability := range declaredCustomerCapabilities() {
		grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-"+string(capability), subject, capability, interval, "SYN-BASIS-GRANT")
		if err != nil {
			return accessidentity.CustomerClientStanding{}, false, err
		}
		recorded, err := accessidentity.NewRecordedCustomerClientGrant(grant, nil)
		if err != nil {
			return accessidentity.CustomerClientStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewCustomerClientStanding(binding, grants)
	return standing, err == nil, err
}

func declaredDecisionKinds() []accessidentity.DecisionKind {
	seen := map[accessidentity.DecisionKind]bool{}
	var kinds []accessidentity.DecisionKind
	for _, line := range declaredEndpointLines() {
		if line.face.decision != "" && !seen[line.face.decision] {
			seen[line.face.decision] = true
			kinds = append(kinds, line.face.decision)
		}
	}
	return kinds
}

func declaredExternalFacts() []accessidentity.ExternalFactType {
	seen := map[accessidentity.ExternalFactType]bool{}
	var facts []accessidentity.ExternalFactType
	for _, line := range declaredEndpointLines() {
		if line.face.fact != "" && !seen[line.face.fact] {
			seen[line.face.fact] = true
			facts = append(facts, line.face.fact)
		}
	}
	return facts
}

func declaredCustomerCapabilities() []accessidentity.CustomerCapability {
	seen := map[accessidentity.CustomerCapability]bool{}
	var capabilities []accessidentity.CustomerCapability
	for _, line := range declaredEndpointLines() {
		if line.face.capability != "" && !seen[line.face.capability] {
			seen[line.face.capability] = true
			capabilities = append(capabilities, line.face.capability)
		}
	}
	return capabilities
}

// writeLinePatterns 交回登记处里读请求体的各行（非 GET），按 ownIntake 筛。
func writeLinePatterns(ownIntake bool) []string {
	return declaredPatterns(func(line endpointLine) bool {
		return line.method != http.MethodGet && line.ownIntake == ownIntake
	})
}

// Covers: 票 document-numbering/16——检查挂在登记处、不靠逐口记得：经共享渠道的每一行写口（各族渠道打开、授予与准入齐备，读口与
// 编排是未接线占位）读请求体时读到 NUL 或非法 UTF-8 即停、答 400；不挂检查的口要读完那个串才解得出值。自建 Intake 的写口由
// TestEveryOwnIntakeWriteLineReadsItsBodyThroughTheCheck 在生产构造上证。
func TestEveryWriteLineReadsItsBodyThroughTheCheck(t *testing.T) {
	operatorMinter, err := buildOperatorMinter(acceptingVerifier{}, everyOperatorGrant{kinds: declaredDecisionKinds()}, admitEverything{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(operatorMinter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(operatorMinter)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	clientMinter, err := buildIntegrationClientMinter(acceptingClientVerifier{},
		clientFactRegister{facts: declaredExternalFacts(), start: now.Add(-time.Hour), end: now.Add(time.Hour)}, admitEverything{})
	if err != nil {
		t.Fatal(err)
	}
	clients, err := buildIntegrationClientIntakes(clientMinter)
	if err != nil {
		t.Fatal(err)
	}
	operators := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(
		decisions, registries, unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil))
	routers := map[faceFamily]http.Handler{
		faceOperationsRead:    operators,
		faceRegistryWrite:     operators,
		faceOperationDecision: operators,
		faceIntegrationClient: httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(
			unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), clients, nil, nil, nil, nil)),
		faceWorkFact: workFactOperatorRouter(t, acceptingVerifier{}, workFactOperators{}, workFactDevices{}),
		faceCustomerClient: httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWith(nil,
			customerSubmissionWith(t, fixedCustomerVerifier{}, everyCustomerCapability{}))),
	}

	lines, probes := declaredEndpointLines(), businessEndpointProbes()
	patterns := writeLinePatterns(false)
	for _, pattern := range patterns {
		line := lines[pattern]
		router, ok := routers[line.face.family]
		if !ok {
			t.Fatalf("%s：族 %s 没有打开的渠道，本用例探不到它", pattern, line.face.family)
		}
		assertEveryBadnessIsJudged(t, router, pattern, line, probes[pattern])
	}
	if len(patterns) == 0 {
		t.Fatal("一行经共享渠道的写口都没探到")
	}
}

// Covers: 票 document-numbering/16——自建 Intake 的写口（编号规则册、节点交接单与协作执行、首方族各口）经登记处的生产构造出端点，
// 认证件取授予齐备的合成铸造器：读请求体时读到非法 UTF-8 或 NUL 即停、答 400。
func TestEveryOwnIntakeWriteLineReadsItsBodyThroughTheCheck(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	operatorMinter, err := buildOperatorMinter(acceptingVerifier{}, everyOperatorGrant{kinds: declaredDecisionKinds()}, admitEverything{})
	if err != nil {
		t.Fatal(err)
	}
	customerMinter, err := buildCustomerClientMinter(fixedCustomerVerifier{}, everyCustomerCapability{}, admitEverything{})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := aipg.NewDeviceRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	node := workScopeFixtures[accessidentity.WorkScopeNode]
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		registration, err := accessidentity.NewDeviceRegistration("SYN-TENANT-01", node.device, mustScope("NODE", node.reference), "SYN-BASIS")
		if err != nil {
			return err
		}
		_, err = devices.RegisterDevice(ctx, registration)
		return err
	}); err != nil {
		t.Fatalf("登设备：%v", err)
	}
	sources := endpointSources{
		db: db, operatorMinter: operatorMinter, customerClientMinter: customerMinter,
		operatorVerifier: acceptingVerifier{}, operatorRegistry: workFactOperators{}, deviceRegistry: devices, admission: admitEverything{},
		documentNumbers: documentNumberingComponents(t, db, docnumber.SecretTable{}),
		isolatedWrite:   isolatedWriteAdmissionForTest(t), selfAuthority: isolatedSelfAuthority,
	}
	patterns := writeLinePatterns(true)
	owned := map[string]bool{}
	for _, pattern := range patterns {
		owned[pattern] = true
	}
	var endpoints []httpapi.BusinessEndpoint
	for _, feature := range endpointRegistry {
		owns := false
		for pattern := range feature.lines {
			owns = owns || owned[pattern]
		}
		if !owns {
			continue
		}
		mounted, err := feature.production(sources, endpointChannels{})
		if err != nil {
			t.Fatalf("装配 %T：%v", feature.wiringKey, err)
		}
		endpoints = append(endpoints, mounted...)
	}
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, endpoints)

	lines, probes := declaredEndpointLines(), businessEndpointProbes()
	for _, pattern := range patterns {
		assertEveryBadnessIsJudged(t, router, pattern, lines[pattern], probes[pattern])
	}
	if len(patterns) == 0 {
		t.Fatal("一行自建 Intake 的写口都没探到")
	}
}

// Covers: 票 document-numbering/16——收价卡工作簿上传的两行，请求体是 multipart/form-data 时不判：上传的文件字节原样到处理器
// 手上。同一行带 application/json、或别的写口带 multipart 的头，照判：那几种请求体处理器都按 JSON 解。
func TestOnlyAMultipartUploadPassesTheBodyCheckUnjudged(t *testing.T) {
	lines := declaredEndpointLines()
	binary := "PK\x03\x04\xff\x00\xfe"
	var read []byte
	var readErr error
	serve := func(pattern, contentType string) {
		guard := requestBodyGuard(lines[pattern])
		if guard == nil {
			t.Fatalf("%s 没挂请求体检查", pattern)
		}
		handler := guard(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			read, readErr = io.ReadAll(request.Body)
		}))
		request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(binary))
		request.Header.Set("Content-Type", contentType)
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}

	for _, pattern := range []string{"/pricing-price-card-previews", "/pricing-price-card-drafts"} {
		serve(pattern, "multipart/form-data; boundary=SYN")
		if readErr != nil || string(read) != binary {
			t.Fatalf("%s multipart：读到 %q（%v），want 原样", pattern, read, readErr)
		}
		serve(pattern, "application/json")
		if !errors.Is(readErr, httpapi.ErrUnstorableBody) {
			t.Fatalf("%s application/json：err %v，want ErrUnstorableBody", pattern, readErr)
		}
	}
	serve("/pricing-price-card-registrations", "multipart/form-data; boundary=SYN")
	if !errors.Is(readErr, httpapi.ErrUnstorableBody) {
		t.Fatalf("不收上传的行带 multipart 头：err %v，want 照判", readErr)
	}
}

// bodyGuarded 给一个直挂的处理器套上登记处为这一行算好的那道请求体检查，与生产装配同源。
func bodyGuarded(t *testing.T, pattern string, handler http.Handler) http.Handler {
	t.Helper()
	guard, ok := requestBodyGuards(declaredEndpointLines())[pattern]
	if !ok {
		t.Fatalf("%s 没挂请求体检查", pattern)
	}
	return guard(handler)
}

// errorCode 读答复里的错误码：操作者各族的旧形状在 error.code，首方族的问题详情在顶层 code。
func errorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var answer struct {
		Code  string `json:"code"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("解答复 %q：%v", response.Body.String(), err)
	}
	if answer.Error.Code != "" {
		return answer.Error.Code
	}
	return answer.Code
}

func assertMalformedBody(t *testing.T, name string, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusBadRequest || errorCode(t, response) != "MALFORMED_REQUEST" {
		t.Errorf("%s：答 %d %s，want 400 MALFORMED_REQUEST", name, response.Code, response.Body.String())
	}
}

func countOf(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var rows int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&rows); err != nil {
		t.Fatalf("数行 %s：%v", query, err)
	}
	return rows
}

// Covers: 票 document-numbering/16 完成判据「cmd 真库测试」——票 15 探到的三个登记口，与评审探到的运输履约、关务各一口（两口的
// basis 只判空白），经登记处的生产构造接真库：三种坏处都答 400 MALFORMED_REQUEST，库里无行（修前 NUL 答 500，非法 UTF-8 与孤代理
// 换成 U+FFFD 落库）。同一格换成合法值照常答 201：参与方名称用中文、emoji 原样、代理对转义与转义的反斜杠，读回的字节与调用方
// 送来的逐字相同，没被改写。
func TestUnstorableBodiesNeverReachTheRegistersAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	sources := endpointSources{db: db, documentNumbers: documentNumberingComponents(t, db, docnumber.SecretTable{})}
	channels := endpointChannels{operatorRegistries: operatorRegistriesForRegistryWriter(t)}
	wanted := map[string]bool{
		"/commercial-business-party-registrations":                             true,
		"/visibility-catalogue-milestone-mapping-registrations":                true,
		"/network-catalog-node-registrations":                                  true,
		"/transport-fulfillment-delivery-dispatch-reask-cadence-registrations": true,
		"/customs-case-requirement-registrations":                              true,
	}
	var endpoints []httpapi.BusinessEndpoint
	for _, feature := range endpointRegistry {
		owns := false
		for pattern := range feature.lines {
			owns = owns || wanted[pattern]
		}
		if !owns {
			continue
		}
		mounted, err := feature.production(sources, channels)
		if err != nil {
			t.Fatalf("装配 %T：%v", feature.wiringKey, err)
		}
		endpoints = append(endpoints, mounted...)
	}
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, endpoints)
	post := func(target, body string) *httptest.ResponseRecorder {
		return postAsOperator(router, target, presentedOperatorToken, body)
	}

	party := func(id, name string) string {
		return `{"businessParties":[{"partyId":"` + id + `","name":"` + name + `","revision":1,` +
			`"basis":"SYN-BASIS/docnum16","effectiveFrom":"2026-01-01T00:00:00Z"}]}`
	}
	mapping := func(version string) string {
		return `{"version":"` + version + `","approvedBy":"SYN-approver-1","effectiveFrom":"2026-09-01T00:00:00Z",` +
			`"entries":[{"source":"PARCEL_SHIPMENT","factKind":"SYN_KIND_DELIVERED","milestone":"SYN-MILESTONE-DELIVERED"}]}`
	}
	node := func(code string) string {
		return `{"code":"` + code + `","version":1,"business_timezone":"Asia/Shanghai","effective_from":"2026-09-01T00:00:00Z"}`
	}
	cadence := func(version, basis string) string {
		return `{"version":"` + version + `","intervalMinutes":60,"basis":"` + basis + `"}`
	}
	requirement := func(procedure, basis string) string {
		return `{"jurisdictionRef":"SYN-JURISDICTION-SG","direction":"IMPORT","procedureRef":"` + procedure + `","required":true,"basis":"` + basis + `"}`
	}
	names := make([]string, 0, len(unstorableBodyValues))
	for name := range unstorableBodyValues {
		names = append(names, name)
	}
	sort.Strings(names)
	for index, name := range names {
		value := unstorableBodyValues[name]
		partyID := fmt.Sprintf("SYN-PARTY-DN16-BAD-%d", index)
		assertMalformedBody(t, "参与方名称 "+name, post("/commercial-business-party-registrations", party(partyID, "SYN"+value+"X")))
		assertMalformedBody(t, "映射版本 "+name, post("/visibility-catalogue-milestone-mapping-registrations", mapping("SYN-MAP-DN16-BAD"+value)))
		assertMalformedBody(t, "节点代码 "+name, post("/network-catalog-node-registrations", node("SYN-NODE-DN16-BAD"+value)))
		assertMalformedBody(t, "重问节奏依据 "+name, post("/transport-fulfillment-delivery-dispatch-reask-cadence-registrations",
			cadence(fmt.Sprintf("SYN-REASK-DN16-BAD-%d", index), "SYN-BASIS"+value)))
		assertMalformedBody(t, "建案要求依据 "+name, post("/customs-case-requirement-registrations",
			requirement(fmt.Sprintf("SYN-PROCEDURE-DN16-BAD-%d", index), "SYN-BASIS"+value)))
	}
	for table, query := range map[string]string{
		"参与方":  `SELECT count(*) FROM party_commercial.business_party_registration WHERE party_id LIKE 'SYN-PARTY-DN16-BAD-%'`,
		"映射版本": `SELECT count(*) FROM visibility_exception.milestone_mapping_version WHERE mapping_version LIKE 'SYN-MAP-DN16-BAD%'`,
		"节点":   `SELECT count(*) FROM network_routing.logistics_node_version WHERE node_code LIKE 'SYN-NODE-DN16-BAD%'`,
		"重问节奏": `SELECT count(*) FROM transport_fulfillment.delivery_dispatch_reask_cadence WHERE version LIKE 'SYN-REASK-DN16-BAD-%'`,
		"建案要求": `SELECT count(*) FROM customs_compliance.case_requirement_rule WHERE procedure_ref LIKE 'SYN-PROCEDURE-DN16-BAD-%'`,
	} {
		if rows := countOf(t, pool, query); rows != 0 {
			t.Errorf("%s册上 %d 行坏值，want 零", table, rows)
		}
	}

	for target, body := range map[string]string{
		"/visibility-catalogue-milestone-mapping-registrations":                mapping("SYN-MAP-DN16-GOOD"),
		"/network-catalog-node-registrations":                                  node("SYN-NODE-DN16-GOOD"),
		"/transport-fulfillment-delivery-dispatch-reask-cadence-registrations": cadence("SYN-REASK-DN16-GOOD", "SYN-BASIS/reask"),
		"/customs-case-requirement-registrations":                              requirement("SYN-PROCEDURE-DN16-GOOD", "SYN-BASIS/requirement"),
	} {
		if response := post(target, body); response.Code != http.StatusCreated {
			t.Fatalf("%s 合规载荷：答 %d %s，want 201", target, response.Code, response.Body.String())
		}
	}
	for index, storable := range []struct{ sent, stored string }{
		{"SYN 合成参与方（中文）", "SYN 合成参与方（中文）"},
		{"SYN 📦 原样", "SYN 📦 原样"},
		{`SYN \ud83d\udce6 转义`, "SYN 📦 转义"},
		{`SYN C:\\u0000`, `SYN C:\u0000`},
	} {
		partyID := fmt.Sprintf("SYN-PARTY-DN16-GOOD-%d", index)
		if response := post("/commercial-business-party-registrations", party(partyID, storable.sent)); response.Code != http.StatusCreated {
			t.Fatalf("参与方名称 %q：答 %d %s，want 201", storable.sent, response.Code, response.Body.String())
		}
		var stored string
		if err := pool.QueryRow(t.Context(), `SELECT party_name FROM party_commercial.business_party_registration WHERE party_id = $1`, partyID).Scan(&stored); err != nil {
			t.Fatalf("读回 %s：%v", partyID, err)
		}
		if stored != storable.stored {
			t.Errorf("参与方名称 %q 落库为 %q，want %q", storable.sent, stored, storable.stored)
		}
	}
}

// Covers: 票 document-numbering/16 完成判据「每族至少一个代表写口」——运营决定（装载分配）、作业事实登记（移动事实）、集成客户端
// （监管凭证登记）、首方客户（提交委托）各取一口，经生产构造的 Intake 与编排接真库，套上登记处为这一行算好的那道检查：三种
// 坏处都答该族的 400 MALFORMED_REQUEST（首方族是问题详情），库里无行。装载分配与移动事实的 NUL 修前答 200 INPUT_NOT_ACCEPTED，
// 修后同答 400。登记册配置写一族见
// TestUnstorableBodiesNeverReachTheRegistersAgainstARealDatabase；主数据与运营查阅读一族的两行写口（试算、编号预览）不落库，
// 只由 TestEveryWriteLineReadsItsBodyThroughTheCheck 与 TestEveryOwnIntakeWriteLineReadsItsBodyThroughTheCheck 证。
func TestEachFamilyAnswersUnstorableBodiesWithItsOwnBadRequestAgainstARealDatabase(t *testing.T) {
	t.Run("运营决定：装载分配", func(t *testing.T) {
		fixture := newChainFaceFixture(t)
		fixture.registerSchedules(t, "SYN-SCHEDULE-01")
		endpoint := bodyGuarded(t, "/transport-fulfillment-load-assignment-registrations", fixture.endpoint)
		for name, value := range unstorableBodyValues {
			// 不经 chainFaceBody：它按 Go 的 %q 引号，会把反斜杠再转义一层。
			request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-load-assignment-registrations", strings.NewReader(
				`{"assignment":"SYN-LA-DN16","schedule":"SYN-SCHEDULE-01","members":["SYN-PARCEL-01"],"version":"SYN-LA-V1`+value+`","assignedAt":"2026-09-25T10:00:00Z"}`))
			request.Header.Set("Authorization", "Bearer "+presentedOperatorToken)
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, request)
			assertMalformedBody(t, name, response)
		}
		if rows := countOf(t, fixture.pool, `SELECT count(*) FROM transport_fulfillment.load_assignment WHERE assignment_ref = 'SYN-LA-DN16'`); rows != 0 {
			t.Errorf("装载分配 %d 行，want 零", rows)
		}
		if status, view, raw := fixture.post(t, chainFaceBody("SYN-LA-DN16", "SYN-SCHEDULE-01", "SYN-LA-V1", "2026-09-25T10:00:00Z", "SYN-PARCEL-01")); status != http.StatusCreated || view.Outcome != "LOAD_ASSIGNMENT_FORMED" {
			t.Fatalf("合规载荷：答 %d %s", status, raw)
		}
	})

	t.Run("作业事实登记：移动事实", func(t *testing.T) {
		pool, _, handler := sameIdentityProductionEndpoint(t)
		endpoint := bodyGuarded(t, "/transport-fulfillment/movement-facts", handler)
		post := func(location string) *httptest.ResponseRecorder {
			request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/movement-facts", strings.NewReader(
				`{"fact":"SYN-MOVE-DN16","schedule":"SYN-SCHEDULE-01","kind":"ARRIVAL","location":"`+location+`","version":"1","occurredAt":"2026-09-25T10:00:00Z"}`))
			stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-A")
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, request)
			return response
		}
		for name, value := range unstorableBodyValues {
			assertMalformedBody(t, name, post("SYN-NODE-01"+value))
		}
		if rows := tableRows(t, pool, "transport_movement_fact"); rows != 0 {
			t.Errorf("移动事实 %d 行，want 零", rows)
		}
		if response := post("SYN-NODE-01"); response.Code != http.StatusCreated {
			t.Fatalf("合规载荷：答 %d %s", response.Code, response.Body.String())
		}
	})

	t.Run("集成客户端：监管凭证登记", func(t *testing.T) {
		intakes, db, count := integrationClientChannelOnRealDB(t, accessidentity.FactRegulatoryCredential)
		registration, err := buildCustomsRegistrationOrchestration(db)
		if err != nil {
			t.Fatal(err)
		}
		endpoint := bodyGuarded(t, "/customs-regulatory-credential-registrations",
			customshttp.NewRegisterRegulatoryCredentialEndpoint(intakes.customs, registration.regulatoryCredential))
		for name, value := range unstorableBodyValues {
			fields := ordinaryCredential("SYN-CRED-DN16")
			fields.holder = "SYN-HOLDER" + value + "/broker-dn16"
			if status, answer := registrationAnswer(t, endpoint, fields.body()); status != http.StatusBadRequest || answer != "MALFORMED_REQUEST" {
				t.Errorf("%s：答 %d %s，want 400 MALFORMED_REQUEST", name, status, answer)
			}
		}
		if rows := count(`SELECT count(*) FROM customs_compliance.regulatory_credential WHERE credential_id = 'SYN-CRED-DN16'`); rows != 0 {
			t.Errorf("监管凭证 %d 行，want 零", rows)
		}
		if status, answer := registrationAnswer(t, endpoint, ordinaryCredential("SYN-CRED-DN16").body()); status != http.StatusCreated {
			t.Fatalf("合规载荷：答 %d %s", status, answer)
		}
	})

	t.Run("首方客户：提交委托", func(t *testing.T) {
		pool := pgtest.Pool(t)
		db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
		if err != nil {
			t.Fatalf("构造框架 DB：%v", err)
		}
		submission, err := buildSubmissionOrchestration(db, isolatedWriteAdmissionForTest(t), isolatedSelfAuthority, documentAssignerForTest(t, db))
		if err != nil {
			t.Fatalf("装配提交编排：%v", err)
		}
		now := time.Now()
		intake := customerSubmissionWith(t, fixedCustomerVerifier{}, customerStandingRegister{start: now.Add(-time.Hour), end: now.Add(time.Hour)})
		endpoint := bodyGuarded(t, "/v1/shipment-requests", shipmenthttp.NewSubmitShipmentRequestEndpoint(intake, submission))
		post := func(accessRequestID, recipientAddress string) *httptest.ResponseRecorder {
			body := strings.Replace(gateDraft(accessRequestID, "SYN-CUSTREF-DN16", "SYN-PCLREF-DN16"), "SYN-RECIPIENT-ADDRESS", recipientAddress, 1)
			request := httptest.NewRequest(http.MethodPost, "/v1/shipment-requests", strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer presented.customer.token")
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, request)
			return response
		}
		for name, value := range unstorableBodyValues {
			response := post("SYN-ACCESS-REQUEST-DN16-BAD", "SYN-RECIPIENT-ADDRESS"+value)
			assertMalformedBody(t, name, response)
			if contentType := response.Header().Get("Content-Type"); contentType != "application/problem+json" {
				t.Errorf("%s：Content-Type %q，want 首方族的问题详情", name, contentType)
			}
		}
		if sources, requests := countOf(t, pool, `SELECT count(*) FROM parcel_shipment.source_submission`),
			countOf(t, pool, `SELECT count(*) FROM parcel_shipment.shipment_request`); sources != 0 || requests != 0 {
			t.Errorf("来源保全 %d 行、委托 %d 行，want 都是零", sources, requests)
		}
		if response := post("SYN-ACCESS-REQUEST-DN16-GOOD", "SYN-RECIPIENT-ADDRESS"); response.Code >= http.StatusBadRequest {
			t.Fatalf("合规载荷：答 %d %s", response.Code, response.Body.String())
		}
	})
}
