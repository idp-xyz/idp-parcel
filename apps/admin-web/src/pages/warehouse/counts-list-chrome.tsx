import { WmsFilterSelect } from './wms-ui';
import type { ActionDrawerControl } from './ActionDrawer';

/** 盘点与调整四张列表共用的货主、仓筛选：名册还没读到时下拉里只有「全部」。 */
export function OwnerWarehouseFilters({
  drawer,
  owner,
  warehouse,
  onOwner,
  onWarehouse,
}: {
  drawer: ActionDrawerControl;
  owner: string;
  warehouse: string;
  onOwner: (value: string) => void;
  onWarehouse: (value: string) => void;
}) {
  return (
    <>
      <WmsFilterSelect
        label="货主"
        value={owner}
        onChange={onOwner}
        options={drawer.lookups.owners.map((account) => ({
          value: account.accountId,
          label: account.customerPartyName ? `${account.customerPartyName} · ${account.accountId}` : account.accountId,
        }))}
      />
      <WmsFilterSelect
        label="仓库"
        value={warehouse}
        onChange={onWarehouse}
        options={drawer.lookups.warehouses.map((item) => ({
          value: item.warehouseCode,
          label: item.name ? `${item.name} · ${item.warehouseCode}` : item.warehouseCode,
        }))}
      />
    </>
  );
}

export function ClearFilters({ onClear }: { onClear: () => void }) {
  return (
    <button
      type="button"
      className="whitespace-nowrap type-body text-idpxyz-textMuted hover:text-idpxyz-textBright focus-visible:underline focus-visible:outline-none"
      onClick={onClear}
    >
      清除
    </button>
  );
}
