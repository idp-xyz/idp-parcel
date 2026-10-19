package transportfulfillment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/transportfulfillment"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证消费侧适配器只翻译不判断（ADR-0025、ADR-0214 决定一）：按四维键读回那一版、问有没有后继、`已交接`译出转出引用、拒收与
// 待确认译不出；对象原样交下去，不再拿字符串去比作业实物——对象怎样对上实物是编排经本上下文册子的事。以及消费门处理方把编排的
// 处置落成三种动作。

var handedOverAt = time.Date(2026, 8, 9, 12, 0, 0, 123456789, time.UTC)

func value[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

// formed 真经 TF 领域形成一份权威交接结果。
func formed(t *testing.T, verdict tfdomain.HandoverVerdict, object, version string) tfdomain.TransportHandover {
	t.Helper()
	spec := tfdomain.TransportHandoverSpec{
		TenantID:   value(t, tfdomain.NewTenantID, "tenant-1"),
		Object:     value(t, tfdomain.NewCarriedObjectReference, object),
		Scope:      value(t, tfdomain.NewHandoverScopeReference, "handover-scope-1"),
		ReleasedBy: value(t, tfdomain.NewHandoverPartyReference, "node-origin"),
		ReceivedBy: value(t, tfdomain.NewHandoverPartyReference, "carrier-1"),
		Verdict:    verdict,
		Version:    value(t, tfdomain.NewHandoverResultVersion, version),
		JudgedAt:   handedOverAt,
	}
	if verdict == tfdomain.ObjectHandedOver {
		spec.ReleasingEvidence = value(t, tfdomain.NewHandoverEvidenceReference, "LOAD-SCAN/7")
		spec.ReceivingEvidence = value(t, tfdomain.NewHandoverEvidenceReference, "CARRIER-ACK/9")
		spec.Rule = value(t, tfdomain.NewHandoverRuleReference, "handover-rules/v1")
	} else {
		spec.Basis = value(t, tfdomain.NewHandoverBasisReference, "SEAL_MISMATCH")
	}
	built, err := tfdomain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("form transport handover (%s): %v", verdict, err)
	}
	return built
}

func keyOf(handover tfdomain.TransportHandover) tfports.TransportHandoverKey {
	return tfports.TransportHandoverKey{
		TenantID: handover.TenantID(), Object: handover.Object(), Scope: handover.Scope(), Version: handover.Version(),
	}
}

// readerDouble 扮 TF 交接登记册：按键存记录，successors 记哪几版已被更正。
type readerDouble struct {
	records    map[tfports.TransportHandoverKey]tfports.TransportHandoverRecord
	successors map[tfports.TransportHandoverKey]bool
	findErr    error
	successErr error
}

func newReader(handovers ...tfdomain.TransportHandover) *readerDouble {
	reader := &readerDouble{
		records:    map[tfports.TransportHandoverKey]tfports.TransportHandoverRecord{},
		successors: map[tfports.TransportHandoverKey]bool{},
	}
	for _, handover := range handovers {
		reader.records[keyOf(handover)] = tfports.TransportHandoverRecord{Key: keyOf(handover), Handover: handover, RecordedAt: handedOverAt}
	}
	return reader
}

func (reader *readerDouble) FindByKey(_ context.Context, key tfports.TransportHandoverKey) (tfports.TransportHandoverRecord, bool, error) {
	if reader.findErr != nil {
		return tfports.TransportHandoverRecord{}, false, reader.findErr
	}
	record, found := reader.records[key]
	return record, found, nil
}

func (reader *readerDouble) FindSuccessor(_ context.Context, key tfports.TransportHandoverKey) (tfports.TransportHandoverRecord, bool, error) {
	if reader.successErr != nil {
		return tfports.TransportHandoverRecord{}, false, reader.successErr
	}
	return tfports.TransportHandoverRecord{}, reader.successors[key], nil
}

func find(t *testing.T, reader *readerDouble, object, version string) (ports.AuthoritativeHandover, bool, error) {
	t.Helper()
	source, err := adapter.NewAuthoritativeHandoverSource(reader)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := nodomain.NewAuthoritativeHandoverReference(object, "handover-scope-1", version)
	if err != nil {
		t.Fatal(err)
	}
	return source.FindAuthoritativeHandover(context.Background(), value(t, nodomain.NewTenantID, "tenant-1"), reference)
}

// Covers: TF CONTEXT「已拒收或待确认不转出控制」与 NO CONTEXT「转出唯一依据是已交接结果」在适配器上对上——`已交接`译出带交接版本的
// 转出引用，拒收与待确认译不出；业务时间折到微秒（转移册的存储精度）；对象是什么串都原样交下去，不与任何作业实物比。
func TestTheSourceTranslatesTheVerdictWithoutComparingObjects(t *testing.T) {
	reader := newReader(
		formed(t, tfdomain.ObjectHandedOver, "parcel-1", "v1"),
		formed(t, tfdomain.HandoverRefused, "parcel-2", "v1"),
		formed(t, tfdomain.HandoverPendingConfirmation, "parcel-3", "v1"),
	)

	answer, found, err := find(t, reader, "parcel-1", "v1")
	if err != nil || !found || !answer.HandedOver || answer.TransferOut.String() != "TRANSPORT-HANDOVER/v1" || answer.Superseded {
		t.Fatalf("已交接 = %+v found=%v err=%v", answer, found, err)
	}
	if want := handedOverAt.Truncate(time.Microsecond); !answer.JudgedAt.Equal(want) {
		t.Fatalf("judged at = %s, want 折到微秒的 %s", answer.JudgedAt, want)
	}
	if answer.Reference.Object() != "parcel-1" {
		t.Fatalf("对象 = %q, want 原样交下去", answer.Reference.Object())
	}
	for _, object := range []string{"parcel-2", "parcel-3"} {
		answer, found, err := find(t, reader, object, "v1")
		if err != nil || !found || answer.HandedOver || answer.TransferOut.String() != "" {
			t.Fatalf("%s：拒收与待确认不该给出转出引用：%+v", object, answer)
		}
	}
}

// Covers: ADR-0214 决定五——这一版已被后一版更正：交回 Superseded，晚到的前版由编排零写入。
func TestASupersededVersionIsMarked(t *testing.T) {
	handover := formed(t, tfdomain.ObjectHandedOver, "parcel-1", "v1")
	reader := newReader(handover)
	reader.successors[keyOf(handover)] = true

	answer, found, err := find(t, reader, "parcel-1", "v1")
	if err != nil || !found || !answer.Superseded {
		t.Fatalf("answer = %+v found=%v err=%v, want Superseded", answer, found, err)
	}
}

// Covers: ADR-0029 的分格——读不回答 false；登记册读不动（两问都算）包 ErrAuthoritativeHandoverUnreadable 让编排答未决；读回的记录
// 指着另一个键是提供方仓储坏了，另一个哨兵，编排原样交回、不当未决。
func TestReadFailuresAreSortedByRecovery(t *testing.T) {
	handover := formed(t, tfdomain.ObjectHandedOver, "parcel-1", "v1")

	if _, found, err := find(t, newReader(), "parcel-1", "v1"); found || err != nil {
		t.Fatalf("读不回：found=%v err=%v", found, err)
	}
	successorUnreadable := newReader(handover)
	successorUnreadable.successErr = errors.New("connection reset")
	for name, reader := range map[string]*readerDouble{
		"按键读不动": {findErr: errors.New("connection reset")},
		"后继读不动": successorUnreadable,
	} {
		if _, _, err := find(t, reader, "parcel-1", "v1"); !errors.Is(err, ports.ErrAuthoritativeHandoverUnreadable) {
			t.Fatalf("%s：err = %v, want ErrAuthoritativeHandoverUnreadable", name, err)
		}
	}
	inconsistent := newReader()
	wrong := formed(t, tfdomain.ObjectHandedOver, "parcel-9", "v1")
	inconsistent.records[keyOf(handover)] = tfports.TransportHandoverRecord{Key: keyOf(wrong), Handover: wrong, RecordedAt: handedOverAt}
	if _, _, err := find(t, inconsistent, "parcel-1", "v1"); !errors.Is(err, adapter.ErrHandoverRecordInconsistent) {
		t.Fatalf("err = %v, want ErrHandoverRecordInconsistent", err)
	}
}

// noUnitResolver 与 recordingRegister 是处理方那一条用例给编排的最小替身：对象认不出任何作业实物，转移册在内存里记行。
type noUnitResolver struct{}

func (noUnitResolver) ResolveHandoverObject(
	context.Context, nodomain.TenantID, nodomain.AuthoritativeHandoverReference,
) (ports.HandoverObjectResolution, error) {
	return ports.HandoverObjectResolution{Kind: ports.HandoverObjectParcel}, nil
}

type recordingRegister struct {
	saved []ports.ControlTransitionRecord
}

func (register *recordingRegister) FindTransition(
	context.Context, nodomain.TenantID, nodomain.AuthoritativeHandoverReference, nodomain.ControlTransitionDirection,
) (ports.ControlTransitionRecord, bool, error) {
	return ports.ControlTransitionRecord{}, false, nil
}

func (register *recordingRegister) FindAdvancedInHandoverChain(
	context.Context, nodomain.TenantID, nodomain.AuthoritativeHandoverReference, nodomain.ControlTransitionDirection,
) (ports.ControlTransitionRecord, bool, error) {
	return ports.ControlTransitionRecord{}, false, nil
}

func (register *recordingRegister) FindActiveControls(
	context.Context, nodomain.TenantID, nodomain.HandlingUnitID,
) ([]nodomain.PhysicalControl, error) {
	return nil, nil
}

func (register *recordingRegister) FindUnitControls(
	context.Context, nodomain.TenantID, nodomain.HandlingUnitID,
) ([]nodomain.PhysicalControl, error) {
	return nil, nil
}

func (register *recordingRegister) Save(_ context.Context, record ports.ControlTransitionRecord) (ports.ControlTransitionSaveOutcome, error) {
	register.saved = append(register.saved, record)
	return ports.ControlTransitionSaved, nil
}

type clockAt struct{ at time.Time }

func (clock clockAt) Now() time.Time { return clock.at }

func handlerOver(t *testing.T, reader *readerDouble, register *recordingRegister) *adapter.TransferControlOnHandoverAdapter {
	t.Helper()
	source, err := adapter.NewAuthoritativeHandoverSource(reader)
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := application.NewTransferControlOnHandoverHandler(application.TransferControlOnHandoverDeps{
		Handovers: source, Objects: noUnitResolver{}, Controls: register, Clock: clockAt{at: handedOverAt.Add(time.Hour)},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := adapter.NewTransferControlOnHandoverAdapter(transfer)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// Covers: ADR-0214 决定六——消费门处理方的三种动作：入账的格（这里是对不上实物、入账一行）交回 nil；交接还读不回交回未决哨兵、不写；
// 信封键过不了本上下文的构造门交回缺陷哨兵，不当未决。
func TestTheHandlerSettlesRetriesOrRaises(t *testing.T) {
	register := &recordingRegister{}
	handler := handlerOver(t, newReader(formed(t, tfdomain.ObjectHandedOver, "parcel-1", "v1")), register)
	registered := noinbox.RegisteredTransportHandover{TenantID: "tenant-1", Object: "parcel-1", Scope: "handover-scope-1", Version: "v1"}

	if err := handler.HandleRegisteredTransportHandover(context.Background(), registered); err != nil {
		t.Fatalf("入账那一格 err = %v, want nil", err)
	}
	if len(register.saved) != 1 || register.saved[0].Transition.Reason() != nodomain.NoAssociatedUnit {
		t.Fatalf("落册 %+v, want 一行 NO_ASSOCIATED_UNIT", register.saved)
	}

	unseen := registered
	unseen.Version = "v9"
	if err := handler.HandleRegisteredTransportHandover(context.Background(), unseen); !errors.Is(err, adapter.ErrControlTransferUndecided) {
		t.Fatalf("读不回 err = %v, want ErrControlTransferUndecided", err)
	}

	blank := registered
	blank.TenantID = "  "
	if err := handler.HandleRegisteredTransportHandover(context.Background(), blank); !errors.Is(err, adapter.ErrControlTransferDefect) {
		t.Fatalf("空白租户 err = %v, want ErrControlTransferDefect", err)
	}
	if len(register.saved) != 1 {
		t.Fatalf("未决与缺陷都不该落册，落了 %d 行", len(register.saved))
	}
}
