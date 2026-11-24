import { moduleInfoById } from '../../navigation';
import { pricingActionSpecs } from './actions';
import { WarehouseActionPanel, useWarehouseActionTargets } from './ActionPanel';

const info = moduleInfoById['warehouse-pricing'];

/**
 * 仓储计价与收费配置页。
 * 涵盖仓储计费方案登记、计费采用、重量体积单位、库龄口径、增值服务项目与仓租日结。
 * 这些格均为租户取值，未登记时答未配置，不写生产默认。
 */
export function WarehousePricingPage() {
  const targets = useWarehouseActionTargets(pricingActionSpecs);

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <div className="px-4 pt-4 pb-2 border-b border-idpxyz-border shrink-0">
        <h2 className="type-title font-medium text-idpxyz-text">{info.title}</h2>
        <p className="mt-0.5 type-body text-idpxyz-textMuted">
          登记仓储计费方案、货主与仓的计费采用、仓的重量体积单位、库龄口径、增值服务项目，以及按自然日触发仓租日结。
        </p>
      </div>
      <WarehouseActionPanel moduleId="warehouse-pricing" targets={targets} />
    </div>
  );
}
