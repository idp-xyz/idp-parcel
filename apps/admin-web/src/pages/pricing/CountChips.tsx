// 列表页过滤条上的「胶囊计数」：全部一格加若干带计数的格，选中实底、`aria-pressed`。样子照 party/VersionStatusChips
// （集团与法人页工作台的状态胶囊同款）；那件钉死了「全部 / 在用 / 其余」三格，这里格由调用方给，供产品与价格区几张
// ListPageTemplate 页按结果或种类即筛。再点一次选中的格回到全部。

export interface CountChip {
  id: string;
  label: string;
  count: number;
}

export function CountChips({
  label,
  total,
  chips,
  value,
  onChange,
}: {
  /** 整组的无障碍名（如「结果」）。 */
  label: string;
  total: number;
  chips: readonly CountChip[];
  /** 当前选中的格 id；null 即全部。 */
  value: string | null;
  onChange: (next: string | null) => void;
}) {
  const all: CountChip = { id: '', label: '全部', count: total };
  return (
    <div role="group" aria-label={label} className="flex shrink-0 flex-wrap items-center gap-1">
      {[all, ...chips].map((chip) => {
        const id = chip.id === '' ? null : chip.id;
        const active = id === value;
        return (
          <button
            key={chip.id || 'all'}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(id === null || active ? null : id)}
            className={`whitespace-nowrap rounded px-2 py-1 text-[12px] transition-colors ${
              active ? 'bg-idpxyz-accent text-white' : 'text-idpxyz-breadcrumb hover:bg-idpxyz-hover hover:text-idpxyz-text'
            }`}
          >
            {chip.label}
            <span className="opacity-70"> {chip.count}</span>
          </button>
        );
      })}
    </div>
  );
}
