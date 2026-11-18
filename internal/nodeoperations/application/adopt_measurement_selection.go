package application

import (
	"context"
	"errors"
	"strings"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

var errMeasurementSelectionUnwired = errors.New("node operations: measurement selection adoption is unwired")

// AdoptMeasurementSelectionOutcome 是一次形态采用的结果。已有结果与依据冲突都已经有了答案。
type AdoptMeasurementSelectionOutcome uint8

const (
	AdoptMeasurementSelectionOutcomeInvalid AdoptMeasurementSelectionOutcome = iota
	MeasurementSelectionAdopted
	MeasurementSelectionExisting
	MeasurementSelectionConflict
	MeasurementSelectionNotAccepted
)

func (outcome AdoptMeasurementSelectionOutcome) String() string {
	switch outcome {
	case MeasurementSelectionAdopted:
		return "ADOPTED"
	case MeasurementSelectionExisting:
		return "EXISTING_RESULT"
	case MeasurementSelectionConflict:
		return "CONFLICT"
	case MeasurementSelectionNotAccepted:
		return "NOT_ACCEPTED"
	case AdoptMeasurementSelectionOutcomeInvalid:
		return ""
	default:
		return ""
	}
}

// AdoptMeasurementSelectionCommand 是租户采用当前有效实测形态。形态必须是产品内置的那一种。
type AdoptMeasurementSelectionCommand struct {
	Tenant domain.TenantID
	Form   domain.MeasurementSelectionForm
	Basis  string
}

// AdoptMeasurementSelectionHandler 把采用写进登记册。记录时刻取时钟，不由登记行携带。
type AdoptMeasurementSelectionHandler struct {
	adoptions ports.MeasurementSelectionAdoptionStore
	clock     ports.Clock
}

func NewAdoptMeasurementSelectionHandler(
	adoptions ports.MeasurementSelectionAdoptionStore,
	clock ports.Clock,
) (*AdoptMeasurementSelectionHandler, error) {
	if adoptions == nil || clock == nil {
		return nil, errMeasurementSelectionUnwired
	}
	return &AdoptMeasurementSelectionHandler{adoptions: adoptions, clock: clock}, nil
}

func (handler *AdoptMeasurementSelectionHandler) Handle(
	ctx context.Context,
	command AdoptMeasurementSelectionCommand,
) (AdoptMeasurementSelectionOutcome, error) {
	if command.Tenant.String() == "" || command.Form != domain.LatestStillValidByOccurredAt || strings.TrimSpace(command.Basis) == "" {
		return MeasurementSelectionNotAccepted, nil
	}
	outcome, err := handler.adoptions.SaveAdoption(ctx, ports.MeasurementSelectionAdoption{
		TenantID:   command.Tenant,
		Form:       command.Form,
		Basis:      strings.TrimSpace(command.Basis),
		RecordedAt: handler.clock.Now().UTC(),
	})
	if err != nil {
		return AdoptMeasurementSelectionOutcomeInvalid, err
	}
	switch outcome {
	case ports.MeasurementSelectionSaved:
		return MeasurementSelectionAdopted, nil
	case ports.MeasurementSelectionAlreadyRecorded:
		return MeasurementSelectionExisting, nil
	case ports.MeasurementSelectionConflicts:
		return MeasurementSelectionConflict, nil
	default:
		return AdoptMeasurementSelectionOutcomeInvalid, errMeasurementSelectionUnwired
	}
}
