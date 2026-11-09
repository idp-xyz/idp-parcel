import type { VersionStatusFilter } from './version-groups';

// 版本列表的「全部 / 在用 / 其余」筛选（票 party-commercial-uiux/08）。样子照集团与法人页工作台的状态胶囊：选中实底、
// `aria-pressed`。选中的那一格下推成读口的 status 维（version-groups.ts）；胶囊不带计数——一页里数出来的不是各状态的总数，
// 按状态的总数读口不给，当前条件下共几份由翻页条说。

const chips: { id: VersionStatusFilter; label: string; title: string }[] = [
  { id: 'all', label: '全部', title: '所有版本' },
  { id: 'in-use', label: '在用', title: '状态为已生效的版本' },
  { id: 'other', label: '其余', title: '已发布未生效、已退役、已到期、已替代的版本' },
];

export function VersionStatusChips({
  value,
  onChange,
}: {
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
            className={`whitespace-nowrap rounded px-2 py-1 type-body transition-colors ${
              active ? 'bg-idpxyz-accent text-white' : 'text-idpxyz-breadcrumb hover:bg-idpxyz-hover hover:text-idpxyz-text'
            }`}
          >
            {chip.label}
          </button>
        );
      })}
    </div>
  );
}
