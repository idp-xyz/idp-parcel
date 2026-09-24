package tfhttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// 命令载荷的两道共用解码门。载荷只有内容没有身份：租户（以及契约点名来自认证结果的其余几格）由 Intake 作为入参交进
// 各载荷的 Command，与本包判断载荷的分工同形。

// decodeClosedPayload 以封闭形状解载荷。未知键拒：载荷里出现 tenantId 之类的键不是可以忽略的噪声，是采信自报的入口；
// 尾随的第二个 JSON 值也拒：一份载荷只许一个文档，放过它就是无声丢掉一段输入。
func decodeClosedPayload(body io.Reader, target any) error {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the payload", ErrMalformedRequest)
	}
	return nil
}

// parseOptionalInstant 按 RFC 3339 解一个时刻。空着交回零值，留给编排答`未受理`（200，形成了的业务答案）；给了却解不出
// 是坏报文（400）——两者的续办不同，不折成一格。缺席也绝不拿服务端时钟补：那是 ADR-0023 禁的代铸。
func parseOptionalInstant(key, raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s is not an RFC 3339 instant: %v", ErrMalformedRequest, key, err)
	}
	return at.UTC(), nil
}
