package nodeoperations_test

import (
	"context"
	"errors"
	"testing"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

type receptionDouble struct {
	record noports.ReceptionRecord
	found  bool
	err    error
	last   noports.ReceptionKey
}

func (double *receptionDouble) FindByKey(
	_ context.Context, key noports.ReceptionKey,
) (noports.ReceptionRecord, bool, error) {
	double.last = key
	if double.err != nil {
		return noports.ReceptionRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

type targetViewDouble struct {
	target psdomain.CurrentAcceptedParcelTarget
	found  bool
	err    error
	tenant string
	parcel string
}

func (double *targetViewDouble) FindCurrentAcceptedByParcel(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	double.tenant = tenant.String()
	double.parcel = parcel.String()
	if double.err != nil {
		return psdomain.CurrentAcceptedParcelTarget{}, false, double.err
	}
	return double.target, double.found, nil
}

type adopterDouble struct {
	calls int
}

func (double *adopterDouble) AdoptFromNodeIntake(
	context.Context, nodomain.NodeIntake, adapter.TargetShipment,
) (psapplication.AdoptNetworkIntakeResult, error) {
	double.calls++
	return psapplication.AdoptNetworkIntakeResult{}, errors.New("adopter should not be called")
}

type recordingHandler struct {
	inner   adapter.NetworkIntakeCommandHandler
	command psapplication.AdoptNetworkIntakeCommand
}

func (recorder *recordingHandler) Handle(
	ctx context.Context, command psapplication.AdoptNetworkIntakeCommand,
) (psapplication.AdoptNetworkIntakeResult, error) {
	recorder.command = command
	return recorder.inner.Handle(ctx, command)
}

func uniqueTarget(t *testing.T) psdomain.CurrentAcceptedParcelTarget {
	t.Helper()
	target, err := psdomain.NewCurrentAcceptedParcelTarget(
		identity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		value(t, psdomain.NewSubmissionVersionID, "version-1"),
	)
	if err != nil {
		t.Fatalf("当前已接受目标：%v", err)
	}
	return target
}

func formedReception(t *testing.T) noports.ReceptionRecord {
	t.Helper()
	return noports.ReceptionRecord{
		Key: noports.ReceptionKey{
			TenantID: value(t, nodomain.NewTenantID, "tenant-1"),
			SourceID: "source-1",
		},
		Kind:       noports.RecordIntakeFormed,
		Intake:     identifiedIntake(t),
		RecordedAt: receivedAt,
	}
}

func formedRef() psinbox.FormedNodeIntake {
	return psinbox.FormedNodeIntake{TenantID: "tenant-1", SourceID: "source-1"}
}

func TestFormedIntakeLooksUpTheUniqueAcceptedTargetAndAdopts(t *testing.T) {
	receptions := &receptionDouble{record: formedReception(t), found: true}
	targets := &targetViewDouble{target: uniqueTarget(t), found: true}
	recorder := &recordingHandler{inner: adoptHandler(t)}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		receptions, targets, adapter.NewNodeIntakeAdapter(recorder))
	if err != nil {
		t.Fatalf("构造处理适配器：%v", err)
	}

	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); err != nil {
		t.Fatalf("处理形成收寄：%v", err)
	}

	if receptions.last.TenantID.String() != "tenant-1" || receptions.last.SourceID != "source-1" {
		t.Fatalf("收寄查询键 = %+v", receptions.last)
	}
	if targets.tenant != "tenant-1" || targets.parcel != "parcel-1" {
		t.Fatalf("反查租户/包裹 = %q / %q", targets.tenant, targets.parcel)
	}

	command := recorder.command
	if command.Identity != identity(t) ||
		command.ShipmentRequestID.String() != "request-1" ||
		command.SubmissionVersion.String() != "version-1" {
		t.Fatalf("目标指名 = %+v", command)
	}
	source := command.Source
	if source.Kind != psdomain.NodeIntakeSource ||
		source.Parcel.String() != "parcel-1" ||
		source.Object.String() != "unit-1" ||
		source.Place.String() != "node-origin" ||
		source.Control.String() != "NODE-INTAKE/SIGN-7" ||
		source.Version.String() != "intake-result/v1" ||
		!source.OccurredAt.Equal(receivedAt) {
		t.Fatalf("采用命令来源 = %+v", source)
	}
}

func TestAMissingReceptionIsContinuableUndecided(t *testing.T) {
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{}, &targetViewDouble{found: true, target: uniqueTarget(t)}, adopter)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrReceptionNotVisible) {
		t.Fatalf("err = %v, want ErrReceptionNotVisible", err)
	}
	if adopter.calls != 0 {
		t.Fatal("缺收寄记录不该走到采用")
	}
}

func TestAnUnreadableReceptionIsContinuableUndecided(t *testing.T) {
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{err: errors.New("store unavailable")},
		&targetViewDouble{found: true, target: uniqueTarget(t)},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrReceptionNotVisible) {
		t.Fatalf("err = %v, want ErrReceptionNotVisible", err)
	}
	if adopter.calls != 0 {
		t.Fatal("读失败不该走到采用")
	}
}

func TestAPendingIdentificationRecordIsNotAnAdoptableIntake(t *testing.T) {
	record := formedReception(t)
	record.Kind = noports.RecordPendingIdentification
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{record: record, found: true},
		&targetViewDouble{found: true, target: uniqueTarget(t)},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrReceptionNotVisible) {
		t.Fatalf("err = %v, want ErrReceptionNotVisible", err)
	}
	if adopter.calls != 0 {
		t.Fatal("待识别格不该走到采用")
	}
}

func TestAnUnidentifiedFormedIntakeKeepsItsSentinel(t *testing.T) {
	pending, err := nodomain.FormNodeIntake(nodomain.NodeIntakeSpec{
		TenantID:    value(t, nodomain.NewTenantID, "tenant-1"),
		Unit:        value(t, nodomain.NewHandlingUnitID, "unit-1"),
		Node:        value(t, nodomain.NewNodeReference, "node-origin"),
		DeliveredBy: value(t, nodomain.NewDeliveringPartyReference, "customer-1"),
		Evidence:    value(t, nodomain.NewReceptionEvidenceReference, "SIGN-7"),
		Version:     value(t, nodomain.NewIntakeResultVersion, "intake-result/v1"),
		ReceivedAt:  receivedAt,
	})
	if err != nil {
		t.Fatalf("构造待识别收寄：%v", err)
	}
	record := formedReception(t)
	record.Intake = pending
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{record: record, found: true},
		&targetViewDouble{found: true, target: uniqueTarget(t)},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrUnidentifiedHandlingUnit) {
		t.Fatalf("err = %v, want ErrUnidentifiedHandlingUnit", err)
	}
	if adopter.calls != 0 {
		t.Fatal("未识别实物不该走到采用")
	}
}

func TestAMissingParcelTargetIsContinuableUndecided(t *testing.T) {
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{record: formedReception(t), found: true},
		&targetViewDouble{},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrParcelTargetNotFound) {
		t.Fatalf("err = %v, want ErrParcelTargetNotFound", err)
	}
	if adopter.calls != 0 {
		t.Fatal("没有可采认目标不该走到采用")
	}
}

func TestAnAmbiguousParcelTargetStaysIdentifiable(t *testing.T) {
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{record: formedReception(t), found: true},
		&targetViewDouble{err: psdomain.ErrAmbiguousParcelTarget},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, psdomain.ErrAmbiguousParcelTarget) {
		t.Fatalf("err = %v, want ErrAmbiguousParcelTarget", err)
	}
	if adopter.calls != 0 {
		t.Fatal("歧义不得按 latest 采认")
	}
}
