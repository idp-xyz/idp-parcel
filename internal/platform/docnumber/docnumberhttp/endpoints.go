// Package docnumberhttp 是单据编号规则册的四个操作者口（ADR-0355 决定六）：读口、预览口与两个登记口。
//
// 契约（管理台「设置 → 单据编号」镜像此处，改这里须同改管理台那一页的接口文件）：
//
//	GET  /document-numbering/rules                    主数据与运营查阅读
//	  200 {"outcome":"DOCUMENT_NUMBERING_READ","at",
//	       "numberingTimeZone":{"status":"CONFIGURED"|"NOT_CONFIGURED","timeZone","effectiveFrom","registeredBy","registeredAt"},
//	       "numberingTimeZoneVersions":[{"timeZone","effectiveFrom","registeredBy","registeredAt"}],
//	       "documentTypes":[{"documentType","defaultTemplate","source":"PRODUCT_DEFAULT"|"TENANT_RULE","template",
//	         "effectiveFrom","resetPeriod","dateTimeZone":{"source","numberingTimeZoneStatus","numberingTimeZone"},
//	         "sample","versions":[{"template","effectiveFrom","registeredBy","registeredAt"}]}]}
//	POST /document-numbering/previews                 主数据与运营查阅读（同试算：不写册、不占号）
//	  {"documentType","template","timeZone"?,"at"?}
//	  200 {"outcome":"PREVIEW_RENDERED","sample","timeZone","at","resetPeriod","overlaps":[{"source","template","effectiveFrom"}]}
//	  200 {"outcome":"PREVIEW_REJECTED","reason"} / {"outcome":"NUMBERING_TIME_ZONE_NOT_CONFIGURED","resetPeriod"}
//	POST /document-numbering/rule-registrations       登记册配置写
//	  {"documentType","template","effectiveFrom"}
//	  201 {"outcome":"VERSION_REGISTERED","version":{...}} / 200 VERSION_EXISTING / 200 EFFECTIVE_FROM_TAKEN
//	POST /document-numbering/time-zone-registrations  登记册配置写
//	  {"timeZone","effectiveFrom"}
//	  201 {"outcome":"VERSION_REGISTERED","version":{...}} / 200 VERSION_EXISTING / 200 EFFECTIVE_FROM_TAKEN
//
// dateTimeZone.source 为 ISSUER_BUSINESS_TIME_ZONE 的类型按开立方业务时区出日期，numberingTimeZone 是开立方给不出时
// 退到的那一个；为 TENANT_NUMBERING_TIME_ZONE 的类型只用它。sample 只在单据编号时区已登时出现。
//
// 失败一律 {"error":{"code"}}，与各上下文的登记口同形：400 MALFORMED_REQUEST 与登记拒因各码（reasonCodes）、
// 401 OPERATOR_CREDENTIAL_REJECTED、403 OPERATOR_NOT_GRANTED / ACCESS_CHANNEL_NOT_CONFIGURED、405 METHOD_NOT_ALLOWED、
// 500 NO_ANSWER_FORMED、503 IDENTITY_DEPENDENCY_UNAVAILABLE。预览把拒因放进 200 的 reason：边填边预览时半截模板
// 被拒是常态，不是请求错了。
//
// 租户只取自认证信封，载荷里不收租户；先认证、后解载荷：令牌不过的调用方看不到载荷校验的结果。
package docnumberhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 四个口的路径。parcel-api 端点表按字面量登记同一批路径（管理台路径门禁只认端点表上的字符串，不认常量引用）。
const (
	RulesPath                 = "/document-numbering/rules"
	PreviewsPath              = "/document-numbering/previews"
	RuleRegistrationsPath     = "/document-numbering/rule-registrations"
	TimeZoneRegistrationsPath = "/document-numbering/time-zone-registrations"
)

const (
	outcomeRead = "DOCUMENT_NUMBERING_READ"

	statusConfigured    = "CONFIGURED"
	statusNotConfigured = "NOT_CONFIGURED"
)

const (
	codeMethodNotAllowed              = "METHOD_NOT_ALLOWED"
	codeAccessChannelNotConfigured    = "ACCESS_CHANNEL_NOT_CONFIGURED"
	codeOperatorCredentialRejected    = "OPERATOR_CREDENTIAL_REJECTED"
	codeOperatorNotGranted            = "OPERATOR_NOT_GRANTED"
	codeIdentityDependencyUnavailable = "IDENTITY_DEPENDENCY_UNAVAILABLE"
	codeIntakeFailed                  = "INTAKE_FAILED"
	codeMalformedRequest              = "MALFORMED_REQUEST"
	codeNoAnswerFormed                = "NO_ANSWER_FORMED"
)

// reasonCodes 是登记拒因到答复码的一张表，登记口与预览口共用：页面按码说人话，同一个拒因在两口不会叫两个名字。
var reasonCodes = []struct {
	reason error
	code   string
}{
	{docnumber.ErrTypeNotDeclared, "DOCUMENT_TYPE_NOT_DECLARED"},
	{docnumber.ErrTemplateUnparseable, "TEMPLATE_UNPARSEABLE"},
	{docnumber.ErrTemplateWithoutSequence, "TEMPLATE_WITHOUT_SEQUENCE"},
	{docnumber.ErrTemplateNeedsUnsuppliedValue, "TEMPLATE_NEEDS_UNSUPPLIED_VALUE"},
	{docnumber.ErrTemplateUnrenderable, "TEMPLATE_UNRENDERABLE"},
	{docnumber.ErrScrambleSecretNotConfigured, "SCRAMBLE_SECRET_NOT_CONFIGURED"},
	{docnumber.ErrTimeZoneNotRecognized, "TIME_ZONE_NOT_RECOGNIZED"},
	{docnumber.ErrRegistrationIncomplete, codeMalformedRequest},
}

func reasonCode(err error) (string, bool) {
	for _, entry := range reasonCodes {
		if errors.Is(err, entry.reason) {
			return entry.code, true
		}
	}
	return "", false
}

// 认证件交回的四格，各对一种恢复动作（ADR-0029）。从共享接入身份译过来的那一层在组合根（cmd/parcel-api）：
// platform 不得导入 accessidentity，所以这里只认自己的哨兵。
var (
	ErrAccessChannelNotConfigured    = errors.New("document numbering http: access channel not configured")
	ErrOperatorCredentialRejected    = errors.New("document numbering http: operator credential rejected")
	ErrOperatorNotGranted            = errors.New("document numbering http: operator holds no grant for this face")
	ErrIdentityDependencyUnavailable = errors.New("document numbering http: identity dependency unavailable")
)

// OperationsReadAuthenticator 认证一次主数据与运营查阅读出示，交回信封上的租户。
type OperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) (tenant string, err error)
}

// Registrant 是一次登记册配置写的出示者：信封上的租户与操作者主体。
type Registrant struct {
	Tenant string
	// Operator 是登记人，写成规则册上的 registeredBy。
	Operator string
}

// RegistryWriteAuthenticator 认证一次登记册配置写出示。
type RegistryWriteAuthenticator interface {
	AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (Registrant, error)
}

// RegisterReader 是读口与预览口转交的编排。
type RegisterReader interface {
	Read(ctx context.Context, tenant string) (docnumber.RegisterView, error)
	Preview(ctx context.Context, request docnumber.PreviewRequest) (docnumber.Preview, error)
}

// RuleRegistrar 是规则版本登记口转交的编排，装配负责把它包进一笔事务。
type RuleRegistrar interface {
	RegisterRule(ctx context.Context, command docnumber.RegisterRuleCommand) (docnumber.RuleRegistration, error)
}

// TimeZoneRegistrar 是单据编号时区登记口转交的编排，装配负责把它包进一笔事务。
type TimeZoneRegistrar interface {
	RegisterTimeZone(ctx context.Context, command docnumber.RegisterTimeZoneCommand) (docnumber.TimeZoneRegistration, error)
}

// RuleRegistrationPayload 是规则版本登记的线格式。受控登记口把它原样嵌进自己的输入、另加租户。
type RuleRegistrationPayload struct {
	DocumentType  string    `json:"documentType"`
	Template      string    `json:"template"`
	EffectiveFrom time.Time `json:"effectiveFrom"`
}

// Command 把载荷连同租户与登记人译成命令。生效时刻缺席不是零，是没说——规则版本没有默认生效时刻。
func (payload RuleRegistrationPayload) Command(tenant, registeredBy string) (docnumber.RegisterRuleCommand, error) {
	if payload.EffectiveFrom.IsZero() {
		return docnumber.RegisterRuleCommand{}, fmt.Errorf("%w: effectiveFrom is required; it has no default", errMalformed)
	}
	if strings.ContainsRune(payload.DocumentType+payload.Template, 0) {
		return docnumber.RegisterRuleCommand{}, fmt.Errorf("%w: payload carries NUL", errMalformed)
	}
	return docnumber.RegisterRuleCommand{
		Tenant:        tenant,
		Type:          docnumber.TypeCode(payload.DocumentType),
		Template:      payload.Template,
		EffectiveFrom: payload.EffectiveFrom,
		RegisteredBy:  registeredBy,
	}, nil
}

// TimeZoneRegistrationPayload 是单据编号时区登记的线格式。时区是租户取值，缺了就拒，不替它补 UTC。
type TimeZoneRegistrationPayload struct {
	TimeZone      string    `json:"timeZone"`
	EffectiveFrom time.Time `json:"effectiveFrom"`
}

func (payload TimeZoneRegistrationPayload) Command(tenant, registeredBy string) (docnumber.RegisterTimeZoneCommand, error) {
	if payload.EffectiveFrom.IsZero() {
		return docnumber.RegisterTimeZoneCommand{}, fmt.Errorf("%w: effectiveFrom is required; it has no default", errMalformed)
	}
	if strings.ContainsRune(payload.TimeZone, 0) {
		return docnumber.RegisterTimeZoneCommand{}, fmt.Errorf("%w: payload carries NUL", errMalformed)
	}
	return docnumber.RegisterTimeZoneCommand{
		Tenant:        tenant,
		TimeZone:      payload.TimeZone,
		EffectiveFrom: payload.EffectiveFrom,
		RegisteredBy:  registeredBy,
	}, nil
}

// previewPayload 是预览口的线格式。timeZone 与 at 可不给：不给时区取租户的单据编号时区，不给时刻取此刻。
type previewPayload struct {
	DocumentType string     `json:"documentType"`
	Template     string     `json:"template"`
	TimeZone     string     `json:"timeZone"`
	At           *time.Time `json:"at"`
}

var errMalformed = errors.New("document numbering http: malformed request")

// maxPayloadBytes 只挡失控的请求体；一份登记载荷是几十个字符的模板加一个时刻。
const maxPayloadBytes = 64 << 10

// NewReadEndpoint 交回读口。
func NewReadEndpoint(authenticator OperationsReadAuthenticator, reader RegisterReader) (http.Handler, error) {
	if authenticator == nil || reader == nil {
		return nil, errors.New("document numbering http: read endpoint needs an authenticator and a reader")
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !allow(response, request, http.MethodGet) {
			return
		}
		tenant, ok := authenticateRead(response, request, authenticator)
		if !ok {
			return
		}
		view, err := reader.Read(request.Context(), tenant)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, readBody(view))
	}), nil
}

// NewPreviewEndpoint 交回预览口。
func NewPreviewEndpoint(authenticator OperationsReadAuthenticator, reader RegisterReader) (http.Handler, error) {
	if authenticator == nil || reader == nil {
		return nil, errors.New("document numbering http: preview endpoint needs an authenticator and a reader")
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !allow(response, request, http.MethodPost) {
			return
		}
		tenant, ok := authenticateRead(response, request, authenticator)
		if !ok {
			return
		}
		var payload previewPayload
		if err := decodeClosedDocument(request, &payload); err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		previewRequest := docnumber.PreviewRequest{
			Tenant:   tenant,
			Type:     docnumber.TypeCode(payload.DocumentType),
			Template: payload.Template,
			TimeZone: payload.TimeZone,
		}
		if payload.At != nil {
			previewRequest.At = *payload.At
		}
		preview, err := reader.Preview(request.Context(), previewRequest)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body, err := previewBody(preview)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, body)
	}), nil
}

// NewRuleRegistrationEndpoint 交回规则版本登记口。按 ADR-0022 映射：首登 201；已在册与生效时刻被占都是形成了的
// 答案，答 200 带 outcome 并回显册上那一版。拒因答 400 与各自的码；册写不动答没形成答案，册上什么也没落。
func NewRuleRegistrationEndpoint(authenticator RegistryWriteAuthenticator, registrar RuleRegistrar) (http.Handler, error) {
	if authenticator == nil || registrar == nil {
		return nil, errors.New("document numbering http: rule registration endpoint needs an authenticator and a registrar")
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !allow(response, request, http.MethodPost) {
			return
		}
		registrant, ok := authenticateWrite(response, request, authenticator)
		if !ok {
			return
		}
		var payload RuleRegistrationPayload
		if err := decodeClosedDocument(request, &payload); err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		command, err := payload.Command(registrant.Tenant, registrant.Operator)
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		result, err := registrar.RegisterRule(request.Context(), command)
		if !writeRegistrationFailure(response, err) {
			return
		}
		writeRegistration(response, result.Outcome, ruleVersionBody(result.Version))
	}), nil
}

// NewTimeZoneRegistrationEndpoint 交回单据编号时区登记口，答复映射同规则版本登记口。
func NewTimeZoneRegistrationEndpoint(authenticator RegistryWriteAuthenticator, registrar TimeZoneRegistrar) (http.Handler, error) {
	if authenticator == nil || registrar == nil {
		return nil, errors.New("document numbering http: time zone registration endpoint needs an authenticator and a registrar")
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !allow(response, request, http.MethodPost) {
			return
		}
		registrant, ok := authenticateWrite(response, request, authenticator)
		if !ok {
			return
		}
		var payload TimeZoneRegistrationPayload
		if err := decodeClosedDocument(request, &payload); err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		command, err := payload.Command(registrant.Tenant, registrant.Operator)
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		result, err := registrar.RegisterTimeZone(request.Context(), command)
		if !writeRegistrationFailure(response, err) {
			return
		}
		writeRegistration(response, result.Outcome, timeZoneVersionBody(result.Version))
	}), nil
}

// HandlerOrUnconfigured 把已装好的处理器交回端点表；没装时答未配置（装配测试不建铸造器，仍要这一行在表上，
// 答复与发行方未设同一格）。method 是这一行收的方法，方法不对仍答 405。
func HandlerOrUnconfigured(wired http.Handler, method string) http.Handler {
	if wired != nil {
		return wired
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !allow(response, request, method) {
			return
		}
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
	})
}

func allow(response http.ResponseWriter, request *http.Request, method string) bool {
	if request.Method == method {
		return true
	}
	response.Header().Set("Allow", method)
	writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
	return false
}

func authenticateRead(response http.ResponseWriter, request *http.Request, authenticator OperationsReadAuthenticator) (string, bool) {
	tenant, err := authenticator.AuthenticateOperationsRead(request.Context(), httpapi.BearerToken(request))
	if err != nil {
		writeAuthenticationProblem(response, err)
		return "", false
	}
	// 铸出的信封没有租户是认证件的毛病，不是调用方的：不放行一个没绑租户的「操作者」。
	if strings.TrimSpace(tenant) == "" {
		writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
		return "", false
	}
	return tenant, true
}

func authenticateWrite(response http.ResponseWriter, request *http.Request, authenticator RegistryWriteAuthenticator) (Registrant, bool) {
	registrant, err := authenticator.AuthenticateRegistryWrite(request.Context(), httpapi.BearerToken(request))
	if err != nil {
		writeAuthenticationProblem(response, err)
		return Registrant{}, false
	}
	if strings.TrimSpace(registrant.Tenant) == "" || strings.TrimSpace(registrant.Operator) == "" {
		writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
		return Registrant{}, false
	}
	return registrant, true
}

func writeAuthenticationProblem(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAccessChannelNotConfigured):
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
	case errors.Is(err, ErrOperatorCredentialRejected):
		writeProblem(response, http.StatusUnauthorized, codeOperatorCredentialRejected)
	case errors.Is(err, ErrOperatorNotGranted):
		writeProblem(response, http.StatusForbidden, codeOperatorNotGranted)
	case errors.Is(err, ErrIdentityDependencyUnavailable):
		writeProblem(response, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
	default:
		writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
	}
}

// writeRegistrationFailure 写登记失败的答复，没有失败时交回真。拒因各有其码；认不出的拒绝与册写不动都答没形成答案。
func writeRegistrationFailure(response http.ResponseWriter, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, docnumber.ErrRegistrationRejected) {
		if code, known := reasonCode(err); known {
			writeProblem(response, http.StatusBadRequest, code)
			return false
		}
	}
	writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
	return false
}

func writeRegistration(response http.ResponseWriter, outcome docnumber.RegistrationOutcome, version any) {
	status := http.StatusOK
	switch outcome {
	case docnumber.VersionRegistered:
		status = http.StatusCreated
	case docnumber.VersionExisting, docnumber.EffectiveFromTaken:
	default:
		// 编排交回一个本口不认识的答案是实现坏了，不是业务答案。
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	writeJSON(response, status, registrationResponse{Outcome: string(outcome), Version: version})
}

type registrationResponse struct {
	Outcome string `json:"outcome"`
	Version any    `json:"version"`
}

type ruleVersionDetails struct {
	DocumentType  string    `json:"documentType"`
	Template      string    `json:"template"`
	EffectiveFrom time.Time `json:"effectiveFrom"`
	RegisteredBy  string    `json:"registeredBy"`
	RegisteredAt  time.Time `json:"registeredAt"`
}

func ruleVersionBody(version docnumber.RuleVersion) ruleVersionDetails {
	return ruleVersionDetails{
		DocumentType:  string(version.Type),
		Template:      version.Template,
		EffectiveFrom: version.EffectiveFrom,
		RegisteredBy:  version.RegisteredBy,
		RegisteredAt:  version.RegisteredAt,
	}
}

type timeZoneVersionDetails struct {
	TimeZone      string    `json:"timeZone"`
	EffectiveFrom time.Time `json:"effectiveFrom"`
	RegisteredBy  string    `json:"registeredBy"`
	RegisteredAt  time.Time `json:"registeredAt"`
}

func timeZoneVersionBody(version docnumber.TimeZoneVersion) timeZoneVersionDetails {
	return timeZoneVersionDetails{
		TimeZone:      version.TimeZone,
		EffectiveFrom: version.EffectiveFrom,
		RegisteredBy:  version.RegisteredBy,
		RegisteredAt:  version.RegisteredAt,
	}
}

type readResponse struct {
	Outcome                   string                   `json:"outcome"`
	At                        time.Time                `json:"at"`
	NumberingTimeZone         numberingTimeZoneStatus  `json:"numberingTimeZone"`
	NumberingTimeZoneVersions []timeZoneVersionDetails `json:"numberingTimeZoneVersions"`
	DocumentTypes             []documentTypeDetails    `json:"documentTypes"`
}

type numberingTimeZoneStatus struct {
	Status        string     `json:"status"`
	TimeZone      string     `json:"timeZone,omitempty"`
	EffectiveFrom *time.Time `json:"effectiveFrom,omitempty"`
	RegisteredBy  string     `json:"registeredBy,omitempty"`
	RegisteredAt  *time.Time `json:"registeredAt,omitempty"`
}

type documentTypeDetails struct {
	DocumentType    string `json:"documentType"`
	DefaultTemplate string `json:"defaultTemplate"`
	Source          string `json:"source"`
	Template        string `json:"template"`
	// EffectiveFrom 只随租户版本在场；产品默认没有生效时刻。
	EffectiveFrom *time.Time           `json:"effectiveFrom,omitempty"`
	ResetPeriod   string               `json:"resetPeriod"`
	DateTimeZone  dateTimeZoneDetails  `json:"dateTimeZone"`
	Sample        string               `json:"sample,omitempty"`
	Versions      []ruleVersionDetails `json:"versions"`
}

type dateTimeZoneDetails struct {
	Source                  string `json:"source"`
	NumberingTimeZoneStatus string `json:"numberingTimeZoneStatus"`
	NumberingTimeZone       string `json:"numberingTimeZone,omitempty"`
}

func readBody(view docnumber.RegisterView) readResponse {
	body := readResponse{
		Outcome:                   outcomeRead,
		At:                        view.At,
		NumberingTimeZone:         numberingTimeZoneStatus{Status: statusNotConfigured},
		NumberingTimeZoneVersions: []timeZoneVersionDetails{},
		DocumentTypes:             []documentTypeDetails{},
	}
	if zone := view.NumberingTimeZone; zone != nil {
		effectiveFrom, registeredAt := zone.EffectiveFrom, zone.RegisteredAt
		body.NumberingTimeZone = numberingTimeZoneStatus{
			Status: statusConfigured, TimeZone: zone.TimeZone,
			EffectiveFrom: &effectiveFrom, RegisteredBy: zone.RegisteredBy, RegisteredAt: &registeredAt,
		}
	}
	for _, version := range view.NumberingTimeZoneVersions {
		body.NumberingTimeZoneVersions = append(body.NumberingTimeZoneVersions, timeZoneVersionBody(version))
	}
	for _, typeView := range view.Types {
		details := documentTypeDetails{
			DocumentType:    string(typeView.Declaration.Code),
			DefaultTemplate: typeView.Declaration.DefaultTemplate,
			Source:          string(typeView.Source),
			Template:        typeView.Template,
			ResetPeriod:     string(typeView.ResetPeriod),
			DateTimeZone: dateTimeZoneDetails{
				Source:                  string(typeView.Declaration.DateTimeZone),
				NumberingTimeZoneStatus: body.NumberingTimeZone.Status,
				NumberingTimeZone:       body.NumberingTimeZone.TimeZone,
			},
			Sample:   typeView.Sample,
			Versions: []ruleVersionDetails{},
		}
		if typeView.EffectiveVersion != nil {
			effectiveFrom := typeView.EffectiveVersion.EffectiveFrom
			details.EffectiveFrom = &effectiveFrom
		}
		for _, version := range typeView.Versions {
			details.Versions = append(details.Versions, ruleVersionBody(version))
		}
		body.DocumentTypes = append(body.DocumentTypes, details)
	}
	return body
}

type previewResponse struct {
	Outcome     string           `json:"outcome"`
	Reason      string           `json:"reason,omitempty"`
	Sample      string           `json:"sample,omitempty"`
	TimeZone    string           `json:"timeZone,omitempty"`
	At          *time.Time       `json:"at,omitempty"`
	ResetPeriod string           `json:"resetPeriod,omitempty"`
	Overlaps    []overlapDetails `json:"overlaps,omitempty"`
}

type overlapDetails struct {
	Source        string     `json:"source"`
	Template      string     `json:"template"`
	EffectiveFrom *time.Time `json:"effectiveFrom,omitempty"`
}

func previewBody(preview docnumber.Preview) (previewResponse, error) {
	body := previewResponse{Outcome: string(preview.Outcome), ResetPeriod: string(preview.ResetPeriod)}
	switch preview.Outcome {
	case docnumber.PreviewRejected:
		code, known := reasonCode(preview.Rejection)
		if !known {
			return previewResponse{}, fmt.Errorf("document numbering http: unnamed preview rejection %v", preview.Rejection)
		}
		body.Reason = code
	case docnumber.PreviewTimeZoneNotConfigured:
	case docnumber.PreviewRendered:
		at := preview.At
		body.Sample, body.TimeZone, body.At = preview.Sample, preview.TimeZone, &at
		body.Overlaps = []overlapDetails{}
		for _, overlap := range preview.Overlaps {
			details := overlapDetails{Source: string(overlap.Source), Template: overlap.Template}
			if !overlap.EffectiveFrom.IsZero() {
				effectiveFrom := overlap.EffectiveFrom
				details.EffectiveFrom = &effectiveFrom
			}
			body.Overlaps = append(body.Overlaps, details)
		}
	default:
		return previewResponse{}, fmt.Errorf("document numbering http: unnamed preview outcome %q", preview.Outcome)
	}
	return body, nil
}

// decodeClosedDocument 解封闭载荷：未知字段、多余内容、读超限都是坏报文——打错的键静默丢弃，会让登记的人以为
// 登进去的比实际多。
func decodeClosedDocument(request *http.Request, target any) error {
	if request.Body == nil {
		return fmt.Errorf("%w: empty body", errMalformed)
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxPayloadBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", errMalformed, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the payload", errMalformed)
	}
	return nil
}

type problemResponse struct {
	Error problemDetail `json:"error"`
}

type problemDetail struct {
	Code string `json:"code"`
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
