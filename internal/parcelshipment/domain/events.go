package domain

import (
	"time"

	bentodomain "go.idp.xyz/idp-bento-go/domain"
)

// EventTypeShipmentRequestSubmitted is the Parcel owned type of the "shipment
// request submitted" fact. It states that the customer confirmed a request and
// Parcel recorded it; it never implies acceptance.
const EventTypeShipmentRequestSubmitted bentodomain.EventType = "idp.parcel.shipment-request.submitted"

// ShipmentRequestSubmitted is the in-process domain fact recorded when a
// shipment request enters Submitted. Its payload carries identities and
// versions only: no address, contact, goods or declaration content.
type ShipmentRequestSubmitted struct {
	Key               ShipmentRequestKey
	SubmissionBatchID SubmissionBatchID
	SubmissionVersion SubmissionVersion
	DeclaredParcelIDs []DeclaredParcelID
	EventID           string
	At                time.Time
}

// Type identifies the business fact family.
func (ShipmentRequestSubmitted) Type() bentodomain.EventType {
	return EventTypeShipmentRequestSubmitted
}

// Version is the structural version of the fact.
func (ShipmentRequestSubmitted) Version() bentodomain.EventVersion { return 1 }

// OccurredAt is the domain occurrence time. It is the source occurrence time,
// never the moment the row was written.
func (e ShipmentRequestSubmitted) OccurredAt() time.Time { return e.At }
