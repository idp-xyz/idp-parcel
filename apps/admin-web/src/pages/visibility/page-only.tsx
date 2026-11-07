// 只作用于本页的控件（票 catalogue-read-pagination-rollout/14 的裁定：读口撑不起的页面能力不悄悄丢，便宜的留成页内控件、字面写明）。
// 检索框与状态胶囊已下推到读口；读口检索不覆盖的列（往来单位名称、码的中文词、规则条目的内容）只能在到手的这一页上找，
// 这里的框只筛这一页，翻页之后照新的一页再筛。

const pageOnlySearchClass =
  'h-6 w-48 shrink-0 rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-caption text-idpxyz-text ' +
  'placeholder:text-idpxyz-textMuted outline-none focus-visible:border-idpxyz-accent';

/** 本页内再筛的小框；提示语后缀「只筛本页」，读屏器念同一句。 */
export function PageOnlySearch({
  value,
  onChange,
  subject,
}: {
  value: string;
  onChange: (value: string) => void;
  /** 能筛什么，如「客户名称、索赔类型」。 */
  subject: string;
}) {
  const label = `${subject}（只筛本页）`;
  return (
    <input
      type="search"
      value={value}
      onChange={(event) => onChange(event.target.value)}
      placeholder={label}
      aria-label={label}
      title={`${label}：读口检索不覆盖这几项，只在当前这一页上筛，翻页后照新的一页再筛。`}
      className={pageOnlySearchClass}
    />
  );
}

/** 页内控件旁的一句说明（如工作台上状态胶囊只筛本页）。 */
export function PageOnlyNote({ children }: { children: string }) {
  return <span className="shrink-0 type-caption text-idpxyz-textMuted">{children}</span>;
}
