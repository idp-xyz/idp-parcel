# 02 — 表单标签不再挂载荷路径

**Parent:** [spec](../spec.md)

**What to build:** `pages/party` 下的登记表单和发布表单，每个格子的标签后面都跟着一串载荷路径，比如 `legalEntities[0].legalEntityId`、`profiles[0].revision`。配置员读到的是一串实现名。

这条路径真正的用处，是把服务端点名的问题挂到对应的格子下面。这件事照旧，只是路径不再显示在标签上。

如果服务端点名的路径对不上任何一格，就按现有做法把路径连同问题一起显示出来，因为那时路径是有用的定位信息。

非载荷格的标签也不再带内部键名，比如法人资料区「按时点查看」后面那个 `at`。

**Blocked by:** None — can start immediately

**Category:** enhancement

**Status:** ready-for-agent

- [ ] `pages/party` 下所有表单的格子标签都不含载荷路径或内部键名
- [ ] 服务端点名的问题照旧挂在对应格子下；对不上任何格子的问题仍带路径显示
- [ ] 三道门退 0
