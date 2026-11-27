package identityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

type nameBookFake struct {
	outcome accessidentity.OperatorNameOutcome
	err     error
	rows    []accessidentity.OperatorNameRow

	saves   int
	tenant  string
	subject accessidentity.OperatorSubject
	name    string
	by      string
}

func (fake *nameBookFake) SaveOperatorName(
	_ context.Context, tenantID string, subject accessidentity.OperatorSubject, name, by string,
) (accessidentity.OperatorNameOutcome, error) {
	fake.saves++
	fake.tenant, fake.subject, fake.name, fake.by = tenantID, subject, name, by
	return fake.outcome, fake.err
}

func (fake *nameBookFake) ListOperatorNames(_ context.Context, tenantID string) ([]accessidentity.OperatorNameRow, error) {
	fake.tenant = tenantID
	return fake.rows, fake.err
}

func postOperatorName(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, identityhttp.OperatorNamesPath, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// 登姓名的口：租户与登的人只取自信封；姓名去掉首尾空白再落；不带 name 是删掉；登上答 201，删掉与查无答 200。
func TestOperatorNameIsRecordedForTheEnvelopeTenant(t *testing.T) {
	const target = `"issuer":"https://id.syn.example/dex","subject":"SYN-OPERATOR-02"`
	cases := []struct {
		label   string
		body    string
		outcome accessidentity.OperatorNameOutcome
		status  int
		name    string
	}{
		{"登上", `{` + target + `,"name":"  甲某某 "}`, accessidentity.OperatorNameRecorded, http.StatusCreated, "甲某某"},
		{"删掉", `{` + target + `}`, accessidentity.OperatorNameCleared, http.StatusOK, ""},
		{"不在本租户册上", `{` + target + `,"name":"甲某某"}`, accessidentity.OperatorNameOperatorNotFound, http.StatusOK, "甲某某"},
	}
	for _, testCase := range cases {
		t.Run(testCase.label, func(t *testing.T) {
			book := &nameBookFake{outcome: testCase.outcome}
			transactor := &transactorFake{}
			handler, err := identityhttp.NewRecordOperatorNameEndpoint(adminAuthenticatorFake{administrator: administrator(t)}, transactor, book)
			if err != nil {
				t.Fatal(err)
			}
			response := postOperatorName(t, handler, testCase.body)
			var answer struct {
				Outcome string `json:"outcome"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
				t.Fatal(err)
			}
			if response.Code != testCase.status || answer.Outcome != testCase.outcome.String() {
				t.Fatalf("答 %d %s", response.Code, response.Body.String())
			}
			if transactor.opened != 1 || book.saves != 1 {
				t.Fatalf("事务 %d 笔、写 %d 次，各要一", transactor.opened, book.saves)
			}
			if book.tenant != catalogueTenant || book.subject != anotherSubject(t) || book.name != testCase.name ||
				book.by != "https://id.syn.example/dex#SYN-OPERATOR-01" {
				t.Fatalf("落的是 租户 %s 主体 %v 姓名 %q 登的人 %s", book.tenant, book.subject, book.name, book.by)
			}
		})
	}
}

// 坏载荷不开事务：空白姓名不是「删掉」的写法，超长、带控制字符、缺主体、未知键（含自报的 tenantId）都答 400。
func TestOperatorNameRefusesMalformedPayloadsBeforeAnyTransaction(t *testing.T) {
	const target = `"issuer":"https://id.syn.example/dex","subject":"SYN-OPERATOR-02"`
	bodies := map[string]string{
		"空白姓名":  `{` + target + `,"name":"   "}`,
		"超长姓名":  `{` + target + `,"name":"` + strings.Repeat("甲", accessidentity.MaxOperatorNameLength+1) + `"}`,
		"带换行":   `{` + target + `,"name":"甲某\n某"}`,
		"带 NUL": `{` + target + `,"name":"甲某\u0000某"}`,
		"缺主体":   `{"issuer":"https://id.syn.example/dex","name":"甲某某"}`,
		"自报租户":  `{` + target + `,"name":"甲某某","tenantId":"SYN-TENANT-02"}`,
		"多余内容":  `{` + target + `,"name":"甲某某"}{}`,
		"不是文档":  `[]`,
		"姓名非文本": `{` + target + `,"name":7}`,
	}
	for label, body := range bodies {
		t.Run(label, func(t *testing.T) {
			book := &nameBookFake{outcome: accessidentity.OperatorNameRecorded}
			transactor := &transactorFake{}
			handler, err := identityhttp.NewRecordOperatorNameEndpoint(adminAuthenticatorFake{administrator: administrator(t)}, transactor, book)
			if err != nil {
				t.Fatal(err)
			}
			response := postOperatorName(t, handler, body)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "MALFORMED_REQUEST") {
				t.Fatalf("答 %d %s", response.Code, response.Body.String())
			}
			if transactor.opened != 0 || book.saves != 0 {
				t.Fatalf("坏载荷开了 %d 笔事务、写了 %d 次", transactor.opened, book.saves)
			}
		})
	}
}

// 准入不过、方法不对、册写不动：都不落姓名。
func TestOperatorNameWriteGuards(t *testing.T) {
	const body = `{"issuer":"https://id.syn.example/dex","subject":"SYN-OPERATOR-02","name":"甲某某"}`

	book := &nameBookFake{outcome: accessidentity.OperatorNameRecorded}
	refused, err := identityhttp.NewRecordOperatorNameEndpoint(adminAuthenticatorFake{err: accessidentity.ErrOperatorNotGranted}, &transactorFake{}, book)
	if err != nil {
		t.Fatal(err)
	}
	if response := postOperatorName(t, refused, body); response.Code != http.StatusForbidden || book.saves != 0 {
		t.Fatalf("未授予：%d %s（写 %d 次）", response.Code, response.Body.String(), book.saves)
	}

	handler, err := identityhttp.NewRecordOperatorNameEndpoint(adminAuthenticatorFake{administrator: administrator(t)}, &transactorFake{}, book)
	if err != nil {
		t.Fatal(err)
	}
	wrongMethod := httptest.NewRecorder()
	handler.ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodGet, identityhttp.OperatorNamesPath, nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET：%d Allow %s", wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}

	failing, err := identityhttp.NewRecordOperatorNameEndpoint(adminAuthenticatorFake{administrator: administrator(t)}, &transactorFake{},
		&nameBookFake{err: errors.New("name book down")})
	if err != nil {
		t.Fatal(err)
	}
	if response := postOperatorName(t, failing, body); response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "NO_ANSWER_FORMED") {
		t.Fatalf("册写不动：%d %s", response.Code, response.Body.String())
	}

	if response := postOperatorName(t, identityhttp.OperatorNamesHandler(nil), body); response.Code != http.StatusForbidden ||
		!strings.Contains(response.Body.String(), "ACCESS_CHANNEL_NOT_CONFIGURED") {
		t.Fatalf("没装：%d %s", response.Code, response.Body.String())
	}
}

// 查阅口：只答本租户登了姓名的操作者，标识写成「发行方#sub」；空册是空列表，不是 null。
func TestOperatorNamesAreListedForTheAuthenticatedTenant(t *testing.T) {
	book := &nameBookFake{rows: []accessidentity.OperatorNameRow{{Subject: anotherSubject(t), Name: "甲某某"}}}
	handler, err := identityhttp.NewQueryOperatorNamesEndpoint(authenticatorFake{tenant: catalogueTenant}, book)
	if err != nil {
		t.Fatal(err)
	}
	get := func(handler http.Handler) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, identityhttp.OperatorNameListPath, nil))
		return response
	}
	response := get(handler)
	const want = `{"outcome":"OPERATOR_NAMES_LISTED","names":[{"operator":"https://id.syn.example/dex#SYN-OPERATOR-02","issuer":"https://id.syn.example/dex","subject":"SYN-OPERATOR-02","name":"甲某某"}]}`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want || book.tenant != catalogueTenant {
		t.Fatalf("答 %d %s（读的租户 %s）", response.Code, response.Body.String(), book.tenant)
	}

	empty, err := identityhttp.NewQueryOperatorNamesEndpoint(authenticatorFake{tenant: catalogueTenant}, &nameBookFake{})
	if err != nil {
		t.Fatal(err)
	}
	if response := get(empty); strings.TrimSpace(response.Body.String()) != `{"outcome":"OPERATOR_NAMES_LISTED","names":[]}` {
		t.Fatalf("空册答 %s", response.Body.String())
	}

	refused, err := identityhttp.NewQueryOperatorNamesEndpoint(authenticatorFake{err: accessidentity.ErrOperatorNotGranted}, book)
	if err != nil {
		t.Fatal(err)
	}
	if response := get(refused); response.Code != http.StatusForbidden {
		t.Fatalf("未授予：%d %s", response.Code, response.Body.String())
	}

	down, err := identityhttp.NewQueryOperatorNamesEndpoint(authenticatorFake{tenant: catalogueTenant}, &nameBookFake{err: errors.New("name book down")})
	if err != nil {
		t.Fatal(err)
	}
	if response := get(down); response.Code != http.StatusInternalServerError {
		t.Fatalf("读不回：%d %s", response.Code, response.Body.String())
	}

	if response := get(identityhttp.OperatorNameListHandler(nil)); response.Code != http.StatusForbidden {
		t.Fatalf("没装：%d %s", response.Code, response.Body.String())
	}
}
