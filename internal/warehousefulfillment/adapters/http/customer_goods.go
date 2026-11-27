package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件是商品的货主口（UC-WF-002；ADR-0384）。问题详情用首方族的 application/problem+json，
// 业务答复仍是 application/json，outcome 用商品登记与查阅核心已有的字面量。
// 租户与货主客户账户只取自信封；货主读口不带出登记责任方，也不带出仓测上的设备与操作者。

// CustomerGoodsAuthenticator 按口认证。写口的 accessRequestID 在令牌、册、授予与准入都过了之后才被调用。
type CustomerGoodsAuthenticator interface {
	AuthenticateGoodsMaintenance(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
	AuthenticateWarehouseRead(ctx context.Context, credential PresentedCustomerCredential) (CustomerReadIdentity, error)
}

// CustomerGoodsPorts 是商品货主写口与商品/仓测货主读口要的 Intake。未配置时一字节业务内容都不读。
type CustomerGoodsPorts interface {
	IntakeGoodsMaintenance(ctx context.Context, request *http.Request) (application.RegisterGoodsCommand, error)
	IntakeWarehouseRead(ctx context.Context, request *http.Request) (CustomerReadIdentity, error)
}

// CustomerGoodsIntake 是首方客户渠道在商品货主口的 Intake。
type CustomerGoodsIntake struct {
	authenticator CustomerGoodsAuthenticator
}

func NewCustomerGoodsIntake(authenticator CustomerGoodsAuthenticator) (*CustomerGoodsIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: goods customer intake needs an authenticator")
	}
	return &CustomerGoodsIntake{authenticator: authenticator}, nil
}

// OpenCustomerGoodsIntake 交回端点用的 Intake。缺席时是字面量未配置，避免把带类型的空指针塞进接口。
func OpenCustomerGoodsIntake(intake *CustomerGoodsIntake) CustomerGoodsPorts {
	if intake == nil {
		return unconfiguredCustomerGoods{}
	}
	return intake
}

type unconfiguredCustomerGoods struct{}

func (unconfiguredCustomerGoods) IntakeGoodsMaintenance(context.Context, *http.Request) (application.RegisterGoodsCommand, error) {
	return application.RegisterGoodsCommand{}, ErrAccessChannelNotConfigured
}

func (unconfiguredCustomerGoods) IntakeWarehouseRead(context.Context, *http.Request) (CustomerReadIdentity, error) {
	return CustomerReadIdentity{}, ErrAccessChannelNotConfigured
}

func (intake *CustomerGoodsIntake) IntakeGoodsMaintenance(
	ctx context.Context,
	request *http.Request,
) (application.RegisterGoodsCommand, error) {
	var none application.RegisterGoodsCommand
	var fields registrationjson.OwnerGoodsFields
	identity, err := intake.authenticator.AuthenticateGoodsMaintenance(ctx, presentedCustomerCredential(request), func() (string, error) {
		read, readErr := registrationjson.ReadOwnerGoods(bodyOrEmpty(request))
		if readErr != nil {
			return "", asMalformed(readErr)
		}
		fields = read
		return read.AccessRequestID, nil
	})
	if err != nil {
		return none, err
	}
	submission, err := domain.NewOwnerSubmission(identity.Source, identity.RequestKey)
	if err != nil {
		return none, err
	}
	return application.RegisterGoodsCommand{
		Tenant:     identity.Tenant,
		Owner:      identity.Owner,
		Code:       fields.Code,
		Basis:      fields.Basis,
		Content:    fields.Content,
		Submission: submission,
		Registrant: identity.Registrant,
	}, nil
}

func (intake *CustomerGoodsIntake) IntakeWarehouseRead(
	ctx context.Context,
	request *http.Request,
) (CustomerReadIdentity, error) {
	return intake.authenticator.AuthenticateWarehouseRead(ctx, presentedCustomerCredential(request))
}

var (
	_ CustomerGoodsPorts = (*CustomerGoodsIntake)(nil)
	_ CustomerGoodsPorts = unconfiguredCustomerGoods{}
)

type customerRegisterGoodsAnswer struct {
	Outcome string             `json:"outcome"`
	Field   string             `json:"field,omitempty"`
	Fields  []string           `json:"fields,omitempty"`
	Current *goodsBasisBody    `json:"current,omitempty"`
	Goods   *customerGoodsBody `json:"goods,omitempty"`
}

type customerGoodsListResponse struct {
	Outcome      string              `json:"outcome"`
	OwnerAccount string              `json:"ownerAccount"`
	Goods        []customerGoodsBody `json:"goods"`
	Page         cataloguepage.Page  `json:"page"`
}

type customerGoodsCurrentResponse struct {
	Outcome      string             `json:"outcome"`
	OwnerAccount string             `json:"ownerAccount"`
	GoodsCode    string             `json:"goodsCode"`
	Found        bool               `json:"found"`
	Goods        *customerGoodsBody `json:"goods,omitempty"`
}

type customerGoodsRevisionsResponse struct {
	Outcome      string              `json:"outcome"`
	OwnerAccount string              `json:"ownerAccount"`
	GoodsCode    string              `json:"goodsCode"`
	Revisions    []customerGoodsBody `json:"revisions"`
}

type customerGoodsFoundResponse struct {
	Outcome      string             `json:"outcome"`
	OwnerAccount string             `json:"ownerAccount"`
	Code         string             `json:"code"`
	Found        bool               `json:"found"`
	Goods        *customerGoodsBody `json:"goods,omitempty"`
}

// NewMaintainCustomerGoodsEndpoint 交回货主商品维护的 HTTP 入口（POST /v1/warehouse/goods）。
// 与商品代录写同一条修订序列；新建答 201，其余业务答复答 200，回显不带登记责任方。
func NewMaintainCustomerGoodsEndpoint(intake CustomerGoodsPorts, registrar GoodsRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeGoodsMaintenance(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		result, err := registrar.Register(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "customer goods registration did not form an answer",
				"ownerAccount", command.Owner.String(), "goodsCode", command.Code.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		writeCustomerGoodsAnswer(response, request, result)
	})
}

func writeCustomerGoodsAnswer(response http.ResponseWriter, request *http.Request, result application.RegisterGoodsResult) {
	name := result.Outcome().String()
	if name == "" {
		writeCustomerProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome, "")
		return
	}
	answer := customerRegisterGoodsAnswer{Outcome: name}
	status := http.StatusOK
	switch result.Outcome() {
	case application.GoodsOutcomeRegistered, application.GoodsOutcomeExistingResult:
		revision, present := result.Revision()
		if !present {
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		body := customerGoodsBodyOfRevision(revision)
		answer.Goods = &body
		if result.Outcome() == application.GoodsOutcomeRegistered {
			status = http.StatusCreated
		}
	case application.GoodsOutcomeBasisStale:
		current := result.CurrentBasis()
		basis := goodsBasisBody{Registered: current.Registered()}
		if current.Registered() {
			revision := current.Revision()
			basis.Revision = &revision
		}
		answer.Current = &basis
	case application.GoodsOutcomeImmutableField:
		answer.Field = result.ImmutableCell().String()
	case application.GoodsOutcomeIdentifierConflict:
		code, present := result.ConflictingCode()
		if !present {
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		answer.Field = code.String()
	case application.GoodsOutcomeContentInvalid:
		cells := result.InvalidCells()
		answer.Fields = make([]string, 0, len(cells))
		for _, cell := range cells {
			answer.Fields = append(answer.Fields, cell.String())
		}
	}
	writeJSON(response, status, answer)
}

// NewListCustomerGoodsEndpoint 交回货主列出自己商品的入口（GET /v1/warehouse/reads/goods）。
func NewListCustomerGoodsEndpoint(intake CustomerGoodsPorts, reader GoodsCatalogueReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeWarehouseRead(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		pageQuery, err := customerGoodsListQuery(request)
		if err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		limit := operatorCatalogueReadLimit
		if pageQuery.PageSize > 0 {
			limit = pageQuery.PageSize
		}
		page, err := reader.ListGoods(request.Context(), identity.Tenant, identity.Owner, limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "customer goods list did not form an answer",
				"ownerAccount", identity.Owner.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		writeJSON(response, http.StatusOK, customerGoodsListResponse{
			Outcome:      outcomeGoodsListed,
			OwnerAccount: identity.Owner.String(),
			Goods:        customerGoodsBodiesOfRows(page.Rows),
			Page:         page.Page(limit),
		})
	})
}

// NewGetCustomerCurrentGoodsEndpoint 交回货主读单个商品当前修订的入口（GET /v1/warehouse/reads/goods-current?goodsCode=…）。
// 别的货主的商品与没登记同答：found 为假，不答 404（AT-WF-023）。
func NewGetCustomerCurrentGoodsEndpoint(intake CustomerGoodsPorts, reader GoodsCurrentReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeWarehouseRead(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		code, err := customerGoodsCodeQuery(request)
		if err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		row, found, err := reader.CurrentGoods(request.Context(), identity.Tenant, identity.Owner, code)
		if err != nil {
			slog.ErrorContext(request.Context(), "customer current goods did not form an answer",
				"ownerAccount", identity.Owner.String(), "goodsCode", code.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		answer := customerGoodsCurrentResponse{
			Outcome: outcomeGoodsCurrent, OwnerAccount: identity.Owner.String(), GoodsCode: code.String(), Found: found,
		}
		if found {
			body := customerGoodsBodyOfRow(row)
			answer.Goods = &body
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

// NewListCustomerGoodsRevisionsEndpoint 交回货主读一个商品全部修订史的入口（GET /v1/warehouse/reads/goods-revisions?goodsCode=…）。
// 每一笔带提交途径，代录另带依据引用，不带登记责任方。
func NewListCustomerGoodsRevisionsEndpoint(intake CustomerGoodsPorts, reader GoodsRevisionsReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeWarehouseRead(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		code, err := customerGoodsCodeQuery(request)
		if err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		rows, err := reader.GoodsRevisions(request.Context(), identity.Tenant, identity.Owner, code)
		if err != nil {
			slog.ErrorContext(request.Context(), "customer goods revisions did not form an answer",
				"ownerAccount", identity.Owner.String(), "goodsCode", code.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		writeJSON(response, http.StatusOK, customerGoodsRevisionsResponse{
			Outcome:      outcomeGoodsRevisionsListed,
			OwnerAccount: identity.Owner.String(),
			GoodsCode:    code.String(),
			Revisions:    customerGoodsBodiesOfRows(rows),
		})
	})
}

// NewFindCustomerGoodsEndpoint 交回货主按码找商品的入口（GET /v1/warehouse/reads/goods-by-code?code=…）。
func NewFindCustomerGoodsEndpoint(intake CustomerGoodsPorts, finder GoodsFinder) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeWarehouseRead(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		code, err := customerGoodsIdentifierQuery(request)
		if err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		row, found, err := finder.FindGoods(request.Context(), identity.Tenant, identity.Owner, code)
		if err != nil {
			slog.ErrorContext(request.Context(), "customer find goods did not form an answer",
				"ownerAccount", identity.Owner.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		answer := customerGoodsFoundResponse{
			Outcome: outcomeGoodsFound, OwnerAccount: identity.Owner.String(), Code: code.String(), Found: found,
		}
		if found {
			body := customerGoodsBodyOfRow(row)
			answer.Goods = &body
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

func customerGoodsListQuery(request *http.Request) (cataloguepage.Query, error) {
	if reason := refuseCarriedQueryIdentity(request); reason != "" {
		return cataloguepage.Query{}, &malformedRequest{reason: reason}
	}
	values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return cataloguepage.Query{}, err
	}
	return ports.GoodsPages.Decode(values)
}

func customerGoodsCodeQuery(request *http.Request) (domain.GoodsCode, error) {
	if reason := refuseCarriedQueryIdentity(request); reason != "" {
		return domain.GoodsCode{}, &malformedRequest{reason: reason}
	}
	cells, reason := queryCells(request, goodsCodeQueryKey)
	if reason != "" {
		return domain.GoodsCode{}, &malformedRequest{reason: reason}
	}
	code, err := domain.NewGoodsCode(cells[0])
	if err != nil {
		return domain.GoodsCode{}, &malformedRequest{reason: blankCellReason(goodsCodeQueryKey)}
	}
	return code, nil
}

func customerGoodsIdentifierQuery(request *http.Request) (domain.IdentifierCode, error) {
	if reason := refuseCarriedQueryIdentity(request); reason != "" {
		return domain.IdentifierCode{}, &malformedRequest{reason: reason}
	}
	cells, reason := queryCells(request, goodsIdentifierQueryKey)
	if reason != "" {
		return domain.IdentifierCode{}, &malformedRequest{reason: reason}
	}
	code, err := domain.NewIdentifierCode(cells[0])
	if err != nil {
		return domain.IdentifierCode{}, &malformedRequest{reason: blankCellReason(goodsIdentifierQueryKey)}
	}
	return code, nil
}
