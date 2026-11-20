package accessidentity

import "context"

// ProcessingInstructionEnvelope 与 RelabelInstructionEnvelope 是加工、换标四个货主写口的来源信封。
// 按口分型，提交的信封递不进撤回口。
type ProcessingInstructionSubmissionEnvelope struct{ envelope SourceEnvelope }
type ProcessingInstructionWithdrawalEnvelope struct{ envelope SourceEnvelope }
type RelabelInstructionSubmissionEnvelope struct{ envelope SourceEnvelope }
type RelabelInstructionWithdrawalEnvelope struct{ envelope SourceEnvelope }

func (submission ProcessingInstructionSubmissionEnvelope) Envelope() SourceEnvelope {
	return submission.envelope
}
func (withdrawal ProcessingInstructionWithdrawalEnvelope) Envelope() SourceEnvelope {
	return withdrawal.envelope
}
func (submission RelabelInstructionSubmissionEnvelope) Envelope() SourceEnvelope {
	return submission.envelope
}
func (withdrawal RelabelInstructionWithdrawalEnvelope) Envelope() SourceEnvelope {
	return withdrawal.envelope
}

// MintProcessingInstructionSubmission 铸一次加工指令提交。准入两格都是 PROCESSING_INSTRUCTION_SUBMISSION。
func (minter *CustomerClientMinter) MintProcessingInstructionSubmission(
	ctx context.Context, credential IntegrationClientCredential, readRequest func() (ChannelRequest, error),
) (ProcessingInstructionSubmissionEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityProcessingInstructionSubmission,
		customerAPIRequestKeys{}.DeriveProcessingInstructionSubmissionRequestKey)
	if err != nil {
		return ProcessingInstructionSubmissionEnvelope{}, err
	}
	return ProcessingInstructionSubmissionEnvelope{envelope: envelope}, nil
}

func (minter *CustomerClientMinter) MintProcessingInstructionWithdrawal(
	ctx context.Context, credential IntegrationClientCredential, readRequest func() (ChannelRequest, error),
) (ProcessingInstructionWithdrawalEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityProcessingInstructionWithdrawal,
		customerAPIRequestKeys{}.DeriveProcessingInstructionWithdrawalRequestKey)
	if err != nil {
		return ProcessingInstructionWithdrawalEnvelope{}, err
	}
	return ProcessingInstructionWithdrawalEnvelope{envelope: envelope}, nil
}

func (minter *CustomerClientMinter) MintRelabelInstructionSubmission(
	ctx context.Context, credential IntegrationClientCredential, readRequest func() (ChannelRequest, error),
) (RelabelInstructionSubmissionEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityRelabelInstructionSubmission,
		customerAPIRequestKeys{}.DeriveRelabelInstructionSubmissionRequestKey)
	if err != nil {
		return RelabelInstructionSubmissionEnvelope{}, err
	}
	return RelabelInstructionSubmissionEnvelope{envelope: envelope}, nil
}

func (minter *CustomerClientMinter) MintRelabelInstructionWithdrawal(
	ctx context.Context, credential IntegrationClientCredential, readRequest func() (ChannelRequest, error),
) (RelabelInstructionWithdrawalEnvelope, error) {
	envelope, err := minter.mintWarehouseCustomerWrite(ctx, credential, readRequest, CustomerCapabilityRelabelInstructionWithdrawal,
		customerAPIRequestKeys{}.DeriveRelabelInstructionWithdrawalRequestKey)
	if err != nil {
		return RelabelInstructionWithdrawalEnvelope{}, err
	}
	return RelabelInstructionWithdrawalEnvelope{envelope: envelope}, nil
}

func (customerAPIRequestKeys) DeriveProcessingInstructionSubmissionRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixProcessingInstruction, request)
}

func (customerAPIRequestKeys) DeriveProcessingInstructionWithdrawalRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixProcessingWithdrawal, request)
}

func (customerAPIRequestKeys) DeriveRelabelInstructionSubmissionRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixRelabelInstruction, request)
}

func (customerAPIRequestKeys) DeriveRelabelInstructionWithdrawalRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return prefixedRequestKeyRejectingReserved(requestKeyPrefixRelabelWithdrawal, request)
}
