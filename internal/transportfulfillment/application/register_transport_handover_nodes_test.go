package application_test

import (
	"context"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件的节点代码都是合成串，证据只记 `S`。

func registerHandoverWithNodes(t *testing.T) application.RegisterTransportHandoverCommand {
	t.Helper()
	command := registerHandoverCommand(t)
	command.ReleasingNode = "SYN-NODE-SHA-HUB"
	command.ReceivingNode = "SYN-NODE-SZX-GW"
	command.OnwardNode = "SYN-NODE-SIN-HUB"
	return command
}

func onlyHandoverRecord(t *testing.T, registry *handoverRegistryDouble) ports.TransportHandoverRecord {
	t.Helper()
	if len(registry.records) != 1 {
		t.Fatalf("登记册里有 %d 条交接，want 1", len(registry.records))
	}
	for _, record := range registry.records {
		return record
	}
	return ports.TransportHandoverRecord{}
}

// Covers: AT-TF-217、AT-TF-218、AT-TF-219——声明了节点的首登照登，三格原样存进记录、随意图交下游，摘要写 TFC-2；
// 原样重放答已有版本并交回在册那一版的声明；同键换一个节点、撤掉一个节点或撤掉全部节点都答冲突，在册那一版不动。
func TestDeclaredHandoverNodesAreContentOfTheRegistration(t *testing.T) {
	fixture := newHandoverFixture(t)
	command := registerHandoverWithNodes(t)

	first, err := fixture.handler.Register(context.Background(), command)
	wantOutcome(t, first.Outcome(), err, application.HandoverRegistered)
	stored := onlyHandoverRecord(t, fixture.registry)
	if !strings.HasPrefix(stored.ContentDigest, "TFC-2:") {
		t.Fatalf("声明了节点的首登摘要 = %q，want TFC-2", stored.ContentDigest)
	}
	nodes := stored.Handover.Nodes()
	if nodes.Releasing.String() != "SYN-NODE-SHA-HUB" || nodes.Receiving.String() != "SYN-NODE-SZX-GW" || nodes.Onward.String() != "SYN-NODE-SIN-HUB" {
		t.Fatalf("在册节点 = %+v", nodes)
	}
	if len(fixture.handoff.intents) != 1 || fixture.handoff.intents[0].Record.Handover.Nodes() != nodes {
		t.Fatalf("交下游的意图没带上在册那一版的节点声明：%+v", fixture.handoff.intents)
	}

	replay, err := fixture.handler.Register(context.Background(), command)
	wantOutcome(t, replay.Outcome(), err, application.HandoverExistingVersion)
	if record, _ := replay.Record(); record.Handover.Nodes() != nodes {
		t.Fatalf("重放交回的节点 = %+v，want 在册那一版", record.Handover.Nodes())
	}

	for name, change := range map[string]func(*application.RegisterTransportHandoverCommand){
		"another receiving node": func(command *application.RegisterTransportHandoverCommand) {
			command.ReceivingNode = "SYN-NODE-SZX-GW-2"
		},
		"onward node withdrawn": func(command *application.RegisterTransportHandoverCommand) { command.OnwardNode = "" },
		"every node withdrawn": func(command *application.RegisterTransportHandoverCommand) {
			command.ReleasingNode, command.ReceivingNode, command.OnwardNode = "", "", ""
		},
		"releasing and onward swapped": func(command *application.RegisterTransportHandoverCommand) {
			command.ReleasingNode, command.OnwardNode = command.OnwardNode, command.ReleasingNode
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := command
			change(&changed)
			result, err := fixture.handler.Register(context.Background(), changed)
			wantOutcome(t, result.Outcome(), err, application.HandoverRegistrationConflict)
			if after := onlyHandoverRecord(t, fixture.registry); after.Handover.Nodes() != nodes || after.ContentDigest != stored.ContentDigest {
				t.Fatalf("冲突顶替了在册那一版：%+v", after.Handover.Nodes())
			}
		})
	}
}

// Covers: AT-TF-216、AT-TF-218、AT-TF-219——不声明节点的首登摘要仍是 TFC-1；节点进文法之前入库的行（TFC-1 与更早的
// 无版本摘要两形）照原内容重放答已有版本，同键补报节点答冲突——旧形里没有节点一格，补报的声明不被当成重放吞掉。
func TestRowsStoredBeforeNodeDeclarationsReplayWithoutNodesAndConflictWithThem(t *testing.T) {
	for name, rewrite := range map[string]func(string) string{
		"TFC-1 row": func(stored string) string { return stored },
		// registerHandoverCommand 在基 1121ba61 上落下的无版本摘要，定值同 handoverUnversionedCase。
		"unversioned row": func(string) string { return "ce748b001b582166475c82743ba05fa163afaefd439dcc396c8a3f3f84281cac" },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newHandoverFixture(t)
			command := registerHandoverCommand(t)
			first, err := fixture.handler.Register(context.Background(), command)
			wantOutcome(t, first.Outcome(), err, application.HandoverRegistered)
			_, rewriteDigest := onlyDigest(t, fixture.registry.records,
				func(record *ports.TransportHandoverRecord) *string { return &record.ContentDigest })
			stored := onlyHandoverRecord(t, fixture.registry).ContentDigest
			if !strings.HasPrefix(stored, "TFC-1:") {
				t.Fatalf("不声明节点的首登摘要 = %q，want TFC-1", stored)
			}
			rewriteDigest(rewrite(stored))

			replay, err := fixture.handler.Register(context.Background(), command)
			wantOutcome(t, replay.Outcome(), err, application.HandoverExistingVersion)
			if record, _ := replay.Record(); record.Handover.Nodes().Declared() {
				t.Fatalf("存量行读回带了节点：%+v", record.Handover.Nodes())
			}

			late := command
			late.ReleasingNode = "SYN-NODE-SHA-HUB"
			conflict, err := fixture.handler.Register(context.Background(), late)
			wantOutcome(t, conflict.Outcome(), err, application.HandoverRegistrationConflict)
		})
	}
}

// Covers: AT-TF-220——更正命令不带节点，新版本照前版承继三格声明；更正之后原样重投更正仍答已有版本。
func TestAHandoverCorrectionInheritsTheDeclaredNodes(t *testing.T) {
	fixture := newHandoverFixture(t)
	registered, err := fixture.handler.Register(context.Background(), registerHandoverWithNodes(t))
	wantOutcome(t, registered.Outcome(), err, application.HandoverRegistered)
	original, _ := registered.Record()

	correction := correctHandoverCommand(t, "handover-result/parcel-1/v1", "handover-result/parcel-1/v2")
	corrected, err := fixture.handler.Correct(context.Background(), correction)
	wantOutcome(t, corrected.Outcome(), err, application.HandoverCorrected)
	record, _ := corrected.Record()
	if record.Handover.Nodes() != original.Handover.Nodes() {
		t.Fatalf("更正版本节点 = %+v，want 承继前版 %+v", record.Handover.Nodes(), original.Handover.Nodes())
	}
	replay, err := fixture.handler.Correct(context.Background(), correction)
	wantOutcome(t, replay.Outcome(), err, application.HandoverExistingVersion)
}

// Covers: AT-TF-222——节点格空白读作没有声明，与不带这一格同答同摘要；含 NUL 的节点存不下，答未受理、不落册。
func TestHandoverNodeDeclarationsBlankIsAbsentAndUnstorableIsNotAccepted(t *testing.T) {
	fixture := newHandoverFixture(t)
	blank := registerHandoverCommand(t)
	blank.ReleasingNode, blank.ReceivingNode, blank.OnwardNode = " ", "\t", ""
	result, err := fixture.handler.Register(context.Background(), blank)
	wantOutcome(t, result.Outcome(), err, application.HandoverRegistered)
	if record, _ := result.Record(); record.Handover.Nodes().Declared() || !strings.HasPrefix(record.ContentDigest, "TFC-1:") {
		t.Fatalf("空白节点格读成了声明：%+v %s", record.Handover.Nodes(), record.ContentDigest)
	}
	replay, err := fixture.handler.Register(context.Background(), registerHandoverCommand(t))
	wantOutcome(t, replay.Outcome(), err, application.HandoverExistingVersion)

	unstorable := registerHandoverCommand(t)
	unstorable.Version = "handover-result/parcel-1/v9"
	unstorable.OnwardNode = "SYN-NODE\x00"
	refused, err := fixture.handler.Register(context.Background(), unstorable)
	wantOutcome(t, refused.Outcome(), err, application.HandoverNotAccepted)
	if len(fixture.registry.records) != 1 {
		t.Fatalf("未受理的那一份落了册：%d 条", len(fixture.registry.records))
	}
}
