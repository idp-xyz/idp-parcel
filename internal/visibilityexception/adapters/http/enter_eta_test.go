package visibilityhttp_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

type capturingETAFormer struct {
	commands []application.FormETACommand
}

func (fake *capturingETAFormer) FormETA(_ context.Context, command application.FormETACommand) (application.FormETAResult, error) {
	fake.commands = append(fake.commands, command)
	return application.FormETAResult{}, nil
}

// Covers: dox/decisions.md「产品形态」——预计到达的手工录入口按它自己那一种决定认证；租户与录入人只取认证结果；输入
// 版本按内容派生并带 ETAMANUAL/ 前缀（同一份内容换一种时间写法派生出同一个、改了范围或原件就是另一个），模型一格
// 恒为 MANUAL_ENTRY；原件必填；来源不在两个词之内、时间不成形、范围倒置、自报输入版本或录入人都是坏报文。
func TestTheETAEntryFaceDerivesTheInputVersionFromContentAndTakesIdentityFromAuthentication(t *testing.T) {
	asked := []visibilityhttp.OperatorDecision{}
	intake, err := visibilityhttp.NewOperatorDecisionIntake(caseDecisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	former := &capturingETAFormer{}
	endpoint := visibilityhttp.NewEnterETAEndpoint(intake, former)
	body := func(from, to, original string) string {
		return `{"parcel":" SYN-PARCEL-1 ","milestone":"DELIVERED","source":"CARRIER_PROVIDED","rangeFrom":"` + from + `","rangeTo":"` + to +
			`","confidence":"HIGH","original":"` + original + `"}`
	}

	// 替身交回一份没有名字的答复：答 500。
	if status, _ := serveRaw(t, endpoint, "t", body("2026-11-20T08:00:00Z", "2026-11-20T18:00:00Z", " SYN-MAIL-1 ")); status != http.StatusInternalServerError {
		t.Fatalf("没有名字的答复 = %d，想要 500", status)
	}
	serveRaw(t, endpoint, "t", body("2026-11-20T16:00:00.000000789+08:00", "2026-11-21T02:00:00+08:00", "SYN-MAIL-1"))
	serveRaw(t, endpoint, "t", body("2026-11-20T08:00:00Z", "2026-11-20T20:00:00Z", "SYN-MAIL-1"))
	serveRaw(t, endpoint, "t", body("2026-11-20T08:00:00Z", "2026-11-20T18:00:00Z", "SYN-MAIL-2"))
	if len(former.commands) != 4 || len(asked) != 4 || asked[0] != visibilityhttp.OperatorDecisionETAEntry {
		t.Fatalf("编排 %d 次，认证问的 = %v", len(former.commands), asked)
	}
	first := former.commands[0]
	if first.TenantID.String() != "SYN-TENANT-01" || first.EnteredBy != "OPERATOR/https://issuer#alice" || first.Original != "SYN-MAIL-1" ||
		first.Parcel.String() != "SYN-PARCEL-1" || first.Milestone.String() != "DELIVERED" || first.Source.String() != "CARRIER_PROVIDED" ||
		first.Model.String() != "MANUAL_ENTRY" || first.RangeFrom.Hour() != 8 || first.RangeTo.Hour() != 18 {
		t.Fatalf("命令 = %+v", first)
	}
	inputs := func(index int) string { return former.commands[index].Inputs.String() }
	if len(inputs(0)) <= len("ETAMANUAL/") || inputs(0)[:len("ETAMANUAL/")] != "ETAMANUAL/" {
		t.Fatalf("输入版本 = %q", inputs(0))
	}
	if inputs(1) != inputs(0) || inputs(2) == inputs(0) || inputs(3) == inputs(0) {
		t.Fatalf("派生的输入版本：同内容 %q / %q，改范围 %q，换原件 %q", inputs(0), inputs(1), inputs(2), inputs(3))
	}

	for name, malformed := range map[string]string{
		"没有原件":    `{"parcel":"P","milestone":"M","source":"CARRIER_PROVIDED","rangeFrom":"2026-11-20T08:00:00Z","rangeTo":"2026-11-20T18:00:00Z","confidence":"HIGH"}`,
		"来源不在词表":  `{"parcel":"P","milestone":"M","source":"GUESSED","rangeFrom":"2026-11-20T08:00:00Z","rangeTo":"2026-11-20T18:00:00Z","confidence":"HIGH","original":"O"}`,
		"时间不成形":   `{"parcel":"P","milestone":"M","source":"CARRIER_PROVIDED","rangeFrom":"明天","rangeTo":"2026-11-20T18:00:00Z","confidence":"HIGH","original":"O"}`,
		"只有一个时间点": `{"parcel":"P","milestone":"M","source":"CARRIER_PROVIDED","rangeFrom":"2026-11-20T08:00:00Z","rangeTo":"2026-11-20T08:00:00Z","confidence":"HIGH","original":"O"}`,
		"年份不合理":   `{"parcel":"P","milestone":"M","source":"CARRIER_PROVIDED","rangeFrom":"1999-11-20T08:00:00Z","rangeTo":"2026-11-20T08:00:00Z","confidence":"HIGH","original":"O"}`,
		"里程碑过长":   `{"parcel":"P","milestone":"` + strings.Repeat("m", 201) + `","source":"CARRIER_PROVIDED","rangeFrom":"2026-11-20T08:00:00Z","rangeTo":"2026-11-20T18:00:00Z","confidence":"HIGH","original":"O"}`,
		"范围倒置":    `{"parcel":"P","milestone":"M","source":"CARRIER_PROVIDED","rangeFrom":"2026-11-20T18:00:00Z","rangeTo":"2026-11-20T08:00:00Z","confidence":"HIGH","original":"O"}`,
		"自报输入版本":  `{"inputs":"x","parcel":"P","milestone":"M","source":"CARRIER_PROVIDED","rangeFrom":"2026-11-20T08:00:00Z","rangeTo":"2026-11-20T18:00:00Z","confidence":"HIGH","original":"O"}`,
		"自报录入人":   `{"enteredBy":"x","parcel":"P","milestone":"M","source":"CARRIER_PROVIDED","rangeFrom":"2026-11-20T08:00:00Z","rangeTo":"2026-11-20T18:00:00Z","confidence":"HIGH","original":"O"}`,
		"没有可信度":   `{"parcel":"P","milestone":"M","source":"CARRIER_PROVIDED","rangeFrom":"2026-11-20T08:00:00Z","rangeTo":"2026-11-20T18:00:00Z","original":"O"}`,
	} {
		if status, _ := serveRaw(t, endpoint, "t", malformed); status != http.StatusBadRequest {
			t.Fatalf("%s 该答 400，实得 %d", name, status)
		}
	}
	if status, _ := serveRaw(t, endpoint, "", body("2026-11-20T08:00:00Z", "2026-11-20T18:00:00Z", "O")); status != http.StatusUnauthorized || len(former.commands) != 4 {
		t.Fatalf("没有凭据 = %d，编排 %d 次", status, len(former.commands))
	}
}
