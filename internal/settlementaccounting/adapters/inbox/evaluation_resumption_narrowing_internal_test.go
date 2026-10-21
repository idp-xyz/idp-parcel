package sainbox

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉续办门按信封收窄（ADR-0229 决定五）：计价的三类必带计价范围与方向，BUY 触发册必带发生项原因，SELL 计费
// 时点采用必带户，缺了是毒丸——按种类放宽会把整个租户里同停因的件推一遍；带了的格原样进取队列的条件。

type recordingQueue struct {
	queries []ports.StalledEvaluationQuery
}

func (queue *recordingQueue) ListStalled(
	_ context.Context, _ domain.TenantID, query ports.StalledEvaluationQuery, _ int,
) ([]ports.StalledEvaluation, error) {
	queue.queries = append(queue.queries, query)
	return nil, nil
}

func TestTheResumptionGateNarrowsByWhatTheEnvelopeCarries(t *testing.T) {
	gate := func(registration EvaluationParameterRegistration) (*EvaluationParameterRegisteredConsumer, *recordingQueue) {
		queue := &recordingQueue{}
		return &EvaluationParameterRegisteredConsumer{
			registration: registration, transactor: inlineTransactor{}, queue: queue,
			resumer: &unwrittenResumer{}, pageSize: 10,
		}, queue
	}
	for _, poison := range []struct {
		label        string
		registration EvaluationParameterRegistration
		payload      string
	}{
		{"价卡缺范围", PricingParameterRegisteredResumption, `{"tenantId":"SYN-TENANT-01","registrationKind":"PRICE_CARD","direction":"BUY"}`},
		{"价卡缺方向", PricingParameterRegisteredResumption, `{"tenantId":"SYN-TENANT-01","registrationKind":"PRICE_CARD","pricingScope":"SYN-SCOPE-01"}`},
		{"价卡内部方向", PricingParameterRegisteredResumption, `{"tenantId":"SYN-TENANT-01","registrationKind":"PRICE_CARD","pricingScope":"SYN-SCOPE-01","direction":"INTERNAL"}`},
		{"序列缺范围", PricingParameterRegisteredResumption, `{"tenantId":"SYN-TENANT-01","registrationKind":"REFERENCE_SERIES","direction":"SELL"}`},
		{"目录缺方向", PricingParameterRegisteredResumption, `{"tenantId":"SYN-TENANT-01","registrationKind":"REFERENCE_CATALOGUE","pricingScope":"SYN-SCOPE-01"}`},
		{"触发册缺原因", EvaluationParameterRegisteredResumption, `{"tenantId":"SYN-TENANT-01","registrationKind":"BUY_EVALUATION_TRIGGER"}`},
		{"采用缺户", EvaluationParameterRegisteredResumption, `{"tenantId":"SYN-TENANT-01","registrationKind":"SELL_BILLING_MOMENT_ADOPTION"}`},
	} {
		consumer, _ := gate(poison.registration)
		if _, err := consumer.decode("SYN-ENVELOPE-1", []byte(poison.payload)); !errors.Is(err, ErrPoisonEnvelope) {
			t.Fatalf("%s err = %v，want 毒丸", poison.label, err)
		}
	}

	for _, narrowed := range []struct {
		label        string
		registration EvaluationParameterRegistration
		payload      string
		check        func(ports.StalledEvaluationQuery) bool
	}{
		{"价卡", PricingParameterRegisteredResumption,
			`{"tenantId":"SYN-TENANT-01","registrationKind":"PRICE_CARD","pricingScope":"SYN-SCOPE-01","direction":"BUY"}`,
			func(query ports.StalledEvaluationQuery) bool {
				return query.PricingScope == "SYN-SCOPE-01" && query.Direction == domain.BuyEvaluationDirection
			}},
		{"序列", PricingParameterRegisteredResumption,
			`{"tenantId":"SYN-TENANT-01","registrationKind":"REFERENCE_SERIES","pricingScope":"SYN-SCOPE-02","direction":"SELL"}`,
			func(query ports.StalledEvaluationQuery) bool {
				return query.PricingScope == "SYN-SCOPE-02" && query.Direction == domain.SellEvaluationDirection
			}},
		{"触发册", EvaluationParameterRegisteredResumption,
			`{"tenantId":"SYN-TENANT-01","registrationKind":"BUY_EVALUATION_TRIGGER","occurrenceReason":"ACTUAL_FULFILLMENT"}`,
			func(query ports.StalledEvaluationQuery) bool {
				return query.OccurrenceReason.String() == "ACTUAL_FULFILLMENT" && query.Account.String() == "" && query.PricingScope == ""
			}},
		{"采用", EvaluationParameterRegisteredResumption,
			`{"tenantId":"SYN-TENANT-01","registrationKind":"SELL_BILLING_MOMENT_ADOPTION","settlementAccountId":"SYN-ACCT-1"}`,
			func(query ports.StalledEvaluationQuery) bool {
				return query.Account.String() == "SYN-ACCT-1" && query.OccurrenceReason.String() == ""
			}},
		{"映射不收窄", EvaluationParameterRegisteredResumption,
			`{"tenantId":"SYN-TENANT-01","registrationKind":"FEE_ITEM_MAPPING","settlementAccountId":"SYN-ACCT-1"}`,
			func(query ports.StalledEvaluationQuery) bool {
				return query.Account.String() == "" && query.Direction == domain.EvaluationDirectionInvalid
			}},
		{"账户不收窄", SettlementAccountRegisteredResumption,
			`{"tenantId":"SYN-TENANT-01","registrationKind":"SETTLEMENT_ACCOUNT"}`,
			func(query ports.StalledEvaluationQuery) bool {
				return query.Account.String() == "" && query.PricingScope == ""
			}},
	} {
		consumer, queue := gate(narrowed.registration)
		registered, err := consumer.decode(eventing.EventID("SYN-ENVELOPE-"+narrowed.label), []byte(narrowed.payload))
		if err != nil {
			t.Fatalf("%s：%v", narrowed.label, err)
		}
		if err := consumer.resumeWaiting(t.Context(), registered); err != nil {
			t.Fatalf("%s：%v", narrowed.label, err)
		}
		if len(queue.queries) != 1 || !narrowed.check(queue.queries[0]) || !queue.queries[0].ExcludeBasis.Present() {
			t.Fatalf("%s 取队列的条件 = %+v", narrowed.label, queue.queries)
		}
	}
}
