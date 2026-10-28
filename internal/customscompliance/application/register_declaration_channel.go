package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 申报通道册的登记用例（ADR-0342 决定三）：一个发送目标登一份端点与凭据引用。端点与凭据是租户取值，本口不预填；同内容重登是
// 重放，异内容是冲突、不覆盖——发送目标被改指到另一个端点，已发出的尝试就指不回当时发到了哪里。事务由进程级入口给出。

type RegisterDeclarationChannelCommand struct {
	TenantID domain.TenantID
	Channel  domain.DeclarationChannel
}

type RegisterDeclarationChannelHandler struct {
	channels ports.DeclarationChannelRegistry
	clock    ports.Clock
}

func NewRegisterDeclarationChannelHandler(channels ports.DeclarationChannelRegistry, clock ports.Clock) *RegisterDeclarationChannelHandler {
	return &RegisterDeclarationChannelHandler{channels: channels, clock: clock}
}

func (handler *RegisterDeclarationChannelHandler) Handle(
	ctx context.Context,
	command RegisterDeclarationChannelCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) || command.Channel.Target() == "" {
		return ConfigurationNotAccepted, nil
	}
	saved, err := handler.channels.SaveDeclarationChannel(ctx, command.TenantID, command.Channel, handler.clock.Now())
	if err != nil {
		return ConfigurationUndecided, nil
	}
	switch saved {
	case ports.SubmissionSendRegistered:
		return ConfigurationRegistered, nil
	case ports.SubmissionSendReplayed:
		return ConfigurationExisting, nil
	default:
		return ConfigurationContentConflict, nil
	}
}
