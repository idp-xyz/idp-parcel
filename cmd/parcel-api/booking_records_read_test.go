package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

const recordsPath = "/transport-fulfillment-records"

// Covers: `AT-TF-332`、`AT-TF-333`、`AT-TF-334` 真库面，票 demo-chain-gaps/48、167 完成判据（S）——
//   - 订舱、答复两口的生产装配落下的东西，查阅口经操作者查阅读读得到：干线订舱被接受全量，读作 CONFIRMED、当前那一份就是那份接受；
//     末端承运方答干线订舱、干线承运方答一个不在册的订舱，挡回册上各一行，依据一个是不是服务提供方、一个是订舱不在册；按订舱筛只剩
//     那一单的。
//   - 同一个发行方签的集成客户端令牌在答复口照常受理（201），拿到查阅口两格都答 401 OPERATOR_CREDENTIAL_REJECTED、不带 outcome：
//     挡回册只开给操作者渠道（ADR-0239 决定一）。两族的令牌核验方都是真的 OIDC 校验器，各认各的受众。
func TestTheBookingAndBounceRegistriesAreReadOnTheOperatorChannelOnly(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	registerSeededTrunkSchedule(t, db)
	endpoints := bookingFacesForTest(t, db)

	issuer := newCustomerIssuerDouble(t)
	environment := fakeGetenv(map[string]string{
		operatorIssuerEnv:            syntheticCustomerIssuer,
		operatorJWKSURLEnv:           issuer.jwks.URL,
		operatorAudienceEnv:          syntheticOperatorAudience,
		integrationClientIssuerEnv:   syntheticCustomerIssuer,
		integrationClientJWKSURLEnv:  issuer.jwks.URL,
		integrationClientAudienceEnv: "idp-parcel-integration",
	})
	operatorVerifier, configured, err := buildOperatorCredentialVerifier(environment)
	if err != nil || !configured {
		t.Fatalf("operator parameters: configured=%v err=%v", configured, err)
	}
	readers, err := buildOperatorMinter(operatorVerifier, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	readIntakes, err := buildOperatorRegistryIntakes(readers)
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := tfpostgres.NewReviewCatalogue(db)
	if err != nil {
		t.Fatalf("查阅读口：%v", err)
	}
	records := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(readIntakes.transportOperationsRead, catalogue)

	clientVerifier, configured, err := buildIntegrationClientCredentialVerifier(environment)
	if err != nil || !configured {
		t.Fatalf("integration client parameters: configured=%v err=%v", configured, err)
	}
	clients, err := aipg.NewIntegrationClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	clientMinter, err := buildIntegrationClientMinter(clientVerifier, clients, admission)
	if err != nil {
		t.Fatal(err)
	}
	clientIntakes, err := buildIntegrationClientIntakes(clientMinter)
	if err != nil {
		t.Fatal(err)
	}
	faces, err := buildBookingFaces(db)
	if err != nil {
		t.Fatalf("装配订舱两口：%v", err)
	}
	trunkAnswers := tfhttp.NewAnswerBookingEndpoint(clientIntakes.transport, faces)
	integrationToken := issuer.token(t, "idp-parcel-integration", "SYN-CLIENT-CARRIER-TRUNK-01")
	operatorToken := issuer.token(t, syntheticOperatorAudience, "SYN-OPERATOR-READER-01")

	if status, answer := postWithToken(t, endpoints.commission, commissionSubmissionPath, "presented.operator.token",
		commissionBody("SYN-COMMISSION-TRUNK-DCG24", trunk.Declarations.SupplierAgreementBody.Supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z")); status != http.StatusCreated {
		t.Fatalf("干线委托 = %d %v", status, answer)
	}
	if status, answer := postWithToken(t, endpoints.booking, bookingSubmissionPath, "presented.operator.token",
		trunkBookingBody("SYN-BOOKING-TRUNK-DCG24", seededTrunkSchedule)); status != http.StatusCreated {
		t.Fatalf("干线订舱 = %d %v", status, answer)
	}
	if status, answer := postWithToken(t, trunkAnswers, bookingAnswerPath, integrationToken, trunkAcceptedAnswer); status != http.StatusCreated || answer["outcome"] != "BOOKING_ANSWERED" {
		t.Fatalf("集成客户端令牌在答复口 = %d %v", status, answer)
	}
	if status, answer := postWithToken(t, endpoints.lastMile, bookingAnswerPath, "presented.client.token", trunkAcceptedAnswer); status != http.StatusOK || answer["outcome"] != "BOOKING_NOT_FOUND" {
		t.Fatalf("末端承运方答干线订舱 = %d %v", status, answer)
	}
	absent := strings.Replace(trunkAcceptedAnswer, "SYN-BOOKING-TRUNK-DCG24", "SYN-BOOKING-DCG167-NOWHERE", 1)
	if status, answer := postWithToken(t, trunkAnswers, bookingAnswerPath, integrationToken, absent); status != http.StatusOK || answer["outcome"] != "BOOKING_NOT_FOUND" {
		t.Fatalf("干线答不在册订舱 = %d %v", status, answer)
	}

	read := func(token, query string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, recordsPath+"?"+query, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		records.ServeHTTP(response, request)
		return response
	}
	var bookings struct {
		Outcome  string `json:"outcome"`
		Bookings []struct {
			Booking       string `json:"booking"`
			Standing      string `json:"standing"`
			CurrentAnswer struct {
				Answer   string `json:"answer"`
				Quantity int64  `json:"quantity"`
			} `json:"currentAnswer"`
		} `json:"bookings"`
	}
	if response := read(operatorToken, "registry=booking-request"); response.Code != http.StatusOK ||
		json.Unmarshal(response.Body.Bytes(), &bookings) != nil {
		t.Fatalf("操作者读订舱册 = %d %s", response.Code, response.Body.String())
	}
	if bookings.Outcome != "BOOKING_REQUESTS_LISTED" || len(bookings.Bookings) != 1 ||
		bookings.Bookings[0].Booking != "SYN-BOOKING-TRUNK-DCG24" || bookings.Bookings[0].Standing != "CONFIRMED" ||
		bookings.Bookings[0].CurrentAnswer.Answer != "SYN-CARRIER-ANSWER-DCG24" || bookings.Bookings[0].CurrentAnswer.Quantity != 120 {
		t.Fatalf("订舱册 = %+v", bookings)
	}

	bounces := func(query string) []string {
		t.Helper()
		response := read(operatorToken, "registry=booking-answer-bounce"+query)
		var body struct {
			Outcome string              `json:"outcome"`
			Bounces []map[string]string `json:"bounces"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Outcome != "BOOKING_ANSWER_BOUNCES_LISTED" {
			t.Fatalf("操作者读挡回册 = %d %s", response.Code, response.Body.String())
		}
		got := make([]string, 0, len(body.Bounces))
		for _, row := range body.Bounces {
			got = append(got, row["source"]+"|"+row["booking"]+"|"+row["answer"]+"|"+row["basis"]+"|"+row["count"])
		}
		return got
	}
	notProvider := "SYN-PARTY-CARRIER-LM-01|SYN-BOOKING-TRUNK-DCG24|SYN-CARRIER-ANSWER-DCG24|SOURCE_NOT_SERVICE_PROVIDER|1"
	notRegistered := "SYN-PARTY-CARRIER-TRUNK-01|SYN-BOOKING-DCG167-NOWHERE|SYN-CARRIER-ANSWER-DCG24|BOOKING_NOT_REGISTERED|1"
	if got := bounces(""); len(got) != 2 || !(got[0] == notRegistered && got[1] == notProvider) && !(got[0] == notProvider && got[1] == notRegistered) {
		t.Fatalf("挡回册 = %v", got)
	}
	if got := bounces("&booking=SYN-BOOKING-TRUNK-DCG24"); len(got) != 1 || got[0] != notProvider {
		t.Fatalf("按订舱筛 = %v", got)
	}

	for _, registry := range []string{"booking-request", "booking-answer-bounce"} {
		response := read(integrationToken, "registry="+registry)
		if response.Code != http.StatusUnauthorized || problemCode(t, response) != "OPERATOR_CREDENTIAL_REJECTED" {
			t.Fatalf("集成客户端令牌读 %s = %d %s", registry, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response, recordsPath)
	}
}
