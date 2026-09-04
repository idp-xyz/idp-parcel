package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// PickupCorrectionIntake 把已认证的接入请求翻译成单对象揽收更正命令（票 tf-segment-lifecycle-closure/08）。
// 接口而非解析代码的理由同 DeliveryIntake。
//
// 另立一个接口而不给 PickupRegistrationIntake 加方法：往既有接口加方法会打断它的每个实现者——这是
// 「会让旧调用点对不上」的那一类改动（ports.HandoverScopeView 注释里的同一条理由）。方法名按事实具名，
// 理由同 HandoverIntake：UnconfiguredIntake 一个类型要堵住本包全部命令面。
type PickupCorrectionIntake interface {
	IntakePickupCorrection(ctx context.Context, request *http.Request) (application.CorrectOffsitePickupCommand, error)
}

// PickupCorrectionHandler 是本适配器转交的应用编排。与 PickupRegistrationHandler 分成两个接口而不是
// 一个带两方法的接口，理由同上——生产装配点上的 RegisterOffsitePickupHandler 同时满足两个。
type PickupCorrectionHandler interface {
	Correct(
		ctx context.Context,
		command application.CorrectOffsitePickupCommand,
	) (application.RegisterOffsitePickupResult, error)
}

// NewCorrectOffsitePickupEndpoint 交回单对象揽收更正的 HTTP 入口。首登与更正分两个端点，理由同交接：
// 命令形状与恢复动作不同，合成一个入口就得靠请求体里的模式字段分路。响应形状与首登口共用
// （pickupRegistrationResponse）：更正落的是同一册里的一版，`corrects` 那一格对首登为空。
func NewCorrectOffsitePickupEndpoint(intake PickupCorrectionIntake, handler PickupCorrectionHandler) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RegisterOffsitePickupResult, error, bool) {
		command, err := intake.IntakePickupCorrection(request.Context(), request)
		if err != nil {
			return application.RegisterOffsitePickupResult{}, err, false
		}
		result, err := handler.Correct(request.Context(), command)
		return result, err, true
	}, writePickupRegistrationOutcome)
}
