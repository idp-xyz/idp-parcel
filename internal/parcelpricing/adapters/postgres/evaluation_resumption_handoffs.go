package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// pricingParameterRegisteredEventType 是「参数已登记」的事件类型，结算的续办门按它收信（ADR-0229 决定五）。与消费侧各写
// 各的字面；两串漂开撞 dispatch.no_subscriber，由 cmd/parcel-dispatch 那几条用本包交接口入队的用例揪红。
const pricingParameterRegisteredEventType eventing.EventType = "parcel-pricing.pricing-parameter.registered"

// pricingParameterRegisteredEventIDPort 是信封 ID 指纹的口名前缀，只求本来源名下几只口的 ID 空间互不相交。
const pricingParameterRegisteredEventIDPort = "pricing-parameter"

// OutboxEvaluationResumptionHandoffs 实现「评价未形成」与「参数已登记」两只交接口：都是计价交给结算续办那一路的信封，
// 一只适配器、两个文件——两口的分区主体不同，各在自己的文件里声明（「评价未形成」那一口见
// evaluation_request_unformed_handoff.go）。
type OutboxEvaluationResumptionHandoffs struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxEvaluationResumptionHandoffs(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxEvaluationResumptionHandoffs, error) {
	if db == nil || store == nil || clock == nil {
		return nil, fmt.Errorf("parcel pricing postgres: evaluation resumption handoffs need db, outbox store and clock")
	}
	return &OutboxEvaluationResumptionHandoffs{db: db, store: store, clock: clock}, nil
}

var (
	_ ports.EvaluationRequestUnformedHandoff  = (*OutboxEvaluationResumptionHandoffs)(nil)
	_ ports.PricingParameterRegisteredHandoff = (*OutboxEvaluationResumptionHandoffs)(nil)
)

type pricingParameterRegisteredPayload struct {
	TenantID         string `json:"tenantId"`
	RegistrationKind string `json:"registrationKind"`
	PricingScope     string `json:"pricingScope,omitempty"`
}

// HandOffPricingParameterRegistered 把一次「参数已登记」入队。ID 取（租户 / 种类 / 那一版）的定长指纹：同一版重放重发同一封、
// 由 EnqueueOnce 吞掉；结算拿这封信的标识当续办依据，同一次登记重放不会把同一条代次链推两代。
func (handoffs *OutboxEvaluationResumptionHandoffs) HandOffPricingParameterRegistered(
	ctx context.Context,
	intent ports.PricingParameterRegisteredIntent,
) error {
	tenant := intent.Tenant.String()
	version := intent.Registration.ID() + "@" + intent.Registration.Version()
	if tenant == "" || intent.Kind.String() == "" || intent.Registration.ID() == "" {
		return fmt.Errorf("hand off pricing parameter registered: tenant, kind and registration are required")
	}
	if (intent.Kind == ports.PricingParameterPriceCard) != (intent.Scope.String() != "") {
		return fmt.Errorf("hand off pricing parameter registered: only a price card carries its pricing scope")
	}
	payload, err := json.Marshal(pricingParameterRegisteredPayload{
		TenantID:         tenant,
		RegistrationKind: intent.Kind.String(),
		PricingScope:     intent.Scope.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off pricing parameter registered: %w", err)
	}
	now := handoffs.clock.Now().UTC()
	return handoffs.enqueue(ctx, eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          outboxintent.FingerprintEventID(pricingParameterRegisteredEventIDPort, tenant, intent.Kind.String(), version),
		// 分区装租户，与结算、party-commercial 的登记续办口同一个字符串：同一租户的续办一轮接一轮，不会两轮并发去推
		// 同一条代次链。
		Source:       ppEventSource,
		Type:         pricingParameterRegisteredEventType,
		Version:      1,
		Scope:        tenant,
		Subject:      intent.Kind.String(),
		PartitionKey: tenant,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}, ports.ErrPricingParameterHandoffRejected)
}

func (handoffs *OutboxEvaluationResumptionHandoffs) enqueue(ctx context.Context, envelope eventing.Envelope, rejected error) error {
	if err := outboxintent.EnqueueOnce(ctx, handoffs.db, handoffs.store, envelope); err != nil {
		if errors.Is(err, eventing.ErrInvalidEnvelope) {
			return fmt.Errorf("hand off %s: %w: %w", envelope.Type, rejected, err)
		}
		return fmt.Errorf("hand off %s: %w", envelope.Type, err)
	}
	return nil
}
