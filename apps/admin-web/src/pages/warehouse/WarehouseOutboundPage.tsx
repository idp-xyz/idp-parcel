import { moduleInfoById } from '../../navigation';
import { outboundWaveActionSpecs } from './actions';
import { WarehouseActionPanel, useWarehouseActionTargets } from './ActionPanel';

const info = moduleInfoById['warehouse-outbound'];

/**
 * 出库、波次与代理仓页。
 * 后端还没有出库请求和波次的运营分页列表口，本页直接摆出所有已接线的运营决定与登记动作；
 * 拣货、复核、打包、出库确认与自提交付是作业事实口，留在一线作业端。
 */
export function WarehouseOutboundPage() {
  const targets = useWarehouseActionTargets(outboundWaveActionSpecs);

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <div className="px-4 pt-4 pb-2 border-b border-idpxyz-border shrink-0">
        <h2 className="type-title font-medium text-idpxyz-text">{info.title}</h2>
        <p className="mt-0.5 type-body text-idpxyz-textMuted">
          出库代录、拆单、截单、放波次、缺货处置、分配策略采用、确认更正回补，以及代理仓连接器、库存声明与结果回传。拣货、复核、打包与出库确认走一线作业端。
        </p>
      </div>
      <WarehouseActionPanel moduleId="warehouse-outbound" targets={targets} />
    </div>
  );
}
