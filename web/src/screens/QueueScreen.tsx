import { useCallback, useEffect, useRef, useState } from 'react';
import {
  advanceCase,
  ApiError,
  deleteAction,
  fetchCases,
  fetchQueue,
  openCase,
  putAction,
  queueStreamUrl,
} from '../api/bff';
import { DetailModal, type DetailSelection } from '../components/DetailModal';
import { FilterBar, type FilterGroup } from '../components/FilterBar';
import { NewItemsPill } from '../components/NewItemsPill';
import { QueueBoard } from '../components/QueueBoard';
import { Toast } from '../components/Toast';
import { CASE_STATES, type CaseItem, type ListFacets, type Signal } from '../domain/types';
import {
  ANDAMENTO_OPTIONS,
  caseOutcome,
  type CaseOutcome,
  columnVisible,
  countLabel,
  dateLabel,
  decorateCase,
  decorateSignal,
  EMPTY_FILTERS,
  type FilterKey,
  type QueueFilters,
  signalsColumnVisible,
  slaFacetsFromCards,
} from '../domain/queueVisual';

const DESKTOP_MQ = '(min-width: 900px)';

/** Abrir caso is confirmed by re-reading the cases: once a second, ten times (ADR 0013). */
const CASE_POLL_MS = 1000;
const CASE_POLL_ATTEMPTS = 10;

interface ToastState {
  text: string;
  action?: { label: string; run: () => void };
}

const RETRY = 'Tentar novamente';

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
  const [pendingCases, setPendingCases] = useState<ReadonlySet<string>>(new Set());
  // One idempotency key per signal command: a retry after a failed or refused
  // POST sends the same key. An accepted command that was never confirmed
  // drops it, because advisory dedupes a known key and would publish nothing.
  const caseKeys = useRef<Map<string, string>>(new Map());
  const gone = useRef(false);
  const pollTimers = useRef<Set<ReturnType<typeof setTimeout>>>(new Set());

  useEffect(() => {
    gone.current = false;
    const timers = pollTimers.current;
    return () => {
      gone.current = true;
      for (const timer of timers) {
        clearTimeout(timer);
      }
      timers.clear();
    };
  }, []);

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

  // A quiet load refreshes the lists behind an action: no skeleton, and a
  // failure keeps what is on screen.
  const load = useCallback(async (quiet = false) => {
    if (!quiet) {
      setLoading(true);
      setError(false);
    }
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
      setError(false);
    } catch {
      if (!quiet) {
        setError(true);
      }
    } finally {
      if (!quiet) {
        setLoading(false);
      }
    }
  }, [query, filters]);
  // Actions that finish later reload with the filters of that moment.
  const loadRef = useRef(load);
  loadRef.current = load;

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

  // Waits for cases to act on an accepted command. Resolves with the outcome,
  // or null when the time runs out or the screen is gone.
  const awaitCaseOutcome = async (signalId: string): Promise<CaseOutcome | null> => {
    for (let attempt = 0; attempt < CASE_POLL_ATTEMPTS; attempt += 1) {
      await new Promise<void>((resolve) => {
        const timer = setTimeout(() => {
          pollTimers.current.delete(timer);
          resolve();
        }, CASE_POLL_MS);
        pollTimers.current.add(timer);
      });
      if (gone.current) {
        return null;
      }
      try {
        const outcome = caseOutcome((await fetchCases()).items, signalId);
        if (outcome) {
          return outcome;
        }
      } catch {
        // A failed read is only a lost attempt.
      }
    }
    return null;
  };

  const onOpenCase = async (id: string) => {
    const signal = items.find((s) => s.id === id);
    if (!signal || pendingCases.has(id)) {
      return;
    }
    const who = signal.name ?? signal.client;
    const key = caseKeys.current.get(id) ?? crypto.randomUUID();
    caseKeys.current.set(id, key);
    const retry = { label: RETRY, run: () => void onOpenCase(id) };
    setToast(null);
    setPendingCases((prev) => new Set(prev).add(id));
    try {
      await openCase(id, key);
      const outcome = await awaitCaseOutcome(id);
      if (gone.current) {
        return;
      }
      if (!outcome) {
        caseKeys.current.delete(id);
        setToast({ text: `O caso de ${who} ainda não foi confirmado. O sinal continua na fila.`, action: retry });
        return;
      }
      caseKeys.current.delete(id);
      await loadRef.current(true);
      setToast({
        text:
          outcome === 'opened'
            ? `Caso aberto para ${who}`
            : `${who} já tinha um caso aberto. O sinal foi adicionado ao histórico desse caso.`,
      });
    } catch (err) {
      if (gone.current) {
        return;
      }
      const status = err instanceof ApiError ? err.status : 0;
      setToast({
        text:
          status === 404
            ? 'Este sinal não está mais na fila. Nenhum caso foi aberto.'
            : status === 429
              ? 'Muitos casos abertos em pouco tempo. Aguarde alguns minutos e tente de novo.'
              : `Não foi possível abrir o caso de ${who}. O sinal continua na fila.`,
        action: retry,
      });
    } finally {
      setPendingCases((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  };

  const undoAction = async (id: string) => {
    await deleteAction(id);
    await loadRef.current(true);
  };

  const onContacted = async (id: string) => {
    const hit = items.find((s) => s.id === id);
    try {
      await putAction(id, 'contact');
      await loadRef.current(true);
      setToast({
        text: `${hit?.name ?? hit?.client ?? 'Cliente'} marcada como contatada`,
        action: { label: 'Desfazer', run: () => void onUndo(id) },
      });
    } catch {
      setToast({ text: 'Não foi possível marcar Contatado' });
    }
  };

  const onSnooze = async (id: string) => {
    try {
      await putAction(id, 'snooze');
      await loadRef.current(true);
      setToast({
        text: 'Sinal adiado por 1 h',
        action: { label: 'Desfazer', run: () => void onUndo(id) },
      });
    } catch {
      setToast({ text: 'Não foi possível adiar' });
    }
  };

  // Desfazer on the toast, right after the action.
  const onUndo = async (id: string) => {
    try {
      await undoAction(id);
      setToast(null);
    } catch {
      // keep the toast so the advisor can retry Desfazer
    }
  };

  // Reativar and Desfazer contato on the card, any time later.
  const onUndoAction = async (id: string) => {
    try {
      await undoAction(id);
      setToast(null);
    } catch {
      setToast({
        text: 'Não foi possível desfazer. Nada foi alterado.',
        action: { label: RETRY, run: () => void onUndoAction(id) },
      });
    }
  };

  const onAdvanceCase = async (id: string) => {
    try {
      const updated = await advanceCase(id);
      // The advance answer does not name the origin signal; the card keeps it.
      const merge = (c: CaseItem): CaseItem => ({ ...updated, origin: updated.origin ?? c.origin });
      setCases((prev) => prev.map((c) => (c.id === id ? merge(c) : c)));
      setSelection((cur) =>
        cur?.type === 'case' && cur.caseItem.id === id
          ? { type: 'case', caseItem: merge(cur.caseItem) }
          : cur,
      );
      setToast(null);
      void loadRef.current(true);
    } catch {
      // The card stays in the state the server last reported.
      setToast({
        text: 'Não foi possível avançar o caso. Nada foi alterado.',
        action: { label: RETRY, run: () => void onAdvanceCase(id) },
      });
    }
  };

  // Server already filtered and sorted; decorate only for card presentation.
  const openSignals = items.map(decorateSignal);
  const decoratedCases = cases.map(decorateCase);
  const visibleCases = (state: number) =>
    decoratedCases.filter((row) => row.caseItem.state === state);
  const showSignals = signalsColumnVisible(filters);
  const caseColumns = CASE_STATES.map((label, state) => ({ label, state }))
    .filter(({ label, state }) => columnVisible(label, filters, state))
    .map(({ label, state }) => ({
      key: `k${state}`,
      label,
      accent: (['info', 'brand', 'warn', 'ok'] as const)[state] ?? 'info',
      cases: visibleCases(state),
    }));
  // The same number the Resolvido filter shows: the backend counts it.
  const resolved = (facets?.andamento ?? []).find((f) => f.label === 'Resolvido')?.count ?? 0;
  // A search that finds nothing on the board may still match resolved cases,
  // which the board hides until the Resolvido filter is on.
  const boardEmpty = !loading && !error && items.length === 0 && cases.length === 0;
  const resolvedHint = boardEmpty && query.trim() !== '' && filters.andamento.length === 0 && resolved > 0;
  const cardStates = [...openSignals.map((row) => row.slaState), ...decoratedCases.map((row) => row.slaState)];
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
      options: slaFacetsFromCards(
        facetOptions(facets, 'sla', ['No prazo', 'Vencendo', 'Vencido']),
        cardStates,
        filters.sla,
      ),
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

  const closeFilter = useCallback(() => setOpenFilter(null), []);

  const showBoard = desktop || !selection;

  return (
    <div className="queue-screen">
      {showBoard ? (
        <div className="queue-toolbar">
          <div className="queue-stats">
            <span className="queue-stats__date">{dateLabel()}</span>
            <span className="queue-stats__dot" aria-hidden />
            <span>
              <strong>{items.length}</strong> {countLabel(items.length, 'sinal', 'sinais')}
            </span>
            <span className="queue-stats__dot" aria-hidden />
            <span>
              <strong>{resolved}</strong> {countLabel(resolved, 'resolvido', 'resolvidos')}
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
            onClose={closeFilter}
            onToggleOption={toggleOption}
            onClear={() => {
              setQuery('');
              setFilters(EMPTY_FILTERS);
              setOpenFilter(null);
            }}
          />
        </div>
      ) : null}
      {showBoard && resolvedHint ? (
        <p className="queue-hint">
          {countLabel(
            resolved,
            '1 caso resolvido corresponde a esta busca.',
            `${resolved} casos resolvidos correspondem a esta busca.`,
          )}{' '}
          <button type="button" onClick={() => toggleOption('andamento', 'Resolvido')}>
            Mostrar resolvidos
          </button>
        </p>
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
          onOpenCase={(id) => void onOpenCase(id)}
          onContacted={(id) => void onContacted(id)}
          onSnooze={(id) => void onSnooze(id)}
          onUndoAction={(id) => void onUndoAction(id)}
          onAdvanceCase={(id) => void onAdvanceCase(id)}
          pendingCases={pendingCases}
        />
      ) : null}
      {selection && !desktop ? (
        <DetailModal selection={selection} onClose={() => setSelection(null)} desktop={false} />
      ) : null}
      {selection && desktop ? (
        <DetailModal selection={selection} onClose={() => setSelection(null)} desktop />
      ) : null}
      {toast ? (
        <Toast text={toast.text} action={toast.action} />
      ) : null}
    </div>
  );
}
