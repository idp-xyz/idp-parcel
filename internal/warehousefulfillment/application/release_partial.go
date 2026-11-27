package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 放波次时一张点名的请求没放进波次的三种缘由。
const (
	// ReleaseNotAllocatable 是分配不出：货不在可拣的库位，或少拣的库位还没盘过。只有这一种记成放不出的例外——
	// 它是这张已预占的请求自己的状况，别的两种是点错了单。
	ReleaseNotAllocatable = "NOT_ALLOCATABLE"
	// ReleaseNotReserved 是这张请求此刻不是已预占。
	ReleaseNotReserved = "NOT_RESERVED"
	// ReleaseNotFound 是没有这张请求，或它不属于这个仓。
	ReleaseNotFound = "NOT_FOUND"
)

// ReleaseException 是放波次时点名了而没放进波次的一张请求。
type ReleaseException struct {
	Request domain.OutboundRequestID
	Reason  string
}

// replayPartialRelease 答一次「放行能放的」的重放：波次已经在了，里面的请求都在这次点名的单里，就是同一次。
// 当时跳过的那几张照此刻读得到的说缘由——找不到的仍是找不到，记着放不出的例外的是分配不出，其余的是当时不是已预占。
func (handler *WaveHandler) replayPartialRelease(ctx context.Context, command ReleaseWaveCommand, existing []domain.OutboundRequestID) (WaveResult, error) {
	named := map[string]struct{}{}
	for _, id := range command.Requests {
		named[id.String()] = struct{}{}
	}
	released := map[string]struct{}{}
	tasks := []ports.PickTask{}
	for _, id := range existing {
		if _, ok := named[id.String()]; !ok {
			return WaveResult{outcome: WaveConflict}, nil
		}
		released[id.String()] = struct{}{}
		listed, err := handler.waves.TasksOf(ctx, command.Tenant, id)
		if err != nil {
			return WaveResult{}, fmt.Errorf("release wave: %w", err)
		}
		tasks = append(tasks, listed...)
	}
	var exceptions []ReleaseException
	for _, id := range command.Requests {
		if _, ok := released[id.String()]; ok {
			continue
		}
		reason := ReleaseNotReserved
		request, found, err := handler.requests.LoadByID(ctx, command.Tenant, id)
		if err != nil {
			return WaveResult{}, fmt.Errorf("release wave: %w", err)
		}
		if !found || request.Warehouse() != command.Warehouse {
			reason = ReleaseNotFound
		} else if skipped, err := handler.waves.HasAllocationException(ctx, command.Tenant, id); err != nil {
			return WaveResult{}, fmt.Errorf("release wave: %w", err)
		} else if skipped {
			reason = ReleaseNotAllocatable
		}
		exceptions = append(exceptions, ReleaseException{Request: id, Reason: reason})
	}
	return WaveResult{outcome: WaveExisting, tasks: tasks, exceptions: exceptions}, nil
}
