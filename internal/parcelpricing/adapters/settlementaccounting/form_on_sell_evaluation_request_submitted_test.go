package settlementaccounting_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	ppinbox "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/settlementaccounting"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件证 ppinbox.SellEvaluationRequestSubmittedConsumer 的处理方（ADR-0190 决定四）：按信封引用向 SA 的 SELL 请求册取
// 那一份 → 译成 SELL·客户费用命令 → 交同一只入口 → 照 BUY 那一路同一套折法落成消费两格。

type sellRequestSourceDouble struct {
	record saports.SellEvaluationRequestRecord
	found  bool
	err    error
	calls  []saports.SellEvaluationRequestKey
}

func (double *sellRequestSourceDouble) FindByID(
	_ context.Context, key saports.SellEvaluationRequestKey,
) (saports.SellEvaluationRequestRecord, bool, error) {
	double.calls = append(double.calls, key)
	return double.record, double.found, double.err
}

func submittedSell() ppinbox.SubmittedSellEvaluationRequest {
	return ppinbox.SubmittedSellEvaluationRequest{TenantID: "tenant-1", SellEvaluationRequestID: "SELLREQ-SYN-1"}
}

func foundSellRecord(t *testing.T) *sellRequestSourceDouble {
	t.Helper()
	return &sellRequestSourceDouble{record: synSellRecord(t, "CNY"), found: true}
}

func newFormOnSellSubmitted(
	t *testing.T,
	source *sellRequestSourceDouble,
	former *formerDouble,
) *adapter.FormOnSellEvaluationRequestSubmittedAdapter {
	t.Helper()
	return newFormOnSellSubmittedWith(t, source, former, &unformedDouble{})
}

func newFormOnSellSubmittedWith(
	t *testing.T,
	source *sellRequestSourceDouble,
	former *formerDouble,
	unformed *unformedDouble,
) *adapter.FormOnSellEvaluationRequestSubmittedAdapter {
	t.Helper()
	handler, err := adapter.NewFormOnSellEvaluationRequestSubmittedAdapter(source, former, unformed, ppdomain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	return handler
}

// Covers: 主路——按（租户 + SELL 请求标识）向 SA 的 SELL 请求册查一次，译出的 SELL·客户费用命令交入口；已形成与已存在
// 都入账不重投（同一请求第二封信封由入口按回指答已存在）。
func TestASellRequestEnvelopeHandsTheTranslatedCommandToTheFormerAndSettles(t *testing.T) {
	for _, outcome := range []ppapplication.FormEvaluationFromRequestOutcome{
		ppapplication.RequestEvaluationFormed, ppapplication.RequestEvaluationExisting,
	} {
		t.Run(outcome.String(), func(t *testing.T) {
			source := foundSellRecord(t)
			former := &formerDouble{result: ppapplication.FormEvaluationFromRequestResult{Outcome: outcome}}

			if err := newFormOnSellSubmitted(t, source, former).HandleSubmittedSellEvaluationRequest(t.Context(), submittedSell()); err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if len(source.calls) != 1 || source.calls[0].TenantID.String() != "tenant-1" || source.calls[0].Request.String() != "SELLREQ-SYN-1" {
				t.Fatalf("source calls = %+v, want one lookup by the envelope's tenant and SELL request ID", source.calls)
			}
			if len(former.commands) != 1 {
				t.Fatalf("former calls = %d, want 1", len(former.commands))
			}
			command := former.commands[0]
			if command.Request.String() != "SELLREQ-SYN-1" || command.Direction != ppdomain.PricingDirectionSell ||
				command.Purpose != ppdomain.PricingPurposeCustomerCharge || command.Sources.Parcel != "SYN-PARCEL-1" ||
				command.Settlement.String() != "CNY" || !command.BasisAt.Equal(sellPricingBasisAt) {
				t.Fatalf("command = %+v", command)
			}
		})
	}
}

// Covers: AT-PP-002 的 SELL 半边——入口答不出评价的三格与 BUY 那一路同样交「评价未形成」后入账，方向是 SELL；未决
// 仍是那一枚哨兵；请求还读不回（读不到或读口答不出）同样未决且不调入口。
func TestEachUnformableSellOutcomeHandsOffUnformedAndWaitingOnesStayUndecided(t *testing.T) {
	candidate := pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-SELL-PLAN-B", "v1")
	for name, tc := range map[string]struct {
		result   ppapplication.FormEvaluationFromRequestResult
		cause    ppports.EvaluationRequestUnformedCause
		mentions string
	}{
		"未配置": {
			result:   ppapplication.FormEvaluationFromRequestResult{Outcome: ppapplication.RequestPriceCardNotConfigured},
			cause:    ppports.UnformedPriceCardNotConfigured,
			mentions: "SELL/CUSTOMER_CHARGE",
		},
		"适用冲突": {
			result: ppapplication.FormEvaluationFromRequestResult{
				Outcome:    ppapplication.RequestPriceCardApplicabilityConflict,
				Candidates: []ppdomain.VersionReference{candidate},
			},
			cause:    ppports.UnformedPriceCardApplicabilityConflict,
			mentions: "SYN-SELL-PLAN-B@v1",
		},
		"输入不可得": {
			result: ppapplication.FormEvaluationFromRequestResult{
				Outcome: ppapplication.RequestPricingInputUnavailable,
				Missing: []string{"parcel-shipment: destination postal code is absent"},
			},
			cause:    ppports.UnformedPricingInputUnavailable,
			mentions: "destination postal code",
		},
	} {
		t.Run(name, func(t *testing.T) {
			unformed := &unformedDouble{}
			former := &formerDouble{result: tc.result}
			err := newFormOnSellSubmittedWith(t, foundSellRecord(t), former, unformed).HandleSubmittedSellEvaluationRequest(t.Context(), submittedSell())
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if len(unformed.intents) != 1 || unformed.intents[0].Cause != tc.cause ||
				unformed.intents[0].Direction != ppdomain.PricingDirectionSell || unformed.intents[0].Request.String() != "SELLREQ-SYN-1" {
				t.Fatalf("intents = %+v", unformed.intents)
			}
			if !strings.Contains(strings.Join(unformed.intents[0].Detail, "; "), tc.mentions) {
				t.Fatalf("说明 %v 没提 %q", unformed.intents[0].Detail, tc.mentions)
			}
		})
	}

	undecided := &formerDouble{result: ppapplication.FormEvaluationFromRequestResult{
		Outcome: ppapplication.RequestEvaluationUndecided,
		Reason:  ppapplication.RequestEvaluationRegistryUnavailable,
	}}
	err := newFormOnSellSubmitted(t, foundSellRecord(t), undecided).HandleSubmittedSellEvaluationRequest(t.Context(), submittedSell())
	if !errors.Is(err, adapter.ErrEvaluationFormationUndecided) || !strings.Contains(err.Error(), "EVALUATION_REGISTRY_UNAVAILABLE") {
		t.Fatalf("入口未决 err = %v", err)
	}

	for name, source := range map[string]*sellRequestSourceDouble{
		"读不回":   {found: false},
		"读口答不出": {err: errors.New("connection refused")},
	} {
		t.Run(name, func(t *testing.T) {
			former := &formerDouble{}
			err := newFormOnSellSubmitted(t, source, former).HandleSubmittedSellEvaluationRequest(t.Context(), submittedSell())
			if !errors.Is(err, adapter.ErrEvaluationRequestNotVisible) {
				t.Fatalf("err = %v, want ErrEvaluationRequestNotVisible", err)
			}
			if len(former.commands) != 0 {
				t.Fatal("the former was called without a record")
			}
		})
	}
}

// Covers: 引用坏了、记录译不出、入口答未受理都原样上抛，不译成未决——两侧词汇表分歧，重投不会变好。
func TestABrokenSellReferenceOrAnUntranslatableRecordIsLoud(t *testing.T) {
	source := foundSellRecord(t)
	err := newFormOnSellSubmitted(t, source, &formerDouble{}).HandleSubmittedSellEvaluationRequest(t.Context(),
		ppinbox.SubmittedSellEvaluationRequest{TenantID: "tenant-1", SellEvaluationRequestID: " "})
	if !errors.Is(err, adapter.ErrUntranslatableReference) || len(source.calls) != 0 {
		t.Fatalf("blank request: err = %v calls = %d, want ErrUntranslatableReference before any lookup", err, len(source.calls))
	}

	lowercase := &sellRequestSourceDouble{record: synSellRecord(t, "cny"), found: true}
	former := &formerDouble{}
	err = newFormOnSellSubmitted(t, lowercase, former).HandleSubmittedSellEvaluationRequest(t.Context(), submittedSell())
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) || len(former.commands) != 0 {
		t.Fatalf("untranslatable record: err = %v former calls = %d", err, len(former.commands))
	}

	notAccepted := &formerDouble{result: ppapplication.FormEvaluationFromRequestResult{Outcome: ppapplication.RequestEvaluationNotAccepted}}
	err = newFormOnSellSubmitted(t, foundSellRecord(t), notAccepted).HandleSubmittedSellEvaluationRequest(t.Context(), submittedSell())
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) || !strings.Contains(err.Error(), "SELLREQ-SYN-1") {
		t.Fatalf("not accepted: err = %v, want ErrUntranslatableAnswer naming the request", err)
	}
}

func TestTheSellAdapterRefusesNilDependenciesAndAnUndeclaredEvidenceKind(t *testing.T) {
	if _, err := adapter.NewFormOnSellEvaluationRequestSubmittedAdapter(nil, &formerDouble{}, &unformedDouble{}, ppdomain.EvidenceSynthetic); err == nil {
		t.Fatal("nil source accepted")
	}
	if _, err := adapter.NewFormOnSellEvaluationRequestSubmittedAdapter(&sellRequestSourceDouble{}, nil, &unformedDouble{}, ppdomain.EvidenceSynthetic); err == nil {
		t.Fatal("nil former accepted")
	}
	if _, err := adapter.NewFormOnSellEvaluationRequestSubmittedAdapter(&sellRequestSourceDouble{}, &formerDouble{}, nil, ppdomain.EvidenceSynthetic); err == nil {
		t.Fatal("nil unformed handoff accepted")
	}
	if _, err := adapter.NewFormOnSellEvaluationRequestSubmittedAdapter(&sellRequestSourceDouble{}, &formerDouble{}, &unformedDouble{}, ""); err == nil {
		t.Fatal("undeclared evidence kind accepted")
	}
}
