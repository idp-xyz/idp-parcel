echo "== 周期安排登记（transport-fulfillment：演示网络三条连接各一份按日运行的周期安排；票 demo-chain-gaps/16） =="
# 周期安排是 TF 自有登记册（ADR-0201 决定一）：方向写演示网络的连接代码与起讫节点，时区取那条连接的业务时区，
# 服务范围取演示的 SYN-SCOPE-01。出发时刻与历时是合成分钟数，不是任何线路的真实班期；准备评估按计划段窗口物化
# 候选，窗口对不上就调这里的时刻，不改匹配规则（票 demo-chain-gaps/18）。具体班次不在种子里预生成：准备评估时
# 物化，或经 schedule 命令 / 在线口按运行日期显式登记。
"$BIN/parcel-transport-register" schedule-template -input "$SEEDS/transport/01-schedule-template-sha-szx.json"
"$BIN/parcel-transport-register" schedule-template -input "$SEEDS/transport/02-schedule-template-szx-sin.json"
"$BIN/parcel-transport-register" schedule-template -input "$SEEDS/transport/03-schedule-template-sin-lm.json"
