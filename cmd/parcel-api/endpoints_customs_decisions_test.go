package main

import (
	"context"

	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

func init() {
	registerUnwired(customsDecisionsWiring{
		caseEstablishment:       unwiredCaseEstablishment{},
		submissionAuthorization: unwiredSubmissionAuthorization{},
		declarationSubmission:   unwiredDeclarationSubmission{},
	})
}

// 关务三口的占位：装配测试里 Intake 拒在它们之前，一次也不会被调到；被调到就交回 errOrchestrationNotWired。三口各一个类型——
// 方法同名、命令不同，一个类型挂不下三份 Handle。
type (
	unwiredCaseEstablishment       struct{}
	unwiredSubmissionAuthorization struct{}
	unwiredDeclarationSubmission   struct{}
)

func (unwiredCaseEstablishment) Handle(context.Context, customsapp.EstablishCaseForParcelsCommand) (customsapp.EstablishCaseResult, error) {
	return customsapp.EstablishCaseResult{}, errOrchestrationNotWired
}

func (unwiredSubmissionAuthorization) Handle(context.Context, customsapp.ConfirmSubmissionAuthorizationCommand) (customsapp.AuthorizationConfirmationResult, error) {
	return customsapp.AuthorizationConfirmationResult{}, errOrchestrationNotWired
}

func (unwiredDeclarationSubmission) Handle(context.Context, customsapp.SubmitDeclarationCommand) (customsapp.SubmitDeclarationResult, error) {
	return customsapp.SubmitDeclarationResult{}, errOrchestrationNotWired
}
