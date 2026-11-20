package registrationjson

import (
	"encoding/json"
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// unclaimedArrivalDocument 是一线作业登记待认领到货的线格式。
// 租户、设备与操作者不在载荷里，键在场即坏载荷；到货种类封闭为 INBOUND / RETURN，别的词即坏载荷（AT-WF-055）。
type unclaimedArrivalDocument struct {
	carriedIdentity
	DeviceID        json.RawMessage `json:"deviceId"`
	Operator        json.RawMessage `json:"operator"`
	FactID          *string         `json:"factId"`
	OccurredAt      *string         `json:"occurredAt"`
	DeviceSequence  *int64          `json:"deviceSequence"`
	Warehouse       *string         `json:"warehouse"`
	Kind            *string         `json:"kind"`
	Observation     *string         `json:"observation"`
	OwnerClue       *string         `json:"ownerClue"`
	PreAdviceClue   *string         `json:"preAdviceClue"`
	StorageLocation *string         `json:"storageLocation"`
}

func (document unclaimedArrivalDocument) refuseCarriedActors() error {
	if err := document.refuseCarriedIdentity(); err != nil {
		return err
	}
	if len(document.DeviceID) > 0 {
		return malformed("deviceId must not be carried in the payload; the device comes from the work-fact headers, not from the request")
	}
	if len(document.Operator) > 0 {
		return malformed("operator must not be carried in the payload; the operator comes from the work-fact envelope, not from the request")
	}
	return nil
}

// UnclaimedArrivalCommand 把作业事实登记待认领到货的载荷译成命令。租户、操作者与设备留空，由 Intake 在认证之后填上。
func UnclaimedArrivalCommand(reader io.Reader) (application.RegisterUnclaimedArrivalCommand, error) {
	var none application.RegisterUnclaimedArrivalCommand
	document, err := decodeDocument[unclaimedArrivalDocument](reader, "unclaimed arrival")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedActors(); err != nil {
		return none, err
	}
	factID, err := requiredText("factId", document.FactID, domain.NewUnclaimedArrivalFactID)
	if err != nil {
		return none, err
	}
	occurredAt, err := requiredInstant("occurredAt", document.OccurredAt)
	if err != nil {
		return none, err
	}
	if document.DeviceSequence == nil {
		return none, malformed("deviceSequence is required")
	}
	if *document.DeviceSequence < 1 {
		return none, malformed("deviceSequence must be a positive integer")
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	kind, err := requiredClosedWord("kind", "unclaimed arrival kind", document.Kind, domain.NewUnclaimedArrivalKind)
	if err != nil {
		return none, err
	}
	observation, err := requiredText("observation", document.Observation, domain.NewUnclaimedArrivalObservation)
	if err != nil {
		return none, err
	}
	var ownerClue domain.OwnerAccountID
	if document.OwnerClue != nil {
		ownerClue, err = textCell("ownerClue", *document.OwnerClue, domain.NewOwnerAccountID)
		if err != nil {
			return none, err
		}
	}
	var preAdviceClue domain.PreAdviceID
	if document.PreAdviceClue != nil {
		preAdviceClue, err = textCell("preAdviceClue", *document.PreAdviceClue, domain.NewPreAdviceID)
		if err != nil {
			return none, err
		}
	}
	var storage domain.UnclaimedArrivalStorageLocation
	if document.StorageLocation != nil {
		storage, err = textCell("storageLocation", *document.StorageLocation, domain.NewUnclaimedArrivalStorageLocation)
		if err != nil {
			return none, err
		}
	}
	return application.RegisterUnclaimedArrivalCommand{
		FactID:          factID,
		OccurredAt:      occurredAt,
		DeviceSequence:  *document.DeviceSequence,
		Warehouse:       warehouse,
		Kind:            kind,
		Observation:     observation,
		OwnerClue:       ownerClue,
		PreAdviceClue:   preAdviceClue,
		StorageLocation: storage,
	}, nil
}

// claimUnclaimedArrivalDocument 是运营认领的线格式。货主是业务内容（ADR-0384 决定四），租户与操作者取自信封。
type claimUnclaimedArrivalDocument struct {
	carriedIdentity
	DecisionID   *string `json:"decisionId"`
	ArrivalID    *string `json:"arrivalId"`
	OwnerAccount *string `json:"ownerAccount"`
	PreAdviceID  *string `json:"preAdviceId"`
}

func ClaimUnclaimedArrivalCommand(
	reader io.Reader,
	tenant domain.TenantID,
	operator domain.Registrant,
) (application.ClaimUnclaimedArrivalCommand, error) {
	var none application.ClaimUnclaimedArrivalCommand
	document, err := decodeDocument[claimUnclaimedArrivalDocument](reader, "unclaimed arrival claim")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	decisionID, err := requiredText("decisionId", document.DecisionID, domain.NewRequestKey)
	if err != nil {
		return none, err
	}
	arrivalID, err := requiredText("arrivalId", document.ArrivalID, domain.NewUnclaimedArrivalID)
	if err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	var preAdvice domain.PreAdviceID
	if document.PreAdviceID != nil {
		preAdvice, err = textCell("preAdviceId", *document.PreAdviceID, domain.NewPreAdviceID)
		if err != nil {
			return none, err
		}
	}
	return application.ClaimUnclaimedArrivalCommand{
		Tenant:     tenant,
		DecisionID: decisionID,
		ArrivalID:  arrivalID,
		Owner:      owner,
		PreAdvice:  preAdvice,
		Operator:   operator,
	}, nil
}

type revokeUnclaimedClaimDocument struct {
	carriedIdentity
	DecisionID *string `json:"decisionId"`
	ArrivalID  *string `json:"arrivalId"`
	Reason     *string `json:"reason"`
}

func RevokeUnclaimedClaimCommand(
	reader io.Reader,
	tenant domain.TenantID,
	operator domain.Registrant,
) (application.RevokeUnclaimedClaimCommand, error) {
	var none application.RevokeUnclaimedClaimCommand
	document, err := decodeDocument[revokeUnclaimedClaimDocument](reader, "unclaimed arrival claim revocation")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	decisionID, err := requiredText("decisionId", document.DecisionID, domain.NewRequestKey)
	if err != nil {
		return none, err
	}
	arrivalID, err := requiredText("arrivalId", document.ArrivalID, domain.NewUnclaimedArrivalID)
	if err != nil {
		return none, err
	}
	var reason domain.UnclaimedArrivalNote
	if document.Reason != nil {
		reason, err = textCell("reason", *document.Reason, domain.NewUnclaimedArrivalNote)
		if err != nil {
			return none, err
		}
	}
	return application.RevokeUnclaimedClaimCommand{
		Tenant:     tenant,
		DecisionID: decisionID,
		ArrivalID:  arrivalID,
		Reason:     reason,
		Operator:   operator,
	}, nil
}

type disposeUnclaimedArrivalDocument struct {
	carriedIdentity
	ArrivalID *string `json:"arrivalId"`
	Method    *string `json:"method"`
	Note      *string `json:"note"`
}

func DisposeUnclaimedArrivalCommand(
	reader io.Reader,
	tenant domain.TenantID,
	operator domain.Registrant,
) (application.DisposeUnclaimedArrivalCommand, error) {
	var none application.DisposeUnclaimedArrivalCommand
	document, err := decodeDocument[disposeUnclaimedArrivalDocument](reader, "unclaimed arrival disposition")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	arrivalID, err := requiredText("arrivalId", document.ArrivalID, domain.NewUnclaimedArrivalID)
	if err != nil {
		return none, err
	}
	method, err := requiredText("method", document.Method, domain.NewUnclaimedDisposalMethod)
	if err != nil {
		return none, err
	}
	var note domain.UnclaimedArrivalNote
	if document.Note != nil {
		note, err = textCell("note", *document.Note, domain.NewUnclaimedArrivalNote)
		if err != nil {
			return none, err
		}
	}
	return application.DisposeUnclaimedArrivalCommand{
		Tenant:    tenant,
		ArrivalID: arrivalID,
		Method:    method,
		Note:      note,
		Operator:  operator,
	}, nil
}
