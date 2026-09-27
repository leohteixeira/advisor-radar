import { useCallback, useEffect, useRef, useState } from 'react';
import {
  advanceCase,
  deleteAction,
  fetchCases,
  fetchQueue,
  putAction,
} from '../api/bff';
import { DetailModal, type DetailSelection } from '../components/DetailModal';
import { FilterBar, type FilterGroup } from '../components/FilterBar';
import { NewItemsPill } from '../components/NewItemsPill';
import { QueueBoard } from '../components/QueueBoard';
import { Toast } from '../components/Toast';
import { clientName } from '../domain/clients';
import type { CaseItem, Signal } from '../domain/types';
import {
  ANDAMENTO_OPTIONS,
  caseMatches,
  columnVisible,
  dateLabel,
  decorateCase,
  decorateSignal,
  EMPTY_FILTERS,
  motivoOptions,
  signalMatches,
  SINAL_OPTIONS,
  SLA_OPTIONS,
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

export function QueueScreen({ isDesktop, disableStream }: QueueScreenProps) {
  const [items, setItems] = useState<Signal[]>([]);
  const [incoming, setIncoming] = useState<Signal[]>([]);
  const [cases, setCases] = useState<CaseItem[]>([]);
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
      const [queue, caseItems] = await Promise.all([fetchQueue(), fetchCases()]);
      const pill = incomingRef.current;
      const pillIds = new Set(pill.map((s) => s.id));
      setItems(queue.filter((s) => !pillIds.has(s.id)));
      setCases(caseItems);
      const seen = new Set(queue.map((s) => s.id));
      for (const id of pillIds) {
        seen.add(id);
      }
      seenIds.current = seen;
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }, []);

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
      const url = '/v1/queue/stream';
      es = new EventSource(url);
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
    setItems((prev) => [...incoming, ...prev]);
    setIncoming([]);
  };

  const onOpen = (id: string) => {
    const signal = items.find((s) => s.id === id);
    if (signal) {
      setSelection({ type: 'signal', signal });
    }
  };

  const onViewCase = (id: string) => {
    const caseItem = cases.find((c) => c.id === id);
    if (caseItem) {
      setSelection({ type: 'case', caseItem });
    }
  };

  const onOpenClient = (clientId: string) => {
    setSelection({ type: 'client', clientId });
  };

  const onOpenCase = (id: string) => {
    const signal = items.find((s) => s.id === id);
    if (!signal) {
      return;
    }
    const opened: CaseItem = {
      id: `k-local-${id}`,
      client: signal.client,
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
      setItems((prev) =>
        prev.map((s) =>
          s.id === id ? { ...s, contacted_at: new Date().toISOString() } : s,
        ),
      );
      setToast({
        text: `${clientName(items.find((s) => s.id === id)?.client ?? '')} marcada como contatada`,
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

  const decorated = items.map(decorateSignal).sort((a, b) => b.score - a.score);
  const decoratedCases = cases.map((c) => decorateCase(c, items));
  const openSignals = decorated.filter((row) => signalMatches(row, query, filters));
  const visibleCases = (state: number) =>
    decoratedCases
      .filter((row) => row.caseItem.state === state && caseMatches(row, decorated, query, filters))
      .sort((a, b) => a.rem - b.rem);
  const showSignals = columnVisible('Novo sinal', filters, null);
  const caseColumns = [0, 1, 2, 3]
    .filter((state) => columnVisible(ANDAMENTO_OPTIONS[state + 1], filters, state))
    .map((state) => ({
      key: `k${state}`,
      label: ANDAMENTO_OPTIONS[state + 1],
      accent: (['info', 'brand', 'warn', 'ok'] as const)[state],
      cases: visibleCases(state),
    }));
  const resolved = cases.filter((c) => c.state === 3).length;
  const emptyBecauseFilter = items.length > 0;
  const groups: FilterGroup[] = [
    {
      key: 'andamento',
      label: 'Andamento',
      align: 'left',
      options: ANDAMENTO_OPTIONS.map((label, i) => ({
        label,
        count:
          i === 0
            ? decorated.filter((row) => signalMatches(row, query, filters, 'andamento')).length
            : decoratedCases.filter(
                (row) =>
                  row.caseItem.state === i - 1 && caseMatches(row, decorated, query, filters, 'andamento'),
              ).length,
      })),
    },
    {
      key: 'sla',
      label: 'SLA',
      align: 'left',
      options: SLA_OPTIONS.map((label) => ({
        label,
        count: decorated.filter((row) => row.slaState === label && signalMatches(row, query, filters, 'sla')).length,
      })),
    },
    {
      key: 'sinal',
      label: 'Sinal',
      align: 'left',
      options: SINAL_OPTIONS.map((label) => ({
        label,
        count: decorated.filter((row) => signalMatches({ ...row, needsReview: label === 'Precisa de revisão' ? true : row.needsReview }, query, filters, 'sinal') && (
          label === 'Risco de saída'
            ? Boolean(row.signal.churn)
            : label === 'Pedido humano'
              ? Boolean(row.signal.human)
              : label === 'Precisa de revisão'
                ? row.needsReview
                : Boolean(row.signal.fallback)
        )).length,
      })),
    },
    {
      key: 'motivo',
      label: 'Motivo',
      align: 'right',
      options: motivoOptions().map((label) => ({
        label,
        count: decorated.filter(
          (row) =>
            (row.typeLabel === label || row.signal.intent === label) &&
            signalMatches(row, query, filters, 'motivo'),
        ).length,
      })),
    },
    {
      key: 'segmento',
      label: 'Segmento',
      align: 'right',
      options: ['Essencial', 'Advance', 'Singular'].map((label) => ({
        label,
        count: decorated.filter((row) => row.segment === label && signalMatches(row, query, filters, 'segmento')).length,
      })),
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
          onOpen={onOpen}
          onViewCase={onViewCase}
          onOpenCase={onOpenCase}
          onContacted={(id) => void onContacted(id)}
          onSnooze={(id) => void onSnooze(id)}
          onAdvanceCase={(id) => void onAdvanceCase(id)}
        />
      ) : null}
      {selection && !desktop ? (
        <DetailModal
          selection={selection}
          onClose={() => setSelection(null)}
          onOpenClient={onOpenClient}
          desktop={false}
        />
      ) : null}
      {selection && desktop ? (
        <DetailModal
          selection={selection}
          onClose={() => setSelection(null)}
          onOpenClient={onOpenClient}
          desktop
        />
      ) : null}
      {toast ? (
        <Toast text={toast.text} onUndo={toast.undoId ? () => void onUndo() : undefined} />
      ) : null}
    </div>
  );
}
