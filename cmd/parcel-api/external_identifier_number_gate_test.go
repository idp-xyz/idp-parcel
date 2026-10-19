package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件经生产装配在真库上钉外部标识关联册「号码存得下」的门（票 ps-external-mark-relations/06）：写的一侧是首方提交口，号码
// 存不进册时答坏报文、点名那一格、委托不落；读的一侧是收寄：超长的号照常去问 PS，查不到答已核对的待识别、不落未核对、
// 不响门铃，门立起来之前进册的超长号照样唯一关联；带 NUL 的号不去问、答零个候选。
// 面单渠道结果那条写册路的真库用例是 PS postgres 适配器包里的 TestAChannelResultWhoseNumberCannotEnterTheRegisterIsNotAccepted。
// 夹具全为合成（S 级）。

// gateDraft 是首方提交口的一份载荷，只有一件包裹；接入请求标识、委托参考与客户包裹参考由用例给。
func gateDraft(accessRequestID, shipmentReference, parcelReference string) string {
	return `{
  "accessRequestId": "` + accessRequestID + `",
  "customerShipmentReference": "` + shipmentReference + `",
  "requestedServiceProduct": "SYN-PRODUCT-STANDARD",
  "senderRelation": "SYN-SENDER-01",
  "senderAddress": "SYN-SENDER-ADDRESS",
  "recipientRelation": "SYN-RECIPIENT-01",
  "recipientAddress": "SYN-RECIPIENT-ADDRESS",
  "destinationServiceScope": "SYN-DEST/US",
  "parcels": [{"customerParcelReference": "` + parcelReference + `", "declaredWeightValue": "1.50", "declaredWeightUnit": "KG"}]
}`
}

// gateSubmissionAnswer 是提交口答复里本文件要看的几格：成功时的 outcome，坏报文时的 error。
type gateSubmissionAnswer struct {
	status  int
	raw     string
	Outcome string `json:"outcome"`
	Error   struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	} `json:"error"`
}

// Covers: 票 ps-external-mark-relations/06 裁定二与完成判据「越界的客户包裹件号答 400 点名该格，委托不落」——提交口经现行生产
// 装配：首方客户渠道 Intake（发行方替身签首方受众的令牌，客户渠道册登一个合成客户端并授提交能力）、提交编排、真库；归属坐标
// 取写开关那一格与登记册里一条匹配的合成区间，同 TestACustomerClientSubmitsThroughTheFirstPartyLineAgainstARealDatabase。客户
// 包裹参考 513 字节或带 NUL 时答 400 MALFORMED_REQUEST，detail 点名 parcels[0].customerParcelReference、不抄号码本身，来源
// 保全与委托一行都不落。恰 512 字节的照收：建单成立，快照里逐字是那个号。
func TestTheWiredSubmissionRefusesACustomerParcelReferenceTheRegisterCannotStore(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	appendIsolatedAuthorityInterval(t, db)
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := accessidentity.NewCustomerClientSubject(syntheticCustomerIssuer, syntheticCustomerSubject)
	if err != nil {
		t.Fatal(err)
	}
	registerCustomerClient(t, db, registry, subject, "SYN-TENANT-01", syntheticCustomerAccount, "SYN-CUSTOMER-GRANT-PSXM06-01")
	issuer := newCustomerIssuerDouble(t)
	verifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil || !configured {
		t.Fatalf("首方发行方参数：configured=%v err=%v", configured, err)
	}
	minter, err := buildCustomerClientMinter(verifier, registry, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	isolated := isolatedWriteAdmissionForTest(t)
	ownership, err := buildProductionOwnership(db, isolated, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := buildCustomerSubmissionIntake(minter, ownership)
	if err != nil {
		t.Fatalf("装配首方提交 Intake：%v", err)
	}
	submission, err := buildSubmissionOrchestration(db, isolated, isolatedSelfAuthority)
	if err != nil {
		t.Fatalf("装配提交编排：%v", err)
	}
	endpoint := shipmenthttp.NewSubmitShipmentRequestEndpoint(intake, submission)
	token := issuer.token(t, syntheticCustomerAudience, syntheticCustomerSubject)
	post := func(body string) gateSubmissionAnswer {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/shipment-requests", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, request)
		answer := gateSubmissionAnswer{status: recorder.Code, raw: recorder.Body.String()}
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
			t.Fatalf("解码响应 %q：%v", answer.raw, err)
		}
		return answer
	}
	rows := func(table string) int {
		t.Helper()
		return countRows(t, pool, `SELECT count(*) FROM parcel_shipment.`+table)
	}

	for _, refused := range []struct{ name, access, shipment, parcel string }{
		{"513 字节", "SYN-ACCESS-REQUEST-PSXM06-513", "SYN-CUSTREF-GATE-513", strings.Repeat("P", 513)},
		{"带 NUL", "SYN-ACCESS-REQUEST-PSXM06-NUL", "SYN-CUSTREF-GATE-NUL", `SYN-PCL\u0000REF`},
	} {
		t.Run(refused.name, func(t *testing.T) {
			answer := post(gateDraft(refused.access, refused.shipment, refused.parcel))
			if answer.status != http.StatusBadRequest || answer.Error.Code != "MALFORMED_REQUEST" {
				t.Fatalf("答 %d %s, want 400 MALFORMED_REQUEST", answer.status, answer.raw)
			}
			if !strings.HasPrefix(answer.Error.Detail, "parcels[0].customerParcelReference: ") {
				t.Fatalf("detail %q, want 点名 parcels[0].customerParcelReference", answer.Error.Detail)
			}
			if strings.Contains(answer.raw, "PPPP") || strings.Contains(answer.raw, "SYN-PCL") {
				t.Fatalf("答复把号码本身抄了回来：%s", answer.raw)
			}
		})
	}
	if sources, requests := rows("source_submission"), rows("shipment_request"); sources != 0 || requests != 0 {
		t.Fatalf("来源保全 %d 行、委托 %d 行, want 都是 0——坏报文不落", sources, requests)
	}

	longest := strings.Repeat("P", 512)
	admitted := post(gateDraft("SYN-ACCESS-REQUEST-PSXM06-512", "SYN-CUSTREF-GATE-512", longest))
	if admitted.status >= 300 || admitted.Outcome != "SUBMITTED" {
		t.Fatalf("512 字节：答 %d %s, want SUBMITTED", admitted.status, admitted.raw)
	}
	verbatim := countRows(t, pool, `SELECT count(*) FROM parcel_shipment.shipment_request
	  WHERE (snapshot -> 'customerParcelNumbers') @> jsonb_build_array(jsonb_build_object('number', $1::text))`, longest)
	if verbatim != 1 {
		t.Fatalf("快照里逐字是那 512 字节的委托 %d 份, want 1", verbatim)
	}
}

// Covers: 票 ps-external-mark-relations/06 裁定三、裁定补与完成判据「设备送来越界号，答待识别（已核对），不落未核对、不响门铃」——
// 收寄经生产装配（编排、收寄库、身份读口的 savepoint、自投门铃都是生产的）：观察到的号 513 字节或 4000 字节，册里没有，
// PS 读口按号比较、不构造索引行，长号不报错。明确接收答 200 PENDING_IDENTIFICATION、identityCheck CHECKED、没有候选、
// 控制照建；重核门铃一封都没有。答成未核对，门铃重投每次拿同一个号去问，永远续不上。
func TestTheWiredReceptionAnswersAMarkTheRegisterCannotHoldAsCheckedPending(t *testing.T) {
	pool, reception, command := wiredIdentifiedReception(t, "SYN-DELIVERY-PSXM06-513")
	for _, overlong := range []struct{ source, unit, mark string }{
		{"SYN-DELIVERY-PSXM06-513", "SYN-UNIT-PSXM06-513", strings.Repeat("M", 513)},
		{"SYN-DELIVERY-PSXM06-4000", "SYN-UNIT-PSXM06-4000", strings.Repeat("M", 4000)},
	} {
		command.SourceID = overlong.source
		command.Unit = mustValue(t, nodomain.NewHandlingUnitID, overlong.unit)
		command.Mark = noports.ExternalMarkObservation{Mark: overlong.mark}
		answer := decodeUnchecked(t, postWiredReception(t, reception, command))
		if answer.status != http.StatusOK || answer.Outcome != "PENDING_IDENTIFICATION" || answer.IdentityCheck != "CHECKED" {
			t.Fatalf("%d 字节的号：答 %d %s, want 200 PENDING_IDENTIFICATION identityCheck CHECKED",
				len(overlong.mark), answer.status, answer.raw)
		}
		if len(answer.Candidates) != 0 || answer.Association != "" || answer.ControlBasis == "" {
			t.Fatalf("%d 字节的号：答 %s, want 没有候选与关联、控制照建", len(overlong.mark), answer.raw)
		}
		if got := receptionRows(t, pool, overlong.source); got != 1 {
			t.Fatalf("%d 字节的号：收寄 %d 行, want 1", len(overlong.mark), got)
		}
	}
	if doorbells := uncheckedDoorbells(t, pool); len(doorbells) != 0 {
		t.Fatalf("重核门铃 %d 封, want 0——不可能在册的号不是身份读不动", len(doorbells))
	}
}

// Covers: 票 ps-external-mark-relations/06 裁定补——门立起来之前进册的超长号，现场扫到照样核得到。库里直写一份委托快照，客户包裹
// 件号是 600 字节（模拟门前留下的行），经仓储读回、接受，号随接受进册；设备送来同一个号，经生产收寄装配答 201
// INTAKE_FORMED、唯一关联那一件、控制照建，不落未核对、重核门铃零封。入册门挡在核对这一侧，这一件就落成待识别。
func TestTheWiredReceptionAssociatesAnOverlongNumberRegisteredBeforeTheGate(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	submission := numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-LEGACY-1", "SYN-PARCEL-LEGACY-1", "SYN-LEGACY-PLACEHOLDER")
	submitOnRealAssembly(t, db, submission)
	legacy := strings.Repeat("SYN-LEGACY-0600-", 37) + "SYN-LEGA"
	tag, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET snapshot = jsonb_set(snapshot, '{customerParcelNumbers,0,number}', to_jsonb($1::text))
		  WHERE snapshot -> 'customerParcelNumbers' -> 0 ->> 'number' = 'SYN-LEGACY-PLACEHOLDER'`, legacy)
	if err != nil || tag.RowsAffected() != 1 || len(legacy) != 600 {
		t.Fatalf("直写 %d 字节的旧号：rows = %d err = %v", len(legacy), tag.RowsAffected(), err)
	}
	acceptNumbered(t, db, submission)
	if rows := countRows(t, pool, `SELECT count(*) FROM parcel_shipment.external_identifier_relation WHERE identifier_number = $1`, legacy); rows != 1 {
		t.Fatalf("旧号在册 %d 行, want 1", rows)
	}

	command := receptionCommand(t, "SYN-DELIVERY-PSXM06-LEGACY", nodeopsapp.ExplicitReception)
	command.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-PSXM06-LEGACY")
	command.Mark = noports.ExternalMarkObservation{Mark: legacy}
	answer := decodeUnchecked(t, postWiredReception(t, reception, command))
	if answer.status != http.StatusCreated || answer.Outcome != "INTAKE_FORMED" || answer.Association != "SYN-PARCEL-LEGACY-1" {
		t.Fatalf("送站答 %d %s, want 201 INTAKE_FORMED 关联 SYN-PARCEL-LEGACY-1", answer.status, answer.raw)
	}
	if answer.IdentityCheck == "UNCHECKED" || answer.ControlBasis == "" {
		t.Fatalf("送站答 %s, want 身份已核对、控制照建", answer.raw)
	}
	if doorbells := uncheckedDoorbells(t, pool); len(doorbells) != 0 {
		t.Fatalf("重核门铃 %d 封, want 0", len(doorbells))
	}
}

// Covers: 票 ps-external-mark-relations/06 裁定补「带 NUL 的号经读口答零候选、不报错」——设备收寄口早已对 NUL 答坏报文
// （nodeopshttp 的 receptionCommand），带 NUL 的号送不到收寄编排，所以钉在 NO 适配器这一层：适配器接真库上的 PS 读口，答零个
// 候选、不带信号、不报错。适配器若把号交过去，Postgres 的 text 收不下 NUL，库面会报错，收寄就把它读成身份读不动。
func TestANulMarkIsAnsweredWithoutAskingTheRealRegister(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	identifiers, err := pspostgres.NewExternalIdentifiers(db)
	if err != nil {
		t.Fatalf("构造 PS 按号核对读口：%v", err)
	}
	view, err := noparcel.NewParcelIdentityView(identifiers)
	if err != nil {
		t.Fatalf("构造 NO 身份核对适配器：%v", err)
	}
	answer, err := view.ResolveParcelIdentity(t.Context(), mustValue(t, nodomain.NewTenantID, "SYN-TENANT-1"),
		noports.ExternalMarkObservation{Mark: "SYN-PCL\x00-NUL"})
	if err != nil || len(answer.Candidates) != 0 || answer.Ambiguous {
		t.Fatalf("带 NUL：answer = %+v err = %v, want 零候选、无信号、不报错", answer, err)
	}
}
