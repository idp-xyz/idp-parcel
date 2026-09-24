package accessidentity

import (
	"context"
	"errors"
	"time"
)

// ErrOutsideAdmissionScope 表示这次生产写不落在治理登记的生产权威区间内（ADR-0149 决定四第二条）。
// 恢复动作是阶段治理去登记区间，不是换令牌、也不是补授予，所以自成一格（403）。
var ErrOutsideAdmissionScope = errors.New("access identity: production write is outside the admission scope")

// ErrAdmissionScopeUnavailable 表示准入范围读不动。它不折进 ErrOutsideAdmissionScope：读不动要
// 运维去救，折成「不在准入范围」会让阶段治理去登一个其实早已登好的区间（分格判据同 ADR-0029）。
var ErrAdmissionScopeUnavailable = errors.New("access identity: admission scope cannot be read")

// AdmissionRequirement 是一个命令口的生产写在治理登记册上的能力与事实类型。它随口定、属产品策略；
// 对象范围不在这里——租户到对象范围的对照是租户随试点登记的实例半边，由 AdmissionScope 的实现去换。
type AdmissionRequirement struct {
	Capability string
	FactKind   string
}

// AdmissionScope 答某租户在 at 那一刻的一笔生产写落不落在准入范围内；读不动时交回错误，不答假。
type AdmissionScope interface {
	Admits(ctx context.Context, tenantID string, requirement AdmissionRequirement, at time.Time) (bool, error)
}

// UnconfiguredAdmissionScope 对一切生产写答不在准入范围：租户到治理坐标的对照没登之前就是这一格，
// 首个租户登记区间之前任何生产写都答「不在准入范围」是设计（ADR-0149 越权风险点 3）。
type UnconfiguredAdmissionScope struct{}

func (UnconfiguredAdmissionScope) Admits(context.Context, string, AdmissionRequirement, time.Time) (bool, error) {
	return false, nil
}
