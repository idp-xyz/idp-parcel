package main

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

func init() {
	registerUnwired(cancellationOrderConflictsWiring{conflicts: unwiredCancellationOrderConflicts{}})
}

type unwiredCancellationOrderConflicts struct{}

func (unwiredCancellationOrderConflicts) ListUnresolvedCancellationOrderConflicts(context.Context, domain.TenantID, int) (ports.UnresolvedCancellationOrderConflicts, error) {
	return ports.UnresolvedCancellationOrderConflicts{}, errors.New("parcel-api: cancellation order conflict catalogue not wired")
}
