/** Web icon set named by the SDUI `icon` prop (specs/http/bff.md, Values). */
const ICONS: Readonly<Record<string, string>> = {
  in: 'M12 4v12M6 10l6 6 6-6M4 20h16',
  out: 'M12 20V8M6 14l6-6 6 6M4 4h16',
  msg: 'M4 5h16v11H8l-4 4z',
  alert: 'M12 3l10 18H2zM12 10v5M12 18v.5',
  segment: 'M12 3l2.6 5.6 6.1.7-4.5 4.2 1.2 6L12 16.6 6.6 19.5l1.2-6L3.3 9.3l6.1-.7z',
  drop: 'M3 7l6 6 4-4 8 8M14 17h7v-7',
  inbox: 'M4 13l2-8h12l2 8v6H4zM4 13h5l1 2h4l1-2h5',
  cash: 'M3 7h18v10H3zM12 9.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 1 0 0-5zM6 12h.5M17.5 12h.5',
  calendar: 'M4 6h16v14H4zM4 10h16M8 3v4M16 3v4',
  spark: 'M12 3v4M12 17v4M3 12h4M17 12h4M6 6l2.5 2.5M15.5 15.5L18 18M6 18l2.5-2.5M15.5 8.5L18 6',
  deposit: 'M12 3a9 9 0 1 0 0 18 9 9 0 1 0 0-18zM12 8v8M8 12h8',
  withdraw: 'M3 10l9-6 9 6M5 10v8M9 10v8M15 10v8M19 10v8M3 20h18',
};

/** Reports whether `name` is in the icon set. */
export function hasIcon(name: unknown): name is string {
  return typeof name === 'string' && Object.hasOwn(ICONS, name);
}

/** Draws a named icon; an unknown name renders no icon. */
export function SduiIcon({ name, size }: { name: unknown; size: number }) {
  if (!hasIcon(name)) {
    return null;
  }
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={ICONS[name]} />
    </svg>
  );
}

export function ArrowIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M5 12h14M13 6l6 6-6 6" />
    </svg>
  );
}

export function EyeIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12zM12 9a3 3 0 1 0 0 6 3 3 0 1 0 0-6z" />
    </svg>
  );
}
