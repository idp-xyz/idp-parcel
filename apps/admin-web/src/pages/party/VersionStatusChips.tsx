import type { VersionStatusCounts, VersionStatusFilter } from './version-groups';

// 版本列表的「全部 / 在用 / 其余」筛选（票 party-commercial-uiux/08）。样子照集团与法人页工作台的状态胶囊：
// 选中实底、带计数、`aria-pressed`。「在用」以服务端答的已生效为准，规则在 version-groups.ts。

const chips: { id: VersionStatusFilter; label: string; title: string; count: (counts: VersionStatusCounts) => number }[] = [
  { id: 'all', label: '全部', title: '所有版本', count: (counts) => counts.all },
  { id: 'in-use', label: '在用', title: '状态为已生效的版本', count: (counts) => counts.inUse },
  { id: 'other', label: '其余', title: '已发布未生效、已退役、已到期、已替代的版本', count: (counts) => counts.other },
];

export function VersionStatusChips({
  counts,
  value,
  onChange,
}: {
  counts: VersionStatusCounts;
  value: VersionStatusFilter;
  onChange: (next: VersionStatusFilter) => void;
}) {
  return (
    <div role="group" aria-label="版本状态" className="flex shrink-0 items-center gap-1">
      {chips.map((chip) => {
        const active = chip.id === value;
        return (
          <button
            key={chip.id}
            type="button"
            title={chip.title}
            aria-pressed={active}
            onClick={() => onChange(chip.id)}
            className={`whitespace-nowrap rounded px-2 py-1 text-[12px] transition-colors ${
              active ? 'bg-idpxyz-accent text-white' : 'text-idpxyz-breadcrumb hover:bg-idpxyz-hover hover:text-idpxyz-text'
            }`}
          >
            {chip.label}
            <span className="opacity-70"> {chip.count(counts)}</span>
          </button>
        );
      })}
    </div>
  );
}
