package registrationjson

import (
	"encoding/json"
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// closePreAdviceDocument 是一笔入库预报完结的线格式。载荷只有预报标识，说明可缺。
// 货主取自预报，操作者取自信封，重放身份是被指名的预报：这三格再带即坏载荷（ADR-0384 决定二、决定四）。
type closePreAdviceDocument struct {
	carriedIdentity
	refusedOwner
	DecisionID  json.RawMessage `json:"decisionId"`
	PreAdviceID *string         `json:"preAdviceId"`
	Note        *string         `json:"note"`
}

// ClosePreAdviceCommand 把一笔完结译成命令。不从载荷收决定标识。
func ClosePreAdviceCommand(
	reader io.Reader,
	tenant domain.TenantID,
	operator domain.Registrant,
) (application.ClosePreAdviceCommand, error) {
	var none application.ClosePreAdviceCommand
	document, err := decodeDocument[closePreAdviceDocument](reader, "inbound pre-advice closure")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if err := document.refuseOwner(); err != nil {
		return none, err
	}
	if len(document.DecisionID) > 0 {
		return none, malformed("decisionId must not be carried in the payload; the close is keyed by the pre-advice")
	}
	id, err := requiredText("preAdviceId", document.PreAdviceID, domain.NewPreAdviceID)
	if err != nil {
		return none, err
	}
	note, err := optionalClosureNote(document.Note)
	if err != nil {
		return none, err
	}
	return application.ClosePreAdviceCommand{Tenant: tenant, ID: id, Operator: operator, Note: note}, nil
}

func optionalClosureNote(raw *string) (domain.ClosureNote, error) {
	if raw == nil {
		return domain.ClosureNote{}, nil
	}
	note, err := textCell("note", *raw, domain.NewClosureNote)
	if err != nil {
		return domain.ClosureNote{}, err
	}
	return note, nil
}
