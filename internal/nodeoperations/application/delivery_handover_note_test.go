package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证送站交接单的编排（UC-NO-006 应用流程步骤 3 至 6；ADR-0212 决定三、六；票 ps-external-mark-relations/12）：开立、归入、
// 关闭经作业事实登记，绑定经运营决定与客户渠道两路，最后一个声称的客户账户得到确认时同一笔自投门铃。交接单册与收寄库都是内存
// 替身，收寄沿用收寄编排那一只。夹具全为合成（S 级）。

// handoverRegisterDouble 照交接单册的代数在内存里落：撞键不覆盖、回显在册那一行；交接单不在册、结论的客户账户不在声称之列答查无
// 此单；向已关闭的交接单加成员答已关闭。
type handoverRegisterDouble struct {
	notes    map[string]domain.DeliveryHandoverNote
	closures map[string]domain.DeliveryHandoverClosure
	bindings map[string][]domain.DeliveryHandoverBinding
	members  []domain.DeliveryHandoverMember
	err      error
}

func newHandoverRegister() *handoverRegisterDouble {
	return &handoverRegisterDouble{
		notes:    map[string]domain.DeliveryHandoverNote{},
		closures: map[string]domain.DeliveryHandoverClosure{},
		bindings: map[string][]domain.DeliveryHandoverBinding{},
	}
}

func (double *handoverRegisterDouble) OpenDeliveryHandoverNote(_ context.Context, note domain.DeliveryHandoverNote) (domain.DeliveryHandoverNote, ports.DeliveryScopeAppendOutcome, error) {
	if double.err != nil {
		return domain.DeliveryHandoverNote{}, ports.DeliveryScopeAppendInvalid, double.err
	}
	if recorded, found := double.notes[note.ID().String()]; found {
		return recorded, ports.DeliveryScopeAlreadyRecorded, nil
	}
	double.notes[note.ID().String()] = note
	return note, ports.DeliveryScopeAppended, nil
}

func (double *handoverRegisterDouble) AppendDeliveryHandoverMember(_ context.Context, member domain.DeliveryHandoverMember) (domain.DeliveryHandoverMember, ports.DeliveryScopeAppendOutcome, error) {
	if double.err != nil {
		return domain.DeliveryHandoverMember{}, ports.DeliveryScopeAppendInvalid, double.err
	}
	if _, found := double.notes[member.Note().String()]; !found {
		return domain.DeliveryHandoverMember{}, ports.DeliveryScopeTargetNotFound, nil
	}
	for _, recorded := range double.members {
		if recorded.Kind() == member.Kind() && recorded.FactID() == member.FactID() {
			return recorded, ports.DeliveryScopeAlreadyRecorded, nil
		}
	}
	if _, closed := double.closures[member.Note().String()]; closed {
		return domain.DeliveryHandoverMember{}, ports.DeliveryScopeNoteClosed, nil
	}
	double.members = append(double.members, member)
	return member, ports.DeliveryScopeAppended, nil
}

func (double *handoverRegisterDouble) CloseDeliveryHandoverNote(_ context.Context, closure domain.DeliveryHandoverClosure) (domain.DeliveryHandoverClosure, ports.DeliveryScopeAppendOutcome, error) {
	if double.err != nil {
		return domain.DeliveryHandoverClosure{}, ports.DeliveryScopeAppendInvalid, double.err
	}
	if _, found := double.notes[closure.Note().String()]; !found {
		return domain.DeliveryHandoverClosure{}, ports.DeliveryScopeTargetNotFound, nil
	}
	if recorded, closed := double.closures[closure.Note().String()]; closed {
		return recorded, ports.DeliveryScopeAlreadyRecorded, nil
	}
	double.closures[closure.Note().String()] = closure
	return closure, ports.DeliveryScopeAppended, nil
}

func (double *handoverRegisterDouble) RecordDeliveryHandoverBinding(_ context.Context, binding domain.DeliveryHandoverBinding) (domain.DeliveryHandoverBinding, ports.DeliveryScopeAppendOutcome, error) {
	if double.err != nil {
		return domain.DeliveryHandoverBinding{}, ports.DeliveryScopeAppendInvalid, double.err
	}
	note, found := double.notes[binding.Note().String()]
	if !found || !note.Claims(binding.Account()) {
		return domain.DeliveryHandoverBinding{}, ports.DeliveryScopeTargetNotFound, nil
	}
	for _, recorded := range double.bindings[binding.Note().String()] {
		if recorded.Account() == binding.Account() {
			return recorded, ports.DeliveryScopeAlreadyRecorded, nil
		}
	}
	double.bindings[binding.Note().String()] = append(double.bindings[binding.Note().String()], binding)
	return binding, ports.DeliveryScopeAppended, nil
}

func (double *handoverRegisterDouble) FindDeliveryHandoverNote(_ context.Context, _ domain.TenantID, id domain.DeliveryHandoverNoteID) (domain.DeliveryHandoverNoteState, bool, error) {
	if double.err != nil {
		return domain.DeliveryHandoverNoteState{}, false, double.err
	}
	note, found := double.notes[id.String()]
	if !found {
		return domain.DeliveryHandoverNoteState{}, false, nil
	}
	return domain.DeliveryHandoverNoteState{
		Note:     note,
		Closure:  double.closures[id.String()],
		Bindings: slices.Clone(double.bindings[id.String()]),
	}, true, nil
}

func (double *handoverRegisterDouble) ListDeliveryHandoverMembers(_ context.Context, _ domain.TenantID, id domain.DeliveryHandoverNoteID) ([]domain.DeliveryHandoverMember, error) {
	var members []domain.DeliveryHandoverMember
	for _, member := range double.members {
		if member.Note() == id {
			members = append(members, member)
		}
	}
	return members, double.err
}

func (double *handoverRegisterDouble) FindLastAssignedHandoverNote(_ context.Context, _ domain.TenantID, unit domain.HandlingUnitID) (domain.DeliveryHandoverNoteID, bool, error) {
	var last domain.DeliveryHandoverMember
	for _, member := range double.members {
		if member.Kind() == domain.MembershipByAssignment && member.Unit() == unit && !member.FormedAt().Before(last.FormedAt()) {
			last = member
		}
	}
	return last.Note(), last.FactID() != "", double.err
}

func (double *handoverRegisterDouble) FindDeliveryHandoverMember(_ context.Context, _ domain.TenantID, kind domain.HandoverMembershipKind, factID string) (domain.DeliveryHandoverMember, bool, error) {
	for _, member := range double.members {
		if member.Kind() == kind && member.FactID() == factID {
			return member, true, double.err
		}
	}
	return domain.DeliveryHandoverMember{}, false, double.err
}

// authorizedDeliveryDouble 记下每一问，交回预设的授权交付判断或读不动。
type authorizedDeliveryDouble struct {
	answer ports.AuthorizedDeliveryAnswer
	err    error
	asked  []ports.AuthorizedDeliveryQuery
}

func (double *authorizedDeliveryDouble) JudgeAuthorizedDelivery(_ context.Context, query ports.AuthorizedDeliveryQuery) (ports.AuthorizedDeliveryAnswer, error) {
	double.asked = append(double.asked, query)
	return double.answer, double.err
}

// scopeDoorbellDouble 记下自投的每一封「交付范围成立」门铃，err 非空时入不了队。
type scopeDoorbellDouble struct {
	rung []ports.DeliveryScopeEstablishedIntent
	err  error
}

func (double *scopeDoorbellDouble) RingDeliveryScopeEstablished(_ context.Context, intent ports.DeliveryScopeEstablishedIntent) error {
	if double.err != nil {
		return double.err
	}
	double.rung = append(double.rung, intent)
	return nil
}

const (
	handoverNode    = "node-origin"
	handoverAccount = "SYN-ACCOUNT-01"
	otherAccount    = "SYN-ACCOUNT-02"
)

type handoverFixture struct {
	*receptionFixture
	notes      *handoverRegisterDouble
	authorized *authorizedDeliveryDouble
	scopeBells *scopeDoorbellDouble
	handover   *application.DeliveryHandoverNoteHandler
}

func newHandoverFixture(t *testing.T) *handoverFixture {
	t.Helper()
	reception := newReceptionFixture(t)
	fixture := &handoverFixture{
		receptionFixture: reception,
		notes:            reception.handover,
		authorized:       &authorizedDeliveryDouble{answer: ports.DeliveryAuthorized},
		scopeBells:       &scopeDoorbellDouble{},
	}
	handler, err := application.NewDeliveryHandoverNoteHandler(fixture.deps())
	if err != nil {
		t.Fatalf("构造交接单编排：%v", err)
	}
	fixture.handover = handler
	return fixture
}

func (fixture *handoverFixture) deps() application.DeliveryHandoverDeps {
	return application.DeliveryHandoverDeps{
		Notes:              fixture.notes,
		Receptions:         fixture.receptions,
		AuthorizedDelivery: fixture.authorized,
		Doorbell:           fixture.scopeBells,
		Clock:              fixedClock{at: deliveredAt.Add(2 * time.Hour)},
	}
}

func openCommand(t *testing.T, note string, accounts ...string) application.OpenDeliveryHandoverNoteCommand {
	t.Helper()
	command := application.OpenDeliveryHandoverNoteCommand{
		TenantID:        mustValue(t, domain.NewTenantID, "tenant-1"),
		Node:            mustValue(t, domain.NewNodeReference, handoverNode),
		Note:            mustValue(t, domain.NewDeliveryHandoverNoteID, note),
		DeliveringParty: mustValue(t, domain.NewDeliveringPartyReference, "SYN-DRIVER-PSXM12"),
		OpenedAt:        deliveredAt.Add(-time.Hour),
	}
	for _, account := range accounts {
		command.ClaimedAccounts = append(command.ClaimedAccounts, mustValue(t, domain.NewCustomerAccountReference, account))
	}
	return command
}

func assignCommand(t *testing.T, fact, note, unit string) application.AssignToDeliveryHandoverNoteCommand {
	t.Helper()
	return application.AssignToDeliveryHandoverNoteCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Node:       mustValue(t, domain.NewNodeReference, handoverNode),
		FactID:     fact,
		Note:       mustValue(t, domain.NewDeliveryHandoverNoteID, note),
		Unit:       mustValue(t, domain.NewHandlingUnitID, unit),
		AssignedAt: deliveredAt.Add(30 * time.Minute),
	}
}

func closeCommand(t *testing.T, fact, note string) application.CloseDeliveryHandoverNoteCommand {
	t.Helper()
	return application.CloseDeliveryHandoverNoteCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Node:     mustValue(t, domain.NewNodeReference, handoverNode),
		FactID:   fact,
		Note:     mustValue(t, domain.NewDeliveryHandoverNoteID, note),
		ClosedAt: deliveredAt.Add(45 * time.Minute),
	}
}

func confirmCommand(t *testing.T, note, account string) application.ConfirmDeliveryHandoverCommand {
	t.Helper()
	return application.ConfirmDeliveryHandoverCommand{
		TenantID:              mustValue(t, domain.NewTenantID, "tenant-1"),
		Operator:              "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-01",
		Note:                  mustValue(t, domain.NewDeliveryHandoverNoteID, note),
		Account:               mustValue(t, domain.NewCustomerAccountReference, account),
		DeliveringParticipant: mustValue(t, domain.NewDeliveringPartyReference, "SYN-PARTY-DELIVERER-01"),
		Basis:                 "SYN-BASIS-AUTH-LETTER-01",
	}
}

func concludeCommand(t *testing.T, note, account, requestKey string, conclusion domain.HandoverBindingConclusion) application.ConcludeDeliveryHandoverCommand {
	t.Helper()
	return application.ConcludeDeliveryHandoverCommand{
		TenantID:        mustValue(t, domain.NewTenantID, "tenant-1"),
		CustomerAccount: mustValue(t, domain.NewCustomerAccountReference, account),
		Channel:         "https://id.syn.example/dex#syn-customer-client-01",
		RequestKey:      requestKey,
		Note:            mustValue(t, domain.NewDeliveryHandoverNoteID, note),
		Conclusion:      conclusion,
	}
}

func expectHandoverOutcome(t *testing.T, step string, result application.DeliveryHandoverResult, err error, want application.DeliveryHandoverOutcome) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s：err = %v, want %s", step, err, want)
	}
	if result.Outcome() != want {
		t.Fatalf("%s：outcome = %s, want %s", step, result.Outcome(), want)
	}
}

// receiveUnit 让一件实物经收寄编排落册：candidates 照 answerCandidates 的口径给，basis 是收寄带的依据引用（零值即没带）。
func (fixture *handoverFixture) receiveUnit(
	t *testing.T,
	source, unit string,
	basis domain.DeliveryScopeBasisReference,
	spaces map[string]string,
	declaredBy []string,
	candidates ...string,
) ports.ReceptionRecord {
	t.Helper()
	fixture.answerCandidates(t, spaces, declaredBy, candidates...)
	command := receiveCommand(t)
	command.SourceID = source
	command.Unit = mustValue(t, domain.NewHandlingUnitID, unit)
	command.Node = mustValue(t, domain.NewNodeReference, handoverNode)
	command.ScopeBasis = basis
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("收寄 %s：%v", source, err)
	}
	record, _ := result.Record()
	return record
}

// receivePendingUnit 落一件只经客户包裹件号命中、范围未证的待识别（ADR-0212 决定五）。
func (fixture *handoverFixture) receivePendingUnit(t *testing.T, source, unit string) ports.ReceptionRecord {
	t.Helper()
	record := fixture.receiveUnit(t, source, unit, domain.DeliveryScopeBasisReference{}, map[string]string{"parcel-" + unit: handoverAccount}, nil, "parcel-"+unit)
	if record.Kind != ports.RecordPendingIdentification || record.IdentityConflict {
		t.Fatalf("%s 没落成不记冲突的待识别：%+v", unit, record)
	}
	return record
}

// Covers: `AT-NO-071`、票 12 完成判据第一格——开立、收寄带它、归入、关闭：开立落册，同一标识原样重投答已有结果、换内容答来源
// 冲突；收寄带它的标识即落成员（形成方式收寄）；一件缺范围的待识别归入落成员（形成方式归入），归入不改写那件的来源、也不追加
// 判断版本；关闭冻结成员，关闭之后归入、归入已识别或记多候选冲突的实物、在别的节点没有当前收寄的实物，都答交接单状态不符。
func TestOpeningAssigningAndClosingAHandoverNote(t *testing.T) {
	fixture := newHandoverFixture(t)
	ctx := context.Background()

	opened, err := fixture.handover.Open(ctx, openCommand(t, "SYN-HANDOVER-01", handoverAccount))
	expectHandoverOutcome(t, "开立", opened, err, application.DeliveryHandoverNoteOpened)
	if opened.Note().Node().String() != handoverNode || len(opened.Note().ClaimedAccounts()) != 1 {
		t.Fatalf("开立落下的交接单 = %+v", opened.Note())
	}
	replay, err := fixture.handover.Open(ctx, openCommand(t, "SYN-HANDOVER-01", handoverAccount))
	expectHandoverOutcome(t, "原样重投开立", replay, err, application.DeliveryHandoverExistingResult)
	conflicting := openCommand(t, "SYN-HANDOVER-01", handoverAccount, otherAccount)
	conflict, err := fixture.handover.Open(ctx, conflicting)
	expectHandoverOutcome(t, "同一标识换内容", conflict, err, application.DeliveryHandoverSourceConflict)
	if len(conflict.Note().ClaimedAccounts()) != 1 {
		t.Fatal("来源冲突没有回显在册那一张")
	}

	// 收寄带交接单的标识：落成员（收寄），身份照收寄当时的证据判。
	carried := fixture.receiveUnit(t, "delivery-carried", "unit-carried",
		basisOf(t, domain.BasisDeliveryHandoverNote, "SYN-HANDOVER-01"), map[string]string{"parcel-carried": handoverAccount}, nil, "parcel-carried")
	pending := fixture.receivePendingUnit(t, "delivery-pending", "unit-pending")
	formed := fixture.receiveUnit(t, "delivery-formed", "unit-formed", domain.DeliveryScopeBasisReference{}, nil, nil, "parcel-formed")
	contested := fixture.receiveUnit(t, "delivery-contested", "unit-contested", domain.DeliveryScopeBasisReference{}, nil, nil, "parcel-x", "parcel-y")
	if formed.Kind != ports.RecordIntakeFormed || !contested.IdentityConflict {
		t.Fatalf("夹具：formed = %s，contested 冲突 = %v", formed.Kind, contested.IdentityConflict)
	}
	if len(fixture.notes.members) != 1 || fixture.notes.members[0].Kind() != domain.MembershipByReception ||
		fixture.notes.members[0].FactID() != carried.Key.SourceID || fixture.notes.members[0].Unit().String() != "unit-carried" {
		t.Fatalf("收寄带它之后的成员 = %+v, want 一个经收寄、事实身份取来源标识", fixture.notes.members)
	}
	rowsBefore := len(fixture.receptions.byKey)

	assigned, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-01", "SYN-HANDOVER-01", "unit-pending"))
	expectHandoverOutcome(t, "归入待识别", assigned, err, application.DeliveryHandoverUnitAssigned)
	member, present := assigned.Member()
	if !present || member.Kind() != domain.MembershipByAssignment || member.Unit().String() != "unit-pending" || member.FactID() != "SYN-ASSIGN-01" {
		t.Fatalf("归入的成员 = %+v", member)
	}
	if after := fixture.receptions.byKey[pending.Key]; after.Source.ScopeBasis != (domain.DeliveryScopeBasisReference{}) || len(fixture.receptions.byKey) != rowsBefore {
		t.Fatalf("归入改写了来源或追加了判断：来源依据 %+v，行 %d → %d", after.Source.ScopeBasis, rowsBefore, len(fixture.receptions.byKey))
	}
	again, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-01", "SYN-HANDOVER-01", "unit-pending"))
	expectHandoverOutcome(t, "原样重投归入", again, err, application.DeliveryHandoverExistingResult)
	sameFactOtherUnit, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-01", "SYN-HANDOVER-01", "unit-carried"))
	expectHandoverOutcome(t, "同一归入事实换实物", sameFactOtherUnit, err, application.DeliveryHandoverSourceConflict)

	for name, unit := range map[string]string{"已识别": "unit-formed", "记多候选冲突": "unit-contested", "在这个节点没有收寄": "unit-elsewhere"} {
		refused, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-"+unit, "SYN-HANDOVER-01", unit))
		expectHandoverOutcome(t, "归入"+name+"的实物", refused, err, application.DeliveryHandoverNoteStateMismatch)
		if refused.StateMismatch() != application.HandoverUnitNotAssignable {
			t.Fatalf("归入%s的实物：不符的是 %s", name, refused.StateMismatch())
		}
	}

	closed, err := fixture.handover.Close(ctx, closeCommand(t, "SYN-CLOSE-01", "SYN-HANDOVER-01"))
	expectHandoverOutcome(t, "关闭", closed, err, application.DeliveryHandoverNoteClosed)
	reclosed, err := fixture.handover.Close(ctx, closeCommand(t, "SYN-CLOSE-01", "SYN-HANDOVER-01"))
	expectHandoverOutcome(t, "原样重投关闭", reclosed, err, application.DeliveryHandoverExistingResult)
	other := closeCommand(t, "SYN-CLOSE-02", "SYN-HANDOVER-01")
	other.ClosedAt = other.ClosedAt.Add(time.Minute)
	closedByOther, err := fixture.handover.Close(ctx, other)
	expectHandoverOutcome(t, "另一份关闭事实", closedByOther, err, application.DeliveryHandoverExistingResult)
	if closure, _ := closedByOther.Closure(); closure.FactID() != "SYN-CLOSE-01" {
		t.Fatalf("回显的关闭 = %q, want 第一份", closure.FactID())
	}
	sameFactOtherInstant := closeCommand(t, "SYN-CLOSE-01", "SYN-HANDOVER-01")
	sameFactOtherInstant.ClosedAt = sameFactOtherInstant.ClosedAt.Add(time.Minute)
	closeConflict, err := fixture.handover.Close(ctx, sameFactOtherInstant)
	expectHandoverOutcome(t, "同一关闭事实换时刻", closeConflict, err, application.DeliveryHandoverSourceConflict)

	pendingAgain := fixture.receivePendingUnit(t, "delivery-late", "unit-late")
	late, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-LATE", "SYN-HANDOVER-01", pendingAgain.Intake.Unit().String()))
	expectHandoverOutcome(t, "关闭之后归入", late, err, application.DeliveryHandoverNoteStateMismatch)
	if late.StateMismatch() != application.HandoverNoteAlreadyClosed {
		t.Fatalf("关闭之后归入：不符的是 %s", late.StateMismatch())
	}
	replayAfterClose, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-01", "SYN-HANDOVER-01", "unit-pending"))
	expectHandoverOutcome(t, "关闭之前那次归入再投", replayAfterClose, err, application.DeliveryHandoverExistingResult)

	// 关闭之后带它收寄、带一张不在册的交接单收寄：收寄照常落册，成员不多一个——关闭冻结成员，扫错了的依据只记在交付范围核对态里。
	membersBefore := len(fixture.notes.members)
	afterClose := fixture.receiveUnit(t, "delivery-after-close", "unit-after-close",
		basisOf(t, domain.BasisDeliveryHandoverNote, "SYN-HANDOVER-01"), map[string]string{"parcel-after-close": handoverAccount}, nil, "parcel-after-close")
	unknown := fixture.receiveUnit(t, "delivery-unknown-note", "unit-unknown-note",
		basisOf(t, domain.BasisDeliveryHandoverNote, "SYN-HANDOVER-NONE"), map[string]string{"parcel-unknown-note": handoverAccount}, nil, "parcel-unknown-note")
	if afterClose.Kind != ports.RecordPendingIdentification || unknown.Kind != ports.RecordPendingIdentification || len(fixture.notes.members) != membersBefore {
		t.Fatalf("关闭之后与不在册的依据：收寄 %s / %s，成员 %d → %d", afterClose.Kind, unknown.Kind, membersBefore, len(fixture.notes.members))
	}
	if len(fixture.scopeBells.rung) != 0 {
		t.Fatalf("没有确认却响了门铃 %d 封", len(fixture.scopeBells.rung))
	}
}

// Covers: `AT-NO-071`（可见与拒形那几格）、票 12 做什么第 1 项「作业实物须在同一节点有当前收寄」与 UC-NO-006「指名的交接单在本租户查不到，与越权探针同答」——作业
// 范围是别的节点的设备指名这张交接单、指名一张不存在的交接单，都答查无此单；命令缺件答拒形，不落任何东西。只因歧义信号的冲突也
// 归入得进（ADR-0212 决定三「经 ADR-0211 放宽」）。
func TestAHandoverNoteIsVisibleOnlyAtItsOwnNode(t *testing.T) {
	fixture := newHandoverFixture(t)
	ctx := context.Background()
	if result, err := fixture.handover.Open(ctx, openCommand(t, "SYN-HANDOVER-02", handoverAccount)); err != nil || result.Outcome() != application.DeliveryHandoverNoteOpened {
		t.Fatalf("开立：%v %v", result.Outcome(), err)
	}
	ambiguous := fixture.receiveUnit(t, "delivery-ambiguous", "unit-ambiguous", domain.DeliveryScopeBasisReference{}, nil, []string{"SYN-ACCOUNT-09"}, "parcel-ambiguous")
	if !ambiguous.IdentityConflict || ambiguous.IdentityConflictCause != ports.ConflictByAmbiguityOnly {
		t.Fatalf("夹具：%+v", ambiguous)
	}
	elsewhere := assignCommand(t, "SYN-ASSIGN-ELSEWHERE", "SYN-HANDOVER-02", "unit-ambiguous")
	elsewhere.Node = mustValue(t, domain.NewNodeReference, "node-other")
	if _, err := fixture.handover.Assign(ctx, elsewhere); !errors.Is(err, application.ErrDeliveryHandoverNoteNotFound) {
		t.Fatalf("别的节点归入：err = %v", err)
	}
	closeElsewhere := closeCommand(t, "SYN-CLOSE-ELSEWHERE", "SYN-HANDOVER-02")
	closeElsewhere.Node = elsewhere.Node
	if _, err := fixture.handover.Close(ctx, closeElsewhere); !errors.Is(err, application.ErrDeliveryHandoverNoteNotFound) {
		t.Fatalf("别的节点关闭：err = %v", err)
	}
	if _, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-X", "SYN-HANDOVER-ABSENT", "unit-ambiguous")); !errors.Is(err, application.ErrDeliveryHandoverNoteNotFound) {
		t.Fatalf("不存在的交接单：err = %v", err)
	}
	assigned, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-AMBIGUOUS", "SYN-HANDOVER-02", "unit-ambiguous"))
	expectHandoverOutcome(t, "只因歧义信号的冲突归入", assigned, err, application.DeliveryHandoverUnitAssigned)

	for name, command := range map[string]application.OpenDeliveryHandoverNoteCommand{
		"一个都不声称": openCommand(t, "SYN-HANDOVER-03"),
		"缺开立时刻": func() application.OpenDeliveryHandoverNoteCommand {
			command := openCommand(t, "SYN-HANDOVER-03", handoverAccount)
			command.OpenedAt = time.Time{}
			return command
		}(),
	} {
		if _, err := fixture.handover.Open(ctx, command); !errors.Is(err, application.ErrDeliveryHandoverCommandRejected) {
			t.Fatalf("%s：err = %v", name, err)
		}
	}
	blankFact := assignCommand(t, " ", "SYN-HANDOVER-02", "unit-ambiguous")
	if _, err := fixture.handover.Assign(ctx, blankFact); !errors.Is(err, application.ErrDeliveryHandoverCommandRejected) {
		t.Fatalf("归入缺事实身份：err = %v", err)
	}
	if _, found := fixture.notes.notes["SYN-HANDOVER-03"]; found {
		t.Fatal("拒形的开立落了册")
	}
}

// Covers: `AT-NO-072`、票 12 完成判据第二格——运营决定确认对客户账户 A 的绑定，问 party-commercial 的授权交付判断，问法是（租户，交付
// 方参与方，客户账户，开立节点，开立时刻）：是即落确认（途径运营决定，带交付方参与方、依据引用与提交操作者）；否答不在授权交付关系
// 内、未配置答未配置，两种都不落结论；交付方是 A 所属参与方本身时 PC 答是，照样落确认；读不动不形成答案。运营决定不限于已关闭的
// 交接单；客户账户不在声称之列与查无此单同答，不问 PC。
func TestAnOperationDecisionConfirmsOnlyWithinTheAuthorizedDeliveryRelationship(t *testing.T) {
	ctx := context.Background()
	for name, testCase := range map[string]struct {
		answer ports.AuthorizedDeliveryAnswer
		want   application.DeliveryHandoverOutcome
	}{
		"有授权交付关系":     {ports.DeliveryAuthorized, application.DeliveryHandoverBindingConfirmed},
		"没有":          {ports.DeliveryNotAuthorized, application.DeliveryHandoverNotAuthorized},
		"本租户一条都没登记":   {ports.DeliveryAuthorizationNotConfigured, application.DeliveryHandoverAuthorizationNotConfigured},
		"交付方就是账户所属本人": {ports.DeliveryAuthorized, application.DeliveryHandoverBindingConfirmed},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newHandoverFixture(t)
			fixture.authorized.answer = testCase.answer
			opened, err := fixture.handover.Open(ctx, openCommand(t, "SYN-HANDOVER-10", handoverAccount))
			expectHandoverOutcome(t, "开立", opened, err, application.DeliveryHandoverNoteOpened)
			result, err := fixture.handover.ConfirmByOperationDecision(ctx, confirmCommand(t, "SYN-HANDOVER-10", handoverAccount))
			expectHandoverOutcome(t, "确认", result, err, testCase.want)
			if len(fixture.authorized.asked) != 1 {
				t.Fatalf("问了 PC %d 次", len(fixture.authorized.asked))
			}
			query := fixture.authorized.asked[0]
			if query.Tenant.String() != "tenant-1" || query.DeliveringParty.String() != "SYN-PARTY-DELIVERER-01" || query.CustomerAccount != handoverAccount ||
				query.Node.String() != handoverNode || !query.OpenedAt.Equal(opened.Note().OpenedAt()) {
				t.Fatalf("问法 = %+v, want（租户，交付方参与方，客户账户，开立节点，开立时刻）", query)
			}
			bindings := fixture.notes.bindings["SYN-HANDOVER-10"]
			if testCase.want != application.DeliveryHandoverBindingConfirmed {
				if len(bindings) != 0 {
					t.Fatalf("拒绝落了结论：%+v", bindings)
				}
				return
			}
			if len(bindings) != 1 || bindings[0].Route() != domain.ConcludedViaOperationDecision || bindings[0].Conclusion() != domain.BindingConfirmed ||
				bindings[0].Concluder() != "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-01" {
				t.Fatalf("落下的结论 = %+v", bindings)
			}
			if participant, basis := bindings[0].OperationDecisionEvidence(); participant != "SYN-PARTY-DELIVERER-01" || basis != "SYN-BASIS-AUTH-LETTER-01" {
				t.Fatalf("证据 = %q %q", participant, basis)
			}
			if at, established := result.ScopeEstablishedAt(); !established || !at.Equal(deliveredAt.Add(2*time.Hour)) {
				t.Fatalf("唯一声称的账户确认之后范围没成立：%s %v", at, established)
			}
		})
	}

	fixture := newHandoverFixture(t)
	if _, err := fixture.handover.Open(ctx, openCommand(t, "SYN-HANDOVER-11", handoverAccount)); err != nil {
		t.Fatal(err)
	}
	fixture.authorized.err = errors.New("party-commercial unreachable")
	if _, err := fixture.handover.ConfirmByOperationDecision(ctx, confirmCommand(t, "SYN-HANDOVER-11", handoverAccount)); err == nil ||
		errors.Is(err, application.ErrDeliveryHandoverNoteNotFound) || errors.Is(err, application.ErrDeliveryHandoverCommandRejected) {
		t.Fatalf("PC 读不动：err = %v, want 不形成答案", err)
	}
	if len(fixture.notes.bindings["SYN-HANDOVER-11"]) != 0 || len(fixture.scopeBells.rung) != 0 {
		t.Fatal("PC 读不动却落了结论或响了门铃")
	}
	fixture.authorized.err = nil
	asked := len(fixture.authorized.asked)
	if _, err := fixture.handover.ConfirmByOperationDecision(ctx, confirmCommand(t, "SYN-HANDOVER-11", otherAccount)); !errors.Is(err, application.ErrDeliveryHandoverNoteNotFound) {
		t.Fatalf("没声称的账户：err = %v", err)
	}
	if _, err := fixture.handover.ConfirmByOperationDecision(ctx, confirmCommand(t, "SYN-HANDOVER-ABSENT", handoverAccount)); !errors.Is(err, application.ErrDeliveryHandoverNoteNotFound) {
		t.Fatalf("不存在的交接单：err = %v", err)
	}
	if len(fixture.authorized.asked) != asked {
		t.Fatal("看不见的交接单也问了 PC")
	}
	blank := confirmCommand(t, "SYN-HANDOVER-11", handoverAccount)
	blank.Basis = " "
	if _, err := fixture.handover.ConfirmByOperationDecision(ctx, blank); !errors.Is(err, application.ErrDeliveryHandoverCommandRejected) {
		t.Fatalf("缺依据引用：err = %v", err)
	}
}

// Covers: `AT-NO-073`（编排半边）、票 12 完成判据第三格与做什么第 4 项——客户渠道对未关闭的交接单答状态不符；已关闭、声称了它的答
// 绑定已确认或已否认（落途径客户渠道、作结论方是客户端、来源请求键）；没声称它的与查无此单同答；同结论重投答已有结果，异结论答
// 已有结论并回显在册结论；运营决定此后再来确认已否认的账户，同样答已有结论。
func TestTheCustomerChannelConcludesOnlyOnAClosedNoteThatClaimsIt(t *testing.T) {
	fixture := newHandoverFixture(t)
	ctx := context.Background()
	if _, err := fixture.handover.Open(ctx, openCommand(t, "SYN-HANDOVER-20", handoverAccount, otherAccount)); err != nil {
		t.Fatal(err)
	}
	open, err := fixture.handover.ConcludeByCustomer(ctx, concludeCommand(t, "SYN-HANDOVER-20", handoverAccount, "SYN-REQ-1", domain.BindingConfirmed))
	expectHandoverOutcome(t, "未关闭", open, err, application.DeliveryHandoverNoteStateMismatch)
	if open.StateMismatch() != application.HandoverNoteNotClosed {
		t.Fatalf("未关闭：不符的是 %s", open.StateMismatch())
	}
	if _, err := fixture.handover.Close(ctx, closeCommand(t, "SYN-CLOSE-20", "SYN-HANDOVER-20")); err != nil {
		t.Fatal(err)
	}

	confirmed, err := fixture.handover.ConcludeByCustomer(ctx, concludeCommand(t, "SYN-HANDOVER-20", handoverAccount, "SYN-REQ-2", domain.BindingConfirmed))
	expectHandoverOutcome(t, "确认", confirmed, err, application.DeliveryHandoverBindingConfirmed)
	binding, present := confirmed.Binding()
	if !present || binding.Route() != domain.ConcludedViaCustomerChannel || binding.Concluder() != "https://id.syn.example/dex#syn-customer-client-01" ||
		binding.SourceRequestKey() != "SYN-REQ-2" || !binding.ConcludedAt().Equal(deliveredAt.Add(2*time.Hour)) {
		t.Fatalf("落下的结论 = %+v", binding)
	}
	if _, established := confirmed.ScopeEstablishedAt(); established {
		t.Fatal("还有一个声称的账户没确认，范围不该成立")
	}
	denied, err := fixture.handover.ConcludeByCustomer(ctx, concludeCommand(t, "SYN-HANDOVER-20", otherAccount, "SYN-REQ-3", domain.BindingDenied))
	expectHandoverOutcome(t, "否认", denied, err, application.DeliveryHandoverBindingDenied)

	replay, err := fixture.handover.ConcludeByCustomer(ctx, concludeCommand(t, "SYN-HANDOVER-20", handoverAccount, "SYN-REQ-4", domain.BindingConfirmed))
	expectHandoverOutcome(t, "同结论重投", replay, err, application.DeliveryHandoverExistingResult)
	flipped, err := fixture.handover.ConcludeByCustomer(ctx, concludeCommand(t, "SYN-HANDOVER-20", handoverAccount, "SYN-REQ-5", domain.BindingDenied))
	expectHandoverOutcome(t, "异结论", flipped, err, application.DeliveryHandoverExistingConclusion)
	if recorded, _ := flipped.Binding(); recorded.Conclusion() != domain.BindingConfirmed || recorded.SourceRequestKey() != "SYN-REQ-2" {
		t.Fatalf("回显的结论 = %+v, want 在册那一份确认", recorded)
	}
	operator, err := fixture.handover.ConfirmByOperationDecision(ctx, confirmCommand(t, "SYN-HANDOVER-20", otherAccount))
	expectHandoverOutcome(t, "运营决定确认已否认的账户", operator, err, application.DeliveryHandoverExistingConclusion)
	if len(fixture.authorized.asked) != 0 {
		t.Fatal("账户已有结论还问了 PC")
	}

	if _, err := fixture.handover.ConcludeByCustomer(ctx, concludeCommand(t, "SYN-HANDOVER-20", "SYN-ACCOUNT-03", "SYN-REQ-6", domain.BindingConfirmed)); !errors.Is(err, application.ErrDeliveryHandoverNoteNotFound) {
		t.Fatalf("没声称的账户：err = %v", err)
	}
	if _, err := fixture.handover.ConcludeByCustomer(ctx, concludeCommand(t, "SYN-HANDOVER-ABSENT", handoverAccount, "SYN-REQ-7", domain.BindingConfirmed)); !errors.Is(err, application.ErrDeliveryHandoverNoteNotFound) {
		t.Fatalf("不存在的交接单：err = %v", err)
	}
	if _, err := fixture.handover.ConcludeByCustomer(ctx, concludeCommand(t, "SYN-HANDOVER-20", handoverAccount, " ", domain.BindingConfirmed)); !errors.Is(err, application.ErrDeliveryHandoverCommandRejected) {
		t.Fatalf("缺来源请求键：err = %v", err)
	}
	if len(fixture.scopeBells.rung) != 0 {
		t.Fatalf("有一个账户否认，却响了门铃 %d 封", len(fixture.scopeBells.rung))
	}
}

// Covers: `AT-NO-074`、票 12 完成判据第四格——交接单声称 A、B，只确认 A 时范围不成立、不交门铃；B 否认后不再成立，名下的待识别照旧
// 待识别，可以归入另一张交接单；那一张唯一声称的账户得到确认时范围成立，同一笔自投门铃（租户，交接单）一封。
func TestAScopeNeedsEveryClaimConfirmedAndADenialFreesTheMembers(t *testing.T) {
	fixture := newHandoverFixture(t)
	ctx := context.Background()
	if _, err := fixture.handover.Open(ctx, openCommand(t, "SYN-HANDOVER-30", handoverAccount, otherAccount)); err != nil {
		t.Fatal(err)
	}
	fixture.receivePendingUnit(t, "delivery-30", "unit-30")
	if result, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-30", "SYN-HANDOVER-30", "unit-30")); err != nil || result.Outcome() != application.DeliveryHandoverUnitAssigned {
		t.Fatalf("归入：%v %v", result.Outcome(), err)
	}
	first, err := fixture.handover.ConfirmByOperationDecision(ctx, confirmCommand(t, "SYN-HANDOVER-30", handoverAccount))
	expectHandoverOutcome(t, "确认 A", first, err, application.DeliveryHandoverBindingConfirmed)
	if _, established := first.ScopeEstablishedAt(); established || len(fixture.scopeBells.rung) != 0 {
		t.Fatalf("只确认 A：成立 %v，门铃 %d", established, len(fixture.scopeBells.rung))
	}
	if _, err := fixture.handover.Close(ctx, closeCommand(t, "SYN-CLOSE-30", "SYN-HANDOVER-30")); err != nil {
		t.Fatal(err)
	}
	denied, err := fixture.handover.ConcludeByCustomer(ctx, concludeCommand(t, "SYN-HANDOVER-30", otherAccount, "SYN-REQ-30", domain.BindingDenied))
	expectHandoverOutcome(t, "B 否认", denied, err, application.DeliveryHandoverBindingDenied)
	if _, established := denied.ScopeEstablishedAt(); established || len(fixture.scopeBells.rung) != 0 {
		t.Fatalf("B 否认之后：成立 %v，门铃 %d", established, len(fixture.scopeBells.rung))
	}

	if _, err := fixture.handover.Open(ctx, openCommand(t, "SYN-HANDOVER-31", handoverAccount)); err != nil {
		t.Fatal(err)
	}
	moved, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-31", "SYN-HANDOVER-31", "unit-30"))
	expectHandoverOutcome(t, "否认之后另行归入", moved, err, application.DeliveryHandoverUnitAssigned)
	established, err := fixture.handover.ConfirmByOperationDecision(ctx, confirmCommand(t, "SYN-HANDOVER-31", handoverAccount))
	expectHandoverOutcome(t, "另一张确认", established, err, application.DeliveryHandoverBindingConfirmed)
	if len(fixture.scopeBells.rung) != 1 {
		t.Fatalf("范围成立后门铃 %d 封, want 1", len(fixture.scopeBells.rung))
	}
	intent := fixture.scopeBells.rung[0]
	if intent.Tenant.String() != "tenant-1" || intent.Note.String() != "SYN-HANDOVER-31" || intent.Version.String() != "" {
		t.Fatalf("门铃 = %+v, want（租户，交接单）", intent)
	}
	if _, err := fixture.handover.ConfirmByOperationDecision(ctx, confirmCommand(t, "SYN-HANDOVER-31", handoverAccount)); err != nil {
		t.Fatal(err)
	}
	if len(fixture.scopeBells.rung) != 1 {
		t.Fatal("确认重投又响了一封门铃")
	}
}

// Covers: ADR-0212 决定六「实物归入一张范围已成立的交接单时，同一笔自投一封（租户，那一版判断版本）」——运营决定先于归入确认（柜台上
// 先确认、再逐件归入），此后每归入一件就为那一件的当前有效版本响一封，重放不再响；门铃入不了队就不形成答案。
func TestAssigningIntoAnEstablishedNoteRingsForThatUnit(t *testing.T) {
	fixture := newHandoverFixture(t)
	ctx := context.Background()
	if _, err := fixture.handover.Open(ctx, openCommand(t, "SYN-HANDOVER-40", handoverAccount)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.handover.ConfirmByOperationDecision(ctx, confirmCommand(t, "SYN-HANDOVER-40", handoverAccount)); err != nil {
		t.Fatal(err)
	}
	if len(fixture.scopeBells.rung) != 1 {
		t.Fatalf("确认之后门铃 %d 封", len(fixture.scopeBells.rung))
	}
	pending := fixture.receivePendingUnit(t, "delivery-40", "unit-40")
	assigned, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-40", "SYN-HANDOVER-40", "unit-40"))
	expectHandoverOutcome(t, "归入已成立的交接单", assigned, err, application.DeliveryHandoverUnitAssigned)
	if len(fixture.scopeBells.rung) != 2 {
		t.Fatalf("归入之后门铃 %d 封, want 2", len(fixture.scopeBells.rung))
	}
	intent := fixture.scopeBells.rung[1]
	if intent.Note.String() != "SYN-HANDOVER-40" || intent.Version != pending.JudgmentVersion {
		t.Fatalf("归入的门铃 = %+v, want 指名那一件的判断版本 %s", intent, pending.JudgmentVersion)
	}
	if _, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-40", "SYN-HANDOVER-40", "unit-40")); err != nil {
		t.Fatal(err)
	}
	if len(fixture.scopeBells.rung) != 2 {
		t.Fatal("归入重放又响了一封")
	}

	fixture.receivePendingUnit(t, "delivery-41", "unit-41")
	fixture.scopeBells.err = errors.New("outbox unreachable")
	if _, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-41", "SYN-HANDOVER-40", "unit-41")); err == nil {
		t.Fatal("门铃入不了队却答了归入")
	}
}

// Covers: 构造期拒缺件——漏装的册、收寄库、授权交付判断、门铃或时钟到第一份交接单才暴露，会被当成一次依赖故障。
func TestDeliveryHandoverNoteHandlerRefusesMissingDependencies(t *testing.T) {
	fixture := newHandoverFixture(t)
	for name, mutate := range map[string]func(*application.DeliveryHandoverDeps){
		"册":      func(deps *application.DeliveryHandoverDeps) { deps.Notes = nil },
		"收寄库":    func(deps *application.DeliveryHandoverDeps) { deps.Receptions = nil },
		"授权交付判断": func(deps *application.DeliveryHandoverDeps) { deps.AuthorizedDelivery = nil },
		"门铃":     func(deps *application.DeliveryHandoverDeps) { deps.Doorbell = nil },
		"时钟":     func(deps *application.DeliveryHandoverDeps) { deps.Clock = nil },
	} {
		deps := fixture.deps()
		mutate(&deps)
		if _, err := application.NewDeliveryHandoverNoteHandler(deps); err == nil {
			t.Fatalf("缺%s也构造成了", name)
		}
	}
	if application.DeliveryHandoverOutcomeInvalid.String() != "" {
		t.Fatal("无效结果答出了名字")
	}
}
