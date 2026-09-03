package commercialhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对商业八个登记端点（ADR-0085，票 admin-write-faces/02 商业片）证传输面：
// 方法门、未配置 403 且不读内容、Intake 失败分流、三族答案各自逐名转写、没有名字的
// 答案不上线；并钉住 ADR-0078 的排除在写面成立——隔离读 Intake 装不进任何一个登记口。
//
// 转写断言一律走真用例，不伪造结果值：三族的结果类型（PartyRegistryResult、
// ProductChannelResult、PublishCommercialAuthorityResult）字段全不导出，测试造得出的
// 只有零值——那恰好是「应用层交回没有名字的答案」那一格。这不是麻烦，是结构上的好事：
// 「转写的是不是生产那份代数」本来就是这几条断言要证的东西，能伪造就证不成了。
//
// 与关务片不同，本片没有「在线口与 CLI 收同一份快照形状」的编译期断言可写：商业的登记
// 快照译装在 cmd/parcel-commercial 的 package main 里，在线口与受控 CLI 之间没有共享的
// 译装类型可锁。两口能锁住的只有同一个登记用例（各 Registrar 契约上的 var _ 断言）。
// 缺口已记在票 admin-write-faces/02 的商业片 Comment。

var (
	pcTenant        = "SYN-TEN-PC02C"
	pcScope         = "SYN-SCOPE-PC02C"
	pcEffectiveFrom = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	pcPublishStart  = time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	pcNow           = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
)

func pcNew[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

type pcClock struct{ at time.Time }

func (clock pcClock) Now() time.Time { return clock.at }

// commercialIntakeDouble 交回测试预先备好的命令，对请求零读取。
//
// 八个方法长在同一个类型上，形照生产侧的 UnconfiguredIntake：八类的命令类型互不相同，
// 把一类的译装接到另一类的端点上编译期就红，拆成八个替身类型换不来第二道保障。
//
// 刻意不让它解析 body：从载荷里读租户正是生产侧被禁的那一件（采信自报租户穿透
// ADR-0003 的隔离边界），替身长成那个形状会让「本包没有采信实现」这句话要靠人去分辨。
type commercialIntakeDouble struct {
	publication     application.PublishCommercialAuthorityCommand
	businessParty   application.RegisterBusinessPartyCommand
	legalEntity     application.RegisterLegalEntityCommand
	customerAccount application.RegisterCustomerAccountCommand
	relationship    application.RegisterPartyRelationshipCommand
	deactivation    application.DeactivatePartyIdentityCommand
	productForm     application.RegisterServiceProductFormCommand
	mapping         application.RegisterProductChannelMappingCommand
	err             error
}

func (double commercialIntakeDouble) IntakeCommercialPublication(
	context.Context, *http.Request,
) (application.PublishCommercialAuthorityCommand, error) {
	return double.publication, double.err
}

func (double commercialIntakeDouble) IntakeBusinessPartyRegistration(
	context.Context, *http.Request,
) (application.RegisterBusinessPartyCommand, error) {
	return double.businessParty, double.err
}

func (double commercialIntakeDouble) IntakeLegalEntityRegistration(
	context.Context, *http.Request,
) (application.RegisterLegalEntityCommand, error) {
	return double.legalEntity, double.err
}

func (double commercialIntakeDouble) IntakeCustomerAccountRegistration(
	context.Context, *http.Request,
) (application.RegisterCustomerAccountCommand, error) {
	return double.customerAccount, double.err
}

func (double commercialIntakeDouble) IntakePartyRelationshipRegistration(
	context.Context, *http.Request,
) (application.RegisterPartyRelationshipCommand, error) {
	return double.relationship, double.err
}

func (double commercialIntakeDouble) IntakePartyIdentityDeactivation(
	context.Context, *http.Request,
) (application.DeactivatePartyIdentityCommand, error) {
	return double.deactivation, double.err
}

func (double commercialIntakeDouble) IntakeServiceProductFormRegistration(
	context.Context, *http.Request,
) (application.RegisterServiceProductFormCommand, error) {
	return double.productForm, double.err
}

func (double commercialIntakeDouble) IntakeProductChannelMappingRegistration(
	context.Context, *http.Request,
) (application.RegisterProductChannelMappingCommand, error) {
	return double.mapping, double.err
}

// 三个「被调即失败」的编排替身，各顶一族的 Registrar 契约。三族的方法名各不相同，
// 因此不能像关务片那样用一个泛型替身顶完——那边四类共一个 Handle。
type unreachablePartyRegistrar struct{ t *testing.T }

func (registrar unreachablePartyRegistrar) refuse() (application.PartyRegistryResult, error) {
	registrar.t.Fatal("请求越过了未配置 Intake，到达了登记编排")
	return application.PartyRegistryResult{}, nil
}

func (registrar unreachablePartyRegistrar) RegisterBusinessParty(
	context.Context, application.RegisterBusinessPartyCommand,
) (application.PartyRegistryResult, error) {
	return registrar.refuse()
}

func (registrar unreachablePartyRegistrar) RegisterLegalEntity(
	context.Context, application.RegisterLegalEntityCommand,
) (application.PartyRegistryResult, error) {
	return registrar.refuse()
}

func (registrar unreachablePartyRegistrar) RegisterCustomerAccount(
	context.Context, application.RegisterCustomerAccountCommand,
) (application.PartyRegistryResult, error) {
	return registrar.refuse()
}

func (registrar unreachablePartyRegistrar) RegisterRelationship(
	context.Context, application.RegisterPartyRelationshipCommand,
) (application.PartyRegistryResult, error) {
	return registrar.refuse()
}

func (registrar unreachablePartyRegistrar) Deactivate(
	context.Context, application.DeactivatePartyIdentityCommand,
) (application.PartyRegistryResult, error) {
	return registrar.refuse()
}

type unreachableProductChannelRegistrar struct{ t *testing.T }

func (registrar unreachableProductChannelRegistrar) RegisterServiceProductForm(
	context.Context, application.RegisterServiceProductFormCommand,
) (application.ProductChannelResult, error) {
	registrar.t.Fatal("请求越过了未配置 Intake，到达了登记编排")
	return application.ProductChannelResult{}, nil
}

func (registrar unreachableProductChannelRegistrar) RegisterMapping(
	context.Context, application.RegisterProductChannelMappingCommand,
) (application.ProductChannelResult, error) {
	registrar.t.Fatal("请求越过了未配置 Intake，到达了登记编排")
	return application.ProductChannelResult{}, nil
}

type unreachablePublisher struct{ t *testing.T }

func (publisher unreachablePublisher) Handle(
	context.Context, application.PublishCommercialAuthorityCommand,
) (application.PublishCommercialAuthorityResult, error) {
	publisher.t.Fatal("请求越过了未配置 Intake，到达了发布编排")
	return application.PublishCommercialAuthorityResult{}, nil
}

// 三个可脚本化的编排替身：交回零值结果演「应用层交回了没有名字的答案」，或交回 error
// 演依赖故障。`called` 用来证 Intake 没交出命令时编排一次也没被调到。
type scriptedPartyRegistrar struct {
	err    error
	called bool
}

func (registrar *scriptedPartyRegistrar) answer() (application.PartyRegistryResult, error) {
	registrar.called = true
	return application.PartyRegistryResult{}, registrar.err
}

func (registrar *scriptedPartyRegistrar) RegisterBusinessParty(
	context.Context, application.RegisterBusinessPartyCommand,
) (application.PartyRegistryResult, error) {
	return registrar.answer()
}

func (registrar *scriptedPartyRegistrar) RegisterLegalEntity(
	context.Context, application.RegisterLegalEntityCommand,
) (application.PartyRegistryResult, error) {
	return registrar.answer()
}

func (registrar *scriptedPartyRegistrar) RegisterCustomerAccount(
	context.Context, application.RegisterCustomerAccountCommand,
) (application.PartyRegistryResult, error) {
	return registrar.answer()
}

func (registrar *scriptedPartyRegistrar) RegisterRelationship(
	context.Context, application.RegisterPartyRelationshipCommand,
) (application.PartyRegistryResult, error) {
	return registrar.answer()
}

func (registrar *scriptedPartyRegistrar) Deactivate(
	context.Context, application.DeactivatePartyIdentityCommand,
) (application.PartyRegistryResult, error) {
	return registrar.answer()
}

type scriptedProductChannelRegistrar struct{ err error }

func (registrar *scriptedProductChannelRegistrar) RegisterServiceProductForm(
	context.Context, application.RegisterServiceProductFormCommand,
) (application.ProductChannelResult, error) {
	return application.ProductChannelResult{}, registrar.err
}

func (registrar *scriptedProductChannelRegistrar) RegisterMapping(
	context.Context, application.RegisterProductChannelMappingCommand,
) (application.ProductChannelResult, error) {
	return application.ProductChannelResult{}, registrar.err
}

type scriptedPublisher struct{ err error }

func (publisher *scriptedPublisher) Handle(
	context.Context, application.PublishCommercialAuthorityCommand,
) (application.PublishCommercialAuthorityResult, error) {
	return application.PublishCommercialAuthorityResult{}, publisher.err
}

// bodyReadProbe 记录端点有没有读过请求体。未配置态下读了就是采信的第一个征兆。
type bodyReadProbe struct{ read bool }

func (probe *bodyReadProbe) Read([]byte) (int, error) {
	probe.read = true
	return 0, errors.New("未配置态不该读登记载荷")
}

// commercialRegistrationEndpoints 遍历八个登记端点，各配生产侧的未配置 Intake 与一个
// 「被调即失败」的编排替身。逐个走一遍而不是只测一族：端点体虽由
// newRegistrationEndpoint 共用，八个构造函数各自把哪个 Intake 方法接到哪个编排方法上
// 是逐个写的，接错一格只有这张表看得见。
func commercialRegistrationEndpoints(t *testing.T) map[string]http.Handler {
	t.Helper()
	unconfigured := commercialhttp.UnconfiguredIntake{}
	parties := unreachablePartyRegistrar{t: t}
	products := unreachableProductChannelRegistrar{t: t}
	return map[string]http.Handler{
		"商业权威依据发布": commercialhttp.NewPublishCommercialAuthorityEndpoint(
			unconfigured, unreachablePublisher{t: t}),
		"业务参与方身份": commercialhttp.NewRegisterBusinessPartyEndpoint(unconfigured, parties),
		"责任法人身份":  commercialhttp.NewRegisterLegalEntityEndpoint(unconfigured, parties),
		"货主客户账户":  commercialhttp.NewRegisterCustomerAccountEndpoint(unconfigured, parties),
		"参与方关系":   commercialhttp.NewRegisterPartyRelationshipEndpoint(unconfigured, parties),
		"身份停用":    commercialhttp.NewDeactivatePartyIdentityEndpoint(unconfigured, parties),
		"服务形态":    commercialhttp.NewRegisterServiceProductFormEndpoint(unconfigured, products),
		"产品渠道映射":  commercialhttp.NewRegisterProductChannelMappingEndpoint(unconfigured, products),
	}
}

func TestCommercialRegistrationEndpointsOnlyAcceptPost(t *testing.T) {
	for name, endpoint := range commercialRegistrationEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))

			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf("GET 答 %d，want 405", recorder.Code)
			}
			if allow := recorder.Header().Get("Allow"); allow != http.MethodPost {
				t.Fatalf("Allow = %q，want POST", allow)
			}
		})
	}
}

// Covers: ADR-0055「未配置即拒、不读内容」——八个登记端点各答 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED，不读报文、不构造命令；按 ADR-0022，4xx 不带
// `outcome`。写准入不另立形（ADR-0085 Decision 一），因此这一格与其余命令面同答。
func TestUnconfiguredCommercialRegistrationRefusesWithoutReadingTheBody(t *testing.T) {
	for name, endpoint := range commercialRegistrationEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			probe := &bodyReadProbe{}
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", probe))

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("未配置答 %d，want 403", recorder.Code)
			}
			if code := errorCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("错误码 = %q，want ACCESS_CHANNEL_NOT_CONFIGURED", code)
			}
			assertNoRegistrationOutcome(t, recorder)
			if probe.read {
				t.Fatal("未配置 Intake 读了登记载荷")
			}
		})
	}
}

// Covers: ADR-0055「答复对一切请求内容与自报身份一致」——登记载荷里的 tenantId 是这一
// 面最危险的自报身份（ADR-0003 隔离边界），答复随它变化就是采信的第一个征兆。
func TestUnconfiguredCommercialRegistrationAnswersEveryRequestIdentically(t *testing.T) {
	for name, endpoint := range commercialRegistrationEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			baseline := httptest.NewRecorder()
			endpoint.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, "/probe", nil))

			variants := map[string]*http.Request{
				"空载荷": httptest.NewRequest(http.MethodPost, "/probe", nil),
				"登记快照": httptest.NewRequest(http.MethodPost, "/probe",
					strings.NewReader(`{"tenantId":"`+pcTenant+`","partyId":"SYN-PARTY-1"}`)),
				"畸形载荷": httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader("!!not-json!!")),
				"查询串":  httptest.NewRequest(http.MethodPost, "/probe?tenant="+pcTenant, nil),
				"另一租户": httptest.NewRequest(http.MethodPost, "/probe",
					strings.NewReader(`{"tenantId":"SYN-TEN-OTHER"}`)),
			}
			reported := httptest.NewRequest(http.MethodPost, "/probe", nil)
			reported.Header.Set("X-Reported-Tenant", pcTenant)
			variants["自报身份头部"] = reported

			for variant, request := range variants {
				recorder := httptest.NewRecorder()
				endpoint.ServeHTTP(recorder, request)
				if recorder.Code != baseline.Code || recorder.Body.String() != baseline.Body.String() {
					t.Fatalf("%s：答复与基线不同：%d %s vs %d %s",
						variant, recorder.Code, recorder.Body.String(),
						baseline.Code, baseline.Body.String())
				}
			}
		})
	}
}

func TestCommercialRegistrationMapsMalformedIntakeToBadRequest(t *testing.T) {
	registrar := &scriptedPartyRegistrar{}
	endpoint := commercialhttp.NewRegisterBusinessPartyEndpoint(
		commercialIntakeDouble{
			err: fmt.Errorf("登记载荷缺格：%w", commercialhttp.ErrMalformedRequest),
		},
		registrar,
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("畸形请求答 %d，want 400", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "MALFORMED_REQUEST" {
		t.Fatalf("错误码 = %q", code)
	}
	if registrar.called {
		t.Fatal("Intake 没交出命令，编排不该被调到")
	}
}

func TestCommercialRegistrationMapsOtherIntakeFailureToIntakeFailed(t *testing.T) {
	registrar := &scriptedPartyRegistrar{}
	endpoint := commercialhttp.NewRegisterBusinessPartyEndpoint(
		commercialIntakeDouble{err: errors.New("认证后端寄了")},
		registrar,
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Intake 故障答 %d，want 500", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "INTAKE_FAILED" {
		t.Fatalf("错误码 = %q", code)
	}
	if registrar.called {
		t.Fatal("Intake 没交出命令，编排不该被调到")
	}
}

// TestCommercialRegistrationAnswersServerErrorWhenTheOrchestrationFails 三族各走一遍：
// 编排交回 error 是依赖故障，登记与否未知，按 ADR-0022 只报「没形成答案」且不带
// `outcome`。
func TestCommercialRegistrationAnswersServerErrorWhenTheOrchestrationFails(t *testing.T) {
	broken := errors.New("库连不上")
	endpoints := map[string]http.Handler{
		"发布": commercialhttp.NewPublishCommercialAuthorityEndpoint(
			commercialIntakeDouble{}, &scriptedPublisher{err: broken}),
		"参与方身份": commercialhttp.NewRegisterBusinessPartyEndpoint(
			commercialIntakeDouble{}, &scriptedPartyRegistrar{err: broken}),
		"产品渠道映射": commercialhttp.NewRegisterProductChannelMappingEndpoint(
			commercialIntakeDouble{}, &scriptedProductChannelRegistrar{err: broken}),
	}
	for name, endpoint := range endpoints {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("编排故障答 %d，want 500", recorder.Code)
			}
			if code := errorCode(t, recorder); code != "NO_ANSWER_FORMED" {
				t.Fatalf("错误码 = %q", code)
			}
			assertNoRegistrationOutcome(t, recorder)
		})
	}
}

// TestCommercialRegistrationRefusesToShipAnAnswerWithoutAName 三族各走一遍：空
// `outcome` 会被调用侧当成一种新的业务结果，而它其实是实现坏了。三族各有自己的转写
// 函数，因此这一格要逐族证，不能只证一族。
func TestCommercialRegistrationRefusesToShipAnAnswerWithoutAName(t *testing.T) {
	endpoints := map[string]http.Handler{
		"发布": commercialhttp.NewPublishCommercialAuthorityEndpoint(
			commercialIntakeDouble{}, &scriptedPublisher{}),
		"参与方身份": commercialhttp.NewRegisterBusinessPartyEndpoint(
			commercialIntakeDouble{}, &scriptedPartyRegistrar{}),
		"产品渠道映射": commercialhttp.NewRegisterProductChannelMappingEndpoint(
			commercialIntakeDouble{}, &scriptedProductChannelRegistrar{}),
	}
	for name, endpoint := range endpoints {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("无名答案答 %d，want 500", recorder.Code)
			}
			if code := errorCode(t, recorder); code != "UNNAMED_OUTCOME" {
				t.Fatalf("错误码 = %q", code)
			}
		})
	}
}

// TestIsolatedReadIntakeCannotServeCommercialRegistration 钉住 ADR-0078 的排除在写面
// 成立：隔离读放行类型不满足八个登记命令 Intake 接口中的任何一个。这条一旦变红，说明
// 有人把隔离放行扩到了写行——那是 ADR-0085 Decision 一明文维持的原判。
func TestIsolatedReadIntakeCannotServeCommercialRegistration(t *testing.T) {
	var intake any = commercialhttp.IsolatedOperationsReadIntake{}
	if _, ok := intake.(commercialhttp.CommercialPublicationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进发布口")
	}
	if _, ok := intake.(commercialhttp.BusinessPartyRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进业务参与方登记口")
	}
	if _, ok := intake.(commercialhttp.LegalEntityRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进责任法人登记口")
	}
	if _, ok := intake.(commercialhttp.CustomerAccountRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进货主客户账户登记口")
	}
	if _, ok := intake.(commercialhttp.PartyRelationshipRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进参与方关系登记口")
	}
	if _, ok := intake.(commercialhttp.PartyIdentityDeactivationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进身份停用口")
	}
	if _, ok := intake.(commercialhttp.ServiceProductFormRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进服务形态登记口")
	}
	if _, ok := intake.(commercialhttp.ProductChannelMappingRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进产品渠道映射登记口")
	}
}

// ---- 参与方身份族：答案逐名转写 ----

// stubPartyIdentityRegistry 顶替身份登记册。落点脚本化，Load* 交回测试摆好的最新修订。
type stubPartyIdentityRegistry struct {
	saveOutcome ports.PartyRegistrySaveOutcome
	latest      domain.BusinessPartyRegistration
	found       bool
}

func (stub stubPartyIdentityRegistry) SaveBusinessParty(
	context.Context, domain.BusinessPartyRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	return stub.saveOutcome, nil
}

func (stub stubPartyIdentityRegistry) SaveLegalEntity(
	context.Context, domain.LegalEntityRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	return stub.saveOutcome, nil
}

func (stub stubPartyIdentityRegistry) SaveCustomerAccount(
	context.Context, domain.CustomerAccountRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	return stub.saveOutcome, nil
}

func (stub stubPartyIdentityRegistry) SaveRelationship(
	context.Context, domain.PartyRelationshipRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	return stub.saveOutcome, nil
}

func (stub stubPartyIdentityRegistry) LoadLatestBusinessParty(
	context.Context, domain.TenantID, domain.PartyID,
) (domain.BusinessPartyRegistration, bool, error) {
	return stub.latest, stub.found, nil
}

func (stub stubPartyIdentityRegistry) LoadLatestLegalEntity(
	context.Context, domain.TenantID, domain.LegalEntityReference,
) (domain.LegalEntityRegistration, bool, error) {
	return domain.LegalEntityRegistration{}, false, nil
}

func (stub stubPartyIdentityRegistry) LoadLatestCustomerAccount(
	context.Context, domain.TenantID, domain.CustomerAccountID,
) (domain.CustomerAccountRegistration, bool, error) {
	return domain.CustomerAccountRegistration{}, false, nil
}

func (stub stubPartyIdentityRegistry) LoadLatestRelationship(
	context.Context, domain.TenantID, domain.RelationshipID,
) (domain.PartyRelationshipRegistration, bool, error) {
	return domain.PartyRelationshipRegistration{}, false, nil
}

func pcBusinessPartyCommand(t *testing.T, revision int) application.RegisterBusinessPartyCommand {
	t.Helper()
	party, err := domain.NewBusinessParty(
		pcNew(t, domain.NewTenantID, pcTenant),
		pcNew(t, domain.NewPartyID, "SYN-PARTY-1"),
		pcNew(t, domain.NewPartyName, "合成参与方一号"),
	)
	if err != nil {
		t.Fatalf("参与方：%v", err)
	}
	return application.RegisterBusinessPartyCommand{
		Party:         party,
		Revision:      revision,
		Basis:         pcNew(t, domain.NewIdentityBasisReference, "SYN-BASIS-1"),
		EffectiveFrom: pcEffectiveFrom,
	}
}

// pcRegisteredParty 造一笔已在册的首笔修订，供停用与修订连续性两格用。走真构造门而不是
// 重建结构：停用要在它上面做真转换，凑出来的壳过不了那道门。
func pcRegisteredParty(t *testing.T) domain.BusinessPartyRegistration {
	t.Helper()
	command := pcBusinessPartyCommand(t, 1)
	lifecycle, err := domain.NewIdentityLifecycle(command.EffectiveFrom)
	if err != nil {
		t.Fatalf("生命周期：%v", err)
	}
	registration, err := domain.NewBusinessPartyRegistration(
		command.Party, 1, command.Basis, lifecycle)
	if err != nil {
		t.Fatalf("首笔修订：%v", err)
	}
	return registration
}

// TestPartyIdentityRegistrationTranscribesTheUseCaseAnswersVerbatim 证身份族的答案逐名
// 过线：本次落库的两格走 201，登记册的其余判断走 200 带原名。
//
// 治理答案与受理拒绝不用 4xx——它们是登记册对这次登记作出的判断，与「请求根本构造不出
// 命令」的恢复动作不同，压成一格登记方就不知道该改内容还是改请求形状。
func TestPartyIdentityRegistrationTranscribesTheUseCaseAnswersVerbatim(t *testing.T) {
	cases := []struct {
		name       string
		registry   stubPartyIdentityRegistry
		revision   int
		wantStatus int
		wantAnswer string
	}{
		{
			name:       "首笔登记",
			registry:   stubPartyIdentityRegistry{saveOutcome: ports.PartyRegistrySaved},
			revision:   1,
			wantStatus: http.StatusCreated,
			wantAnswer: "REGISTERED",
		},
		{
			name:       "同键同内容重放",
			registry:   stubPartyIdentityRegistry{saveOutcome: ports.PartyRegistryAlreadyRegistered},
			revision:   1,
			wantStatus: http.StatusOK,
			wantAnswer: "ALREADY_REGISTERED",
		},
		{
			name:       "同修订异内容是冲突",
			registry:   stubPartyIdentityRegistry{saveOutcome: ports.PartyRegistryContentConflict},
			revision:   1,
			wantStatus: http.StatusOK,
			wantAnswer: "CONTENT_CONFLICT",
		},
		{
			// 从未登记的身份只收修订 1；跳号说明操作者看到的册面已陈旧。
			name:       "修订错位是受理拒绝",
			registry:   stubPartyIdentityRegistry{saveOutcome: ports.PartyRegistrySaved},
			revision:   3,
			wantStatus: http.StatusOK,
			wantAnswer: "NOT_ACCEPTED",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := application.NewRegisterPartyIdentityHandler(testCase.registry)
			endpoint := commercialhttp.NewRegisterBusinessPartyEndpoint(
				commercialIntakeDouble{businessParty: pcBusinessPartyCommand(t, testCase.revision)},
				handler,
			)
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("答 %d，want %d（body %s）", recorder.Code, testCase.wantStatus, recorder.Body)
			}
			if answer := registrationAnswerOf(t, recorder).Outcome; answer != testCase.wantAnswer {
				t.Fatalf("outcome = %q，want %q", answer, testCase.wantAnswer)
			}
		})
	}
}

// TestNotAcceptedCarriesTheReasonAsProse 钉住 `cause` 在场：登记方拿一个没有指名的
// `未受理`什么也补不了——他既不知道该改修订号还是改引用，也不知道要不要重来。
//
// 断言只查「非空且提到修订」，不逐字比对那句话：它是散文不是格，用例改一个字不该让
// 这条测试变红；真要可判别的理由代数，得在用例侧立封闭枚举（先例是网络目录登记的
// RefusalReason），那时这条断言换成逐格比对。
func TestNotAcceptedCarriesTheReasonAsProse(t *testing.T) {
	handler := application.NewRegisterPartyIdentityHandler(
		stubPartyIdentityRegistry{saveOutcome: ports.PartyRegistrySaved})
	endpoint := commercialhttp.NewRegisterBusinessPartyEndpoint(
		commercialIntakeDouble{businessParty: pcBusinessPartyCommand(t, 3)},
		handler,
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	answer := registrationAnswerOf(t, recorder)
	if answer.Outcome != "NOT_ACCEPTED" {
		t.Fatalf("outcome = %q，want NOT_ACCEPTED", answer.Outcome)
	}
	if answer.Cause == "" {
		t.Fatal("未受理没带原因，登记方无从判断该改哪一样")
	}
	if !strings.Contains(answer.Cause, "修订") {
		t.Fatalf("cause = %q，未提到修订错位这件事", answer.Cause)
	}
}

// TestSuccessfulRegistrationCarriesNoCause 证 `cause` 只在被拒时在场：登记成了就没有
// 「差哪格」可言，给一个空串会让调用侧先判字段有没有值再判答案。
func TestSuccessfulRegistrationCarriesNoCause(t *testing.T) {
	handler := application.NewRegisterPartyIdentityHandler(
		stubPartyIdentityRegistry{saveOutcome: ports.PartyRegistrySaved})
	endpoint := commercialhttp.NewRegisterBusinessPartyEndpoint(
		commercialIntakeDouble{businessParty: pcBusinessPartyCommand(t, 1)},
		handler,
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	body := decodeBody(t, recorder)
	if _, present := body["cause"]; present {
		t.Fatalf("已登记的答复带了 cause：%s", recorder.Body)
	}
}

// TestDeactivationTranscribesItsOwnTwoGrades 证停用那两格：册上有这一笔时走真转换、
// 答`已停用`取 201；册上没有时答`未找到`取 200。
//
// `未找到`不折成 404：404 说的是「这个产品没有这条能力」，而这里能力在、册也在，只是
// 册上没有这一个身份——登记方要去查的是册面不是路由。
func TestDeactivationTranscribesItsOwnTwoGrades(t *testing.T) {
	command := application.DeactivatePartyIdentityCommand{
		Tenant:   pcNew(t, domain.NewTenantID, pcTenant),
		Kind:     application.BusinessPartyIdentity,
		ID:       "SYN-PARTY-1",
		Revision: 2,
		Basis:    pcNew(t, domain.NewIdentityBasisReference, "SYN-BASIS-STOP"),
		At:       pcNow,
	}

	cases := []struct {
		name       string
		registry   stubPartyIdentityRegistry
		wantStatus int
		wantAnswer string
	}{
		{
			name: "册上有这一笔",
			registry: stubPartyIdentityRegistry{
				saveOutcome: ports.PartyRegistrySaved,
				latest:      pcRegisteredParty(t),
				found:       true,
			},
			wantStatus: http.StatusCreated,
			wantAnswer: "DEACTIVATED",
		},
		{
			name:       "册上从未登记",
			registry:   stubPartyIdentityRegistry{saveOutcome: ports.PartyRegistrySaved},
			wantStatus: http.StatusOK,
			wantAnswer: "NOT_FOUND",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := application.NewRegisterPartyIdentityHandler(testCase.registry)
			endpoint := commercialhttp.NewDeactivatePartyIdentityEndpoint(
				commercialIntakeDouble{deactivation: command}, handler)
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("答 %d，want %d（body %s）", recorder.Code, testCase.wantStatus, recorder.Body)
			}
			if answer := registrationAnswerOf(t, recorder).Outcome; answer != testCase.wantAnswer {
				t.Fatalf("outcome = %q，want %q", answer, testCase.wantAnswer)
			}
		})
	}
}

// ---- 产品渠道族：答案逐名转写 ----

// stubProductChannelPorts 同时顶形态册（发布册的切面）与映射册。整册由测试摆好：映射
// 登记要在册上找得到它钉的那个产品版本。
type stubProductChannelPorts struct {
	registry    *domain.CommercialRegistry
	saveMapping ports.MappingSaveOutcome
}

func (stub stubProductChannelPorts) LoadForScope(
	context.Context, domain.TenantID, domain.CommercialScopeReference,
) (*domain.CommercialRegistry, error) {
	if stub.registry == nil {
		return domain.NewCommercialRegistry(), nil
	}
	return stub.registry, nil
}

func (stub stubProductChannelPorts) SaveServiceProduct(
	context.Context, domain.ServiceProduct,
) (ports.ServiceProductSaveOutcome, error) {
	return ports.ServiceProductSaved, nil
}

func (stub stubProductChannelPorts) SaveMapping(
	context.Context, domain.ProductChannelMappingRegistration,
) (ports.MappingSaveOutcome, error) {
	return stub.saveMapping, nil
}

func (stub stubProductChannelPorts) LoadLatestMapping(
	context.Context, domain.TenantID, domain.ProductChannelMappingID,
) (domain.ProductChannelMappingRegistration, bool, error) {
	return domain.ProductChannelMappingRegistration{}, false, nil
}

// pcPublishedProduct 把一份服务产品版本按已生效走完生命周期后放进整册，演「库里已有这个
// 产品版本」。走真 Publish/TakeEffect 而不是重建：映射的引用检查看的就是发布固定下来的
// 那份内容。
func pcPublishedProduct(t *testing.T) *domain.CommercialRegistry {
	t.Helper()
	registry := domain.NewCommercialRegistry()
	interval, err := domain.NewEffectiveInterval(pcPublishStart, time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	draft, err := domain.NewCommercialDraft(domain.CommercialVersionSpec{
		TenantID:      pcNew(t, domain.NewTenantID, pcTenant),
		Kind:          domain.ServiceProductObject,
		ObjectID:      pcNew(t, domain.NewCommercialObjectID, "SYN-PROD-1"),
		Version:       pcNew(t, domain.NewCommercialVersionLabel, "v1"),
		Scope:         pcNew(t, domain.NewCommercialScopeReference, pcScope),
		ContentDigest: pcNew(t, domain.NewCommercialContentDigest, "sha256:SYN-PROD-1-v1"),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("草稿：%v", err)
	}
	published, err := draft.Publish(
		pcApproval(t, "SYN-PROD-1"), domain.ApprovalRoleConfirmed, pcNow.Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	live, err := published.TakeEffect(pcNow.Add(-time.Hour))
	if err != nil {
		t.Fatalf("取效：%v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("入册：%v", err)
	}
	return registry
}

func pcApproval(t *testing.T, objectID string) domain.ApprovalBasis {
	t.Helper()
	approval, err := domain.NewApprovalBasis(
		pcNew(t, domain.NewApprovalReference, "SYN-APPROVAL-"+objectID),
		pcNew(t, domain.NewCommercialSourceReference, "SYN-SOURCE-"+objectID),
		pcPublishStart.Add(-24*time.Hour),
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	return approval
}

func pcMappingCommand(t *testing.T, product string) application.RegisterProductChannelMappingCommand {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(pcPublishStart, time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	return application.RegisterProductChannelMappingCommand{
		Tenant:   pcNew(t, domain.NewTenantID, pcTenant),
		Scope:    pcNew(t, domain.NewCommercialScopeReference, pcScope),
		ID:       pcNew(t, domain.NewProductChannelMappingID, "SYN-MAP-1"),
		Revision: 1,
		Spec: domain.ProductChannelMappingSpec{
			Product:        pcNew(t, domain.NewCommercialObjectID, product),
			ProductVersion: pcNew(t, domain.NewCommercialVersionLabel, "v1"),
			// 显式的“未配置”绑定：登记者说出「该产品尚无可用渠道候选」，不是缺件。
			Binding:   domain.UnconfiguredChannelBinding(),
			Effective: interval,
			Basis:     pcNew(t, domain.NewMappingBasisReference, "SYN-MAP-BASIS-1"),
		},
	}
}

// TestProductChannelRegistrationTranscribesTheUseCaseAnswersVerbatim 证产品渠道族有
// 自己的一份转写：`已登记`取 201，受理拒绝取 200 带散文原因。这一族与身份族共用响应
// 形状却各写一份转写函数，因此两族都要各证一遍——共用的是形状，不是判据。
func TestProductChannelRegistrationTranscribesTheUseCaseAnswersVerbatim(t *testing.T) {
	cases := []struct {
		name       string
		product    string
		save       ports.MappingSaveOutcome
		wantStatus int
		wantAnswer string
	}{
		{
			name:       "钉在册上产品版本的首笔映射",
			product:    "SYN-PROD-1",
			save:       ports.MappingSaved,
			wantStatus: http.StatusCreated,
			wantAnswer: "REGISTERED",
		},
		{
			name:       "同键同内容重放",
			product:    "SYN-PROD-1",
			save:       ports.MappingAlreadyRegistered,
			wantStatus: http.StatusOK,
			wantAnswer: "ALREADY_REGISTERED",
		},
		{
			// 册上没有这个产品版本：映射不钉悬空引用。
			name:       "悬空产品引用是受理拒绝",
			product:    "SYN-PROD-ABSENT",
			save:       ports.MappingSaved,
			wantStatus: http.StatusOK,
			wantAnswer: "NOT_ACCEPTED",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stub := stubProductChannelPorts{registry: pcPublishedProduct(t), saveMapping: testCase.save}
			handler := application.NewRegisterProductChannelHandler(stub, stub)
			endpoint := commercialhttp.NewRegisterProductChannelMappingEndpoint(
				commercialIntakeDouble{mapping: pcMappingCommand(t, testCase.product)}, handler)
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("答 %d，want %d（body %s）", recorder.Code, testCase.wantStatus, recorder.Body)
			}
			answer := registrationAnswerOf(t, recorder)
			if answer.Outcome != testCase.wantAnswer {
				t.Fatalf("outcome = %q，want %q", answer.Outcome, testCase.wantAnswer)
			}
			if testCase.wantAnswer == "NOT_ACCEPTED" && answer.Cause == "" {
				t.Fatal("受理拒绝没带原因")
			}
		})
	}
}

// ---- 发布族：答案逐名转写 ----

// stubPublicationRegistry 顶替发布登记册。九个声明 Save 共用一个落点脚本；本片只用到
// 收寄资格一族，其余照样实现——端口是整只，实现半只编不过。
type stubPublicationRegistry struct {
	versionOutcome     ports.PublicationSaveOutcome
	declarationOutcome ports.DeclarationSaveOutcome
}

func (stub stubPublicationRegistry) declaration() (ports.DeclarationSaveOutcome, error) {
	if stub.declarationOutcome == ports.DeclarationSaveOutcomeInvalid {
		return ports.DeclarationSaved, nil
	}
	return stub.declarationOutcome, nil
}

func (stub stubPublicationRegistry) LoadForScope(
	context.Context, domain.TenantID, domain.CommercialScopeReference,
) (*domain.CommercialRegistry, error) {
	return domain.NewCommercialRegistry(), nil
}

func (stub stubPublicationRegistry) SaveVersion(
	context.Context, domain.CommercialVersion,
) (ports.PublicationSaveOutcome, error) {
	if stub.versionOutcome == ports.PublicationSaveOutcomeInvalid {
		return ports.PublicationSaved, nil
	}
	return stub.versionOutcome, nil
}

func (stub stubPublicationRegistry) SaveServiceProduct(
	context.Context, domain.ServiceProduct,
) (ports.ServiceProductSaveOutcome, error) {
	return ports.ServiceProductSaveOutcomeInvalid, errors.New("发布用例不该触碰服务形态册")
}

func (stub stubPublicationRegistry) SaveValidityCorrection(
	context.Context, domain.ValidityCorrection,
) (ports.ValidityCorrectionSaveOutcome, error) {
	return ports.ValidityCorrectionSaveOutcomeInvalid, errors.New("发布用例不该触碰更正册")
}

func (stub stubPublicationRegistry) SavePricePolicy(
	context.Context, domain.CommercialPricePolicy, domain.PriceDirection, domain.PlanBindingConversion,
) (ports.PricePolicySaveOutcome, error) {
	return ports.PricePolicySaveOutcomeInvalid, errors.New("本片不用价格政策册")
}

func (stub stubPublicationRegistry) SaveSettlementPolicy(
	context.Context, domain.SettlementPolicy,
) (ports.SettlementPolicySaveOutcome, error) {
	return ports.SettlementPolicySaveOutcomeInvalid, errors.New("本片不用结算政策册")
}

func (stub stubPublicationRegistry) SaveCreditPolicy(
	context.Context, domain.CreditPolicy,
) (ports.CreditPolicySaveOutcome, error) {
	return ports.CreditPolicySaveOutcomeInvalid, errors.New("本片不用信用政策册")
}

func (stub stubPublicationRegistry) SaveSupplierAgreement(
	context.Context, domain.SupplierAgreement,
) (ports.SupplierAgreementSaveOutcome, error) {
	return ports.SupplierAgreementSaveOutcomeInvalid, errors.New("本片不用供应商协议册")
}

func (stub stubPublicationRegistry) SaveAsOfPolicies(
	context.Context, domain.AsOfDeclaration,
) (ports.DeclarationSaveOutcome, error) {
	return stub.declaration()
}

func (stub stubPublicationRegistry) SaveAcceptanceRuleContent(
	context.Context, domain.AcceptanceRuleContent,
) (ports.DeclarationSaveOutcome, error) {
	return stub.declaration()
}

func (stub stubPublicationRegistry) SavePendingRoutingPermission(
	context.Context, domain.PendingRoutingPermission,
) (ports.DeclarationSaveOutcome, error) {
	return stub.declaration()
}

func (stub stubPublicationRegistry) SavePreAcceptanceControl(
	context.Context, domain.PreAcceptanceControlDeclaration,
) (ports.DeclarationSaveOutcome, error) {
	return stub.declaration()
}

func (stub stubPublicationRegistry) SaveCustomerContractContent(
	context.Context, domain.CustomerContract,
) (ports.DeclarationSaveOutcome, error) {
	return stub.declaration()
}

func (stub stubPublicationRegistry) SaveIntakeQualification(
	context.Context, domain.IntakeQualificationContent,
) (ports.DeclarationSaveOutcome, error) {
	return stub.declaration()
}

func (stub stubPublicationRegistry) SaveFinalRule(
	context.Context, domain.FinalRuleContent,
) (ports.DeclarationSaveOutcome, error) {
	return stub.declaration()
}

func (stub stubPublicationRegistry) SaveCancellationAuthority(
	context.Context, domain.CancellationAuthorityContent,
) (ports.DeclarationSaveOutcome, error) {
	return stub.declaration()
}

func (stub stubPublicationRegistry) SaveAcceptanceRulePackage(
	context.Context, domain.AcceptanceRulePackage,
) (ports.DeclarationSaveOutcome, error) {
	return stub.declaration()
}

func pcPublishCommand(
	t *testing.T,
	standing domain.ApprovalRoleStanding,
	startsAt time.Time,
) application.PublishCommercialAuthorityCommand {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(startsAt, time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	return application.PublishCommercialAuthorityCommand{
		Spec: domain.CommercialVersionSpec{
			TenantID:      pcNew(t, domain.NewTenantID, pcTenant),
			Kind:          domain.AcceptanceRulePackageObject,
			ObjectID:      pcNew(t, domain.NewCommercialObjectID, "SYN-RULES-1"),
			Version:       pcNew(t, domain.NewCommercialVersionLabel, "v1"),
			Scope:         pcNew(t, domain.NewCommercialScopeReference, pcScope),
			ContentDigest: pcNew(t, domain.NewCommercialContentDigest, "sha256:SYN-RULES-1-v1"),
			Effective:     interval,
		},
		Approval:     pcApproval(t, "SYN-RULES-1"),
		RoleStanding: standing,
	}
}

func pcPublishEndpoint(
	t *testing.T,
	command application.PublishCommercialAuthorityCommand,
	registry stubPublicationRegistry,
) http.Handler {
	t.Helper()
	handler := application.NewPublishCommercialAuthorityHandler(registry, pcClock{at: pcNow})
	return commercialhttp.NewPublishCommercialAuthorityEndpoint(
		commercialIntakeDouble{publication: command}, handler)
}

// TestPublicationTranscribesTheUseCaseAnswersVerbatim 证发布族的四格：边界已开的取效后
// 入册答`已发布已生效`、边界未开答`已计划生效`，两格都是本次落库因此取 201；重放与内容
// 冲突是登记册的治理答案，取 200。
//
// `已计划生效`取 201 而不是另设一格：它确实入了册，只是不得用于生产解析——那一格由
// 消费侧 AppliesAt 结构性保证，传输层不替它把关，也不靠状态码去暗示。
func TestPublicationTranscribesTheUseCaseAnswersVerbatim(t *testing.T) {
	cases := []struct {
		name       string
		startsAt   time.Time
		registry   stubPublicationRegistry
		wantStatus int
		wantAnswer string
	}{
		{
			name:       "边界已开当场取效",
			startsAt:   pcPublishStart,
			wantStatus: http.StatusCreated,
			wantAnswer: "PUBLISHED_EFFECTIVE",
		},
		{
			name:       "边界未开是已计划生效",
			startsAt:   pcNow.Add(30 * 24 * time.Hour),
			wantStatus: http.StatusCreated,
			wantAnswer: "PLANNED_EFFECTIVE",
		},
		{
			name:       "同一份重放",
			startsAt:   pcPublishStart,
			registry:   stubPublicationRegistry{versionOutcome: ports.PublicationAlreadyRegistered},
			wantStatus: http.StatusOK,
			wantAnswer: "REPLAYED",
		},
		{
			name:       "同键异内容是冲突",
			startsAt:   pcPublishStart,
			registry:   stubPublicationRegistry{versionOutcome: ports.PublicationContentConflict},
			wantStatus: http.StatusOK,
			wantAnswer: "CONTENT_CONFLICT",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			endpoint := pcPublishEndpoint(t,
				pcPublishCommand(t, domain.ApprovalRoleConfirmed, testCase.startsAt),
				testCase.registry)
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("答 %d，want %d（body %s）", recorder.Code, testCase.wantStatus, recorder.Body)
			}
			if answer := publicationAnswerOf(t, recorder).Outcome; answer != testCase.wantAnswer {
				t.Fatalf("outcome = %q，want %q", answer, testCase.wantAnswer)
			}
		})
	}
}

// TestPublicationPendingIsAnAnswerNotAMissingOne 钉住本片与关务片分道的那一格：关务把
// 它那个 `UNDECIDED` 折成 500「没形成答案」，这里的`发布未决`走 200。
//
// 两者同名不同物。关务那一格是用例把依赖故障折成的值，说的是「登记与否未知」，续办是
// 重跑同一份；这里的未决是 AT-PC-010 指名的业务答案：批准角色未确认，一个字节没写，
// 续办是去确认角色，重跑同一份不会变。折成 5xx 会让调用侧把一件等人办的事留队重发。
func TestPublicationPendingIsAnAnswerNotAMissingOne(t *testing.T) {
	endpoint := pcPublishEndpoint(t,
		pcPublishCommand(t, domain.ApprovalRoleUnconfirmed, pcPublishStart),
		stubPublicationRegistry{})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("发布未决答 %d，want 200（body %s）", recorder.Code, recorder.Body)
	}
	answer := publicationAnswerOf(t, recorder)
	if answer.Outcome != "PENDING" {
		t.Fatalf("outcome = %q，want PENDING", answer.Outcome)
	}
	if answer.PendingCause == "" {
		t.Fatal("未决没带原因：等角色确认与等被引对象发布的续办动作不同，靠格分辨不出来")
	}
}

// TestDeclarationLandingsRideAlongWithTheVersionAnswer 钉住声明落点必须在场：声明与版本
// 同笔落库，某个通道撞上同键异内容时版本仍可能是`已发布已生效`。省掉声明落点，一次
// 半数声明没进去的发布在调用侧看起来与全都落定的发布一模一样——而受控 CLI 恰恰按这一格
// 抬退出码要商业责任方去看。
func TestDeclarationLandingsRideAlongWithTheVersionAnswer(t *testing.T) {
	command := pcPublishCommand(t, domain.ApprovalRoleConfirmed, pcPublishStart)
	command.Declarations = application.CommercialDeclarations{
		IntakeQualification: &application.IntakeQualificationDeclaration{
			Sources: []domain.DeclaredIntakeSource{domain.DeclaredNodeIntake},
		},
	}
	endpoint := pcPublishEndpoint(t, command,
		stubPublicationRegistry{declarationOutcome: ports.DeclarationContentConflict})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("答 %d，want 201（body %s）", recorder.Code, recorder.Body)
	}
	answer := publicationAnswerOf(t, recorder)
	if answer.Outcome != "PUBLISHED_EFFECTIVE" {
		t.Fatalf("outcome = %q，want PUBLISHED_EFFECTIVE", answer.Outcome)
	}
	if len(answer.Declarations) != 1 {
		t.Fatalf("声明落点 %d 条，want 1（body %s）", len(answer.Declarations), recorder.Body)
	}
	if answer.Declarations[0].Channel != "INTAKE_QUALIFICATION" {
		t.Fatalf("通道 = %q，want INTAKE_QUALIFICATION", answer.Declarations[0].Channel)
	}
	if answer.Declarations[0].Outcome != "CONTENT_CONFLICT" {
		t.Fatalf("声明落点 = %q，want CONTENT_CONFLICT", answer.Declarations[0].Outcome)
	}
}

// TestPublicationWithoutDeclarationsOmitsTheLandingList 证没带声明时那一栏缺席，而不是
// 一个空数组：空数组会读成「跑了声明通道、一条也没落」，而实际是根本没有声明可跑。
func TestPublicationWithoutDeclarationsOmitsTheLandingList(t *testing.T) {
	endpoint := pcPublishEndpoint(t,
		pcPublishCommand(t, domain.ApprovalRoleConfirmed, pcPublishStart),
		stubPublicationRegistry{})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))

	body := decodeBody(t, recorder)
	if _, present := body["declarations"]; present {
		t.Fatalf("没带声明的发布答复里出现了 declarations：%s", recorder.Body)
	}
}

// ---- 解码助手 ----

// 用具名结构而不是 map：字段名若与响应对不上，map 那种写法会静默拿到空串并通过，而
// 字段名正是这几条断言要钉的东西。
type registrationAnswerBody struct {
	Outcome string `json:"outcome"`
	Cause   string `json:"cause"`
}

type publicationAnswerBody struct {
	Outcome      string `json:"outcome"`
	PendingCause string `json:"pendingCause"`
	Declarations []struct {
		Channel string `json:"channel"`
		Outcome string `json:"outcome"`
	} `json:"declarations"`
}

func registrationAnswerOf(t *testing.T, recorder *httptest.ResponseRecorder) registrationAnswerBody {
	t.Helper()
	var body registrationAnswerBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("解登记答复 %s：%v", recorder.Body.Bytes(), err)
	}
	return body
}

func publicationAnswerOf(t *testing.T, recorder *httptest.ResponseRecorder) publicationAnswerBody {
	t.Helper()
	var body publicationAnswerBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("解发布答复 %s：%v", recorder.Body.Bytes(), err)
	}
	return body
}

// assertNoRegistrationOutcome 证 4xx/5xx 不带 `outcome`（ADR-0022）：带上就会让一个
// 「没形成答案」冒充一种业务结果。
func assertNoRegistrationOutcome(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if _, present := decodeBody(t, recorder)["outcome"]; present {
		t.Fatalf("非 2xx 答复带了 outcome：%s", recorder.Body)
	}
}
