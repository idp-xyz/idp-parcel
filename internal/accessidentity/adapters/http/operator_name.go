package identityhttp

import (
	"context"
	"errors"
	"net/http"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/accessidentity/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 操作者姓名的两口。parcel-api 端点表按下面的路径常量各挂一行。
const (
	// OperatorNamesPath 登、改或删一位操作者的姓名。守口是操作者接入管理这一格：能管谁有权的人，才能写这是谁。
	OperatorNamesPath = "/operator-names"
	// OperatorNameListPath 列出本租户登了姓名的操作者。守口是主数据与运营查阅读：波次的拣货人、放波次人这些格
	// 要把标识示成人认得的名字，看得到这些格的人就看得到名字。
	OperatorNameListPath = "/operator-name-list"
)

const outcomeOperatorNamesListed = "OPERATOR_NAMES_LISTED"

// OperatorNamesHandler 与 OperatorNameListHandler 把已装好的处理器交回端点表；没装时答未配置。
func OperatorNamesHandler(wired http.Handler) http.Handler {
	return writeHandlerOrUnconfigured(wired)
}

func OperatorNameListHandler(wired http.Handler) http.Handler {
	if wired == nil {
		return unconfiguredOperatorRead()
	}
	return wired
}

// NewRecordOperatorNameEndpoint 交回登操作者姓名的口。载荷是 registrationjson.OperatorNameItem，与受控命令行的单项同一份：
// 主体（issuer、subject）与姓名，不带 name 是删掉。租户只取自信封：只能给本租户册上的主体登，别的租户的主体与不存在的
// 主体同答 OPERATOR_NOT_FOUND。登的人记调用者的「发行方#sub」。登上或换了答 201，删掉与查无答 200。
func NewRecordOperatorNameEndpoint(
	authenticator OperatorAccessAdministrationAuthenticator,
	transactor bentoapp.Transactor,
	book accessidentity.OperatorNameBook,
) (http.Handler, error) {
	if authenticator == nil || transactor == nil || book == nil {
		return nil, errors.New("access identity http: operator name endpoint needs an authenticator, a transactor and a name book")
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !allowMethod(response, request, http.MethodPost) {
			return
		}
		administrator, err := authenticator.AuthenticateAccessAdministration(request.Context(), httpapi.BearerToken(request))
		if err != nil {
			writeCatalogueProblem(response, err)
			return
		}
		var payload registrationjson.OperatorNameItem
		if err := decodeClosedDocument(request, &payload); err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		subject, name, err := registrationjson.OperatorNameFrom(payload)
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		recordedBy := administrator.Subject.Issuer() + "#" + administrator.Subject.Subject()
		var outcome accessidentity.OperatorNameOutcome
		if err := transactor.WithinTransaction(request.Context(), func(txCtx context.Context) error {
			var saveErr error
			outcome, saveErr = book.SaveOperatorName(txCtx, administrator.TenantID, subject, name, recordedBy)
			return saveErr
		}); err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		status := http.StatusOK
		if outcome == accessidentity.OperatorNameRecorded {
			status = http.StatusCreated
		}
		writeJSON(response, status, registrationResponse{Outcome: outcome.String()})
	}), nil
}

// NewQueryOperatorNamesEndpoint 交回操作者姓名的查阅口。只接受 GET，租户只来自认证结果。只答登了姓名的操作者：
// 标识写成「发行方#sub」，与各册上记操作者的是同一个串，调用方拿它去对拣货人、放波次人这些格。
func NewQueryOperatorNamesEndpoint(authenticator OperatorCatalogueAuthenticator, book accessidentity.OperatorNameBook) (http.Handler, error) {
	if authenticator == nil || book == nil {
		return nil, errors.New("access identity http: operator name list endpoint needs an authenticator and a name book")
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !allowMethod(response, request, http.MethodGet) {
			return
		}
		tenant, err := authenticator.AuthenticateCatalogueRead(request.Context(), httpapi.BearerToken(request))
		if err != nil {
			writeCatalogueProblem(response, err)
			return
		}
		rows, err := book.ListOperatorNames(request.Context(), tenant)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		names := make([]operatorNameBody, 0, len(rows))
		for _, row := range rows {
			names = append(names, operatorNameBody{
				Operator: row.Subject.Issuer() + "#" + row.Subject.Subject(), Issuer: row.Subject.Issuer(), Subject: row.Subject.Subject(), Name: row.Name,
			})
		}
		writeJSON(response, http.StatusOK, operatorNamesResponse{Outcome: outcomeOperatorNamesListed, Names: names})
	}), nil
}

type operatorNamesResponse struct {
	Outcome string             `json:"outcome"`
	Names   []operatorNameBody `json:"names"`
}

type operatorNameBody struct {
	// Operator 是「发行方#sub」。
	Operator string `json:"operator"`
	Issuer   string `json:"issuer"`
	Subject  string `json:"subject"`
	Name     string `json:"name"`
}
