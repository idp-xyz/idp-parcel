package shipmenthttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// rawParcels 逐字读答复里的 `parcels`：键名是首方契约（ADR-0223 决定一），客户系统按名读，所以按原始 JSON 对象断言，
// 不经测试自己的结构体——那会把多出的键或缺了的键一并吞掉。
func rawParcels(t *testing.T, response *httptest.ResponseRecorder) ([]map[string]string, bool) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.Bytes(), err)
	}
	raw, present := fields["parcels"]
	if !present {
		return nil, false
	}
	var parcels []map[string]string
	if err := json.Unmarshal(raw, &parcels); err != nil {
		t.Fatalf("decode parcels %s: %v", raw, err)
	}
	return parcels, true
}

// Covers: `AT-PS-101`、`AT-PS-105`（ADR-0223 决定一）的传输半边——建成委托的答复带 `parcels`，每件一个对象：`parcelId` 是
// 委托上的内部标识，`customerParcelReference` 与草案那一格同名；没声明件号的那一件整键不出现。
func TestASubmittedAnswerCarriesEachParcelUnderTheContractNames(t *testing.T) {
	fixture := newFixture(t)
	command := fixture.intake.commands[0]
	command.DeclaredParcelIDs = append(command.DeclaredParcelIDs, mustValue(t, domain.NewDeclaredParcelID, "parcel-2"))
	declaration, err := domain.DeclareCustomerParcelNumber(
		command.DeclaredParcelIDs[1], mustValue(t, domain.NewExternalIdentifierNumber, "CUST-0002"))
	if err != nil {
		t.Fatalf("配对：%v", err)
	}
	command.CustomerParcelNumbers = []domain.CustomerParcelNumberDeclaration{declaration}
	fixture.intake.commands[0] = command

	response := fixture.post(t)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body)
	}
	parcels, present := rawParcels(t, response)
	if !present || len(parcels) != 2 {
		t.Fatalf("parcels = %v（在场 %v），want 两件: %s", parcels, present, response.Body)
	}
	if got := parcels[0]; len(got) != 1 || got["parcelId"] != "parcel-1" {
		t.Fatalf("没声明件号的那一件 = %v, want 只有 parcelId=parcel-1", got)
	}
	if got := parcels[1]; len(got) != 2 || got["parcelId"] != "parcel-2" || got["customerParcelReference"] != "CUST-0002" {
		t.Fatalf("第二件 = %v, want parcelId=parcel-2、customerParcelReference=CUST-0002", got)
	}
}

// Covers: `AT-PS-102`、`AT-PS-104`（ADR-0223 决定一、二）的传输半边——重放答 200 `EXISTING_RESULT`，`parcels` 与首答逐字节
// 相同；没有建成委托的答复整键不带 `parcels`，不是空数组。
func TestAReplayRepeatsTheParcelsAndAnAnswerWithoutARequestOmitsThem(t *testing.T) {
	fixture := newFixture(t)
	replayed := fixture.intake.commands[0]
	replayed.DeclaredParcelIDs = []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "parcel-9")}
	fixture.intake.commands = append(fixture.intake.commands, replayed)

	first := fixture.post(t)
	replay := fixture.post(t)

	if replay.Code != http.StatusOK || decodeBody(t, replay).Outcome != "EXISTING_RESULT" {
		t.Fatalf("replay = %d %s, want 200 EXISTING_RESULT", replay.Code, replay.Body)
	}
	var firstFields, replayFields map[string]json.RawMessage
	if err := json.Unmarshal(first.Body.Bytes(), &firstFields); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &replayFields); err != nil {
		t.Fatal(err)
	}
	if string(replayFields["parcels"]) != string(firstFields["parcels"]) || string(firstFields["parcels"]) == "" {
		t.Fatalf("重放 parcels = %s, want 首答的 %s", replayFields["parcels"], firstFields["parcels"])
	}

	unresolved := newFixture(t)
	unresolved.ownership.authority = domain.ProductionAuthorityUnresolved
	refused := unresolved.post(t)
	if decodeBody(t, refused).Outcome != "OWNERSHIP_UNRESOLVED" {
		t.Fatalf("outcome = %s, want OWNERSHIP_UNRESOLVED", refused.Body)
	}
	if _, present := rawParcels(t, refused); present {
		t.Fatalf("没有建成委托的答复带了 parcels: %s", refused.Body)
	}
}
