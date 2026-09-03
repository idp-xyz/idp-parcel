# 20 轨迹拉取节拍：`TrackingSource.Pull` 与 `Adopt` 之间没有生产入口

Category: enhancement
Status: draft
Blocked by: 18

## 缺口

票 `15` 落了出向端口 `TrackingSource.Pull`（拉取为首发形态），票 `16` 落了收编执行器
`AdoptTrackingMaterialHandler.Adopt`。两者之间要有一个东西按节奏调 `Pull`、把交回的每条 `TrackingMaterial`
喂给 `Adopt`、把 `NextCursor` 存起来下次接着拉——**今天没有**。`16` 刻意没接：没有任何一家真源，接一个空转的
节拍只会挂未配置（票 `16` 完成记录「刻意留下的三格」第 1 条）。

## 做什么

随**第一家真源**的适配器票一起立（渠道适配缝备忘「一类数据一张票」），本票只先记形状：

1. 节拍的宿主：照 `cmd/parcel-dispatch` 的拍子形状，还是另起 `cmd/parcel-tracking-pull`——要答，理由是两者的
   失败预算、租约与观察口不同。
2. 游标持久化：`PullCursor` 按（租户，轨迹源）存，不透明字符串照存；一拍的多个对象（`Subjects`）从哪里来
   ——是「该源下所有在途凭证」的读面，属票 `18` 的登记册。
3. 一拍内逐条 `Adopt` 的结果处置：`已认领`／`重复投递`／`留痕`入账；`未决`（登记册不可用等）如实按未决处理
   ——但**`凭证登记册未配置`那一格不是等依赖**，节拍在装配时就该拒绝启动，不要让它每拍报一次未决。
4. `答案未确定`不准在同一拍里重拉（票 `15` Answer 第三节）；多久再拉属 `PAR-INT-02`。

## 红线

- 不接任何真源、不填账号／地址／频率。
- 节拍不判断任何业务：三个时间、状态词、更正关系全部原样交 `Adopt`。

## 完成判据

随第一家真源的票一并给出；本票在此之前保持 `draft`。

## 参照

票 `15` Answer 第一、五节；票 `16` 完成记录；`internal/transportfulfillment/ports/tracking_source.go`。
