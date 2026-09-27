import { useCallback, useEffect, useRef, useState } from 'react';
import {
  advanceCase,
  deleteAction,
  fetchCases,
  fetchQueue,
  putAction,
  queueStreamUrl,
} from '../api/bff';
import { DetailModal, type DetailSelection } from '../components/DetailModal';
import { FilterBar, type FilterGroup } from '../components/FilterBar';
import { NewItemsPill } from '../components/NewItemsPill';
import { QueueBoard } from '../components/QueueBoard';
import { Toast } from '../components/Toast';
import type { CaseItem, ListFacets, Signal } from '../domain/types';
import {
  ANDAMENTO_OPTIONS,
  columnVisible,
  dateLabel,
  decorateCase,
  decorateSignal,
  EMPTY_FILTERS,
  type FilterKey,
  type QueueFilters,
} from '../domain/queueVisual';

const DESKTOP_MQ = '(min-width: 900px)';

interface ToastState {
  text: string;
  undoId?: string;
}

export interface QueueScreenProps {
  /** Override matchMedia for tests. */
  isDesktop?: boolean;
  /** Skip live SSE in tests. */
  disableStream?: boolean;
}

function listParams(query: string, filters: QueueFilters) {
  return {
    q: query,
    andamento: filters.andamento,
    sla: filters.sla,
    sinal: filters.sinal,
    motivo: filters.motivo,
    segmento: filters.segmento,
  };
}

function facetOptions(
  facets: ListFacets | null,
  key: FilterKey,
  fallbackLabels: readonly string[],
): { label: string; count: number }[] {
  const rows = facets?.[key] ?? [];
  if (rows.length > 0) {
    return rows.map((r) => ({ label: r.label, count: r.count }));
  }
  return fallbackLabels.map((label) => ({ label, count: 0 }));
}

export function QueueScreen({ isDesktop, disableStream }: QueueScreenProps) {
  const [items, setItems] = useState<Signal[]>([]);
  const [incoming, setIncoming] = useState<Signal[]>([]);
  const [cases, setCases] = useState<CaseItem[]>([]);
  const [facets, setFacets] = useState<ListFacets | null>(null);
  const [selection, setSelection] = useState<DetailSelection | null>(null);
  const [toast, setToast] = useState<ToastState | null>(null);
  const [desktop, setDesktop] = useState(() => {
    if (typeof isDesktop === 'boolean') {
      return isDesktop;
    }
    return window.matchMedia(DESKTOP_MQ).matches;
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [query, setQuery] = useState('');
  const [filters, setFilters] = useState<QueueFilters>(EMPTY_FILTERS);
  const [openFilter, setOpenFilter] = useState<FilterKey | null>(null);
  const lastEventId = useRef<string>('');
  const seenIds = useRef<Set<string>>(new Set());
  const incomingRef = useRef<Signal[]>([]);
  incomingRef.current = incoming;

  useEffect(() => {
    if (typeof isDesktop === 'boolean') {
      setDesktop(isDesktop);
      return;
    }
    const mq = window.matchMedia(DESKTOP_MQ);
    const onChange = () => setDesktop(mq.matches);
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, [isDesktop]);

  const load = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const params = listParams(query, filters);
      const [queue, caseRes] = await Promise.all([fetchQueue(params), fetchCases(params)]);
      const pill = incomingRef.current;
      const pillIds = new Set(pill.map((s) => s.id));
      setItems(queue.items.filter((s) => !pillIds.has(s.id)));
      setCases(caseRes.items);
      setFacets(queue.facets);
      const seen = new Set(queue.items.map((s) => s.id));
      for (const id of pillIds) {
        seen.add(id);
      }
      seenIds.current = seen;
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }, [query, filters]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (disableStream) {
      return;
    }
    let es: EventSource | null = null;
    let closed = false;

    const connect = () => {
      if (closed) {
        return;
      }
      es = new EventSource(queueStreamUrl());
      es.addEventListener('signal', (ev) => {
        const msg = ev as MessageEvent<string>;
        if (msg.lastEventId) {
          lastEventId.current = msg.lastEventId;
        }
        try {
          const signal = JSON.parse(msg.data) as Signal;
          if (seenIds.current.has(signal.id)) {
            return;
          }
          seenIds.current.add(signal.id);
          setIncoming((prev) => {
            if (prev.some((p) => p.id === signal.id)) {
              return prev;
            }
            return [signal, ...prev];
          });
        } catch {
          // ignore malformed frames
        }
      });
    };

    connect();
    return () => {
      closed = true;
      es?.close();
    };
  }, [disableStream]);

  const mergeIncoming = () => {
    incomingRef.current = [];
    setIncoming([]);
    void load();
  };

  const onViewCase = (id: string) => {
    const caseItem = cases.find((c) => c.id === id);
    if (caseItem) {
      setSelection({ type: 'case', caseItem });
    }
  };

  const onOpenCase = (id: string) => {
    const signal = items.find((s) => s.id === id);
    if (!signal) {
      return;
    }
    const opened: CaseItem = {
      id: `k-local-${id}`,
      client: signal.client,
      name: signal.name,
      segment: signal.segment,
      signal: id,
      state: 0,
      openedAgo: 0,
      slaTotal: 120,
      escalated: false,
      history: [],
    };
    setCases((prev) => [opened, ...prev]);
    setItems((prev) => prev.filter((s) => s.id !== id));
    setSelection((cur) => {
      if (cur?.type === 'signal' && cur.signal.id === id) {
        return null;
      }
      if (!desktop) {
        return null;
      }
      return cur;
    });
  };

  const onContacted = async (id: string) => {
    try {
      await putAction(id, 'contact');
      const hit = items.find((s) => s.id === id);
      setItems((prev) =>
        prev.map((s) =>
          s.id === id ? { ...s, contacted_at: new Date().toISOString() } : s,
        ),
      );
      setToast({
        text: `${hit?.name ?? hit?.client ?? 'Cliente'} marcada como contatada`,
        undoId: id,
      });
    } catch {
      setToast({ text: 'Não foi possível marcar Contatado' });
    }
  };

  const onSnooze = async (id: string) => {
    try {
      await putAction(id, 'snooze');
      setItems((prev) => prev.filter((s) => s.id !== id));
      setSelection((cur) =>
        cur?.type === 'signal' && cur.signal.id === id ? null : cur,
      );
      setToast({
        text: 'Sinal adiado por 1 h',
        undoId: id,
      });
    } catch {
      setToast({ text: 'Não foi possível adiar' });
    }
  };

  const onUndo = async () => {
    if (!toast?.undoId) {
      setToast(null);
      return;
    }
    try {
      await deleteAction(toast.undoId);
      await load();
      setToast(null);
    } catch {
      // keep the toast so the advisor can retry Desfazer
    }
  };

  const onAdvanceCase = async (id: string) => {
    try {
      const updated = await advanceCase(id);
      setCases((prev) => prev.map((c) => (c.id === id ? updated : c)));
      setSelection((cur) =>
        cur?.type === 'case' && cur.caseItem.id === id
          ? { type: 'case', caseItem: updated }
          : cur,
      );
    } catch {
      setCases((prev) =>
        prev.map((c) => (c.id === id ? { ...c, state: Math.min(c.state + 1, 3) } : c)),
      );
    }
  };

  // Server already filtered and sorted; decorate only for card presentation.
  const openSignals = items.map(decorateSignal);
  const decoratedCases = cases.map((c) => decorateCase(c, items));
  const visibleCases = (state: number) =>
    decoratedCases.filter((row) => row.caseItem.state === state);
  const showSignals = columnVisible('Novo sinal', filters, null);
  const caseColumns = ([0, 1, 2, 3] as const)
    .filter((state) => columnVisible(ANDAMENTO_OPTIONS[state + 1] ?? '', filters, state))
    .map((state) => ({
      key: `k${state}`,
      label: ANDAMENTO_OPTIONS[state + 1] ?? '',
      accent: (['info', 'brand', 'warn', 'ok'] as const)[state] ?? 'info',
      cases: visibleCases(state),
    }));
  const resolved = cases.filter((c) => c.state === 3).length;
  const emptyBecauseFilter =
    query.trim() !== '' ||
    Object.values(filters).some((list) => list.length > 0);
  const groups: FilterGroup[] = [
    {
      key: 'andamento',
      label: 'Andamento',
      align: 'left',
      options: facetOptions(facets, 'andamento', ANDAMENTO_OPTIONS),
    },
    {
      key: 'sla',
      label: 'SLA',
      align: 'left',
      options: facetOptions(facets, 'sla', ['No prazo', 'Vencendo', 'Vencido']),
    },
    {
      key: 'sinal',
      label: 'Sinal',
      align: 'left',
      options: facetOptions(facets, 'sinal', [
        'Risco de saída',
        'Pedido humano',
        'Precisa de revisão',
        'Classificação simplificada',
      ]),
    },
    {
      key: 'motivo',
      label: 'Motivo',
      align: 'right',
      options: facetOptions(facets, 'motivo', []),
    },
    {
      key: 'segmento',
      label: 'Segmento',
      align: 'right',
      options: facetOptions(facets, 'segmento', ['Essencial', 'Advance', 'Singular']),
    },
  ];

  const toggleOption = (key: FilterKey, option: string) => {
    setFilters((prev) => {
      const list = prev[key];
      const next = list.includes(option) ? list.filter((x) => x !== option) : [...list, option];
      return { ...prev, [key]: next };
    });
  };

  const showBoard = desktop || !selection;

  return (
    <div className="queue-screen">
      {showBoard ? (
        <div className="queue-toolbar">
          <div className="queue-stats">
            <span className="queue-stats__date">{dateLabel()}</span>
            <span className="queue-stats__dot" aria-hidden />
            <span>
              <strong>{items.length}</strong> sinais
            </span>
            <span className="queue-stats__dot" aria-hidden />
            <span>
              <strong>{resolved}</strong> resolvidos
            </span>
            <NewItemsPill count={incoming.length} onMerge={mergeIncoming} />
          </div>
          <FilterBar
            query={query}
            onQuery={setQuery}
            filters={filters}
            groups={groups}
            open={openFilter}
            onToggle={(key) => setOpenFilter((cur) => (cur === key ? null : key))}
            onClose={() => setOpenFilter(null)}
            onToggleOption={toggleOption}
            onClear={() => {
              setQuery('');
              setFilters(EMPTY_FILTERS);
              setOpenFilter(null);
            }}
          />
        </div>
      ) : null}
      {showBoard ? (
        <QueueBoard
          signals={openSignals}
          columns={caseColumns}
          showSignals={showSignals}
          desktop={desktop}
          loading={loading}
          error={error}
          emptyTitle={emptyBecauseFilter ? 'Nada neste filtro' : 'Fila em dia'}
          emptyText={
            emptyBecauseFilter
              ? 'Nenhum item corresponde a este filtro no momento.'
              : 'Nenhum cliente precisa de atenção agora. Novos sinais aparecem aqui em tempo real.'
          }
          onRetry={() => void load()}
          onViewCase={onViewCase}
          onOpenCase={onOpenCase}
          onContacted={(id) => void onContacted(id)}
          onSnooze={(id) => void onSnooze(id)}
          onAdvanceCase={(id) => void onAdvanceCase(id)}
        />
      ) : null}
      {selection && !desktop ? (
        <DetailModal selection={selection} onClose={() => setSelection(null)} desktop={false} />
      ) : null}
      {selection && desktop ? (
        <DetailModal selection={selection} onClose={() => setSelection(null)} desktop />
      ) : null}
      {toast ? (
        <Toast text={toast.text} onUndo={toast.undoId ? () => void onUndo() : undefined} />
      ) : null}
    </div>
  );
}
