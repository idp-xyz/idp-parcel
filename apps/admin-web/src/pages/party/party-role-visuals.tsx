import { Building2, KeyRound, Route, ShieldCheck, Store, Truck, Users, type LucideIcon } from 'lucide-react';
import type { PartyTileKind } from './business-parties-workbench';

// 往来单位按类型取的图标与颜色（票 business-party-redesign/02）：行首的图标块、类型标签上的色点都从这里取，一种类型只有一套。
// 此前每一行都是同一个记号，占着位置却不帮人认行；现在客户、供应商、代理与我方公司一眼分得出。颜色不是唯一的载体——
// 类型一列照旧写着字。
//
// 色用 Tailwind 调色板的中间档配同色的淡底：浅色与深色主题下都分得出。主题色令牌在 tailwind.config.js 里是裸变量，没有
// 透明度变体，调不出淡底，所以只有我方公司（实底）与「都不是」（中性底）用主题色。
export const partyTileVisuals: Record<PartyTileKind, { icon: LucideIcon; tile: string; dot: string }> = {
  OURS: { icon: Building2, tile: 'bg-idpxyz-accent text-white', dot: 'bg-idpxyz-accent' },
  CUSTOMER: { icon: Users, tile: 'bg-sky-500/15 text-sky-600', dot: 'bg-sky-500' },
  SUPPLIER: { icon: Truck, tile: 'bg-teal-500/15 text-teal-600', dot: 'bg-teal-500' },
  CARRIER_AGENT: { icon: Route, tile: 'bg-violet-500/15 text-violet-500', dot: 'bg-violet-500' },
  RESELLER: { icon: Store, tile: 'bg-amber-500/15 text-amber-600', dot: 'bg-amber-500' },
  ACCOUNT_HOLDER: { icon: KeyRound, tile: 'bg-indigo-500/15 text-indigo-500', dot: 'bg-indigo-500' },
  AUTHORIZED_DELIVERING_PARTY: { icon: ShieldCheck, tile: 'bg-emerald-500/15 text-emerald-600', dot: 'bg-emerald-500' },
  NONE: { icon: Building2, tile: 'bg-idpxyz-hover text-idpxyz-breadcrumb', dot: 'bg-idpxyz-breadcrumb' },
};

/** 行首的图标块。只是视觉记号，对读屏隐藏：这一行是什么类型，类型一列有字。 */
export function PartyTile({ kind }: { kind: PartyTileKind }) {
  const { icon: Icon, tile } = partyTileVisuals[kind];
  return (
    <span className={`grid h-8 w-8 shrink-0 place-items-center rounded-lg ${tile}`} aria-hidden="true">
      <Icon className="h-4 w-4" />
    </span>
  );
}
