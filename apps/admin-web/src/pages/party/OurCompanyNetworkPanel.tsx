import { X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { businessPartyObjectHash } from './business-parties-address';
import { PartyRelationshipGraph } from './PartyRelationshipGraph';
import { relationshipGraphOf, type RelationshipGraphLimits } from './party-relationship-graph';
import type { OurCompanyNetworkRow } from './our-company-network';

// 「我方关联」一册的右栏（票 business-party-relationship-graph/02）：以选中的那家我方公司为中心的关系图。图与往来单位「关联」签
// 是同一个组件、同一套摆法（party-relationship-graph.ts）；这里地方更大，合成门槛放宽一些，一家公司名下十来个客户也逐家摆出。
const networkLimits: RelationshipGraphLimits = { role: 8, column: 12 };

export function OurCompanyNetworkPanel({
  row,
  relationshipsComplete,
  onClose,
  onShowRelationships,
}: {
  row: OurCompanyNetworkRow;
  /** 合作关系册不带条件那一问已是整册（complete-register.ts）：不是就在图下说一句，数与图只按到手的那一页。 */
  relationshipsComplete: boolean;
  onClose: () => void;
  /** 合成节点、「已结束」那句点了：去合作关系册看这家公司名下的全部关系。 */
  onShowRelationships: () => void;
}) {
  const name = row.partyName ?? row.partyId;
  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`${name}的关联`}>
      <div className="space-y-2 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate type-title font-semibold text-idpxyz-textBright">{name}</h2>
              <span className="rounded border border-idpxyz-border px-1.5 py-0.5 type-body text-idpxyz-text">我方公司</span>
            </div>
            <p className="mt-0.5 type-body text-idpxyz-breadcrumb">
              在用合作关系 {row.live} 段{row.candidates > 0 ? `，其中 ${row.candidates} 段待确认` : ''}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <a
              href={businessPartyObjectHash(row.partyId)}
              className="rounded border border-idpxyz-border px-2 py-1 type-body text-idpxyz-text hover:bg-idpxyz-hover"
            >
              打开往来单位
            </a>
            <Button className="text-idpxyz-breadcrumb" variant="ghost" size="icon" onClick={onClose} aria-label="关闭详情" title="关闭（Esc）">
              <X className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
        </div>
        <dl className="flex flex-wrap gap-x-4 gap-y-1 type-body">
          <div className="flex gap-1.5">
            <dt className="text-idpxyz-breadcrumb">编码</dt>
            <dd className="font-mono text-idpxyz-text">{row.partyId}</dd>
          </div>
          <div className="flex gap-1.5">
            <dt className="text-idpxyz-breadcrumb">公司主体</dt>
            <dd className="flex flex-wrap gap-x-2 font-mono">
              {row.legalEntityIds.map((id) => (
                <a key={id} href={`#/group-legal-entities/${encodeURIComponent(id)}`} className="text-idpxyz-accent hover:underline">
                  {id}
                </a>
              ))}
            </dd>
          </div>
        </dl>
      </div>
      <div className="flex-1 overflow-auto px-5 py-4">
        <PartyRelationshipGraph
          graph={relationshipGraphOf(row.views, networkLimits)}
          centerName={row.partyName}
          centerId={row.partyId}
          ours
          onShowDetail={onShowRelationships}
        />
        {relationshipsComplete ? null : (
          <p className="mt-2 type-caption text-idpxyz-breadcrumb">
            合作关系册不止一页，这里只按已取到的那一页画；全部关系在「合作关系」册里按这家公司筛看。
          </p>
        )}
      </div>
    </div>
  );
}
