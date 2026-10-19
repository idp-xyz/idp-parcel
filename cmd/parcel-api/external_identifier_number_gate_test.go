package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件经生产装配在真库上钉外部标识关联册「号码存得下」的门（票 ps-external-mark-relations/06）：写的一侧是提交口，号码存
// 不进册时答坏报文、点名那一格、委托不落；读的一侧是收寄，设备送来不可能在册的号答已核对的待识别，不落未核对、不响门铃。
// 面单渠道结果那条写册路的真库用例是 PS postgres 适配器包里的 TestAChannelResultWhoseNumberCannotEnterTheRegisterIsNotAccepted。
// 夹具全为合成（S 级）。

// gateDraft 是管理台提交页送来的一份草案，只有一件包裹；委托参考与客户包裹参考由用例给。
func gateDraft(shipmentReference, parcelReference string) string {
	return `{
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
		Code  string `json:"code"`
		Field string `json:"field"`
	} `json:"error"`
}

// Covers: 票 ps-external-mark-relations/06 裁定二与完成判据「越界的客户包裹件号答 400 点名该格，委托不落」——提交口经生产装配
// （隔离提交 Intake、提交编排、真库、登记册里一条匹配的合成权威区间）：客户包裹参考 513 字节或带 NUL 时答 400
// MALFORMED_REQUEST，error.field 点名 parcels[0].customerParcelReference，来源保全与委托一行都不落。修前 513 字节的照收、建单
// 成立，号码进了快照，接受时才去撞册；带 NUL 的在落库那一步报错，答 500。恰 512 字节的照收：建单成立，快照里逐字是那个号。
func TestTheWiredSubmissionRefusesACustomerParcelReferenceTheRegisterCannotStore(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	appendIsolatedAuthorityInterval(t, db)
	admission := isolatedWriteAdmissionForTest(t)
	submission, err := buildSubmissionOrchestration(db, admission, isolatedSelfAuthority)
	if err != nil {
		t.Fatalf("装配隔离形态提交编排：%v", err)
	}
	intake, err := buildIsolatedSubmissionIntake(db, admission)
	if err != nil {
		t.Fatalf("装配隔离提交 Intake：%v", err)
	}
	endpoint := shipmenthttp.NewSubmitShipmentRequestEndpoint(intake, submission)
	post := func(body string) gateSubmissionAnswer {
		t.Helper()
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/shipment-requests", strings.NewReader(body)))
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

	for _, refused := range []struct{ name, shipment, parcel string }{
		{"513 字节", "SYN-CUSTREF-GATE-513", strings.Repeat("P", 513)},
		{"带 NUL", "SYN-CUSTREF-GATE-NUL", `SYN-PCL\u0000REF`},
	} {
		t.Run(refused.name, func(t *testing.T) {
			answer := post(gateDraft(refused.shipment, refused.parcel))
			if answer.status != http.StatusBadRequest || answer.Error.Code != "MALFORMED_REQUEST" {
				t.Fatalf("答 %d %s, want 400 MALFORMED_REQUEST", answer.status, answer.raw)
			}
			if answer.Error.Field != "parcels[0].customerParcelReference" {
				t.Fatalf("field %q, want 点名 parcels[0].customerParcelReference", answer.Error.Field)
			}
			if strings.Contains(answer.raw, "PPPP") {
				t.Fatalf("答复把号码本身抄了回来：%s", answer.raw)
			}
		})
	}
	if sources, requests := rows("source_submission"), rows("shipment_request"); sources != 0 || requests != 0 {
		t.Fatalf("来源保全 %d 行、委托 %d 行, want 都是 0——坏报文不落", sources, requests)
	}

	longest := strings.Repeat("P", 512)
	admitted := post(gateDraft("SYN-CUSTREF-GATE-512", longest))
	if admitted.status >= 300 || admitted.Outcome != "SUBMITTED" {
		t.Fatalf("512 字节：答 %d %s, want SUBMITTED", admitted.status, admitted.raw)
	}
	verbatim := countRows(t, pool, `SELECT count(*) FROM parcel_shipment.shipment_request
	  WHERE (snapshot -> 'customerParcelNumbers') @> jsonb_build_array(jsonb_build_object('number', $1::text))`, longest)
	if verbatim != 1 {
		t.Fatalf("快照里逐字是那 512 字节的委托 %d 份, want 1", verbatim)
	}
}

// Covers: 票 ps-external-mark-relations/06 裁定三与完成判据「设备送来越界号，答待识别（已核对），不落未核对、不响门铃」——收寄经
// 生产装配（编排、收寄库、身份读口的 savepoint、自投门铃都是生产的）：观察到的号 513 字节或 4000 字节，PS 号码的构造门
// 立不起它，它就不可能在册。明确接收答 200 PENDING_IDENTIFICATION、identityCheck CHECKED、没有候选、控制照建；重核门铃
// 一封都没有。答成未核对，门铃重投每次拿同一个号去问，永远续不上。
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
