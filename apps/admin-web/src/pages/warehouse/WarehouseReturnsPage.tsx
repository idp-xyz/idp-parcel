import { useState } from 'react';
import { PackageCheck, PackageX, TabletSmartphone } from 'lucide-react';
import { Button, Card, CardContent, CardHeader, CardTitle } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import { useWarehouseLookups } from './use-catalogues';

const info = moduleInfoById['warehouse-returns'];

/** 退货管理页：按「退货预报 → 到仓收货 → 回库处置」三步清晰组织，不与入库预报或待认领混杂。 */
export function WarehouseReturnsPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(receivingActionSpecs);
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);

  const openAction = (id: string) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, nonce: (current?.nonce ?? 0) + 1 }));
  };

  return (
    <div className="flex-1 overflow-y-auto bg-idpxyz-editor">
      <div className="max-w-[1040px] mx-auto px-6 py-6 space-y-6">
        <header>
          <h1 className="type-title font-bold text-idpxyz-textBright">{info.title}</h1>
          <p className="mt-1 type-body text-idpxyz-textMuted">
            退货入库与原出库确认分离处理：先建退货预报，退件到仓后扫描入退货暂存位，再按货主指令执行良品上架、转次品或退还货主。
          </p>
        </header>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Card>
            <CardHeader className="pb-2">
              <div className="flex items-center gap-2 text-idpxyz-accent">
                <PackageX className="h-4 w-4" />
                <span className="font-mono type-caption font-bold">步骤 01</span>
              </div>
              <CardTitle className="type-section mt-1">退货预报</CardTitle>
              <p className="type-body text-idpxyz-textMuted">
                货主可经客户接口提交；运营代录预报时填写货主、退件单号、入库仓与预计退回明细。预计商品不入库存账。
              </p>
            </CardHeader>
            <CardContent>
              {targetMap.has('return-pre-advice') ? (
                <Button type="button" className={primaryActionClass} onClick={() => openAction('return-pre-advice')}>
                  代录退货预报
                </Button>
              ) : (
                <p className="type-caption text-idpxyz-textMuted">需持有退货代录（RETURN_RECORD_ON_BEHALF）授予。</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <div className="flex items-center gap-2 text-idpxyz-accent">
                <TabletSmartphone className="h-4 w-4" />
                <span className="font-mono type-caption font-bold">步骤 02</span>
              </div>
              <CardTitle className="type-section mt-1">退件到仓扫描入库</CardTitle>
              <p className="type-body text-idpxyz-textMuted">
                在仓内作业台扫描退件单号入退货暂存位；认不出货主时自动生成待认领到货，认领后可带待认领号入账。
              </p>
            </CardHeader>
            <CardContent>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  window.location.hash = '#/warehouse-workstation';
                }}
              >
                打开作业台 → 退货到仓入库
              </Button>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <div className="flex items-center gap-2 text-idpxyz-accent">
                <PackageCheck className="h-4 w-4" />
                <span className="font-mono type-caption font-bold">步骤 03</span>
              </div>
              <CardTitle className="type-section mt-1">退货回库处置</CardTitle>
              <p className="type-body text-idpxyz-textMuted">
                针对已入退货暂存位的到仓记录作出处置：良品上架（PUTAWAY）、转次品（DEFECTIVE）或退还货主（RETURN_TO_OWNER）。
              </p>
            </CardHeader>
            <CardContent>
              {targetMap.has('return-disposition') ? (
                <Button type="button" className={primaryActionClass} onClick={() => openAction('return-disposition')}>
                  代录回库处置
                </Button>
              ) : (
                <p className="type-caption text-idpxyz-textMuted">需持有退货代录（RETURN_RECORD_ON_BEHALF）授予。</p>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      <ActionDrawer
        moduleId="warehouse-returns"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
      />
    </div>
  );
}
