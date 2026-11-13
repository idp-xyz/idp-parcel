package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

type notifierStub struct {
	commands []application.NotifySignalCommand
	err      error
}

func (stub *notifierStub) HandleSignal(_ context.Context, command application.NotifySignalCommand) (application.NotifyCustomerResult, error) {
	stub.commands = append(stub.commands, command)
	return application.NotifyCustomerResult{}, stub.err
}

// Covers: 客户异常通知口按自己那一种决定认证，租户与操作者只取认证结果，载荷封闭且只指名包裹、信号类型与哪一期；编排
// 报错答 500，编排交回一份没有名字的答复也答 500 而不是空 200。带通知的答复由装配测试在真库上验。
func TestTheNotificationFaceAuthenticatesUnderItsOwnDecisionKindAndTakesIdentityFromAuthentication(t *testing.T) {
	asked := []visibilityhttp.OperatorDecision{}
	intake, err := visibilityhttp.NewOperatorDecisionIntake(caseDecisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	stub := &notifierStub{}
	endpoint := visibilityhttp.NewNotifyCustomerExceptionEndpoint(intake, stub)
	body := `{"parcel":"SYN-PARCEL-1","signalKind":"ETA_BREACH_RISK","episodeId":"SYN-EPISODE-1"}`

	status, raw := serveRaw(t, endpoint, "t", body)
	var answer struct{ Error struct{ Code string } }
	_ = json.Unmarshal(raw, &answer)
	if status != http.StatusInternalServerError || answer.Error.Code == "" {
		t.Fatalf("没有名字的答复该答 500，实得 %d %s", status, raw)
	}
	if len(asked) != 1 || asked[0] != visibilityhttp.OperatorDecisionCustomerExceptionNotification {
		t.Fatalf("认证问的决定种类 = %v", asked)
	}
	command := stub.commands[0]
	if command.TenantID.String() != "SYN-TENANT-01" || command.Operator != "https://issuer#alice" || command.Parcel.String() != "SYN-PARCEL-1" ||
		command.Kind.String() != "ETA_BREACH_RISK" || command.Episode.String() != "SYN-EPISODE-1" {
		t.Fatalf("命令 = %+v", command)
	}
	stub.err = errors.New("down")
	if status, _ := serveRaw(t, endpoint, "t", body); status != http.StatusInternalServerError {
		t.Fatalf("编排报错该答 500，实得 %d", status)
	}
	for name, malformed := range map[string]string{
		"带了客户":  `{"parcel":"SYN-PARCEL-1","signalKind":"ETA_BREACH_RISK","episodeId":"SYN-EPISODE-1","customer":"x"}`,
		"带了渠道":  `{"parcel":"SYN-PARCEL-1","signalKind":"ETA_BREACH_RISK","episodeId":"SYN-EPISODE-1","channel":"x"}`,
		"没有哪一期": `{"parcel":"SYN-PARCEL-1","signalKind":"ETA_BREACH_RISK"}`,
	} {
		if status, _ := serveRaw(t, endpoint, "t", malformed); status != http.StatusBadRequest {
			t.Fatalf("%s 该答 400，实得 %d", name, status)
		}
	}
	if status, _ := serveRaw(t, endpoint, "", body); status != http.StatusUnauthorized || len(stub.commands) != 2 {
		t.Fatalf("没有凭据该答 401 且不到编排，实得 %d / %d 次", status, len(stub.commands))
	}
}

// Covers: dox/decisions.md「产品形态」——手工标记口与发送口按同一种决定认证；载荷另必带通知方式与所凭原件，去首尾
// 空白后随命令走；缺一格、带了客户一类的格都是坏报文。
func TestTheManualNotificationFaceRequiresHowAndFromWhichOriginalTheCustomerWasNotified(t *testing.T) {
	asked := []visibilityhttp.OperatorDecision{}
	intake, err := visibilityhttp.NewOperatorDecisionIntake(caseDecisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	stub := &notifierStub{err: errors.New("编排不在本用例的范围")}
	endpoint := visibilityhttp.NewRecordManualCustomerNotificationEndpoint(intake, stub)
	target := `"parcel":"SYN-PARCEL-1","signalKind":"ETA_BREACH_RISK","episodeId":"SYN-EPISODE-1"`

	if status, _ := serveRaw(t, endpoint, "t", `{`+target+`,"method":" 电话 ","original":" SYN-CALL-LOG-1 "}`); status != http.StatusInternalServerError {
		t.Fatalf("编排报错该答 500，实得 %d", status)
	}
	if len(asked) != 1 || asked[0] != visibilityhttp.OperatorDecisionCustomerExceptionNotification || len(stub.commands) != 1 {
		t.Fatalf("认证问的决定种类 = %v，编排 %d 次", asked, len(stub.commands))
	}
	if command := stub.commands[0]; command.Method != "电话" || command.Original != "SYN-CALL-LOG-1" || command.Operator != "https://issuer#alice" ||
		command.Episode.String() != "SYN-EPISODE-1" {
		t.Fatalf("命令 = %+v", command)
	}
	for name, malformed := range map[string]string{
		"没有方式":    `{` + target + `,"original":"SYN-CALL-LOG-1"}`,
		"没有原件":    `{` + target + `,"method":"电话"}`,
		"原件只有空白":  `{` + target + `,"method":"电话","original":"  "}`,
		"带了客户":    `{` + target + `,"method":"电话","original":"O","customer":"x"}`,
		"方式带控制字符": `{` + target + `,"method":"电\u0000话","original":"O"}`,
	} {
		if status, _ := serveRaw(t, endpoint, "t", malformed); status != http.StatusBadRequest {
			t.Fatalf("%s 该答 400，实得 %d", name, status)
		}
	}
	if len(stub.commands) != 1 {
		t.Fatal("坏载荷到了编排")
	}
}
