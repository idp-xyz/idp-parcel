package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// acceptedParcelsDouble 是 parcel-shipment 当前已接受委托的替身：包裹到客户归属与来源引用。
type acceptedParcelsDouble struct {
	sources map[string]ports.AcceptedParcelSource
	err     error
	asked   []string
}

func (double *acceptedParcelsDouble) FindAcceptedParcelSource(
	_ context.Context,
	_ domain.TenantID,
	parcel string,
) (ports.AcceptedParcelSource, bool, error) {
	double.asked = append(double.asked, parcel)
	if double.err != nil {
		return ports.AcceptedParcelSource{}, false, double.err
	}
	source, found := double.sources[parcel]
	return source, found, nil
}

func newParcelCaseFixture(t *testing.T) (*application.EstablishCaseForParcelsHandler, *acceptedParcelsDouble, *caseFixture) {
	t.Helper()
	cases := newCaseFixture(t)
	parcels := &acceptedParcelsDouble{sources: map[string]ports.AcceptedParcelSource{
		"parcel-1": {Customer: "customer-1", SourceRef: "shipment-request-1/submission-1"},
		"parcel-2": {Customer: "customer-2", SourceRef: "shipment-request-2/submission-1"},
	}}
	return application.NewEstablishCaseForParcelsHandler(application.EstablishCaseForParcelsDeps{
		Parcels: parcels,
		Cases:   cases.handler,
	}), parcels, cases
}

func parcelCaseCommand(t *testing.T, parcels ...string) application.EstablishCaseForParcelsCommand {
	t.Helper()
	return application.EstablishCaseForParcelsCommand{
		TenantID:     mustValue(t, domain.NewTenantID, "tenant-1"),
		Jurisdiction: mustValue(t, domain.NewRegulatoryJurisdictionReference, "CN"),
		Direction:    domain.ExportManifest,
		Procedure:    mustValue(t, domain.NewCustomsProcedureReference, "CN-EXPORT/9610"),
		Obligation:   mustValue(t, domain.NewObligationScopeReference, "EXPORT-DECLARATION"),
		Parcels:      parcels,
	}
}

// Covers: AT-CC-437（编排半边）——命令面只指名包裹，客户归属与来源资料引用取自 parcel-shipment 的当前已接受委托，逐件原样
// 进案件关联；要不要建、幂等与范围冲突照旧由建案编排答（重放答已有案件）。
func TestACaseForNamedParcelsTakesCustomerAndSourceFromTheAcceptedRequests(t *testing.T) {
	handler, _, cases := newParcelCaseFixture(t)

	result, err := handler.Handle(context.Background(), parcelCaseCommand(t, "parcel-1", "parcel-2"))
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	customsCase, present := result.Case()
	if result.Outcome() != application.CaseEstablished || !present {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	got := map[string]domain.CaseParcelAssociation{}
	for _, association := range customsCase.Parcels() {
		got[association.Parcel] = association
	}
	if got["parcel-1"].Customer != "customer-1" || got["parcel-1"].SourceRef != "shipment-request-1/submission-1" ||
		got["parcel-2"].Customer != "customer-2" || got["parcel-2"].SourceRef != "shipment-request-2/submission-1" {
		t.Fatalf("parcels = %+v", customsCase.Parcels())
	}

	replay, err := handler.Handle(context.Background(), parcelCaseCommand(t, "parcel-2", "parcel-1"))
	if err != nil || replay.Outcome() != application.CaseExisting {
		t.Fatalf("replay = %s, %v", replay.Outcome(), err)
	}
	if cases.store.saved != 1 {
		t.Fatalf("saved = %d", cases.store.saved)
	}
}

// Covers: AT-CC-438——包裹不在任何当前已接受委托里答包裹未接受并逐件指名（不在第一件处停下），建案要求规则未登记答未决并
// 指名未配置；两格都不建占位案件。源上下文读不动答未决并指名读不动。点名 AT-CC-020「没有可验证接受决定和接受基线→请求
// 未受理或未形成案件」的命令面半边。
func TestACaseIsNotEstablishedForUnacceptedParcelsOrAnUnconfiguredRequirement(t *testing.T) {
	handler, parcels, cases := newParcelCaseFixture(t)

	unaccepted, err := handler.Handle(context.Background(), parcelCaseCommand(t, "parcel-9", "parcel-1", "parcel-8"))
	if err != nil {
		t.Fatalf("unaccepted: %v", err)
	}
	if unaccepted.Outcome() != application.CaseParcelNotAccepted || unaccepted.Outcome().String() != "PARCEL_NOT_ACCEPTED" {
		t.Fatalf("outcome = %s", unaccepted.Outcome())
	}
	if missing := unaccepted.UnacceptedParcels(); len(missing) != 2 || missing[0] != "parcel-9" || missing[1] != "parcel-8" {
		t.Fatalf("unaccepted parcels = %v", missing)
	}

	cases.requirement.configured = false
	unconfigured, err := handler.Handle(context.Background(), parcelCaseCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("unconfigured: %v", err)
	}
	if unconfigured.Outcome() != application.EstablishCaseUndecided || unconfigured.UndecidedReason().String() != "CASE_REQUIREMENT_UNCONFIGURED" {
		t.Fatalf("outcome = %s reason = %s", unconfigured.Outcome(), unconfigured.UndecidedReason())
	}

	cases.requirement.configured = true
	parcels.err = errors.New("down")
	unavailable, err := handler.Handle(context.Background(), parcelCaseCommand(t, "parcel-1"))
	if err != nil || unavailable.Outcome() != application.EstablishCaseUndecided ||
		unavailable.UndecidedReason().String() != "ACCEPTED_PARCEL_SOURCE_UNAVAILABLE" {
		t.Fatalf("outcome = %s reason = %s err = %v", unavailable.Outcome(), unavailable.UndecidedReason(), err)
	}
	if cases.store.saved != 0 || cases.identity.minted != 0 {
		t.Fatalf("saved = %d minted = %d；前提不足却建了案", cases.store.saved, cases.identity.minted)
	}

	blank, err := handler.Handle(context.Background(), parcelCaseCommand(t, " "))
	if err != nil || blank.Outcome() != application.EstablishCaseNotAccepted {
		t.Fatalf("blank = %s, %v", blank.Outcome(), err)
	}
}
