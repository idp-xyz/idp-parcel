import { EmptyState } from '../components/states';
import { TechnicalDetails } from '../components/TechnicalDetails';
import { WAREHOUSE_SWITCH_ENV } from './warehouse-switch';

// 部署开关关着时，落在仓配模块地址上的标签（收藏、恢复的标签、深链）页面区摆的说明，代替仓配页本身。
//
// 摆成空态而不是错误态，也不走接入那一格的未配置态：这不是故障，也不是没接渠道——共享的未配置态会把
// ACCESS_CHANNEL_NOT_CONFIGURED 折进技术详情，而那个码说的是另一件事（要接入方去配渠道，这里要部署方打开开关），
// 把人指到错的地方（同一取舍见 notConnectedOverride）。正文说业务上怎么回事，开关名只放在折叠的技术详情里；
// 它与 workFactProblemNote 对同一个码的那句是同一个意思，只是不在明面上写开关名。
export function WarehouseNotEnabled() {
  return (
    <div className="flex flex-1 items-center justify-center overflow-auto">
      <div className="flex w-full max-w-[560px] flex-col items-center px-6">
        <EmptyState
          title="本部署没有启用仓配履约"
          description="仓配履约是可选能力，是否提供由部署方的一项开关决定，这个部署没有打开它。这不是故障：重试、重新登录或换账号都不会改变结果，需要时请联系部署方打开开关。"
        />
        <TechnicalDetails className="mt-3 text-idpxyz-textMuted" rows={[{ label: '开关', value: WAREHOUSE_SWITCH_ENV }]} />
      </div>
    </div>
  );
}
