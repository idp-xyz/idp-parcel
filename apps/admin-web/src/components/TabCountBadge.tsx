export function TabCountBadge({ count }: { count: number }) {
  return (
    <span className="ml-1 inline-flex h-5 min-w-5 items-center justify-center rounded bg-idpxyz-hover px-1.5 text-[11px] font-medium leading-none tabular-nums text-idpxyz-breadcrumb">
      {count}
    </span>
  );
}
