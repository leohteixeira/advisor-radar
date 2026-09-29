import { useEffect, useId, useState, type ReactNode } from 'react';
import { useSdui, type OpenPanel } from './context';
import { SLUGS, type Slug } from './types';

const OPEN_PANELS: readonly OpenPanel[] = ['deposit', 'withdraw', 'message', 'complaint'];

/** An action checked against the contract, ready to become a control. */
export type Resolved =
  | { kind: 'navigate'; label: string; slug: Slug }
  | { kind: 'panel'; label: string; panel: OpenPanel }
  | { kind: 'note'; label: string; text: string }
  | { kind: 'link'; label: string; href: string }
  | { kind: 'purchase'; label: string; productID: string }
  | { kind: 'hidden'; problem: string };

function hidden(problem: string): Resolved {
  return { kind: 'hidden', problem };
}

function isHTTPS(href: string): boolean {
  try {
    return new URL(href).protocol === 'https:';
  } catch {
    return false;
  }
}

/**
 * Checks an action from the envelope. Anything outside the contract resolves
 * to hidden with the problem to report, including a purchase panel without a
 * `product_id`.
 */
export function resolveAction(action: unknown): Resolved {
  if (typeof action !== 'object' || action === null) {
    return hidden('sdui: action is not an object');
  }
  const a = action as Record<string, unknown>;
  if (typeof a.label !== 'string' || a.label === '') {
    return hidden(`sdui: action ${String(a.type)} has no label`);
  }
  const label = a.label;
  switch (a.type) {
    case 'navigate': {
      const slug = SLUGS.find((value) => value === a.target);
      if (!slug) {
        return hidden(`sdui: unknown navigate target ${String(a.target)}`);
      }
      return { kind: 'navigate', label, slug };
    }
    case 'panel': {
      if (a.target === 'purchase') {
        if (typeof a.product_id !== 'string' || a.product_id === '') {
          return hidden('sdui: purchase panel has no product_id');
        }
        return { kind: 'purchase', label, productID: a.product_id };
      }
      const panel = OPEN_PANELS.find((value) => value === a.target);
      if (!panel) {
        return hidden(`sdui: unknown panel target ${String(a.target)}`);
      }
      return { kind: 'panel', label, panel };
    }
    case 'note':
      if (typeof a.text !== 'string') {
        return hidden('sdui: note action has no text');
      }
      return { kind: 'note', label, text: a.text };
    case 'link':
      if (typeof a.href !== 'string' || !isHTTPS(a.href)) {
        return hidden('sdui: link refused, href is not https');
      }
      return { kind: 'link', label, href: a.href };
    default:
      return hidden(`sdui: unknown action type ${String(a.type)}`);
  }
}

/**
 * Renders one server action as a control. `render` draws the inside of the
 * control from the action label; it defaults to the label. A hidden action
 * renders nothing and reports its problem once with console.error.
 */
export function ActionControl({ action, className, render }: { action: unknown; className: string; render?: (label: string) => ReactNode }) {
  const { onNavigate, onPanel, onPurchase } = useSdui();
  const [open, setOpen] = useState(false);
  const noteID = useId();
  const resolved = resolveAction(action);
  const problem = resolved.kind === 'hidden' ? resolved.problem : null;

  useEffect(() => {
    if (problem) {
      console.error(problem);
    }
  }, [problem]);

  if (resolved.kind === 'hidden') {
    return null;
  }
  const inside = render ? render(resolved.label) : resolved.label;
  switch (resolved.kind) {
    case 'navigate':
      return (
        <button type="button" className={className} onClick={() => onNavigate(resolved.slug)}>
          {inside}
        </button>
      );
    case 'panel':
      return (
        <button type="button" className={className} onClick={() => onPanel(resolved.panel)}>
          {inside}
        </button>
      );
    case 'purchase':
      return (
        <button type="button" className={className} onClick={() => onPurchase(resolved.productID)}>
          {inside}
        </button>
      );
    case 'link':
      return (
        <a className={className} href={resolved.href} target="_blank" rel="noopener noreferrer">
          {inside}
        </a>
      );
    case 'note':
      return (
        <>
          <button type="button" className={className} aria-expanded={open} aria-controls={open ? noteID : undefined} onClick={() => setOpen((value) => !value)}>
            {inside}
          </button>
          {open ? (
            <p id={noteID} className="sdui-note">
              {resolved.text}
            </p>
          ) : null}
        </>
      );
  }
}
