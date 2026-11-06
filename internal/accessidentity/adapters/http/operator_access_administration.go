package identityhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/accessidentity/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 操作者接入管理面的各口（ADR-0363 决定二）。parcel-api 端点表按下面的路径常量各挂一行。
const (
	// OperatorsPath 是本租户操作者册的查阅读：操作者、授予与撤销记录，外加产品词表与本进程信任的发行方。
	OperatorsPath = "/operators"
	// OperatorRegistrationsPath 登一个操作者主体。
	OperatorRegistrationsPath = "/operator-registrations"
	// OperatorGrantRegistrationsPath 登一笔授予。
	OperatorGrantRegistrationsPath = "/operator-grant-registrations"
	// OperatorGrantRevocationsPath 撤一笔授予。
	OperatorGrantRevocationsPath = "/operator-grant-revocations"
)

// outcomeOperatorsListed 是查阅口唯一的业务成格。空册也是这一格：空列表不是未配置。
const outcomeOperatorsListed = "OPERATORS_LISTED"

const codeMalformedRequest = "MALFORMED_REQUEST"

// maxAdministrationPayloadBytes 是一次登记载荷的读取上限：单项不过几百字节，超出的只会是误投的批文或别的东西。
const maxAdministrationPayloadBytes = 64 << 10

// OperatorAccessAdministrator 是认证件交回的：信封上的租户与调用者主体。主体给撤销口判防锁死（ADR-0363 决定三），别的口不读它。
type OperatorAccessAdministrator struct {
	TenantID string
	Subject  accessidentity.OperatorSubject
}

// OperatorAccessAdministrationAuthenticator 认证一次操作者接入管理面的出示。
type OperatorAccessAdministrationAuthenticator interface {
	AuthenticateAccessAdministration(ctx context.Context, bearerToken string) (OperatorAccessAdministrator, error)
}

// accessAdministrationAuthenticator 以操作者接入管理面铸信封，不带准入要求：登记操作者与授予不是生产写，不判准入范围
// （ADR-0100 决定四「登记写面不适用」同款）。
type accessAdministrationAuthenticator struct {
	minter operatorMinter
}

// NewOperatorAccessAdministrationAuthenticator 的 minter 不许缺。
func NewOperatorAccessAdministrationAuthenticator(minter operatorMinter) (OperatorAccessAdministrationAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("access identity http: operator access administration authenticator needs a minter")
	}
	return accessAdministrationAuthenticator{minter: minter}, nil
}

// AuthenticateAccessAdministration 不把请求里的租户交进铸造：租户只取自信封上的绑定。
func (authenticator accessAdministrationAuthenticator) AuthenticateAccessAdministration(
	ctx context.Context,
	bearerToken string,
) (OperatorAccessAdministrator, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, accessidentity.NewOperatorCredential(bearerToken),
		accessidentity.OperatorRequest{Face: accessidentity.CapabilityOperatorAccessAdministration})
	if err != nil {
		return OperatorAccessAdministrator{}, err
	}
	if !envelope.Minted() || envelope.TenantID() == "" {
		return OperatorAccessAdministrator{}, errors.New("access identity http: minted envelope has no tenant")
	}
	return OperatorAccessAdministrator{TenantID: envelope.TenantID(), Subject: envelope.Subject()}, nil
}

// OperatorsHandler 把已装好的查阅处理器交回端点表；没装时答未配置，同 WorkDeviceCatalogueHandler。
func OperatorsHandler(wired http.Handler) http.Handler {
	if wired == nil {
		return unconfiguredOperatorRead()
	}
	return wired
}

// OperatorRegistrationsHandler、OperatorGrantRegistrationsHandler 与 OperatorGrantRevocationsHandler 把已装好的写处理器交回
// 端点表；没装时答未配置，只收 POST。
func OperatorRegistrationsHandler(wired http.Handler) http.Handler {
	return writeHandlerOrUnconfigured(wired)
}
func OperatorGrantRegistrationsHandler(wired http.Handler) http.Handler {
	return writeHandlerOrUnconfigured(wired)
}
func OperatorGrantRevocationsHandler(wired http.Handler) http.Handler {
	return writeHandlerOrUnconfigured(wired)
}

func writeHandlerOrUnconfigured(wired http.Handler) http.Handler {
	if wired == nil {
		return unconfiguredOperatorWrite()
	}
	return wired
}

// unconfiguredOperatorWrite 是本包各写口没接铸造器时的同一个答复：只收 POST，答未配置，与发行方未设同一格。
func unconfiguredOperatorWrite() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !allowMethod(response, request, http.MethodPost) {
			return
		}
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
	})
}

func allowMethod(response http.ResponseWriter, request *http.Request, method string) bool {
	if request.Method == method {
		return true
	}
	response.Header().Set("Allow", method)
	writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
	return false
}

// NewQueryOperatorsEndpoint 交回操作者册查阅的 HTTP 入口（ADR-0363 决定二）。只接受 GET，租户只来自认证结果；查询参数在
// 认证之后按 accessidentity.OperatorCataloguePages 解码，页大小是 operatorCatalogueReadLimit。
//
// 答复里的词表（可授能力面、决定种类、作业范围种类）与 issuer 是产品与部署的事实，不是租户取值：管理台据此摆选项、预填
// 发行方，不在前端抄第二份。issuer 是本进程信任的那一个，渠道未配置时整族答 403，这一格不会被读到。词表与 issuer 不随页
// 变，每一页都带。
func NewQueryOperatorsEndpoint(
	authenticator OperatorAccessAdministrationAuthenticator,
	catalogue accessidentity.OperatorCatalogue,
	issuer string,
) (http.Handler, error) {
	if authenticator == nil || catalogue == nil {
		return nil, errors.New("access identity http: operators endpoint needs an authenticator and a catalogue")
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !allowMethod(response, request, http.MethodGet) {
			return
		}
		administrator, err := authenticator.AuthenticateAccessAdministration(request.Context(), httpapi.BearerToken(request))
		if err != nil {
			writeCatalogueProblem(response, err)
			return
		}
		query, err := accessidentity.OperatorCataloguePages.Decode(request.URL.Query())
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		result, err := catalogue.ListOperators(request.Context(), administrator.TenantID, operatorCatalogueReadLimit, query)
		if err != nil {
			// 读不回是答案未形成，不是空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body := operatorsResponseOf(issuer, result.Rows)
		body.Page = result.Page(operatorCatalogueReadLimit)
		writeJSON(response, http.StatusOK, body)
	}), nil
}

// NewRegisterOperatorEndpoint 交回操作者登记口。载荷照 parcel-access-register 的单项去掉 tenantId（ADR-0363 决定二）：租户只取自
// 信封，带了即未知键、答 400。答复沿用 OperatorRegistrationOutcome：RECORDED 答 201，其余已形成的答案答 200（ADR-0022）。
func NewRegisterOperatorEndpoint(
	authenticator OperatorAccessAdministrationAuthenticator,
	transactor bentoapp.Transactor,
	registrar accessidentity.OperatorRegistrar,
) (http.Handler, error) {
	if authenticator == nil || transactor == nil || registrar == nil {
		return nil, errors.New("access identity http: operator registration endpoint needs an authenticator, a transactor and a registrar")
	}
	return writeEndpoint(authenticator, transactor, func(
		ctx context.Context, administrator OperatorAccessAdministrator, request *http.Request,
	) (func(context.Context) (accessidentity.OperatorRegistrationOutcome, error), error) {
		var payload registrationjson.OperatorItem
		if err := decodeClosedDocument(request, &payload); err != nil {
			return nil, err
		}
		binding, err := registrationjson.BindingFrom(administrator.TenantID, payload)
		if err != nil {
			return nil, err
		}
		return func(txCtx context.Context) (accessidentity.OperatorRegistrationOutcome, error) {
			return registrar.RegisterOperator(txCtx, binding)
		}, nil
	}), nil
}

// NewRegisterOperatorGrantEndpoint 交回授予登记口，载荷与答复纪律同操作者登记口。
func NewRegisterOperatorGrantEndpoint(
	authenticator OperatorAccessAdministrationAuthenticator,
	transactor bentoapp.Transactor,
	registrar accessidentity.OperatorRegistrar,
) (http.Handler, error) {
	if authenticator == nil || transactor == nil || registrar == nil {
		return nil, errors.New("access identity http: operator grant registration endpoint needs an authenticator, a transactor and a registrar")
	}
	return writeEndpoint(authenticator, transactor, func(
		ctx context.Context, administrator OperatorAccessAdministrator, request *http.Request,
	) (func(context.Context) (accessidentity.OperatorRegistrationOutcome, error), error) {
		var payload registrationjson.GrantItem
		if err := decodeClosedDocument(request, &payload); err != nil {
			return nil, err
		}
		grant, err := registrationjson.GrantFrom(administrator.TenantID, payload)
		if err != nil {
			return nil, err
		}
		return func(txCtx context.Context) (accessidentity.OperatorRegistrationOutcome, error) {
			return registrar.RegisterGrant(txCtx, grant)
		}, nil
	}), nil
}

// NewRegisterOperatorGrantRevocationEndpoint 交回撤销登记口。写入前在同一事务里取那笔授予：落在调用者自己此刻生效的操作者
// 接入管理授予上答 REVOCATION_WOULD_LOCK_OUT_CALLER、不写（ADR-0363 决定三）；「此刻」取 now。授予不在册时不在这里答——
// 登记口自己答 GRANT_NOT_REGISTERED，两个口不各养一套答复。
func NewRegisterOperatorGrantRevocationEndpoint(
	authenticator OperatorAccessAdministrationAuthenticator,
	transactor bentoapp.Transactor,
	registrar accessidentity.OperatorRegistrar,
	catalogue accessidentity.OperatorCatalogue,
	now func() time.Time,
) (http.Handler, error) {
	if authenticator == nil || transactor == nil || registrar == nil || catalogue == nil || now == nil {
		return nil, errors.New("access identity http: operator grant revocation endpoint needs an authenticator, a transactor, a registrar, a catalogue and a clock")
	}
	return writeEndpoint(authenticator, transactor, func(
		ctx context.Context, administrator OperatorAccessAdministrator, request *http.Request,
	) (func(context.Context) (accessidentity.OperatorRegistrationOutcome, error), error) {
		var payload registrationjson.RevocationItem
		if err := decodeClosedDocument(request, &payload); err != nil {
			return nil, err
		}
		revocation, err := registrationjson.RevocationFrom(administrator.TenantID, payload)
		if err != nil {
			return nil, err
		}
		return func(txCtx context.Context) (accessidentity.OperatorRegistrationOutcome, error) {
			recorded, found, err := catalogue.FindGrant(txCtx, revocation.TenantID(), revocation.GrantID())
			if err != nil {
				return "", err
			}
			if found && recorded.LocksOutCallerIfRevoked(administrator.Subject, now()) {
				return accessidentity.OperatorRevocationWouldLockOutCaller, nil
			}
			return registrar.RegisterRevocation(txCtx, revocation)
		}, nil
	}), nil
}

// writeEndpoint 是三个写口共同的骨架：只收 POST → 认证 → 译载荷（坏形状答 400）→ 在自己的事务里登记 → 按答复定状态码。
// translate 交回要在事务里跑的那一步，译失败即 400：译在事务之外，坏载荷不开事务。
func writeEndpoint(
	authenticator OperatorAccessAdministrationAuthenticator,
	transactor bentoapp.Transactor,
	translate func(context.Context, OperatorAccessAdministrator, *http.Request) (func(context.Context) (accessidentity.OperatorRegistrationOutcome, error), error),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !allowMethod(response, request, http.MethodPost) {
			return
		}
		administrator, err := authenticator.AuthenticateAccessAdministration(request.Context(), httpapi.BearerToken(request))
		if err != nil {
			writeCatalogueProblem(response, err)
			return
		}
		register, err := translate(request.Context(), administrator, request)
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		var outcome accessidentity.OperatorRegistrationOutcome
		if err := transactor.WithinTransaction(request.Context(), func(txCtx context.Context) error {
			var registerErr error
			outcome, registerErr = register(txCtx)
			return registerErr
		}); err != nil {
			// 册写不动是答案未形成，册上什么也没落。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		status := http.StatusOK
		if outcome == accessidentity.OperatorRegistrationRecorded {
			status = http.StatusCreated
		}
		writeJSON(response, status, registrationResponse{Outcome: outcome.String()})
	})
}

var errMalformedPayload = errors.New("access identity http: malformed payload")

// decodeClosedDocument 解封闭载荷：未知字段、多余内容、读超限都是坏报文——打错的键静默丢弃，会让登记的人以为登进去的
// 比实际多；tenantId 也是未知键，租户只取自信封。
func decodeClosedDocument(request *http.Request, target any) error {
	if request.Body == nil {
		return fmt.Errorf("%w: empty body", errMalformedPayload)
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxAdministrationPayloadBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", errMalformedPayload, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the payload", errMalformedPayload)
	}
	return nil
}

type registrationResponse struct {
	Outcome string `json:"outcome"`
}

type operatorsResponse struct {
	Outcome        string             `json:"outcome"`
	Issuer         string             `json:"issuer"`
	Faces          []string           `json:"faces"`
	DecisionKinds  []string           `json:"decisionKinds"`
	WorkScopeKinds []string           `json:"workScopeKinds"`
	Operators      []operatorBody     `json:"operators"`
	Page           cataloguepage.Page `json:"page"`
}

type operatorBody struct {
	Issuer     string              `json:"issuer"`
	Subject    string              `json:"subject"`
	Basis      string              `json:"basis"`
	RecordedAt string              `json:"recordedAt"`
	Grants     []operatorGrantBody `json:"grants"`
}

// operatorGrantBody 的可选格按能力面在场：决定种类只在「运营决定」上，作业范围只在「作业事实登记」上，终点不设即缺席，
// 没撤销即没有 revocation。缺席而不是 null，管理台按键在不在判。
type operatorGrantBody struct {
	GrantID           string          `json:"grantId"`
	CapabilityFace    string          `json:"capabilityFace"`
	DecisionKind      string          `json:"decisionKind,omitempty"`
	WorkScope         *workScopeBody  `json:"workScope,omitempty"`
	EffectiveStartsAt string          `json:"effectiveStartsAt"`
	EffectiveEndsAt   string          `json:"effectiveEndsAt,omitempty"`
	Basis             string          `json:"basis"`
	RecordedAt        string          `json:"recordedAt"`
	Revocation        *revocationBody `json:"revocation,omitempty"`
}

type workScopeBody struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

type revocationBody struct {
	RevokedAt string `json:"revokedAt"`
	Basis     string `json:"basis"`
}

func operatorsResponseOf(issuer string, rows []accessidentity.OperatorCatalogueRow) operatorsResponse {
	body := operatorsResponse{
		Outcome:        outcomeOperatorsListed,
		Issuer:         issuer,
		Faces:          make([]string, 0),
		DecisionKinds:  make([]string, 0),
		WorkScopeKinds: make([]string, 0),
		Operators:      make([]operatorBody, 0, len(rows)),
	}
	for _, face := range accessidentity.GrantableCapabilityFaces() {
		body.Faces = append(body.Faces, face.String())
	}
	for _, kind := range accessidentity.DecisionKinds() {
		body.DecisionKinds = append(body.DecisionKinds, kind.String())
	}
	for _, kind := range accessidentity.WorkScopeKinds() {
		body.WorkScopeKinds = append(body.WorkScopeKinds, kind.String())
	}
	for _, row := range rows {
		body.Operators = append(body.Operators, operatorBodyOf(row))
	}
	return body
}

func operatorBodyOf(row accessidentity.OperatorCatalogueRow) operatorBody {
	binding := row.Binding()
	body := operatorBody{
		Issuer:     binding.Subject().Issuer(),
		Subject:    binding.Subject().Subject(),
		Basis:      binding.Basis(),
		RecordedAt: instant(row.RecordedAt()),
		Grants:     make([]operatorGrantBody, 0),
	}
	for _, grantRow := range row.Grants() {
		body.Grants = append(body.Grants, operatorGrantBodyOf(grantRow))
	}
	return body
}

func operatorGrantBodyOf(row accessidentity.RecordedGrantRow) operatorGrantBody {
	grant := row.Recorded().Grant()
	body := operatorGrantBody{
		GrantID:           grant.GrantID(),
		CapabilityFace:    grant.Face().String(),
		EffectiveStartsAt: instant(grant.Interval().StartsAt()),
		Basis:             grant.Basis(),
		RecordedAt:        instant(row.RecordedAt()),
	}
	if grant.Face() == accessidentity.CapabilityOperationDecision {
		body.DecisionKind = grant.DecisionKind().String()
	}
	if grant.Face() == accessidentity.CapabilityWorkFactRegistration {
		body.WorkScope = &workScopeBody{Kind: grant.WorkScope().Kind().String(), Reference: grant.WorkScope().Reference()}
	}
	if endsAt, bounded := grant.Interval().EndsAt(); bounded {
		body.EffectiveEndsAt = instant(endsAt)
	}
	if revocation, revoked := row.Recorded().Revocation(); revoked {
		body.Revocation = &revocationBody{RevokedAt: instant(revocation.RevokedAt()), Basis: revocation.Basis()}
	}
	return body
}

// instant 按 UTC 的 RFC 3339 写时刻，带库里存得下的亚秒精度，与作业设备册查阅的停用时刻同一写法。
func instant(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}
