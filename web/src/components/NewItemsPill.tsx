interface NewItemsPillProps {
  count: number;
  onMerge: () => void;
}

export function NewItemsPill({ count, onMerge }: NewItemsPillProps) {
  if (count <= 0) {
    return null;
  }
  const label = count === 1 ? '1 novo' : `${count} novos`;
  return (
    <button type="button" className="new-items-pill" onClick={onMerge}>
      {label}
    </button>
  );
}
