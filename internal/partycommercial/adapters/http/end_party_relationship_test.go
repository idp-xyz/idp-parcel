package commercialhttp_test

import (
	"context"
	"errors"
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

// 本文件证参与方关系三个结束口的在线面（票 ps-external-mark-relations/17）：各收受控批文自己那一节恰一项、租户取认证
// 结果；替代缺批准即请求畸形；没有登记册配置写授予在读载荷之前就拒、编排一次也不被调到；答复代数照身份族——`已结束`
// 201，重放、未找到与未受理 200。

const (
	revocationItem   = `{"relationshipId": "SYN-REL-01", "revision": 2, "basis": "SYN-REVOCATION/01", "at": "2026-07-01T00:00:00Z"}`
	expirationItem   = `{"relationshipId": "SYN-REL-02", "revision": 2, "basis": "SYN-TERM-ENDED/02", "at": "2026-06-01T00:00:00Z"}`
	supersessionItem = `{"relationshipId": "SYN-REL-03", "revision": 2, "successor": "SYN-PARTY-09", "at": "2026-07-01T00:00:00Z",` +
		` "successorRelationshipId": "SYN-REL-03-NEXT", "basis": "SYN-HANDOVER/03",` +
		` "approval": {"reference": "SYN-APPROVAL/03", "approvedAt": "2026-07-01T00:00:00Z"}}`
)

func endingIntake(intake *commercialhttp.OperatorRegistryIntake, line string) func(*http.Request) error {
	return func(request *http.Request) error {
		var err error
		switch line {
		case "revocations":
			_, err = intake.IntakePartyRelationshipRevocation(context.Background(), request)
		case "expirations":
			_, err = intake.IntakePartyRelationshipExpiration(context.Background(), request)
		default:
			_, err = intake.IntakePartyRelationshipSupersession(context.Background(), request)
		}
		return err
	}
}

// Covers: 票 17 完成判据「……HTTP……各层用例」的形状门——三口各收自己那一节恰一项、租户取认证结果、逐格不丢；自报租户、
// 零项、别一节的项、替代缺批准都答请求畸形；不带令牌在解载荷之前答凭证被拒。
func TestPartyRelationshipEndingIntakesTakeTheirOwnLineWithTheAuthenticatedTenant(t *testing.T) {
	intake := operatorIntake(t)
	revoke, err := intake.IntakePartyRelationshipRevocation(context.Background(), operatorRequest("t", `{"revocations": [`+revocationItem+`]}`))
	if err != nil || revoke.Tenant.String() != operatorAuthenticatedTenant || revoke.ID.String() != "SYN-REL-01" ||
		revoke.Revision != 2 || revoke.Basis.String() != "SYN-REVOCATION/01" || !revoke.At.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("revocation = (%+v, %v)", revoke, err)
	}
	expire, err := intake.IntakePartyRelationshipExpiration(context.Background(), operatorRequest("t", `{"expirations": [`+expirationItem+`]}`))
	if err != nil || expire.Tenant.String() != operatorAuthenticatedTenant || expire.Basis.String() != "SYN-TERM-ENDED/02" {
		t.Fatalf("expiration = (%+v, %v)", expire, err)
	}
	supersede, err := intake.IntakePartyRelationshipSupersession(context.Background(), operatorRequest("t", `{"supersessions": [`+supersessionItem+`]}`))
	if err != nil || supersede.Tenant.String() != operatorAuthenticatedTenant || supersede.Successor.String() != "SYN-PARTY-09" ||
		supersede.SuccessorRelationship.String() != "SYN-REL-03-NEXT" || supersede.Approval.Reference.String() != "SYN-APPROVAL/03" {
		t.Fatalf("supersession = (%+v, %v)", supersede, err)
	}

	for name, probe := range map[string]struct {
		body string
		line string
	}{
		"自报租户":     {`{"tenantId": "SYN-TENANT-01", "revocations": [` + revocationItem + `]}`, "revocations"},
		"零项":       {`{"revocations": []}`, "revocations"},
		"别一节的项":    {`{"revocations": [` + revocationItem + `], "expirations": [` + expirationItem + `]}`, "revocations"},
		"撤销项投到到期口": {`{"revocations": [` + revocationItem + `]}`, "expirations"},
		"替代缺批准":    {`{"supersessions": [` + strings.Replace(supersessionItem, `, "approval": {"reference": "SYN-APPROVAL/03", "approvedAt": "2026-07-01T00:00:00Z"}`, "", 1) + `]}`, "supersessions"},
		"未知键":      {`{"revocations": [` + strings.Replace(revocationItem, `"basis"`, `"reason": "x", "basis"`, 1) + `]}`, "revocations"},
	} {
		if err := endingIntake(intake, probe.line)(operatorRequest("t", probe.body)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
	if err := endingIntake(intake, "supersessions")(operatorRequest("", `not json`)); !errors.Is(err, commercialhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v, want ErrOperatorCredentialRejected", err)
	}
}

// unreachableEndingRegistrar 被调即失败：越权那一格要证编排一次也没被调到。
type unreachableEndingRegistrar struct{ calls *int }

func (registrar unreachableEndingRegistrar) Revoke(context.Context, application.RevokePartyRelationshipCommand) (application.PartyRegistryResult, error) {
	*registrar.calls++
	return application.PartyRegistryResult{}, errors.New("不该被调到")
}

func (registrar unreachableEndingRegistrar) Expire(context.Context, application.ExpirePartyRelationshipCommand) (application.PartyRegistryResult, error) {
	*registrar.calls++
	return application.PartyRegistryResult{}, errors.New("不该被调到")
}

func (registrar unreachableEndingRegistrar) Supersede(context.Context, application.SupersedePartyRelationshipCommand) (application.PartyRegistryResult, error) {
	*registrar.calls++
	return application.PartyRegistryResult{}, errors.New("不该被调到")
}

// Covers: 票 17 三口的越权一格——令牌有效却没有登记册配置写授予，三口都答 403 OPERATOR_NOT_GRANTED，载荷一个字节不读，
// 编排一次也不被调到。
func TestPartyRelationshipEndingEndpointsRefuseAnOperatorWithoutTheRegistryGrant(t *testing.T) {
	intake := publicationOperatorIntake(t, envelopeOperators{err: commercialhttp.ErrOperatorNotGranted})
	calls := 0
	registrar := unreachableEndingRegistrar{calls: &calls}
	for target, endpoint := range map[string]http.Handler{
		"/commercial-party-relationship-revocations":   commercialhttp.NewRevokePartyRelationshipEndpoint(intake, registrar),
		"/commercial-party-relationship-expirations":   commercialhttp.NewExpirePartyRelationshipEndpoint(intake, registrar),
		"/commercial-party-relationship-supersessions": commercialhttp.NewSupersedePartyRelationshipEndpoint(intake, registrar),
	} {
		probe := &bodyReadProbe{}
		request := httptest.NewRequest(http.MethodPost, target, probe)
		request.Header.Set("Authorization", "Bearer "+presentedToken)
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden || errorCode(t, recorder) != "OPERATOR_NOT_GRANTED" {
			t.Fatalf("%s：答 %d %s，want 403 OPERATOR_NOT_GRANTED", target, recorder.Code, recorder.Body)
		}
		assertNoRegistrationOutcome(t, recorder)
		if probe.read {
			t.Fatalf("%s：没有授予却读了载荷", target)
		}
	}
	if calls != 0 {
		t.Fatal("结束编排被调到了")
	}
}

// endingRegistryStub 在身份登记册替身上摆一段关系的最新修订与落点。
type endingRegistryStub struct {
	stubPartyIdentityRegistry
	relationship domain.PartyRelationshipRegistration
	found        bool
	saved        ports.PartyRegistrySaveOutcome
}

func (stub endingRegistryStub) LoadLatestRelationship(
	context.Context, domain.TenantID, domain.RelationshipID,
) (domain.PartyRelationshipRegistration, bool, error) {
	return stub.relationship, stub.found, nil
}

func (stub endingRegistryStub) SaveRelationship(
	context.Context, domain.PartyRelationshipRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	return stub.saved, nil
}

type endingClock struct{}

func (endingClock) Now() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }

func liveRelationshipRegistration(t *testing.T) domain.PartyRelationshipRegistration {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := domain.NewCandidateRelationship(domain.PartyRelationshipSpec{
		Holder:       pcNew(t, domain.NewPartyID, "SYN-PARTY-01"),
		Counterparty: pcNew(t, domain.NewPartyID, "SYN-PARTY-02"),
		Role:         domain.CustomerRole,
		Scope:        pcNew(t, domain.NewCommercialScopeReference, "SYN-SCOPE"),
		Basis:        pcNew(t, domain.NewRelationshipBasisReference, "SYN-CONTRACT"),
		Effective:    interval,
	})
	if err != nil {
		t.Fatal(err)
	}
	live, err := candidate.Approve(pcNew(t, domain.NewApprovalReference, "SYN-APPROVAL"), interval.StartsAt())
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.NewPartyRelationshipRegistration(
		pcNew(t, domain.NewTenantID, operatorAuthenticatedTenant), pcNew(t, domain.NewRelationshipID, "SYN-REL-01"), 1, live,
	)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

// Covers: 票 17 三口的答复代数——落下终止修订答 201 `ENDED`；落点被同内容占住答 200 `ALREADY_REGISTERED`；关系不在册
// 答 200 `NOT_FOUND`；结束时刻晚于登记时刻答 200 `NOT_ACCEPTED` 并带原因。编排是真用例，册是替身。
func TestPartyRelationshipEndingEndpointsAnswerThroughTheUseCase(t *testing.T) {
	live := liveRelationshipRegistration(t)
	serve := func(registry endingRegistryStub, body string) servedAnswer {
		t.Helper()
		handler := application.NewPartyRelationshipEndingHandler(registry, endingClock{})
		return serveClosure(t, commercialhttp.NewRevokePartyRelationshipEndpoint(operatorIntake(t), handler), body)
	}
	ended := serve(endingRegistryStub{relationship: live, found: true, saved: ports.PartyRegistrySaved}, `{"revocations": [`+revocationItem+`]}`)
	if ended.Code != http.StatusCreated || ended.answer["outcome"] != "ENDED" {
		t.Fatalf("ended = %d %v", ended.Code, ended.answer)
	}
	replay := serve(endingRegistryStub{relationship: live, found: true, saved: ports.PartyRegistryAlreadyRegistered}, `{"revocations": [`+revocationItem+`]}`)
	if replay.Code != http.StatusOK || replay.answer["outcome"] != "ALREADY_REGISTERED" {
		t.Fatalf("replay = %d %v", replay.Code, replay.answer)
	}
	missing := serve(endingRegistryStub{}, `{"revocations": [`+revocationItem+`]}`)
	if missing.Code != http.StatusOK || missing.answer["outcome"] != "NOT_FOUND" {
		t.Fatalf("missing = %d %v", missing.Code, missing.answer)
	}
	future := serve(endingRegistryStub{relationship: live, found: true, saved: ports.PartyRegistrySaved},
		`{"revocations": [`+strings.Replace(revocationItem, "2026-07-01", "2026-12-01", 1)+`]}`)
	if future.Code != http.StatusOK || future.answer["outcome"] != "NOT_ACCEPTED" || future.answer["cause"] == nil {
		t.Fatalf("future = %d %v", future.Code, future.answer)
	}
}
