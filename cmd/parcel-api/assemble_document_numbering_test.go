package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberhttp"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: ADR-0355 决定六——端点表上的字面量路径与 docnumberhttp 的路径常量是同一批；读口与预览口挂查阅读，两个登记口
// 挂登记册配置写，都由本特性自建 Intake。
func TestDocumentNumberingPathsMatchTheEndpointTable(t *testing.T) {
	want := map[string]struct {
		constant string
		method   string
		family   faceFamily
	}{
		"/document-numbering/rules":                   {docnumberhttp.RulesPath, http.MethodGet, faceOperationsRead},
		"/document-numbering/previews":                {docnumberhttp.PreviewsPath, http.MethodPost, faceOperationsRead},
		"/document-numbering/rule-registrations":      {docnumberhttp.RuleRegistrationsPath, http.MethodPost, faceRegistryWrite},
		"/document-numbering/time-zone-registrations": {docnumberhttp.TimeZoneRegistrationsPath, http.MethodPost, faceRegistryWrite},
	}
	declared := declaredEndpointLines()
	for literal, expected := range want {
		if literal != expected.constant {
			t.Errorf("端点表字面量 %s 与路径常量 %s 对不上", literal, expected.constant)
		}
		line, found := declared[literal]
		if !found || !line.ownIntake || line.face.family != expected.family || line.method != expected.method {
			t.Errorf("%s 应登记为 %s 面、自建 Intake 的 %s 行：%+v", literal, expected.family, expected.method, line)
		}
	}
}

func documentNumberingDB(t *testing.T) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func documentNumberingRequest(t *testing.T, handler http.Handler, method, target, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func documentNumberingBody(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v %s", err, response.Body.String())
	}
	return body
}

// Covers: ADR-0355 决定六、ADR-0100 决定四、ADR-0055——四口接在操作者渠道上：发行方未配置答未配置；没带令牌答凭证被拒；
// 读口与预览口只认查阅读授予、登记口只认登记写授予，持另一面授予的答未授予。
func TestDocumentNumberingEndpointsAnswerFromTheOperatorChannel(t *testing.T) {
	db := documentNumberingDB(t)
	reader, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	writer, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	unconfigured, err := buildOperatorMinter(accessidentity.UnconfiguredOperatorCredentialVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}

	const token = "presented.operator.token"
	lines := []struct {
		pick    func(documentNumberingHandlers) http.Handler
		method  string
		path    string
		body    string
		granted *accessidentity.OperatorMinter
		other   *accessidentity.OperatorMinter
	}{
		{func(h documentNumberingHandlers) http.Handler { return h.read }, http.MethodGet, docnumberhttp.RulesPath, "", reader, writer},
		{func(h documentNumberingHandlers) http.Handler { return h.preview }, http.MethodPost, docnumberhttp.PreviewsPath,
			`{"documentType":"SHIPMENT","template":"P{yyyy}{seq:6}","timeZone":"Asia/Shanghai"}`, reader, writer},
		{func(h documentNumberingHandlers) http.Handler { return h.ruleRegistration }, http.MethodPost, docnumberhttp.RuleRegistrationsPath,
			`{"documentType":"SHIPMENT","template":"P{yyyy}{seq:6}","effectiveFrom":"2026-12-01T00:00:00+08:00"}`, writer, reader},
		{func(h documentNumberingHandlers) http.Handler { return h.timeZoneRegistration }, http.MethodPost, docnumberhttp.TimeZoneRegistrationsPath,
			`{"timeZone":"Asia/Shanghai","effectiveFrom":"2026-01-01T00:00:00+08:00"}`, writer, reader},
	}
	for _, line := range lines {
		cases := []struct {
			name   string
			minter *accessidentity.OperatorMinter
			token  string
			status int
			code   string
		}{
			{"issuer unconfigured", unconfigured, token, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
			{"no bearer token", line.granted, "", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
			{"the other face's grant", line.other, token, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		}
		for _, testCase := range cases {
			t.Run(line.path+" "+testCase.name, func(t *testing.T) {
				handlers, err := buildDocumentNumberingHandlers(db, testCase.minter, docnumber.SecretTable{})
				if err != nil {
					t.Fatal(err)
				}
				response := documentNumberingRequest(t, line.pick(handlers), line.method, line.path, testCase.token, line.body)
				if response.Code != testCase.status || problemCode(t, response) != testCase.code {
					t.Fatalf("status %d body %s", response.Code, response.Body.String())
				}
			})
		}
		t.Run(line.path+" granted", func(t *testing.T) {
			handlers, err := buildDocumentNumberingHandlers(db, line.granted, docnumber.SecretTable{})
			if err != nil {
				t.Fatal(err)
			}
			response := documentNumberingRequest(t, line.pick(handlers), line.method, line.path, token, line.body)
			if response.Code != http.StatusOK && response.Code != http.StatusCreated {
				t.Fatalf("status %d body %s", response.Code, response.Body.String())
			}
		})
	}

	if _, err := buildDocumentNumberingHandlers(db, nil, docnumber.SecretTable{}); err == nil {
		t.Fatal("nil minter was accepted")
	}
}

// Covers: ADR-0355 决定五至七——对真库走一遍：读口先答单据编号时区未配置、各类产品默认；登记时区与规则（登记人取自信封、
// 重放答已存在）；读口随之答租户版、样例按单据编号时区出日期；用加扰而租户没配密钥的模板登记即拒、配了即收；
// 预览答样例与可能重叠的产品默认。
func TestDocumentNumberingRegisterRoundTrip(t *testing.T) {
	db := documentNumberingDB(t)
	reader, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	writer, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	secrets := docnumber.SecretTable{}
	readHandlers, err := buildDocumentNumberingHandlers(db, reader, secrets)
	if err != nil {
		t.Fatal(err)
	}
	writeHandlers, err := buildDocumentNumberingHandlers(db, writer, secrets)
	if err != nil {
		t.Fatal(err)
	}
	const token = "presented.operator.token"

	body := documentNumberingBody(t, documentNumberingRequest(t, readHandlers.read, http.MethodGet, docnumberhttp.RulesPath, token, ""))
	zone, _ := body["numberingTimeZone"].(map[string]any)
	types, _ := body["documentTypes"].([]any)
	if zone["status"] != "NOT_CONFIGURED" || len(types) != 11 {
		t.Fatalf("empty register: %v", body)
	}
	for _, raw := range types {
		row, _ := raw.(map[string]any)
		if row["source"] != "PRODUCT_DEFAULT" || row["sample"] != nil {
			t.Fatalf("%v should be the product default without a sample", row)
		}
	}

	zoneBody := `{"timeZone":"Asia/Shanghai","effectiveFrom":"2026-01-01T00:00:00+08:00"}`
	first := documentNumberingRequest(t, writeHandlers.timeZoneRegistration, http.MethodPost, docnumberhttp.TimeZoneRegistrationsPath, token, zoneBody)
	replay := documentNumberingRequest(t, writeHandlers.timeZoneRegistration, http.MethodPost, docnumberhttp.TimeZoneRegistrationsPath, token, zoneBody)
	if first.Code != http.StatusCreated || replay.Code != http.StatusOK || documentNumberingBody(t, replay)["outcome"] != "VERSION_EXISTING" {
		t.Fatalf("time zone: %d %d %s", first.Code, replay.Code, replay.Body.String())
	}
	version, _ := documentNumberingBody(t, first)["version"].(map[string]any)
	if version["registeredBy"] != "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("registrant not taken from the envelope: %v", version)
	}

	ruleBody := `{"documentType":"EXCEPTION_CASE","template":"SYN-EX-{yyyy}{mm}{dd}-{seq:4}","effectiveFrom":"2026-02-01T00:00:00+08:00"}`
	if response := documentNumberingRequest(t, writeHandlers.ruleRegistration, http.MethodPost, docnumberhttp.RuleRegistrationsPath, token, ruleBody); response.Code != http.StatusCreated {
		t.Fatalf("rule: %d %s", response.Code, response.Body.String())
	}
	scrambled := `{"documentType":"CLAIM","template":"CLM-{yyyy}-{seqx:6}","effectiveFrom":"2026-02-01T00:00:00+08:00"}`
	if response := documentNumberingRequest(t, writeHandlers.ruleRegistration, http.MethodPost, docnumberhttp.RuleRegistrationsPath, token, scrambled); response.Code != http.StatusBadRequest || problemCode(t, response) != "SCRAMBLE_SECRET_NOT_CONFIGURED" {
		t.Fatalf("scrambled without a secret: %d %s", response.Code, response.Body.String())
	}
	withSecret, err := buildDocumentNumberingHandlers(db, writer, docnumber.SecretTable{"SYN-TENANT-01": "syn-scramble-secret-0123456789abcdef"})
	if err != nil {
		t.Fatal(err)
	}
	if response := documentNumberingRequest(t, withSecret.ruleRegistration, http.MethodPost, docnumberhttp.RuleRegistrationsPath, token, scrambled); response.Code != http.StatusCreated {
		t.Fatalf("scrambled with a secret: %d %s", response.Code, response.Body.String())
	}

	body = documentNumberingBody(t, documentNumberingRequest(t, readHandlers.read, http.MethodGet, docnumberhttp.RulesPath, token, ""))
	zone, _ = body["numberingTimeZone"].(map[string]any)
	if zone["status"] != "CONFIGURED" || zone["timeZone"] != "Asia/Shanghai" {
		t.Fatalf("numbering time zone: %v", zone)
	}
	types, _ = body["documentTypes"].([]any)
	found := false
	for _, raw := range types {
		row, _ := raw.(map[string]any)
		switch row["documentType"] {
		case "EXCEPTION_CASE":
			found = true
			sample, _ := row["sample"].(string)
			versions, _ := row["versions"].([]any)
			if row["source"] != "TENANT_RULE" || !strings.HasPrefix(sample, "SYN-EX-") || !strings.HasSuffix(sample, "-0001") || len(versions) != 1 {
				t.Fatalf("exception case: %v", row)
			}
		case "SHIPMENT":
			if sample, _ := row["sample"].(string); row["source"] != "PRODUCT_DEFAULT" || !strings.HasPrefix(sample, "P20") {
				t.Fatalf("shipment: %v", row)
			}
		}
	}
	if !found {
		t.Fatal("exception case missing from the read")
	}

	preview := documentNumberingBody(t, documentNumberingRequest(t, readHandlers.preview, http.MethodPost, docnumberhttp.PreviewsPath, token,
		`{"documentType":"SHIPMENT","template":"P{yyyy}{mm}{seq:8}","at":"2026-12-01T00:00:00+08:00"}`))
	overlaps, _ := preview["overlaps"].([]any)
	if preview["outcome"] != "PREVIEW_RENDERED" || preview["sample"] != "P20261200000001" || preview["timeZone"] != "Asia/Shanghai" || len(overlaps) != 1 {
		t.Fatalf("preview: %v", preview)
	}
}
