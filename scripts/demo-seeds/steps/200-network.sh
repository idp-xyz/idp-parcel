echo "== 网络目录登记（network-routing：演示租户采用参考配置 network-routing/network-catalog/SYN-CN-SG@1） =="
# 演示网络随产品发布为参考配置（票 routing-first-cut/11，ADR-0147）。下面每一行稳定定义都是一笔采用：
# 登记行只给身份、修订号与生效时点，内容取自那一版参考配置，依据格写成 REFCFG-1 引用串。登记行不点名
# adopt，参考配置就不进任何租户的目录。日历是四个节点各一份（处理时长与截单）加三条连接各一份（衔接
# 缓冲）——候选经过的每个节点都要有处理时长才得到时间投影（ADR-0175）。
# 上海枢纽的修订 2 不是采用：它演示「采用后再改就是租户取值」，依据换成演示租户自己的 SYN- 依据。
"$BIN/parcel-network-register" -kind node -file "$SEEDS/network/01-node-sha-hub-v1.json"
"$BIN/parcel-network-register" -kind node -file "$SEEDS/network/02-node-szx-gate-v1.json"
"$BIN/parcel-network-register" -kind node -file "$SEEDS/network/03-node-sin-hub-v1.json"
"$BIN/parcel-network-register" -kind node -file "$SEEDS/network/04-node-sin-lm-v1.json"
"$BIN/parcel-network-register" -kind node -file "$SEEDS/network/05-node-sha-hub-v2.json"
"$BIN/parcel-network-register" -kind connection -file "$SEEDS/network/06-conn-sha-szx-v1.json"
"$BIN/parcel-network-register" -kind connection -file "$SEEDS/network/07-conn-szx-sin-v1.json"
"$BIN/parcel-network-register" -kind connection -file "$SEEDS/network/08-conn-sin-lm-v1.json"
"$BIN/parcel-network-register" -kind line -file "$SEEDS/network/09-line-cn-sg-v1.json"
"$BIN/parcel-network-register" -kind service-area -file "$SEEDS/network/10-area-cn-east-v1.json"
"$BIN/parcel-network-register" -kind service-area -file "$SEEDS/network/11-area-sg-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/12-calendar-node-sha-hub-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/13-calendar-node-szx-gate-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/14-calendar-node-sin-hub-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/15-calendar-node-sin-lm-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/16-calendar-conn-sha-szx-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/17-calendar-conn-szx-sin-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/18-calendar-conn-sin-lm-v1.json"
# 台风停运是一条带来源的临时调整陈述，不随参考配置发，照旧直接登记。
"$BIN/parcel-network-register" -kind availability-adjustment -file "$SEEDS/network/19-adjustment-typhoon-v1.json"
"$BIN/parcel-network-register" -kind route-strategy -file "$SEEDS/network/20-route-strategy-cn-sg-v1.json"
# 自动改路事实是登记方折算完的陈述（0009），不是初始路由跑出来的计划。判断键对齐
# SYN-ACCOUNT-01 与 SYN-RS-CN-SG-01，不指向一笔已经受理的委托。同键再登答退出码 2，
# 已灌过的库重跑本节请用 --reset。
"$BIN/parcel-network-register" -kind auto-reroute-facts -file "$SEEDS/network/21-auto-reroute-facts-cn-sg-v1.json"
# 段计价区域（ADR-0189 决定二）：演示线路各段共引那张成本卡，卡没绑分区目录、只有 Z1，每段都登 Z1——这是演示租户的
# 租户取值，不是产品默认。没登时候选成本停在待判断。区域挂在已在册的线路版本上，所以排在线路之后。
"$BIN/parcel-network-register" -kind leg-pricing-zone -file "$SEEDS/network/22-leg-pricing-zones-cn-sg-v1.json"
# 路由策略修订 2 是采用之后的租户修订（同上海枢纽修订 2）：声明冻结形态「剩余段数」，剩余不超过一段即冻结。采用行装不下
# 这一格——冻结段数是租户取值，参考配置不带。没有它，收寄后的路由复核在计划仍适用时答冻结形态未配置（票
# product-strategy-boundary/34）。
"$BIN/parcel-network-register" -kind route-strategy -file "$SEEDS/network/23-route-strategy-cn-sg-v2.json"
# 新加坡末端日历修订 2 也是采用之后的租户修订（ADR-0217）：照修订 1 重登处理时长，另登派送时长——合成值，不是任何
# 地区的惯例，取值见 README 组成表 24-calendar-node-sin-lm-v2.json 一行。登了它，计划在末端之后多一段交付段，末端交接
# 声明这一段，派送任务窗口就取它；没登时计划止于末端。修订 2 无终点，登记时接续闭合修订 1，判断时点只有一版适用。
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/24-calendar-node-sin-lm-v2.json"
