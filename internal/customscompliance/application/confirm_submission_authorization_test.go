package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var confirmationAt = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

// authorityRegisterDouble 是提交授权册的内存替身：一单元一行，同键第二份答已登记、不覆盖（写入代数同 ADR-0031）。
type authorityRegisterDouble struct {
	rows    map[string]domain.SubmissionAuthorization
	saveErr error
	loadErr error
	saved   int
}

func newAuthorityRegisterDouble() *authorityRegisterDouble {
	return &authorityRegisterDouble{rows: map[string]domain.SubmissionAuthorization{}}
}

func (double *authorityRegisterDouble) GrantSubmissionAuthority(
	_ context.Context,
	tenant domain.TenantID,
	authorization domain.SubmissionAuthorization,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.saveErr
	}
	key := tenant.String() + "/" + authorization.Unit().String()
	if _, exists := double.rows[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	double.rows[key] = authorization
	double.saved++
	return ports.CaseConfigurationRegistered, nil
}

func (double *authorityRegisterDouble) RevokeSubmissionAuthority(
	_ context.Context,
	tenant domain.TenantID,
	authorization domain.SubmissionAuthorization,
) error {
	double.rows[tenant.String()+"/"+authorization.Unit().String()] = authorization
	return nil
}

func (double *authorityRegisterDouble) LoadSubmissionAuthority(
	_ context.Context,
	tenant domain.TenantID,
	unit domain.DeclarationUnitID,
) (domain.SubmissionAuthorization, bool, error) {
	if double.loadErr != nil {
		return domain.SubmissionAuthorization{}, false, double.loadErr
	}
	authorization, found := double.rows[tenant.String()+"/"+unit.String()]
	return authorization, found, nil
}

type confirmationFixture struct {
	handler   *application.ConfirmSubmissionAuthorizationHandler
	readiness *readinessViewDouble
	register  *authorityRegisterDouble
}

func newConfirmationFixture() *confirmationFixture {
	fixture := &confirmationFixture{
		readiness: &readinessViewDouble{configured: true},
		register:  newAuthorityRegisterDouble(),
	}
	fixture.handler = application.NewConfirmSubmissionAuthorizationHandler(application.ConfirmSubmissionAuthorizationDeps{
		Readiness:     fixture.readiness,
		Authorities:   fixture.register,
		AuthorityView: fixture.register,
		Clock:         fixedClock{at: confirmationAt},
	})
	return fixture
}

func confirmationCommand(t *testing.T, confirmer string) application.ConfirmSubmissionAuthorizationCommand {
	t.Helper()
	return application.ConfirmSubmissionAuthorizationCommand{
		TenantID:  mustValue(t, domain.NewTenantID, "tenant-1"),
		UnitID:    "unit-1",
		Confirmer: confirmer,
		Grant:     "CUSTOMS_SUBMISSION_AUTHORIZATION",
	}
}

// Covers: AT-CC-440——单元就绪当前有效时，确认形成提交授权，授权依据写成「所凭授予@确认人」、授予时刻取编排时钟；AT-CC-078
// 的人工确认半边（「不创建提交版本或尝试」由本编排没有那两本册的依赖在结构上承载）。
func TestAConfirmationFormsTheAuthorizationNamingTheConfirmer(t *testing.T) {
	fixture := newConfirmationFixture()

	result, err := fixture.handler.Handle(context.Background(), confirmationCommand(t, "https://issuer#alice"))
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if result.Outcome() != application.SubmissionAuthorizationConfirmed {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	authorization, present := result.Authorization()
	if !present || authorization.Authority().String() != "CUSTOMS_SUBMISSION_AUTHORIZATION@https://issuer#alice" ||
		!authorization.GrantedAt().Equal(confirmationAt) || !authorization.Effective() {
		t.Fatalf("authorization = %+v", authorization)
	}
	if fixture.register.saved != 1 {
		t.Fatalf("saved = %d", fixture.register.saved)
	}
}

// Covers: AT-CC-442——单元已有授权时不另形成：同一确认的重放与另一位确认人的第二次确认都答已有授权，交回原确认人；原授权
// 已失效答授权不再有效，不重新形成、不恢复。
func TestARecordedAuthorizationIsNeitherReformedNorDisplaced(t *testing.T) {
	fixture := newConfirmationFixture()
	if _, err := fixture.handler.Handle(context.Background(), confirmationCommand(t, "https://issuer#alice")); err != nil {
		t.Fatalf("first: %v", err)
	}

	for _, confirmer := range []string{"https://issuer#alice", "https://issuer#bob"} {
		replay, err := fixture.handler.Handle(context.Background(), confirmationCommand(t, confirmer))
		if err != nil {
			t.Fatalf("%s: %v", confirmer, err)
		}
		authorization, present := replay.Authorization()
		if replay.Outcome() != application.SubmissionAuthorizationExisting || !present ||
			authorization.Authority().String() != "CUSTOMS_SUBMISSION_AUTHORIZATION@https://issuer#alice" {
			t.Fatalf("%s: outcome = %s authority = %s", confirmer, replay.Outcome(), authorization.Authority())
		}
	}
	if fixture.register.saved != 1 {
		t.Fatalf("saved = %d；已有授权被第二份顶替或叠加", fixture.register.saved)
	}

	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	unit := mustValue(t, domain.NewDeclarationUnitID, "unit-1")
	recorded, _, _ := fixture.register.LoadSubmissionAuthority(context.Background(), tenant, unit)
	revoked, err := recorded.Revoke("MANDATE_WITHDRAWN", confirmationAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.register.RevokeSubmissionAuthority(context.Background(), tenant, revoked); err != nil {
		t.Fatal(err)
	}
	again, err := fixture.handler.Handle(context.Background(), confirmationCommand(t, "https://issuer#bob"))
	if err != nil {
		t.Fatalf("after revoke: %v", err)
	}
	if again.Outcome() != application.SubmissionAuthorizationNoLongerEffective {
		t.Fatalf("outcome = %s；失效授权被重新确认成了有效", again.Outcome())
	}
}

// Covers: AT-CC-444——就绪没登答未决并指名就绪未配置，就绪已失效答不再就绪，读不动答未决并指名读不动；三格都不形成授权。
// 「就绪没登」不折成请求未受理，与提交执行口同一口径（ADR-0227 决定八）。
func TestAConfirmationWithoutCurrentReadinessFormsNoAuthorization(t *testing.T) {
	for _, test := range []struct {
		name      string
		readiness readinessViewDouble
		outcome   application.AuthorizationConfirmationOutcome
		reason    string
	}{
		{name: "未登记", readiness: readinessViewDouble{configured: false}, outcome: application.AuthorizationConfirmationUndecided, reason: "READINESS_UNCONFIGURED"},
		{name: "已失效", readiness: readinessViewDouble{configured: true, revoked: true}, outcome: application.AuthorizationReadinessNotEffective},
		{name: "读不动", readiness: readinessViewDouble{err: errors.New("down")}, outcome: application.AuthorizationConfirmationUndecided, reason: "READINESS_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newConfirmationFixture()
			*fixture.readiness = test.readiness
			result, err := fixture.handler.Handle(context.Background(), confirmationCommand(t, "https://issuer#alice"))
			if err != nil {
				t.Fatalf("confirm: %v", err)
			}
			if result.Outcome() != test.outcome || result.UndecidedReason().String() != test.reason {
				t.Fatalf("outcome = %s reason = %q", result.Outcome(), result.UndecidedReason())
			}
			if fixture.register.saved != 0 {
				t.Fatal("没有当前就绪也形成了授权")
			}
		})
	}
}

// Covers: 确认人与所凭授予缺一即未受理（确认人只从接入渠道来，空着就是渠道没给）；授权册读写不动答未决，不当成「没有授权」
// 去形成第二份。
func TestAConfirmationWithoutAConfirmerOrAReadableRegisterFormsNothing(t *testing.T) {
	fixture := newConfirmationFixture()
	for _, command := range []application.ConfirmSubmissionAuthorizationCommand{
		confirmationCommand(t, ""),
		{TenantID: mustValue(t, domain.NewTenantID, "tenant-1"), UnitID: "unit-1", Confirmer: "https://issuer#alice"},
		{TenantID: mustValue(t, domain.NewTenantID, "tenant-1"), Confirmer: "https://issuer#alice", Grant: "CUSTOMS_SUBMISSION_AUTHORIZATION"},
	} {
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil || result.Outcome() != application.AuthorizationConfirmationNotAccepted {
			t.Fatalf("command %+v: outcome = %s err = %v", command, result.Outcome(), err)
		}
	}

	fixture.register.loadErr = errors.New("down")
	unreadable, err := fixture.handler.Handle(context.Background(), confirmationCommand(t, "https://issuer#alice"))
	if err != nil || unreadable.Outcome() != application.AuthorizationConfirmationUndecided ||
		unreadable.UndecidedReason().String() != "AUTHORITY_STORE_UNAVAILABLE" {
		t.Fatalf("outcome = %s reason = %s err = %v", unreadable.Outcome(), unreadable.UndecidedReason(), err)
	}
	fixture.register.loadErr = nil
	fixture.register.saveErr = errors.New("down")
	unwritable, err := fixture.handler.Handle(context.Background(), confirmationCommand(t, "https://issuer#alice"))
	if err != nil || unwritable.Outcome() != application.AuthorizationConfirmationUndecided {
		t.Fatalf("outcome = %s err = %v", unwritable.Outcome(), err)
	}
	if fixture.register.saved != 0 {
		t.Fatal("册读写不动时形成了授权")
	}
}
