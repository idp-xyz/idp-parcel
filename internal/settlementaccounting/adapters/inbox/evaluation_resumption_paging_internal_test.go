package sainbox

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉续办门的翻页（ADR-0229 决定五）：办过却没写回的件——重跑被拒、撞上矛盾、未决——仍满足队列的口径、停在队首；
// 续办门按位置往后翻，排在它们后面的照样办到，不会被它们占满的一页挡住。

type pagingQueue struct {
	items []ports.StalledEvaluation
}

func (queue *pagingQueue) ListStalled(
	_ context.Context, _ domain.TenantID, query ports.StalledEvaluationQuery, limit int,
) ([]ports.StalledEvaluation, error) {
	var page []ports.StalledEvaluation
	for _, item := range queue.items {
		if query.After.Present() && !after(item.Position, query.After) {
			continue
		}
		page = append(page, item)
		if len(page) == limit {
			break
		}
	}
	return page, nil
}

func after(position, cursor ports.StalledEvaluationPosition) bool {
	if !position.StoppedAt.Equal(cursor.StoppedAt) {
		return position.StoppedAt.After(cursor.StoppedAt)
	}
	return position.Kind+"/"+position.FirstKey > cursor.Kind+"/"+cursor.FirstKey
}

// unwrittenResumer 每一件都办、都不写回：办过的件留在队列里，与重跑被拒、撞上矛盾、未决的件同一个处境。
type unwrittenResumer struct {
	handled []string
}

func (resumer *unwrittenResumer) Handle(
	_ context.Context, command saapplication.ResumeStalledEvaluationCommand,
) (saapplication.ResumeStalledEvaluationResult, error) {
	resumer.handled = append(resumer.handled, command.Item.SellRequest.String())
	return saapplication.ResumeStalledEvaluationResult{}, nil
}

type inlineTransactor struct{}

func (inlineTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	return fn(ctx)
}

func TestTheResumptionGatePagesPastItemsThatStayInTheQueue(t *testing.T) {
	start := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	queue := &pagingQueue{}
	for index := 0; index < 7; index++ {
		id, err := domain.NewSellEvaluationRequestID(fmt.Sprintf("SELLREQ-SYN-%02d", index))
		if err != nil {
			t.Fatal(err)
		}
		queue.items = append(queue.items, ports.StalledEvaluation{
			Kind: ports.StalledSellRequest, Cause: domain.StopPriceCardNotConfigured, SellRequest: id,
			Position: ports.StalledEvaluationPosition{
				StoppedAt: start.Add(time.Duration(index) * time.Minute), Kind: ports.StalledSellRequest.String(), FirstKey: id.String(),
			},
		})
	}
	sort.Slice(queue.items, func(i, j int) bool {
		return queue.items[i].Position.StoppedAt.Before(queue.items[j].Position.StoppedAt)
	})
	resumer := &unwrittenResumer{}
	consumer := &EvaluationParameterRegisteredConsumer{
		registration: PricingParameterRegisteredResumption,
		transactor:   inlineTransactor{},
		queue:        queue,
		resumer:      resumer,
		pageSize:     2,
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	basis, err := domain.NewParameterResumeBasis(domain.ParameterPriceCard, "pricing-parameter/1")
	if err != nil {
		t.Fatal(err)
	}
	// 七件始终都在队列里：每页重读队首只会办到前两件；按位置往后翻才办得到全部。
	if err := consumer.resumeWaiting(t.Context(), registeredParameter{tenant: tenant, kind: domain.ParameterPriceCard, basis: basis}); err != nil {
		t.Fatalf("续办门：%v", err)
	}
	if len(resumer.handled) != len(queue.items) {
		t.Fatalf("办到 %d 件 %v，want 全部 %d 件", len(resumer.handled), resumer.handled, len(queue.items))
	}
}
