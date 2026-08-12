package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var handoverJudgedAt = time.Date(2026, 8, 10, 14, 0, 0, 0, time.UTC)

func handoverSpec(t *testing.T, object string, verdict domain.HandoverVerdict) domain.TransportHandoverSpec {
	t.Helper()
	spec := domain.TransportHandoverSpec{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Object:     mustValue(t, domain.NewCarriedObjectReference, object),
		Scope:      mustValue(t, domain.NewHandoverScopeReference, "handover-scope-1"),
		ReleasedBy: mustValue(t, domain.NewHandoverPartyReference, "node-1"),
		ReceivedBy: mustValue(t, domain.NewHandoverPartyReference, "carrier-1"),
		Verdict:    verdict,
		Version:    mustValue(t, domain.NewHandoverResultVersion, "handover-result/"+object+"/v1"),
		JudgedAt:   handoverJudgedAt,
	}
	if verdict == domain.ObjectHandedOver {
		spec.ReleasingEvidence = mustValue(t, domain.NewHandoverEvidenceReference, "evidence-release-"+object)
		spec.ReceivingEvidence = mustValue(t, domain.NewHandoverEvidenceReference, "evidence-receive-"+object)
		spec.Rule = mustValue(t, domain.NewHandoverRuleReference, "handover-rule/v1")
	} else {
		spec.Basis = mustValue(t, domain.NewHandoverBasisReference, "basis-"+object)
	}
	return spec
}

// Covers: `AT-TF-051`「节点交出和运输方接收均满足规则 → 逐对象形成已交接」与 UC-TF-005
// 结果契约「必须保存双方证据、对象范围、业务时间、结果版本」——缺任一侧证据或规则都
// 立不成已交接；已交接不携带拒收/待确认依据。
func TestAHandedOverObjectCarriesBothEvidencesAndTransfersControl(t *testing.T) {
	handover, err := domain.FormTransportHandover(handoverSpec(t, "parcel-1", domain.ObjectHandedOver))
	if err != nil {
		t.Fatalf("form transport handover: %v", err)
	}
	if !handover.TransfersControl() {
		t.Fatal("已交接没有转移控制")
	}
	reference, present := handover.TransferOutBasis()
	if !present || reference != "TRANSPORT-HANDOVER/handover-result/parcel-1/v1" {
		t.Fatalf("transfer out basis = %q present=%v", reference, present)
	}
	if _, present := handover.ReleasingEvidence(); !present {
		t.Fatal("已交接丢了交出方证据")
	}
	if _, present := handover.ReceivingEvidence(); !present {
		t.Fatal("已交接丢了接收方证据")
	}
	if _, present := handover.Basis(); present {
		t.Fatal("已交接凭空带上了拒收/待确认依据")
	}
	if !handover.JudgedAt().Equal(handoverJudgedAt) {
		t.Fatalf("judged at = %s", handover.JudgedAt())
	}

	broken := map[string]func(*domain.TransportHandoverSpec){
		"no releasing evidence": func(spec *domain.TransportHandoverSpec) {
			spec.ReleasingEvidence = domain.HandoverEvidenceReference{}
		},
		"no receiving evidence": func(spec *domain.TransportHandoverSpec) {
			spec.ReceivingEvidence = domain.HandoverEvidenceReference{}
		},
		"no rule": func(spec *domain.TransportHandoverSpec) { spec.Rule = domain.HandoverRuleReference{} },
		"handed over carrying a basis": func(spec *domain.TransportHandoverSpec) {
			spec.Basis = mustValue(t, domain.NewHandoverBasisReference, "basis-x")
		},
		"no version":   func(spec *domain.TransportHandoverSpec) { spec.Version = domain.HandoverResultVersion{} },
		"no judged at": func(spec *domain.TransportHandoverSpec) { spec.JudgedAt = time.Time{} },
		"no scope":     func(spec *domain.TransportHandoverSpec) { spec.Scope = domain.HandoverScopeReference{} },
		"no receiver":  func(spec *domain.TransportHandoverSpec) { spec.ReceivedBy = domain.HandoverPartyReference{} },
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			spec := handoverSpec(t, "parcel-x", domain.ObjectHandedOver)
			breakSpec(&spec)
			if _, err := domain.FormTransportHandover(spec); !errors.Is(err, domain.ErrInvalidTransportHandover) {
				t.Fatalf("error = %v, want ErrInvalidTransportHandover", err)
			}
		})
	}
}

// Covers: CONTEXT「已拒收只结束本次交接尝试，不转移控制」与 `AT-TF-053`「双方证据冲突
// → 保持待确认，原控制不转移」——两格都给不出 TransferOutBasis，node-operations 的
// TransferOutReference 从源头就构造不出来；没有依据的拒收/待确认立不成。
func TestRefusedAndUnconfirmedDoNotTransferControlOut(t *testing.T) {
	for name, verdict := range map[string]domain.HandoverVerdict{
		"refused":     domain.HandoverRefused,
		"unconfirmed": domain.HandoverPendingConfirmation,
	} {
		t.Run(name, func(t *testing.T) {
			handover, err := domain.FormTransportHandover(handoverSpec(t, "parcel-1", verdict))
			if err != nil {
				t.Fatalf("form handover: %v", err)
			}
			if handover.TransfersControl() {
				t.Fatalf("%s 竟然转移了控制", verdict)
			}
			if reference, present := handover.TransferOutBasis(); present {
				t.Fatalf("%s 交出了转出引用 %q——NO 侧就能拿它结束控制", verdict, reference)
			}
			if _, present := handover.Basis(); !present {
				t.Fatal("拒收/待确认丢了依据")
			}
		})

		t.Run(name+" without a basis is refused", func(t *testing.T) {
			spec := handoverSpec(t, "parcel-1", verdict)
			spec.Basis = domain.HandoverBasisReference{}
			if _, err := domain.FormTransportHandover(spec); !errors.Is(err, domain.ErrInvalidTransportHandover) {
				t.Fatalf("error = %v; 没有原因的%s与数据丢失无从分辨", err, verdict)
			}
		})
	}
}

// Covers: CONTEXT 交接硬句「整批/整车/整袋结论只能由对象级结果派生」与 `AT-TF-052`
// 「一批对象只有部分被接收 → 已接收对象进入履约，拒收对象继续由节点控制」——汇总只有
// 计数与派生问答，混合结果不塌成一个批次状态；跨范围混入被拒。
func TestBatchConclusionsDeriveOnlyFromObjectResults(t *testing.T) {
	handed, err := domain.FormTransportHandover(handoverSpec(t, "parcel-1", domain.ObjectHandedOver))
	if err != nil {
		t.Fatalf("form handed over: %v", err)
	}
	refused, err := domain.FormTransportHandover(handoverSpec(t, "parcel-2", domain.HandoverRefused))
	if err != nil {
		t.Fatalf("form refused: %v", err)
	}
	unconfirmed, err := domain.FormTransportHandover(handoverSpec(t, "parcel-3", domain.HandoverPendingConfirmation))
	if err != nil {
		t.Fatalf("form unconfirmed: %v", err)
	}

	summary, err := domain.SummarizeHandovers([]domain.TransportHandover{handed, refused, unconfirmed})
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	// 计数锚定本夹具：同一 handover-scope-1 下三对象（判于 2026-08-10T14:00Z）。
	if summary.Total() != 3 || summary.HandedOver() != 1 || summary.Refused() != 1 || summary.Unconfirmed() != 1 {
		t.Fatalf("summary = %+v; 成员差异被吞并", summary)
	}
	if summary.AllHandedOver() {
		t.Fatal("部分接收被读成整批成功")
	}

	t.Run("an all-handed-over scope derives success", func(t *testing.T) {
		second, err := domain.FormTransportHandover(handoverSpec(t, "parcel-4", domain.ObjectHandedOver))
		if err != nil {
			t.Fatalf("form second handed over: %v", err)
		}
		summary, err := domain.SummarizeHandovers([]domain.TransportHandover{handed, second})
		if err != nil {
			t.Fatalf("summarize: %v", err)
		}
		if !summary.AllHandedOver() {
			t.Fatal("全部已交接没有派生出整批成功")
		}
	})

	t.Run("an empty scope has no conclusion", func(t *testing.T) {
		if _, err := domain.SummarizeHandovers(nil); !errors.Is(err, domain.ErrInvalidTransportHandover) {
			t.Fatalf("error = %v, want ErrInvalidTransportHandover", err)
		}
	})

	t.Run("results from another scope are rejected", func(t *testing.T) {
		foreignSpec := handoverSpec(t, "parcel-9", domain.ObjectHandedOver)
		foreignSpec.Scope = mustValue(t, domain.NewHandoverScopeReference, "handover-scope-2")
		foreign, err := domain.FormTransportHandover(foreignSpec)
		if err != nil {
			t.Fatalf("form foreign handover: %v", err)
		}
		if _, err := domain.SummarizeHandovers([]domain.TransportHandover{handed, foreign}); !errors.Is(err, domain.ErrMixedHandoverScopes) {
			t.Fatalf("error = %v, want ErrMixedHandoverScopes", err)
		}
	})

	t.Run("the verdict set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, verdict := range []domain.HandoverVerdict{
			domain.ObjectHandedOver, domain.HandoverRefused, domain.HandoverPendingConfirmation,
		} {
			label := verdict.String()
			if label == "" {
				t.Fatalf("verdict %d has no label", verdict)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 3 {
			t.Fatalf("verdict labels collapsed into %d", len(labels))
		}
		if domain.HandoverVerdict(len(labels)+1).String() != "" {
			t.Fatal("第四个交接取值带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: `AT-TF-062`「迟到更正使原交接证据失效 → 保留原判断及全部后续事实」与 CONTEXT
// 「来源证据被更正时，保留原事实和原判断，形成失效、替代及重新派生结果」——更正回指
// 前身、原判断不动；被更正成拒收的新版本给不出转出引用（NO 侧已按原版本转出的控制
// 不可逆，来源链重算由后续处置表达）。
func TestAHandoverCorrectionFormsANewVersionWithoutOverwriting(t *testing.T) {
	original, err := domain.FormTransportHandover(handoverSpec(t, "parcel-1", domain.ObjectHandedOver))
	if err != nil {
		t.Fatalf("form original handover: %v", err)
	}
	correctedAt := handoverJudgedAt.Add(36 * time.Hour)

	corrected, err := original.Correct(domain.HandoverCorrection{
		Verdict:     domain.HandoverRefused,
		Basis:       mustValue(t, domain.NewHandoverBasisReference, "basis-late-correction"),
		Version:     mustValue(t, domain.NewHandoverResultVersion, "handover-result/parcel-1/v2"),
		CorrectedAt: correctedAt,
	})
	if err != nil {
		t.Fatalf("correct handover: %v", err)
	}
	predecessor, present := corrected.Corrects()
	if !present || predecessor != original.Version() {
		t.Fatalf("corrects = %q present=%v, want v1", predecessor, present)
	}
	if corrected.Verdict() != domain.HandoverRefused {
		t.Fatalf("verdict = %q, want REFUSED", corrected.Verdict())
	}
	if _, leaks := corrected.TransferOutBasis(); leaks {
		t.Fatal("被更正成拒收的新版本仍交出了转出引用")
	}
	if at, present := corrected.CorrectedAt(); !present || !at.Equal(correctedAt) {
		t.Fatalf("corrected at = %v present=%v", at, present)
	}
	if original.Verdict() != domain.ObjectHandedOver {
		t.Fatal("更正改写了原判断")
	}
	if _, present := original.TransferOutBasis(); !present {
		t.Fatal("原版本的转出引用被更正抹掉——历史派生就断了")
	}
	if _, present := original.Corrects(); present {
		t.Fatal("原版本被更正动作反向打上了更正标记")
	}

	t.Run("a chain keeps every predecessor", func(t *testing.T) {
		third, err := corrected.Correct(domain.HandoverCorrection{
			Verdict:           domain.ObjectHandedOver,
			ReleasingEvidence: mustValue(t, domain.NewHandoverEvidenceReference, "evidence-release-recheck"),
			ReceivingEvidence: mustValue(t, domain.NewHandoverEvidenceReference, "evidence-receive-recheck"),
			Rule:              mustValue(t, domain.NewHandoverRuleReference, "handover-rule/v2"),
			Version:           mustValue(t, domain.NewHandoverResultVersion, "handover-result/parcel-1/v3"),
			CorrectedAt:       correctedAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("second correction: %v", err)
		}
		predecessor, _ := third.Corrects()
		if predecessor != corrected.Version() {
			t.Fatalf("chain broke: corrects = %q, want v2", predecessor)
		}
	})

	t.Run("reusing the original version is an overwrite and is refused", func(t *testing.T) {
		if _, err := original.Correct(domain.HandoverCorrection{
			Verdict:     domain.HandoverRefused,
			Basis:       mustValue(t, domain.NewHandoverBasisReference, "basis-x"),
			Version:     original.Version(),
			CorrectedAt: correctedAt,
		}); !errors.Is(err, domain.ErrInvalidTransportHandover) {
			t.Fatalf("error = %v; 沿用原版本号就是覆盖", err)
		}
	})

	t.Run("a correction to handed-over still needs both evidences and the rule", func(t *testing.T) {
		// 三件各自单独缺席，逐一隔离——一次全缺只证明得了「至少查了一件」。
		complete := func() domain.HandoverCorrection {
			return domain.HandoverCorrection{
				Verdict:           domain.ObjectHandedOver,
				ReleasingEvidence: mustValue(t, domain.NewHandoverEvidenceReference, "evidence-release-recheck"),
				ReceivingEvidence: mustValue(t, domain.NewHandoverEvidenceReference, "evidence-receive-recheck"),
				Rule:              mustValue(t, domain.NewHandoverRuleReference, "handover-rule/v2"),
				Version:           mustValue(t, domain.NewHandoverResultVersion, "handover-result/parcel-1/v4"),
				CorrectedAt:       correctedAt.Add(2 * time.Hour),
			}
		}
		missing := map[string]func(*domain.HandoverCorrection){
			"receiving evidence": func(c *domain.HandoverCorrection) { c.ReceivingEvidence = domain.HandoverEvidenceReference{} },
			"releasing evidence": func(c *domain.HandoverCorrection) { c.ReleasingEvidence = domain.HandoverEvidenceReference{} },
			"rule":               func(c *domain.HandoverCorrection) { c.Rule = domain.HandoverRuleReference{} },
		}
		for name, drop := range missing {
			t.Run(name, func(t *testing.T) {
				correction := complete()
				drop(&correction)
				if _, err := corrected.Correct(correction); !errors.Is(err, domain.ErrInvalidTransportHandover) {
					t.Fatalf("error = %v; 更正成已交接的完备性不得低于首次裁决（缺 %s）", err, name)
				}
			})
		}
	})

	t.Run("a correction before the judgment time is refused", func(t *testing.T) {
		if _, err := original.Correct(domain.HandoverCorrection{
			Verdict:     domain.HandoverRefused,
			Basis:       mustValue(t, domain.NewHandoverBasisReference, "basis-x"),
			Version:     mustValue(t, domain.NewHandoverResultVersion, "handover-result/parcel-1/v5"),
			CorrectedAt: handoverJudgedAt.Add(-time.Hour),
		}); !errors.Is(err, domain.ErrInvalidTransportHandover) {
			t.Fatalf("error = %v; 更正不可能发生在被更正的判断之前", err)
		}
	})
}
