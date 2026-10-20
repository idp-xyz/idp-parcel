echo "== 操作者册登记（access-identity：合成操作者主体与能力面授予；票 operator-channel/01、17） =="
# 合成主体绑演示租户：配置员扮运营主管，持有全部运营决定（票 operator-channel/24「裁定」）——授登记册配置写、主数据与
# 运营查阅读，运营决定按 accessidentity 的 decisionKinds 逐种各授一行（各种名目见 README 授予表）；decisionKinds 多一种，
# data/access/02 的授予、03 的准入范围对照与 data/governance/ 的权威区间各补一行。授予只开门，不替任何人形成决定；
# 决定方从哪来逐口照 ADR-0151 决定二：复核完成、主动拒绝、授权处置取信封里的提交操作者作复核人、决定人、处置人，他有
# 没有权、凭哪一份授权由编排问 party-commercial；受控关闭与重开的命令里没有提交操作者，决定方、授权角色与授权依据快照
# 都由授权答复给出。委托侧各口越过 Intake 的前提与停点见 README「操作者渠道」一节授予表后的说明。查阅员只授查阅读，
# 继续证「授予不覆盖即拒」。
# 证据层级只记 S，不登任何真实操作者。发行方是演示发行方 Dex（operator-channel/compose.yaml），主体是
# 它给 dex.yaml 里静态用户真签出的令牌里的 sub（operator-token.sh … claims 读出），不按配置推算。
# 只登演示部署信任的那只发行方签得出的主体：iss 没有任何发行方签得出的主体，在册上与能登录的主体
# 长得一模一样，却永远登不进来。册上的主体不能改指到新发行方：换发行方就按新发行方的标识另登一批，
# 授予号与依据也另起。
"$BIN/parcel-access-register" operator-register -input "$SEEDS/access/01-operators.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/02-grants.json"
# 作业事实各口（收寄、揽收、交接、移动、派送、交付）要「操作者 × 设备 × 作业范围」三样都对上（ADR-0149 决定二）：
# 配置员的作业事实登记授予按作业范围给，每个范围登一台合成设备（票 product-strategy-boundary/30）。收寄只认节点
# 范围，给在演示网络的各节点上；运输履约各口只认场外范围，给在华东场外揽收与新加坡尾程派送两个范围上。场外范围是
# 租户自起的作业区名，不是网络目录对象，只比相等。作业事实不判准入范围，所以不登对照与区间。
"$BIN/parcel-access-register" device-register -input "$SEEDS/access/04-devices.json"
# 集成客户端族（ADR-0149 决定三）：外部结果、监管凭证与外部资金事实三类口要「客户端 × 授予」对上。合成报关服务商与
# 合成银行各一台，主体是演示的集成客户端发行方（operator-channel/integration-dex.yaml）签出的 sub；授予按事实类型给
# （票 product-strategy-boundary/31）。这三类事实的准入对照随下面的准入范围对照一起登。
# 另有合成干线承运商的承运系统一台（票 demo-chain-gaps/24）：只授承运订舱答复一类，来源身份是干线承运方 SYN-PARTY-CARRIER-TRUNK-01。
"$BIN/parcel-access-register" integration-client-register -input "$SEEDS/access/05-integration-clients.json"
"$BIN/parcel-access-register" integration-client-grant -input "$SEEDS/access/06-integration-client-grants.json"
# 首方客户族（ADR-0139 决定二；票 first-party-api/00）：提交口要「客户端 × 授予」对上。合成货主系统一台，主体是演示发行方
# （operator-channel/integration-dex.yaml，与集成客户端族同一只、受众不同）签出的 sub，绑 SYN-ACCOUNT-01——与隔离读面注入的
# 可见账户同一个，提交的委托才在委托查阅上看得见（读开关退场前）。授予按客户面能力给：委托提交，与送站预报的提交、撤回
# （票 ps-external-mark-relations/09）；预报两口的准入对照随下面的准入范围对照一起登。
"$BIN/parcel-access-register" customer-client-register -input "$SEEDS/access/07-customer-clients.json"
"$BIN/parcel-access-register" customer-client-grant -input "$SEEDS/access/08-customer-client-grants.json"
# 准入范围对照（ADR-0180 决定一、二；票 operator-channel/19）：演示租户在上面那几种（能力、事实类型）上的生产写
# 落在哪个治理对象范围。对照住接入身份、与操作者授予同处，不进 data/governance/；它只换坐标，区间才答「谁在写
# 生产」。演示租户越过准入范围只有这一条普通登记的路（ADR-0150 决定一）。配置员持有哪一种运营决定，这里就要有那一种
# 的对照，少了那一行，那一口授予齐备也答 OUTSIDE_ADMISSION_SCOPE；运营决定各口不论由管理台的页面调还是直接调 API 都经
# 它走，终止参与另供 probe-operator-channel.sh 取证。
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/03-admission-scope-mappings.json"
