import { useCallback, useEffect, useRef, useState } from 'react';
import {
  advanceCase,
  deleteAction,
  fetchCases,
  fetchQueue,
  putAction,
} from '../api/bff';
import { DetailModal } from '../components/DetailModal';
import { NewItemsPill } from '../components/NewItemsPill';
import { QueueBoard } from '../components/QueueBoard';
import { Toast } from '../components/Toast';
import { clientName } from '../domain/clients';
import type { CaseItem, Signal } from '../domain/types';

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
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [toast, setToast] = useState<ToastState | null>(null);
  const [desktop, setDesktop] = useState(() => {
    if (typeof isDesktop === 'boolean') {
      return isDesktop;
    }
    return window.matchMedia(DESKTOP_MQ).matches;
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
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
      setCases(caseItems.filter((c) => c.state < 3));
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

  const selected = selectedId ? items.find((s) => s.id === selectedId) ?? null : null;

  const mergeIncoming = () => {
    setItems((prev) => [...incoming, ...prev]);
    setIncoming([]);
  };

  const onOpen = (id: string) => setSelectedId(id);

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
    if (!desktop) {
      setSelectedId(null);
    }
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
      setSelectedId((cur) => (cur === id ? null : cur));
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
      setCases((prev) =>
        prev
          .map((c) => (c.id === id ? updated : c))
          .filter((c) => c.state < 3),
      );
    } catch {
      setCases((prev) =>
        prev
          .map((c) => (c.id === id ? { ...c, state: Math.min(c.state + 1, 3) } : c))
          .filter((c) => c.state < 3),
      );
    }
  };

  if (loading) {
    return <p className="queue-status">Carregando fila…</p>;
  }
  if (error) {
    return (
      <div className="queue-status">
        <p>Não foi possível carregar a fila.</p>
        <button type="button" onClick={() => void load()}>
          Tentar de novo
        </button>
      </div>
    );
  }

  const showBoard = desktop || !selected;

  return (
    <div className="queue-screen">
      <NewItemsPill count={incoming.length} onMerge={mergeIncoming} />
      {showBoard ? (
        <QueueBoard
          signals={items}
          cases={cases}
          desktop={desktop}
          onOpen={onOpen}
          onOpenCase={onOpenCase}
          onContacted={(id) => void onContacted(id)}
          onSnooze={(id) => void onSnooze(id)}
          onAdvanceCase={(id) => void onAdvanceCase(id)}
        />
      ) : null}
      {selected && !desktop ? (
        <DetailModal signal={selected} onClose={() => setSelectedId(null)} desktop={false} />
      ) : null}
      {selected && desktop ? (
        <DetailModal signal={selected} onClose={() => setSelectedId(null)} desktop />
      ) : null}
      {toast ? (
        <Toast text={toast.text} onUndo={toast.undoId ? () => void onUndo() : undefined} />
      ) : null}
    </div>
  );
}
