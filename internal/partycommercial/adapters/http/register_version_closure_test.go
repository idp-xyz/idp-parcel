package commercialhttp_test

import (
	"context"
	"encoding/json"
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

// 本文件证按后继收尾旧版的两个在线口（ADR-0233）：各收受控批文自己那一节恰一项、租户取认证结果；退役项装不下退役时刻；
// 没有登记册配置写授予在读载荷之前就拒、编排一次也不被调到；答复回显落下的那一次收尾。

const (
	cutItem        = `{"kind": "CUSTOMER_CONTRACT", "scope": "SYN-SCOPE", "objectId": "SYN-CONTRACT-01", "version": "v1", "successorVersion": "v2", "reference": "SYN-CUT-01", "correctedAt": "2026-10-20T00:00:00Z"}`
	retirementItem = `{"kind": "SUPPLIER_AGREEMENT", "scope": "SYN-SCOPE", "objectId": "SYN-AGREEMENT-01", "version": "v1", "successorVersion": "v2", "reference": "SYN-REISSUE-01"}`
)

// Covers: `AT-PC-057` 在线口那一半的形状门——两口各收自己那一节恰一项、租户取认证结果；自报租户、零项、别一节的项、
// 退役项带时刻、集外类别都答请求畸形；不带令牌在解载荷之前答凭证被拒。
func TestVersionClosureIntakesTakeTheirOwnLineWithTheAuthenticatedTenant(t *testing.T) {
	intake := operatorIntake(t)
	cut, err := intake.IntakeSuccessorCut(context.Background(), operatorRequest("t", `{"cuts": [`+cutItem+`]}`))
	if err != nil {
		t.Fatalf("cut: %v", err)
	}
	if cut.Tenant.String() != operatorAuthenticatedTenant || cut.Kind != domain.CustomerContractObject ||
		cut.Successor.String() != "v2" || !cut.CorrectedAt.Equal(time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("cut command = %+v", cut)
	}
	retire, err := intake.IntakeSameStartRetirement(context.Background(), operatorRequest("t", `{"retirements": [`+retirementItem+`]}`))
	if err != nil {
		t.Fatalf("retirement: %v", err)
	}
	if retire.Tenant.String() != operatorAuthenticatedTenant || retire.Kind != domain.SupplierAgreementObject || retire.Reference.String() != "SYN-REISSUE-01" {
		t.Fatalf("retirement command = %+v", retire)
	}

	for name, probe := range map[string]struct {
		body    string
		intakes func(*http.Request) error
	}{
		"自报租户":     {`{"tenantId": "SYN-TENANT-01", "cuts": [` + cutItem + `]}`, cutIntake(intake)},
		"零项":       {`{"cuts": []}`, cutIntake(intake)},
		"别一节的项":    {`{"cuts": [` + cutItem + `], "retirements": [` + retirementItem + `]}`, cutIntake(intake)},
		"截止项投到退役口": {`{"cuts": [` + cutItem + `]}`, retirementIntake(intake)},
		"退役项带时刻":   {`{"retirements": [` + strings.Replace(retirementItem, `"reference"`, `"retiredAt": "2026-10-20T00:00:00Z", "reference"`, 1) + `]}`, retirementIntake(intake)},
		"集外类别":     {`{"cuts": [` + strings.Replace(cutItem, "CUSTOMER_CONTRACT", "NOT_A_KIND", 1) + `]}`, cutIntake(intake)},
	} {
		if err := probe.intakes(operatorRequest("t", probe.body)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
	if _, err := intake.IntakeSameStartRetirement(context.Background(), operatorRequest("", `not json`)); !errors.Is(err, commercialhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v, want ErrOperatorCredentialRejected", err)
	}
}

// Covers: `AT-PC-057` 越权那一格——令牌有效却没有登记册配置写授予，两口都答 403 OPERATOR_NOT_GRANTED，载荷一个字节不读，
// 编排一次也不被调到。
func TestVersionClosureEndpointsRefuseAnOperatorWithoutTheRegistryGrant(t *testing.T) {
	intake := publicationOperatorIntake(t, envelopeOperators{err: commercialhttp.ErrOperatorNotGranted})
	registrar := &versionClosureRegistrarDouble{}
	for target, endpoint := range map[string]http.Handler{
		"/commercial-version-successor-cuts": commercialhttp.NewCutCommercialVersionAtSuccessorEndpoint(intake, registrar),
		"/commercial-version-retirements":    commercialhttp.NewRetireCommercialVersionAtSameStartEndpoint(intake, registrar),
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
	if registrar.calls != 0 {
		t.Fatal("收尾编排被调到了")
	}
}

// Covers: 两口的答复代数——`已登记` 201 并回显截止后的选用区间或后继与退役时刻；`未受理` 200 带原因、不回显区间。编排是
// 真用例，册是内存里的一份。
func TestVersionClosureEndpointsEchoTheClosure(t *testing.T) {
	handler := application.NewCloseCommercialVersionHandler(closureRegistryDouble{registry: closureRegistry(t)}, closureClock{})
	cutEndpoint := commercialhttp.NewCutCommercialVersionAtSuccessorEndpoint(operatorIntake(t), handler)
	retireEndpoint := commercialhttp.NewRetireCommercialVersionAtSameStartEndpoint(operatorIntake(t), handler)

	cut := serveClosure(t, cutEndpoint, `{"cuts": [`+cutItem+`]}`)
	if cut.Code != http.StatusCreated || cut.answer["outcome"] != "REGISTERED" ||
		cut.answer["correctedStartsAt"] != "2026-01-01T00:00:00Z" || cut.answer["correctedEndsAt"] != "2026-04-01T00:00:00Z" {
		t.Fatalf("cut answer = %d %v", cut.Code, cut.answer)
	}

	retired := serveClosure(t, retireEndpoint, `{"retirements": [`+retirementItem+`]}`)
	if retired.Code != http.StatusCreated || retired.answer["successorVersion"] != "v2" || retired.answer["retiredAt"] != "2026-05-01T00:00:00Z" {
		t.Fatalf("retirement answer = %d %v", retired.Code, retired.answer)
	}

	// 倒着截：把较晚的 v2 截在较早的 v1 起点上，截止点不在 v2 的区间之内。
	backwards := `{"kind": "CUSTOMER_CONTRACT", "scope": "SYN-SCOPE", "objectId": "SYN-CONTRACT-01", "version": "v2", "successorVersion": "v1", "reference": "SYN-CUT-02", "correctedAt": "2026-10-20T00:00:00Z"}`
	refused := serveClosure(t, cutEndpoint, `{"cuts": [`+backwards+`]}`)
	if refused.Code != http.StatusOK || refused.answer["outcome"] != "NOT_ACCEPTED" || refused.answer["cause"] == nil || refused.answer["correctedEndsAt"] != nil {
		t.Fatalf("refused answer = %d %v", refused.Code, refused.answer)
	}
}

type servedAnswer struct {
	Code   int
	answer map[string]any
}

func serveClosure(t *testing.T, endpoint http.Handler, body string) servedAnswer {
	t.Helper()
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, operatorRequest("t", body))
	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复不是 JSON：%s", recorder.Body)
	}
	return servedAnswer{Code: recorder.Code, answer: answer}
}

func cutIntake(intake *commercialhttp.OperatorRegistryIntake) func(*http.Request) error {
	return func(request *http.Request) error {
		_, err := intake.IntakeSuccessorCut(context.Background(), request)
		return err
	}
}

func retirementIntake(intake *commercialhttp.OperatorRegistryIntake) func(*http.Request) error {
	return func(request *http.Request) error {
		_, err := intake.IntakeSameStartRetirement(context.Background(), request)
		return err
	}
}

// versionClosureRegistrarDouble 只数被调了几次：越权那一格要证的是编排一次也不被调到。
type versionClosureRegistrarDouble struct{ calls int }

func (double *versionClosureRegistrarDouble) Cut(context.Context, application.CutCommercialVersionAtSuccessorCommand) (application.VersionClosureResult, error) {
	double.calls++
	return application.VersionClosureResult{}, errors.New("not reached")
}

func (double *versionClosureRegistrarDouble) RetireAtSameStart(context.Context, application.RetireCommercialVersionAtSameStartCommand) (application.VersionClosureResult, error) {
	double.calls++
	return application.VersionClosureResult{}, errors.New("not reached")
}

// closureRegistryDouble 是内存里的一份册：锁恒得、写恒落，判都在领域里。
type closureRegistryDouble struct{ registry *domain.CommercialRegistry }

func (double closureRegistryDouble) LoadForScope(context.Context, domain.TenantID, domain.CommercialScopeReference) (*domain.CommercialRegistry, error) {
	return double.registry, nil
}

func (double closureRegistryDouble) LockVersionForClosure(context.Context, domain.TenantID, domain.CommercialObjectKind, domain.CommercialObjectID, domain.CommercialVersionLabel) (bool, error) {
	return true, nil
}

func (double closureRegistryDouble) SaveValidityCorrection(context.Context, domain.ValidityCorrection) (ports.ValidityCorrectionSaveOutcome, error) {
	return ports.ValidityCorrectionSaved, nil
}

func (double closureRegistryDouble) SaveVersionRetirement(context.Context, domain.VersionRetirement) (ports.VersionRetirementSaveOutcome, error) {
	return ports.VersionRetirementSaved, nil
}

type closureClock struct{}

func (closureClock) Now() time.Time { return time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC) }

// closureRegistry 登两对：合同 v2 晚于 v1 起点（走截止），协议 v2 与 v1 同起点（走退役）。
func closureRegistry(t *testing.T) *domain.CommercialRegistry {
	t.Helper()
	registry := domain.NewCommercialRegistry()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	for _, version := range []domain.CommercialVersion{
		closureVersion(t, domain.CustomerContractObject, "SYN-CONTRACT-01", "v1", start),
		closureVersion(t, domain.CustomerContractObject, "SYN-CONTRACT-01", "v2", later),
		closureVersion(t, domain.SupplierAgreementObject, "SYN-AGREEMENT-01", "v1", start),
		closureVersion(t, domain.SupplierAgreementObject, "SYN-AGREEMENT-01", "v2", start),
	} {
		if _, err := registry.Register(version); err != nil {
			t.Fatal(err)
		}
	}
	return registry
}

func closureVersion(t *testing.T, kind domain.CommercialObjectKind, objectID, label string, startsAt time.Time) domain.CommercialVersion {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	tenant, err := domain.NewTenantID(operatorAuthenticatedTenant)
	must(err)
	object, err := domain.NewCommercialObjectID(objectID)
	must(err)
	version, err := domain.NewCommercialVersionLabel(label)
	must(err)
	scope, err := domain.NewCommercialScopeReference("SYN-SCOPE")
	must(err)
	digest, err := domain.NewCommercialContentDigest("sha256:" + objectID + "-" + label)
	must(err)
	interval, err := domain.NewEffectiveInterval(startsAt, time.Time{})
	must(err)
	reference, err := domain.NewApprovalReference("SYN-APPROVAL-" + label)
	must(err)
	source, err := domain.NewCommercialSourceReference("SYN-SOURCE")
	must(err)
	approval, err := domain.NewApprovalBasis(reference, source, startsAt)
	must(err)
	built, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID: tenant, Kind: kind, ObjectID: object, Version: version, Scope: scope, ContentDigest: digest,
		Effective: interval, Status: domain.CommercialVersionEffective, Approval: approval,
		PublishedAt: startsAt, EffectiveAt: startsAt,
	})
	must(err)
	return built
}
