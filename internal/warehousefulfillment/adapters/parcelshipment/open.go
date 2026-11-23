package parcelshipment

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

type agencyAllowance interface {
	Allow(ctx context.Context, tenant, owner string) (bool, error)
}

type shipmentOpener interface {
	Handle(ctx context.Context, command psapplication.SubmitShipmentRequestCommand) (psapplication.SubmitShipmentRequestResult, error)
}

type ownershipAuthority interface {
	DecideProductionOwnership(ctx context.Context, scope psdomain.AdmissionScope) (psdomain.ProductionOwnershipDecision, error)
}

type versionFormer interface {
	Handle(ctx context.Context, command psapplication.FormNewSubmissionVersionCommand) (psapplication.FormNewSubmissionVersionResult, error)
}

// Opener 把一份出库包裹交给小包托运。代办没获准时不调用提交编排。
type Opener struct {
	agency    agencyAllowance
	shipments shipmentOpener
	ownership ownershipAuthority
	clock     interface{ Now() time.Time }
	requests  identity.Minter
	batches   identity.Minter
	parcels   identity.Minter
	versions  versionFormer
}

func NewOpener(
	agency agencyAllowance,
	shipments shipmentOpener,
	ownership ownershipAuthority,
	clock interface{ Now() time.Time },
) (*Opener, error) {
	if agency == nil || shipments == nil || ownership == nil || clock == nil {
		return nil, fmt.Errorf("warehouse fulfillment parcel shipment: opener needs an agency allowance, a submission handler, an ownership authority and a clock")
	}
	requests, err := identity.NewMinter(psidentity.ShipmentRequestPrefix)
	if err != nil {
		return nil, err
	}
	batches, err := identity.NewMinter(psidentity.SubmissionBatchPrefix)
	if err != nil {
		return nil, err
	}
	parcels, err := identity.NewMinter(psidentity.DeclaredParcelPrefix)
	if err != nil {
		return nil, err
	}
	return &Opener{agency: agency, shipments: shipments, ownership: ownership, clock: clock, requests: requests, batches: batches, parcels: parcels}, nil
}

// WithVersionFormer 接上同一委托的新提交版本。没接时补正答未配置。
func (opener *Opener) WithVersionFormer(former versionFormer) *Opener {
	opener.versions = former
	return opener
}

func (opener *Opener) Revise(ctx context.Context, revision ports.CommissionRevision) (ports.CommissionAnswer, error) {
	if opener.versions == nil {
		return ports.CommissionAnswer{Outcome: ports.CommissionNotConfiguredWord}, nil
	}
	granted, err := opener.agency.Allow(ctx, revision.Tenant, revision.Owner)
	if err != nil {
		return ports.CommissionAnswer{}, err
	}
	if !granted {
		return ports.CommissionAnswer{Outcome: ports.CommissionNotConfiguredWord}, nil
	}
	command, err := opener.revision(ctx, revision)
	if err != nil {
		return ports.CommissionAnswer{}, err
	}
	result, err := opener.versions.Handle(ctx, command)
	if err != nil {
		return ports.CommissionAnswer{}, err
	}
	return ports.CommissionAnswer{Outcome: result.Outcome().String()}, nil
}

func (opener *Opener) Submit(ctx context.Context, submission ports.CommissionSubmission) (ports.CommissionAnswer, error) {
	granted, err := opener.agency.Allow(ctx, submission.Tenant, submission.Owner)
	if err != nil {
		return ports.CommissionAnswer{}, err
	}
	if !granted {
		return ports.CommissionAnswer{Outcome: ports.CommissionNotConfiguredWord}, nil
	}
	command, err := opener.command(ctx, submission)
	if err != nil {
		return ports.CommissionAnswer{}, err
	}
	result, err := opener.shipments.Handle(ctx, command)
	if err != nil {
		return ports.CommissionAnswer{}, err
	}
	answer := ports.CommissionAnswer{Outcome: result.Outcome().String()}
	if id, present := result.ShipmentRequestID(); present {
		answer.ShipmentRequestID = id.String()
	}
	packed := submissionParcels(submission)
	if len(command.DeclaredParcelIDs) == len(packed) {
		answer.Members = make([]ports.CommissionMember, len(packed))
		for index, parcel := range packed {
			answer.Members[index] = ports.CommissionMember{Sequence: parcel.Sequence, DeclaredParcelID: command.DeclaredParcelIDs[index].String()}
		}
	}
	if len(answer.Members) == 1 {
		answer.DeclaredParcelID = answer.Members[0].DeclaredParcelID
	}
	return answer, nil
}

func (opener *Opener) command(ctx context.Context, submission ports.CommissionSubmission) (psapplication.SubmitShipmentRequestCommand, error) {
	var none psapplication.SubmitShipmentRequestCommand
	tenant, err := psdomain.NewTenantID(submission.Tenant)
	if err != nil {
		return none, err
	}
	owner, err := psdomain.NewCustomerAccountID(submission.Owner)
	if err != nil {
		return none, err
	}
	source, err := psdomain.NewSource(psdomain.SourceWarehouseOutbound)
	if err != nil {
		return none, err
	}
	keyText := submission.Request + "/" + strconv.Itoa(submission.Sequence)
	if submission.Round > 0 {
		keyText = submission.Request + "/round/" + strconv.Itoa(submission.Round)
	}
	key, err := psdomain.NewSourceRequestKey(keyText)
	if err != nil {
		return none, err
	}
	identityValue, err := psdomain.NewSourceIdentity(tenant, owner, source, key)
	if err != nil {
		return none, err
	}
	reference, err := psdomain.NewShipmentRequestID(keyText)
	if err != nil {
		return none, err
	}
	packed := submissionParcels(submission)
	customerParcels := make([]psdomain.DeclaredParcelID, 0, len(packed))
	for _, parcel := range packed {
		customerParcel, err := psdomain.NewDeclaredParcelID(submission.Request + "/" + strconv.Itoa(parcel.Sequence))
		if err != nil {
			return none, err
		}
		customerParcels = append(customerParcels, customerParcel)
	}
	requestValue, err := opener.requests.Next()
	if err != nil {
		return none, err
	}
	requestID, err := psdomain.NewShipmentRequestID(requestValue)
	if err != nil {
		return none, err
	}
	batchValue, err := opener.batches.Next()
	if err != nil {
		return none, err
	}
	batchID, err := psdomain.NewSubmissionBatchID(batchValue)
	if err != nil {
		return none, err
	}
	parcelIDs := make([]psdomain.DeclaredParcelID, 0, len(packed))
	profiles := make([]psdomain.DeclaredParcelProfile, 0, len(packed))
	lines := make([]psdomain.GoodsLineSpec, 0, len(packed))
	measures := map[string]string{psdomain.RequestedServiceProductEntryName: submission.Service}
	for _, parcel := range packed {
		parcelValue, err := opener.parcels.Next()
		if err != nil {
			return none, err
		}
		parcelID, err := psdomain.NewDeclaredParcelID(parcelValue)
		if err != nil {
			return none, err
		}
		profile, err := measurementProfile(parcelID, parcel)
		if err != nil {
			return none, err
		}
		parcelIDs = append(parcelIDs, parcelID)
		profiles = append(profiles, profile)
		lines = append(lines, goodsSpec(parcel).Lines...)
		member := submission.Request + "/" + strconv.Itoa(parcel.Sequence)
		measures["parcel["+member+"].weight.value"] = parcel.Weight
		measures["parcel["+member+"].weight.unit"] = parcel.WeightUnit
		measures["parcel["+member+"].dimensions.length"] = parcel.Length
		measures["parcel["+member+"].dimensions.width"] = parcel.Width
		measures["parcel["+member+"].dimensions.height"] = parcel.Height
		measures["parcel["+member+"].dimensions.unit"] = parcel.Dimension
	}
	goods, err := psdomain.NewGoodsDetails(psdomain.GoodsDetailsSpec{Lines: lines}, parcelIDs)
	if err != nil {
		return none, err
	}
	scope := psdomain.NewCustomerAccountAdmissionScope
	admission, err := scope(tenant, owner)
	if err != nil {
		return none, err
	}
	decision, err := opener.ownership.DecideProductionOwnership(ctx, admission)
	if err != nil {
		return none, err
	}
	canonical, err := psdomain.CanonicalizeSubmission(psdomain.SubmissionPayloadSpec{
		RequestReference:  reference,
		DeclaredParcelIDs: customerParcels,
		Scope:             entries(map[string]string{"recipient.relation": submission.Recipient, "recipient.address": submission.Address}),
		Service:           entries(measures),
		Goods:             goods,
	})
	if err != nil {
		return none, err
	}
	now := opener.clock.Now().UTC()
	return psapplication.SubmitShipmentRequestCommand{
		Identity: identityValue, PayloadDigest: canonical.Digest(), OccurredAt: now, ReceivedAt: now,
		BatchID: batchID, ShipmentRequestID: requestID, DeclaredParcelIDs: parcelIDs,
		AdmissionScope: admission, ExpectedRevision: decision.Revision(),
		DeclaredProfiles: profiles, DeclaredElements: canonical.DeclaredElements(),
		RequestedServiceProduct: canonical.RequestedServiceProduct(), DeclaredGoods: goods,
	}, nil
}

func submissionParcels(submission ports.CommissionSubmission) []ports.CommissionParcel {
	if len(submission.Parcels) > 0 {
		return submission.Parcels
	}
	return []ports.CommissionParcel{submission.Parcel}
}

func (opener *Opener) revision(ctx context.Context, revision ports.CommissionRevision) (psapplication.FormNewSubmissionVersionCommand, error) {
	var none psapplication.FormNewSubmissionVersionCommand
	base, err := opener.command(ctx, revision.CommissionSubmission)
	if err != nil {
		return none, err
	}
	member, err := psdomain.NewDeclaredParcelID(revision.DeclaredParcelID)
	if err != nil {
		return none, err
	}
	requestID, err := psdomain.NewShipmentRequestID(revision.ShipmentRequest)
	if err != nil {
		return none, err
	}
	basis, err := psdomain.NewSubmissionVersionID(revision.BasisVersion)
	if err != nil {
		return none, err
	}
	supplementKey, err := psdomain.NewSourceRequestKey(revision.RequestKey)
	if err != nil {
		return none, err
	}
	profile, err := measurementProfile(member, revision.Parcel)
	if err != nil {
		return none, err
	}
	goods, err := psdomain.NewGoodsDetails(goodsSpec(revision.Parcel), []psdomain.DeclaredParcelID{member})
	if err != nil {
		return none, err
	}
	supplement, err := psdomain.NewSourceIdentity(base.Identity.TenantID(), base.Identity.CustomerAccountID(), base.Identity.Source(), supplementKey)
	if err != nil {
		return none, err
	}
	return psapplication.FormNewSubmissionVersionCommand{
		Identity: base.Identity, SupplementIdentity: supplement,
		PayloadDigest: base.PayloadDigest, OccurredAt: base.OccurredAt, ReceivedAt: base.ReceivedAt,
		ShipmentRequestID: requestID, BasisVersion: basis, DeclaredParcelIDs: []psdomain.DeclaredParcelID{member},
		DeclaredProfiles: []psdomain.DeclaredParcelProfile{profile}, DeclaredElements: base.DeclaredElements,
		RequestedServiceProduct: base.RequestedServiceProduct, DeclaredGoods: goods,
	}, nil
}

func measurementProfile(parcel psdomain.DeclaredParcelID, packed ports.CommissionParcel) (psdomain.DeclaredParcelProfile, error) {
	var none psdomain.DeclaredParcelProfile
	weightValue, err := psdomain.NewMeasurementValue(packed.Weight)
	if err != nil {
		return none, err
	}
	weightUnit, err := psdomain.NewMeasurementUnitReference(packed.WeightUnit)
	if err != nil {
		return none, err
	}
	weight, err := psdomain.NewDeclaredWeight(weightValue, weightUnit)
	if err != nil {
		return none, err
	}
	length, err := psdomain.NewMeasurementValue(packed.Length)
	if err != nil {
		return none, err
	}
	width, err := psdomain.NewMeasurementValue(packed.Width)
	if err != nil {
		return none, err
	}
	height, err := psdomain.NewMeasurementValue(packed.Height)
	if err != nil {
		return none, err
	}
	dimensionUnit, err := psdomain.NewMeasurementUnitReference(packed.Dimension)
	if err != nil {
		return none, err
	}
	dimensions, err := psdomain.NewDeclaredDimensions(length, width, height, dimensionUnit)
	if err != nil {
		return none, err
	}
	measurement, err := psdomain.NewDeclaredMeasurement(weight, dimensions)
	if err != nil {
		return none, err
	}
	return psdomain.NewDeclaredParcelProfile(parcel, measurement)
}

func goodsSpec(packed ports.CommissionParcel) psdomain.GoodsDetailsSpec {
	mark := psdomain.SourceMarkThisDeclaration
	cell := func(value string) psdomain.GoodsCellSpec {
		return psdomain.GoodsCellSpec{Value: value, Mark: mark}
	}
	return psdomain.GoodsDetailsSpec{Lines: []psdomain.GoodsLineSpec{{
		Quantity: cell(strconv.FormatInt(packed.Quantity, 10)),
		Unit:     cell(packed.Unit),
		OwnerSKU: cell(packed.Goods),
	}}}
}

func entries(values map[string]string) []psdomain.CanonicalContentEntry {
	named := make([]string, 0, len(values))
	for name := range values {
		named = append(named, name)
	}
	// 摘要按条目排序。这里先按名字排，避免 map 迭代让同一内容两次摘要不同。
	for left := 0; left < len(named); left++ {
		for right := left + 1; right < len(named); right++ {
			if named[right] < named[left] {
				named[left], named[right] = named[right], named[left]
			}
		}
	}
	collected := make([]psdomain.CanonicalContentEntry, 0, len(named))
	for _, name := range named {
		value := strings.TrimSpace(values[name])
		if value == "" {
			continue
		}
		entry, err := psdomain.NewCanonicalContentEntry(name, value)
		if err != nil {
			continue
		}
		collected = append(collected, entry)
	}
	return collected
}
