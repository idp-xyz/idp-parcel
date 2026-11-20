package identity

import (
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const instructionPrefix = "INS"

// InstructionIdentities 实现 ports.InstructionIdentities。
type InstructionIdentities struct {
	instructions platformidentity.Minter
}

func NewInstructionIdentities(options ...platformidentity.Option) (*InstructionIdentities, error) {
	instructions, err := platformidentity.NewMinter(instructionPrefix, options...)
	if err != nil {
		return nil, err
	}
	return &InstructionIdentities{instructions: instructions}, nil
}

var _ ports.InstructionIdentities = (*InstructionIdentities)(nil)

func (identities *InstructionIdentities) NextInstructionID() (domain.InstructionID, error) {
	value, err := identities.instructions.Next()
	if err != nil {
		return domain.InstructionID{}, err
	}
	return domain.NewInstructionID(value)
}
