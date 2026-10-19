package nodeopshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证在线收寄口多出的交付范围依据引用一格（ADR-0212 决定七），与收寄、重核答复带的交付范围核对态（决定五）。

const scopedReceptionBody = `{"sourceId":"SYN-DEVICE-07/000200","deliveredBy":"SYN-COURIER-01","handlingUnit":"SYN-HU-0200",` +
	`"mark":"SYN-MARK-0200","claim":"RECEIVED","evidenceRef":"SYN-EVIDENCE/tally-0200","occurredAt":"2026-09-24T09:15:00+08:00"`

// Covers: ADR-0212 决定七「在线收寄口多一个可选格，原样存」——载荷带依据种类与标识时译进命令、标识逐字；不带或写 null 都是没有依据。
func TestWorkFactReceptionIntakeTakesAnOptionalDeliveryScopeBasis(t *testing.T) {
	intake := receptionIntakeForTest(t)
	for name, testCase := range map[string]struct {
		extra      string
		kind       domain.DeliveryScopeBasisKind
		identifier string
	}{
		"带预报":    {`,"deliveryScopeBasis":{"kind":"DELIVERY_PRE_ADVICE","id":" PREADV-0200 "}}`, domain.BasisDeliveryPreAdvice, " PREADV-0200 "},
		"带交接单":   {`,"deliveryScopeBasis":{"kind":"DELIVERY_HANDOVER_NOTE","id":"SYN-HANDOVER-0200"}}`, domain.BasisDeliveryHandoverNote, "SYN-HANDOVER-0200"},
		"不带":     {`}`, domain.DeliveryScopeBasisKindInvalid, ""},
		"写 null": {`,"deliveryScopeBasis":null}`, domain.DeliveryScopeBasisKindInvalid, ""},
	} {
		command, err := intake.IntakeReception(context.Background(), receptionRequest(scopedReceptionBody+testCase.extra))
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if command.ScopeBasis.Kind() != testCase.kind || command.ScopeBasis.Identifier() != testCase.identifier {
			t.Fatalf("%s：依据 = %s %q, want %s %q", name, command.ScopeBasis.Kind(), command.ScopeBasis.Identifier(),
				testCase.kind, testCase.identifier)
		}
	}
}

// Covers: 依据一格给了就要给全：种类不在词表、标识空、缺种类、带未知键、不是对象，都答坏载荷——静默丢掉会让设备以为带上了依据。
func TestWorkFactReceptionIntakeRefusesAMalformedDeliveryScopeBasis(t *testing.T) {
	intake := receptionIntakeForTest(t)
	for name, extra := range map[string]string{
		"种类不在词表": `,"deliveryScopeBasis":{"kind":"PRE_ADVICE","id":"PREADV-0200"}}`,
		"标识空":    `,"deliveryScopeBasis":{"kind":"DELIVERY_PRE_ADVICE","id":"  "}}`,
		"缺种类":    `,"deliveryScopeBasis":{"id":"PREADV-0200"}}`,
		"带未知键":   `,"deliveryScopeBasis":{"kind":"DELIVERY_PRE_ADVICE","id":"PREADV-0200","customerAccountId":"SYN-CUSTOMER-1"}}`,
		"不是对象":   `,"deliveryScopeBasis":"PREADV-0200"}`,
		"空对象":    `,"deliveryScopeBasis":{}}`,
	} {
		if _, err := intake.IntakeReception(context.Background(), receptionRequest(scopedReceptionBody+extra)); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

type scopeCheckView struct {
	Outcome                       string  `json:"outcome"`
	DeliveryScopeCheck            *string `json:"deliveryScopeCheck"`
	DeliveryScopeUnverifiedReason *string `json:"deliveryScopeUnverifiedReason"`
	DeliveryScopeInapplicability  *string `json:"deliveryScopeInapplicability"`
	JudgmentVersion               string  `json:"judgmentVersion"`
}

func decodeScopeCheck(t *testing.T, raw []byte) scopeCheckView {
	t.Helper()
	var view scopeCheckView
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("响应不是 JSON：%v（%s）", err, raw)
	}
	return view
}

func textOf(value *string) string {
	if value == nil {
		return "<缺席>"
	}
	return *value
}

// Covers: ADR-0212 决定五「收寄答复带交付范围核对态」——不带依据送站，只经客户包裹件号命中的那一件答待识别，答复带核对态未证、原因
// 没有依据；渠道包裹件号那种照常形成的收寄也带。身份核对读不动的未核对待识别没做成核对，三键都不出现。
func TestTheDeliveryScopeCheckTravelsWithTheReceptionAnswer(t *testing.T) {
	pending := newAdapterFixture(t)
	pending.identity.customerNumbered = true
	answer := decodeScopeCheck(t, pending.post(t, defaultBody()).Body.Bytes())
	if answer.Outcome != "PENDING_IDENTIFICATION" || textOf(answer.DeliveryScopeCheck) != "UNVERIFIED" ||
		textOf(answer.DeliveryScopeUnverifiedReason) != "NO_BASIS" || answer.DeliveryScopeInapplicability != nil {
		t.Fatalf("范围未证的待识别答复 = %+v（%s / %s）", answer, textOf(answer.DeliveryScopeCheck), textOf(answer.DeliveryScopeUnverifiedReason))
	}

	formed := newAdapterFixture(t)
	if answer := decodeScopeCheck(t, formed.post(t, defaultBody()).Body.Bytes()); answer.Outcome != "INTAKE_FORMED" ||
		textOf(answer.DeliveryScopeCheck) != "UNVERIFIED" {
		t.Fatalf("渠道包裹件号命中的收寄答复 = %+v（%s）", answer, textOf(answer.DeliveryScopeCheck))
	}

	unchecked := newAdapterFixture(t)
	unchecked.identity.err = errors.New("parcel shipment unreachable")
	if answer := decodeScopeCheck(t, unchecked.post(t, defaultBody()).Body.Bytes()); answer.DeliveryScopeCheck != nil ||
		answer.DeliveryScopeUnverifiedReason != nil {
		t.Fatalf("未核对的待识别答复带了核对态：%+v", answer)
	}
}

// Covers: `AT-NO-062` 末句的命令口面——只因歧义信号的冲突在声明撤除后重核，只缺范围：追加了一版不记冲突的待识别，答 201、outcome 取
// 编排结果原名、带追加那一版的判断版本与核对态未证。
func TestAManualRecheckThatReleasesAnAmbiguityIntoAnUnverifiedUnitAnswersCreated(t *testing.T) {
	fixture := newRecheckEndpointFixture(t)
	contested := fixture.pending
	contested.Key.SourceID = "SYN-DELIVERY-NRS03-AMBIGUOUS"
	contested.JudgmentVersion = adapterValue(t, domain.NewIntakeResultVersion, "SYN-INTAKEV-NRS03-AMBIGUOUS")
	contested.IdentityConflict, contested.IdentityConflictCause = true, ports.ConflictByAmbiguityOnly
	contested.Candidates = []domain.ParcelAssociationReference{adapterValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRS03")}
	fixture.store.byKey[contested.Key] = contested
	fixture.identity.candidates, fixture.identity.customerNumbered = []string{"SYN-PARCEL-NRS03"}, true

	response := postRecheck(nodeopshttp.NewRecheckPendingIdentityEndpoint(namedIntake{}, fixture.handler), "SYN-INTAKEV-NRS03-AMBIGUOUS")
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201（%s）", response.Code, response.Body.String())
	}
	answer := decodeScopeCheck(t, response.Body.Bytes())
	if answer.Outcome != "AMBIGUITY_GONE_SCOPE_UNVERIFIED" || answer.JudgmentVersion == "" ||
		textOf(answer.DeliveryScopeCheck) != "UNVERIFIED" || textOf(answer.DeliveryScopeUnverifiedReason) != "NO_BASIS" {
		t.Fatalf("响应 = %+v（%s / %s）", answer, textOf(answer.DeliveryScopeCheck), textOf(answer.DeliveryScopeUnverifiedReason))
	}
	if len(fixture.intents.intents) != 0 {
		t.Fatalf("意图 %d 封, want 没有", len(fixture.intents.intents))
	}
}
