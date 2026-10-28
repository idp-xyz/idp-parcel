package main

import (
	"context"

	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

func init() {
	registerUnwired(caseClosureWiring{closer: unwiredCaseCloser{}})
}

// unwiredCaseCloser 是关闭口的未接线占位：装配测试只看接入面，编排走不到。
type unwiredCaseCloser struct{}

func (unwiredCaseCloser) Handle(context.Context, customsapp.CloseCustomsCaseCommand) (customsapp.CloseCustomsCaseResult, error) {
	return customsapp.CloseCustomsCaseResult{}, errOrchestrationNotWired
}
