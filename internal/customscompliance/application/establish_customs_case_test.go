package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var caseAt = time.Date(2026, 8, 13, 21, 0, 0, 0, time.UTC)

type caseRequirementViewDouble struct {
	judgment   ports.CaseRequirementJudgment
	configured bool
}

func (double *caseRequirementViewDouble) JudgeCaseRequirement(
	_ context.Context,
	_ domain.TenantID,
	_ domain.RegulatoryJurisdictionReference,
	_ domain.ManifestDirection,
	_ domain.CustomsProcedureReference,
) (ports.CaseRequirementJudgment, bool, error) {
	return double.judgment, double.configured, nil
}

type customsCaseStoreDouble struct {
	byKey map[ports.CustomsCaseKey]domain.CustomsCase
	saved int
}

func (double *customsCaseStoreDouble) FindByKey(
	_ context.Context,
	key ports.CustomsCaseKey,
) (domain.CustomsCase, bool, error) {
	customsCase, found := double.byKey[key]
	return customsCase, found, nil
}

func (double *customsCaseStoreDouble) Save(
	_ context.Context,
	key ports.CustomsCaseKey,
	customsCase domain.CustomsCase,
) (ports.CustomsCaseSaveOutcome, error) {
	if _, exists := double.byKey[key]; exists {
		return ports.CustomsCaseAlreadyRecorded, nil
	}
	double.byKey[key] = customsCase
	double.saved++
	return ports.CustomsCaseSaved, nil
}

type caseIdentityFactoryDouble struct{ minted int }

func (double *caseIdentityFactoryDouble) MintCaseID(_ context.Context) (domain.CustomsCaseID, error) {
	double.minted++
	return domain.NewCustomsCaseID("customs-case-" + string(rune('0'+double.minted)))
}

type caseDownstreamDouble struct {
	intents []ports.CustomsCaseHandoffIntent
}

func (double *caseDownstreamDouble) HandOffCase(
	_ context.Context,
	intent ports.CustomsCaseHandoffIntent,
) error {
	double.intents = append(double.intents, intent)
	return nil
}

type caseFixture struct {
	handler     *application.EstablishCaseHandler
	requirement *caseRequirementViewDouble
	store       *customsCaseStoreDouble
	identity    *caseIdentityFactoryDouble
}

func newCaseFixture(t *testing.T) *caseFixture {
	t.Helper()
	fixture := &caseFixture{
		requirement: &caseRequirementViewDouble{
			judgment:   ports.CaseRequirementJudgment{Required: true, Basis: "PRODUCT-RULE/import-obligation"},
			configured: true,
		},
		store:    &customsCaseStoreDouble{byKey: map[ports.CustomsCaseKey]domain.CustomsCase{}},
		identity: &caseIdentityFactoryDouble{},
	}
	fixture.handler = application.NewEstablishCaseHandler(application.EstablishCaseDeps{
		Requirement: fixture.requirement,
		Store:       fixture.store,
		Identity:    fixture.identity,
		Downstream:  &caseDownstreamDouble{},
		Clock:       fixedClock{at: caseAt},
	})
	return fixture
}

func caseCommand(t *testing.T, parcels ...string) application.EstablishCaseCommand {
	t.Helper()
	associations := make([]domain.CaseParcelAssociation, 0, len(parcels))
	for _, parcel := range parcels {
		associations = append(associations, domain.CaseParcelAssociation{
			Parcel:    parcel,
			Customer:  "customer-1",
			SourceRef: "PS-SOURCE/" + parcel,
		})
	}
	return application.EstablishCaseCommand{
		TenantID:     mustValue(t, domain.NewTenantID, "tenant-1"),
		Jurisdiction: mustValue(t, domain.NewRegulatoryJurisdictionReference, "US"),
		Direction:    domain.ImportManifest,
		Procedure:    mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		Obligation:   mustValue(t, domain.NewObligationScopeReference, "IMPORT-DECLARATION"),
		Parcels:      associations,
		Roles: []domain.CaseRoleSnapshot{{
			Role:      "IMPORTER_OF_RECORD",
			Party:     "party-1",
			Authority: "PC-AUTHORITY/mandate-7",
		}},
		RequestedAt: caseAt.Add(-time.Hour),
	}
}

// Covers: UC-CC-001「同一委托、同袋、同总单、同舱单、同班次或同一路由都不能自动证明
// 应建立同一个关务案件」与四维身份——同键同包裹集重放返原案件不重立（一案一次），
// 同键异包裹集是范围冲突不顶替（扩大范围走案件自己的变更，不经建案入口吸收）；换
// 监管程序自然换键立独立案件（一包裹可关联多个彼此独立的案件）。
func TestOneCasePerRegulatoryScopeAndNoSilentScopeGrowth(t *testing.T) {
	fixture := newCaseFixture(t)

	established, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1", "parcel-2"))
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	if established.Outcome() != application.CaseEstablished {
		t.Fatalf("outcome = %q", established.Outcome())
	}

	replay, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-2", "parcel-1"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.CaseExisting {
		t.Fatalf("replay = %q; 同包裹集换序也是重放", replay.Outcome())
	}
	if fixture.store.saved != 1 || fixture.identity.minted != 1 {
		t.Fatalf("saved = %d minted = %d", fixture.store.saved, fixture.identity.minted)
	}

	grown, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1", "parcel-2", "parcel-3"))
	if err != nil {
		t.Fatalf("grown: %v", err)
	}
	if grown.Outcome() != application.CaseScopeConflict {
		t.Fatalf("grown = %q; 建案入口吸收了范围扩大", grown.Outcome())
	}

	exportCommand := caseCommand(t, "parcel-1")
	exportCommand.Procedure = mustValue(t, domain.NewCustomsProcedureReference, "CN-EXPORT/9610")
	exportCommand.Direction = domain.ExportManifest
	independent, err := fixture.handler.Handle(context.Background(), exportCommand)
	if err != nil {
		t.Fatalf("independent: %v", err)
	}
	if independent.Outcome() != application.CaseEstablished {
		t.Fatalf("independent = %q; 另一监管程序是独立案件", independent.Outcome())
	}
	if fixture.store.saved != 2 {
		t.Fatalf("saved = %d", fixture.store.saved)
	}
}

// Covers: UC-CC-001 裁决分格——建案规则未登记未决（不是「不要求」）；明确不要求是
// 不适用格带判断依据（服务责任不含此范围的关务履责）；空包裹集未受理（领域把门）。
func TestCaseRequirementGapsAndRefusalsStayDistinct(t *testing.T) {
	fixture := newCaseFixture(t)
	fixture.requirement.configured = false

	unconfigured, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("unconfigured: %v", err)
	}
	if unconfigured.Outcome() != application.EstablishCaseUndecided {
		t.Fatalf("outcome = %q; 规则未登记不是不要求", unconfigured.Outcome())
	}

	fixture.requirement.configured = true
	fixture.requirement.judgment = ports.CaseRequirementJudgment{
		Required: false,
		Basis:    "PRODUCT-RULE/domestic-only",
	}
	notRequired, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("not required: %v", err)
	}
	if notRequired.Outcome() != application.CaseNotRequired ||
		notRequired.Basis() != "PRODUCT-RULE/domestic-only" {
		t.Fatalf("outcome = %q basis = %q", notRequired.Outcome(), notRequired.Basis())
	}
	if fixture.store.saved != 0 {
		t.Fatal("不适用还立了案")
	}

	fixture.requirement.judgment = ports.CaseRequirementJudgment{Required: true, Basis: "PRODUCT-RULE/import"}
	empty, err := fixture.handler.Handle(context.Background(), caseCommand(t))
	if err != nil {
		t.Fatalf("empty parcels: %v", err)
	}
	if empty.Outcome() != application.EstablishCaseNotAccepted {
		t.Fatalf("outcome = %q; 没有包裹的案件被收下了", empty.Outcome())
	}
}
