package ports

import "go.idp.xyz/idp-parcel/internal/platform/cataloguepage"

// CataloguePage 已并入统一结果类型 cataloguepage.Result（ADR-0144 决定七「一处定义」）。别名只为不拆正在并行迁移的
// 分支：那些分支上的读端口、占位与替身还写着 ports.CataloguePage，删掉类型它们合进来就编不过。
type CataloguePage[Row any] = cataloguepage.Result[Row]
