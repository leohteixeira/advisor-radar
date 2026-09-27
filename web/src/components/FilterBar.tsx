import { ICONS } from '../domain/queueVisual';
import type { FilterKey, QueueFilters } from '../domain/queueVisual';

export interface FilterGroup {
  key: FilterKey;
  label: string;
  align: 'left' | 'right';
  options: { label: string; count: number }[];
}

interface FilterBarProps {
  query: string;
  onQuery: (value: string) => void;
  filters: QueueFilters;
  groups: FilterGroup[];
  open: FilterKey | null;
  onToggle: (key: FilterKey) => void;
  onClose: () => void;
  onToggleOption: (key: FilterKey, option: string) => void;
  onClear: () => void;
}

function Icon({ d, size = 14 }: { d: string; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d={d} />
    </svg>
  );
}

const ICON_SEARCH = ICONS.search ?? '';
const ICON_CHEVRON = ICONS.chevron ?? '';
const ICON_CHECK = ICONS.check ?? '';
const ICON_CLOSE = ICONS.close ?? '';

export function FilterBar({
  query,
  onQuery,
  filters,
  groups,
  open,
  onToggle,
  onClose,
  onToggleOption,
  onClear,
}: FilterBarProps) {
  const tags = groups.flatMap((g) =>
    filters[g.key].map((option) => ({
      key: `${g.key}:${option}`,
      label: `${g.label}: ${option}`,
      remove: () => onToggleOption(g.key, option),
    })),
  );
  if (query.trim()) {
    tags.unshift({
      key: 'q',
      label: `Busca: “${query.trim()}”`,
      remove: () => onQuery(''),
    });
  }

  return (
    <div className="filter-bar">
      <div className="filter-bar__row">
        <label className="filter-bar__search">
          <span className="filter-bar__search-icon">
            <Icon d={ICON_SEARCH} size={15} />
          </span>
          <input
            value={query}
            onChange={(e) => onQuery(e.target.value)}
            placeholder="Buscar cliente"
            aria-label="Buscar cliente"
          />
        </label>
        {groups.map((g) => {
          const selected = filters[g.key];
          const isOpen = open === g.key;
          return (
            <div key={g.key} className={`filter-bar__group filter-bar__group--${g.align}`}>
              <button
                type="button"
                className={
                  isOpen || selected.length > 0 ? 'filter-bar__btn filter-bar__btn--on' : 'filter-bar__btn'
                }
                aria-expanded={isOpen}
                onClick={() => onToggle(g.key)}
              >
                {g.label}
                {selected.length > 0 ? <span className="filter-bar__count">{selected.length}</span> : null}
                <Icon d={ICON_CHEVRON} size={12} />
              </button>
              {isOpen ? (
                <>
                  <button type="button" className="filter-bar__backdrop" aria-label="Fechar filtros" onClick={onClose} />
                  <div className="filter-bar__menu" role="group" aria-label={g.label}>
                    {g.options.map((o) => {
                      const on = selected.includes(o.label);
                      return (
                        <button
                          key={o.label}
                          type="button"
                          className="filter-bar__option"
                          aria-pressed={on}
                          onClick={() => onToggleOption(g.key, o.label)}
                        >
                          <span className={on ? 'filter-bar__box filter-bar__box--on' : 'filter-bar__box'}>
                            {on ? <Icon d={ICON_CHECK} size={11} /> : null}
                          </span>
                          <span>{o.label}</span>
                          <span className="filter-bar__option-count">{o.count}</span>
                        </button>
                      );
                    })}
                  </div>
                </>
              ) : null}
            </div>
          );
        })}
      </div>
      {tags.length > 0 ? (
        <div className="filter-bar__tags">
          {tags.map((t) => (
            <button key={t.key} type="button" className="filter-bar__tag" onClick={t.remove}>
              {t.label}
              <Icon d={ICON_CLOSE} size={11} />
            </button>
          ))}
          <button type="button" className="filter-bar__clear" onClick={onClear}>
            Limpar filtros
          </button>
        </div>
      ) : null}
    </div>
  );
}
