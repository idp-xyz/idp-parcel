package main

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

func init() {
	registerUnwired(controlReleaseRefusalsWiring{refusals: unwiredControlReleaseRefusals{}})
}

type unwiredControlReleaseRefusals struct{}

func (unwiredControlReleaseRefusals) ListUnhandledControlReleaseRefusals(context.Context, domain.TenantID, int, cataloguepage.Query) (ports.UnhandledControlReleaseRefusals, error) {
	return ports.UnhandledControlReleaseRefusals{}, errors.New("parcel-api: control release refusal catalogue not wired")
}
