# 01 注册号类型目录：按注册国家 / 地区登记注册号类型、格式与所属层

Category: enhancement
Status: in-progress——2026-09-24 通道 4 认领（派单 `task-c785cb7e` ← 通道 3），隔离 worktree 分支 `mcp4-lep01`，基本笔
Blocked by: 无
地盘：party-commercial 的领域、应用、postgres 与 http 适配器里新增的一本登记册，`migrations/` 下 party-commercial 模块的新迁移，`scripts/demo-seeds` 的合成条目。
出处：[ADR-0145](../../../docs/adr/0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定一；CONTEXT Rules「责任法人身份登记必须带注册国家 / 地区……」一句。

## 做什么

1. 一本按租户的登记册，每个类型带：注册国家 / 地区、类型代码、名称、格式校验、所属层（身份层的终身注册号 / 资料层的税务登记号）。按修订版本化、不可覆盖，
   登记与停用都带依据——与本上下文其余登记册同一纪律。
2. 登记写面进端点表（未配置即拒，沿既有登记写面的通例）；目录读口沿 ADR-0077。
3. 给 02、03 用的领域校验：给定国家 / 地区与层，某个号是否属目录里的某一类型且格式合格；目录里没有该国家 / 地区时答「未登记」，不以默认格式代替。
4. 演示种子只在合成租户下登记演示用条目。

## 不做

- 不改责任法人身份登记（归 02）。不带任何国家的生产默认条目。

## 完成判据

- 真库用例：登记、修订、停用；层不符、格式不符、国家未登记三种拒绝各一条。
- 迁移按字节无 CR、无 BOM；`go test ./internal/architecture/ -count=1` 过。
