package domain_test

import (
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件的节点代码都是合成串，证据只记 `S`。

func handoverNodes(t *testing.T, releasing, receiving, onward string) domain.HandoverNodeDeclarations {
	t.Helper()
	var nodes domain.HandoverNodeDeclarations
	if releasing != "" {
		nodes.Releasing = mustValue(t, domain.NewNetworkNodeReference, releasing)
	}
	if receiving != "" {
		nodes.Receiving = mustValue(t, domain.NewNetworkNodeReference, receiving)
	}
	if onward != "" {
		nodes.Onward = mustValue(t, domain.NewNetworkNodeReference, onward)
	}
	return nodes
}

// Covers: AT-TF-217——三格节点声明随交接原样保存、各自可缺席，裁决不限制它们：拒收与待确认同样带得上；不声明的那格
// 读回「没有声明」，不由交出方或接收方补出。
func TestAHandoverKeepsTheNodesItDeclaresWhateverItsVerdict(t *testing.T) {
	for name, verdict := range map[string]domain.HandoverVerdict{
		"handed over": domain.ObjectHandedOver,
		"refused":     domain.HandoverRefused,
		"pending":     domain.HandoverPendingConfirmation,
	} {
		t.Run(name, func(t *testing.T) {
			spec := handoverSpec(t, "parcel-n", verdict)
			spec.Nodes = handoverNodes(t, "SYN-NODE-SHA-HUB", "", "SYN-NODE-SZX-GW")
			handover, err := domain.FormTransportHandover(spec)
			if err != nil {
				t.Fatalf("form: %v", err)
			}
			if node, present := handover.ReleasingNode(); !present || node.String() != "SYN-NODE-SHA-HUB" {
				t.Fatalf("releasing node = %q present=%v", node, present)
			}
			if node, present := handover.ReceivingNode(); present {
				t.Fatalf("没声明的接收节点读回了 %q", node)
			}
			if node, present := handover.OnwardNode(); !present || node.String() != "SYN-NODE-SZX-GW" {
				t.Fatalf("onward node = %q present=%v", node, present)
			}
		})
	}

	t.Run("no declaration at all", func(t *testing.T) {
		handover, err := domain.FormTransportHandover(handoverSpec(t, "parcel-m", domain.ObjectHandedOver))
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if handover.Nodes().Declared() {
			t.Fatal("一格都没声明却读作声明了节点")
		}
		for name, read := range map[string]func() (domain.NetworkNodeReference, bool){
			"releasing": handover.ReleasingNode,
			"receiving": handover.ReceivingNode,
			"onward":    handover.OnwardNode,
		} {
			if node, present := read(); present {
				t.Fatalf("%s node = %q，want 缺席", name, node)
			}
		}
	})
}

// Covers: AT-TF-220——更正改的是判断，不是那次交接发生过什么：新版本照前版承继三格节点声明，前版原样不动；读回的
// 更正版本也带着承继来的声明。
func TestACorrectionInheritsThePredecessorsNodeDeclarations(t *testing.T) {
	spec := handoverSpec(t, "parcel-c", domain.ObjectHandedOver)
	spec.Nodes = handoverNodes(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GW", "")
	original, err := domain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	corrected, err := original.Correct(domain.HandoverCorrection{
		Verdict:     domain.HandoverRefused,
		Basis:       mustValue(t, domain.NewHandoverBasisReference, "basis-late"),
		Version:     mustValue(t, domain.NewHandoverResultVersion, "handover-result/parcel-c/v2"),
		CorrectedAt: handoverJudgedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if corrected.Nodes() != original.Nodes() {
		t.Fatalf("更正版本节点 = %+v，want 承继前版 %+v", corrected.Nodes(), original.Nodes())
	}

	predecessor, _ := corrected.Corrects()
	correctedAt, _ := corrected.CorrectedAt()
	rehydrated, err := domain.RehydrateTransportHandover(domain.RehydrateTransportHandoverSpec{
		TenantID:    corrected.TenantID(),
		Object:      corrected.Object(),
		Scope:       corrected.Scope(),
		ReleasedBy:  corrected.ReleasedBy(),
		ReceivedBy:  corrected.ReceivedBy(),
		Verdict:     corrected.Verdict(),
		Basis:       mustValue(t, domain.NewHandoverBasisReference, "basis-late"),
		Version:     corrected.Version(),
		JudgedAt:    corrected.JudgedAt(),
		Nodes:       corrected.Nodes(),
		Corrects:    predecessor,
		CorrectedAt: correctedAt,
	})
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if rehydrated.Nodes() != original.Nodes() {
		t.Fatalf("读回的更正版本节点 = %+v，want %+v", rehydrated.Nodes(), original.Nodes())
	}
}

// Covers: AT-TF-216、AT-TF-217——一格都没声明时摘要仍是 TFC-1、文档里没有节点键（与节点进文法之前算出的同一串，见
// TestCommandPayloadsRoundTripAsTFC1 的 handover 一例）；声明了任一格就写 TFC-2，三格都写、没声明的写 null，哪一格
// 缺席本身是内容。
func TestHandoverNodeDeclarationsCanonicalizeAsTFC2(t *testing.T) {
	at := tfcInstant()
	document, digest, err := domain.CanonicalizeHandoverPayload(domain.ObjectHandedOver, "REL", "RCV", "RE", "VE", "RULE", "", at,
		handoverNodes(t, "SYN-NODE-SHA-HUB", "", "SYN-NODE-SZX-GW"))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	const wantDocument = `{"canonicalization":"TFC-2","face":"REGISTER_TRANSPORT_HANDOVER","verdict":"HANDED_OVER","released_by":"REL","received_by":"RCV","releasing_evidence":"RE","receiving_evidence":"VE","rule":"RULE","basis":"","judged_at":"2026-03-05T02:11:12.345678901Z","releasing_node":"SYN-NODE-SHA-HUB","receiving_node":null,"onward_node":"SYN-NODE-SZX-GW"}`
	if string(document) != wantDocument {
		t.Fatalf("document\n got %s\nwant %s", document, wantDocument)
	}
	const wantDigest = "TFC-2:685ca638c7b34e700a0482f9d305ff35f5825b36b36ffb4e7d1a0bcd2a457c44"
	if digest != wantDigest {
		t.Fatalf("digest %s, want %s", digest, wantDigest)
	}

	digestOf := func(nodes domain.HandoverNodeDeclarations) string {
		t.Helper()
		_, digest, err := domain.CanonicalizeHandoverPayload(domain.ObjectHandedOver, "REL", "RCV", "RE", "VE", "RULE", "", at, nodes)
		if err != nil {
			t.Fatalf("canonicalize: %v", err)
		}
		return digest
	}
	undeclared := digestOf(domain.HandoverNodeDeclarations{})
	if !strings.HasPrefix(undeclared, "TFC-1:") {
		t.Fatalf("不声明节点的摘要 = %s，want 仍是 TFC-1", undeclared)
	}
	seen := map[string]string{undeclared: "none"}
	for name, nodes := range map[string]domain.HandoverNodeDeclarations{
		"releasing only": handoverNodes(t, "SYN-NODE-A", "", ""),
		"receiving only": handoverNodes(t, "", "SYN-NODE-A", ""),
		"onward only":    handoverNodes(t, "", "", "SYN-NODE-A"),
		"another node":   handoverNodes(t, "SYN-NODE-B", "", ""),
	} {
		got := digestOf(nodes)
		if previous, clash := seen[got]; clash {
			t.Fatalf("%s 与 %s 算出同一摘要：哪一格声明了什么都是内容", name, previous)
		}
		seen[got] = name
	}
}

// Covers: AT-TF-218、AT-TF-219——交接口的比对在 TFC-1 与无版本两形之上认 TFC-2：同形同串是重放；在册那一版声明了节点
// 而这一份没有，或这一份声明了节点而在册的是没有节点一格的旧形，都是冲突——旧形之下节点没有写法，不按丢掉节点认作重放。
func TestStoredHandoverDigestComparesNodeDeclarationsAsContent(t *testing.T) {
	const unversioned = "bb"
	cases := []struct {
		name            string
		stored, current string
		want            domain.StoredDigestComparison
	}{
		{"TFC-2 same content", "TFC-2:aa", "TFC-2:aa", domain.SamePayload},
		{"TFC-2 different nodes", "TFC-2:aa", "TFC-2:cc", domain.DifferentPayload},
		{"stored declared nodes, this one declares none", "TFC-2:aa", "TFC-1:aa", domain.DifferentPayload},
		{"TFC-1 row, this one declares nodes", "TFC-1:aa", "TFC-2:aa", domain.DifferentPayload},
		{"unversioned row, this one declares nodes", "bb", "TFC-2:aa", domain.DifferentPayload},
		{"TFC-1 row replayed without nodes", "TFC-1:aa", "TFC-1:aa", domain.SamePayload},
		{"unversioned row replayed without nodes", "bb", "TFC-1:aa", domain.SamePayload},
		{"unknown shape stays unknown", "TFC-9:aa", "TFC-2:aa", domain.UnknownPayloadShape},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := domain.CompareStoredHandoverDigest(testCase.stored, testCase.current, unversioned); got != testCase.want {
				t.Fatalf("CompareStoredHandoverDigest(%q, %q) = %d, want %d", testCase.stored, testCase.current, got, testCase.want)
			}
		})
	}
}
