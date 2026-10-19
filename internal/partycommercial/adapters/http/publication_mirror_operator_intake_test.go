package commercialhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/adapters/registrationjson"
)

// 本文件证受控批文的在线发布口挂上操作者渠道之后的译法（票 operator-channel/18）：外壳与逐项翻译就是受控批量口那一份，
// 租户取认证出的身份，批准三格与角色确认取批文；载荷里的租户格一律拒，令牌不过时请求体一个字节都不读。

// mirrorItem 是受控批文里的一项。tenant 非空时项上带 tenantId，就是受控批量口吃的那一项；为空时是在线镜像。
func mirrorItem(tenant string) string {
	tenantKey := ""
	if tenant != "" {
		tenantKey = `"tenantId": "` + tenant + `", `
	}
	return `{` + tenantKey + `"kind": "ACCEPTANCE_RULE_PACKAGE", "objectId": "SYN-OC18-RULES", "version": "v1",
      "scope": "SYN-OC18-SCOPE", "contentDigest": "sha256:SYN-OC18-RULES", "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {"reference": "SYN-OC18-APPROVAL", "source": "SYN-OC18-SOURCE", "approvedAt": "2026-01-02T00:00:00Z"},
      "approvalRoleStanding": "CONFIRMED"}`
}

func mirrorRequest(token, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

// Covers: 票 operator-channel/18「批文译装下沉」与「`/commercial-publications` 换操作者 Intake」——在线口与受控批量口对
// 同一项译出同一条命令：租户是认证出的那一个（批文里没有这一格），批准三格与角色确认是批文里的，不是信封里的主体。
func TestPublicationMirrorTranslatesAsTheControlledBatchWithTheAuthenticatedTenant(t *testing.T) {
	intake := publicationOperatorIntake(t, envelopeOperators{})
	online, err := intake.IntakeCommercialPublication(context.Background(),
		mirrorRequest("SYN-OPERATOR-01", `{"items": [`+mirrorItem("")+`]}`))
	if err != nil {
		t.Fatalf("在线口：%v", err)
	}

	var document registrationjson.PublicationBatchDocument
	if err := json.Unmarshal([]byte(`{"items": [`+mirrorItem(pcTenant)+`]}`), &document); err != nil {
		t.Fatalf("受控批文：%v", err)
	}
	controlled, err := registrationjson.PublicationCommand(document.Items[0])
	if err != nil {
		t.Fatalf("受控批量口：%v", err)
	}
	if !reflect.DeepEqual(online, controlled) {
		t.Fatalf("两口对同一项译出的命令不同：\n online = %+v\n controlled = %+v", online, controlled)
	}
	if online.Spec.TenantID.String() != pcTenant {
		t.Fatalf("租户 = %q, want 认证出的 %q", online.Spec.TenantID, pcTenant)
	}
}

// Covers: 在线镜像只收受控批文的恰一项——项上带 tenantId（null 也算）、外壳上带 tenantId、零项、两项、集合外的类别，都答
// 请求畸形；租户只从信封来。
func TestPublicationMirrorRefusesSelfReportedTenantsAndAnythingButOneItem(t *testing.T) {
	intake := publicationOperatorIntake(t, envelopeOperators{})
	for name, body := range map[string]string{
		"tenant on the item":           `{"items": [` + mirrorItem("SYN-TENANT-02") + `]}`,
		"null tenant on the item":      `{"items": [` + strings.Replace(mirrorItem(""), `{`, `{"tenantId": null, `, 1) + `]}`,
		"tenant on the shell":          `{"tenantId": "SYN-TENANT-02", "items": [` + mirrorItem("") + `]}`,
		"no items":                     `{"items": []}`,
		"two items":                    `{"items": [` + mirrorItem("") + `, ` + mirrorItem("") + `]}`,
		"kind outside the closed set":  `{"items": [` + strings.Replace(mirrorItem(""), "ACCEPTANCE_RULE_PACKAGE", "NOPE", 1) + `]}`,
		"unknown key next to the item": `{"items": [` + strings.Replace(mirrorItem(""), `{`, `{"approver": "SYN-OPERATOR-01", `, 1) + `]}`,
	} {
		if _, err := intake.IntakeCommercialPublication(context.Background(), mirrorRequest("SYN-OPERATOR-01", body)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

// Covers: 先认证、后解载荷——令牌不过、未授予、身份依赖读不动，发布口都答认证方那一格，请求体一个字节都不读。
func TestPublicationMirrorAuthenticatesBeforeReadingTheBody(t *testing.T) {
	for name, testCase := range map[string]struct {
		operators envelopeOperators
		token     string
		want      error
	}{
		"no token":                {envelopeOperators{}, "", commercialhttp.ErrOperatorCredentialRejected},
		"not granted":             {envelopeOperators{err: commercialhttp.ErrOperatorNotGranted}, "SYN-OPERATOR-01", commercialhttp.ErrOperatorNotGranted},
		"dependency unavailable":  {envelopeOperators{err: commercialhttp.ErrIdentityDependencyUnavailable}, "SYN-OPERATOR-01", commercialhttp.ErrIdentityDependencyUnavailable},
		"access channel disabled": {envelopeOperators{err: commercialhttp.ErrAccessChannelNotConfigured}, "SYN-OPERATOR-01", commercialhttp.ErrAccessChannelNotConfigured},
	} {
		intake := publicationOperatorIntake(t, testCase.operators)
		probe := &bodyReadProbe{}
		request := httptest.NewRequest(http.MethodPost, "/", probe)
		if testCase.token != "" {
			request.Header.Set("Authorization", "Bearer "+testCase.token)
		}
		if _, err := intake.IntakeCommercialPublication(context.Background(), request); !errors.Is(err, testCase.want) || probe.read {
			t.Fatalf("%s: err = %v (want %v), body read = %v", name, err, testCase.want, probe.read)
		}
	}
}
