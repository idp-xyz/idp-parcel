import { exchangeMasterData } from '../pages/catalogue-api';
import { warehouseSwitchOf, type WarehouseSwitch } from './warehouse-switch';

// 仓配部署开关的探针：问一次、交回开关的三态。外壳入口块静态引本文件，所以它只引 exchangeMasterData 与判读函数——
// 多引一个域内模块（读口、答复整形、状态件），那个模块与它带的一串就一起进了入口块。
//
// 读仓库册的第一页，不带任何条件：开关关着时查询串不被读，开着时缺省的第一页就够了。选这一口是因为它最轻——不要货主、不要
// 仓编码，一页里只有这个租户登记过的几个仓；开关关着时仓配各口都在认证之前答同一份固定的 403，选哪一口答的都一样，不必为探它
// 另开一口。路径与 pages/warehouse/rows.ts 的 catalogueReadPaths.warehouse 是同一条；探针不能引 rows（它带着域内的一串），
// 由探针的测试把两处钉在一起。
const WAREHOUSE_SWITCH_PROBE_PATH = '/warehouse-fulfillment-warehouses';

/** 交回的永远是三态之一：读口函数不会拒绝（传输失败也落成答复），拒绝那一支只防真有意外——不知道，就照旧摆。 */
export function probeWarehouseSwitch(): Promise<WarehouseSwitch> {
  return exchangeMasterData<unknown>(WAREHOUSE_SWITCH_PROBE_PATH).then(
    warehouseSwitchOf,
    (): WarehouseSwitch => 'unknown',
  );
}
