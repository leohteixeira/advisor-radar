import { useEffect, useRef, useState } from 'react';
import { SduiShowcase } from '../selection/SduiShowcase';

/** Same-origin presentation embed; the product keeps ownership of its live renderer. */
export function SduiEmbedScreen() {
  const root = useRef<HTMLElement>(null);
  const [wide, setWide] = useState(() => window.matchMedia('(min-width: 900px)').matches);
  useEffect(() => {
    const media = window.matchMedia('(min-width: 900px)');
    const changed = () => setWide(media.matches);
    media.addEventListener('change', changed);
    const element = root.current;
    if (!element) return () => media.removeEventListener('change', changed);
    const sendHeight = () => window.parent.postMessage({ type: 'advisor-radar:sdui-height', height: Math.ceil(element.getBoundingClientRect().height) }, window.location.origin);
    const observer = new ResizeObserver(sendHeight);
    observer.observe(element);
    sendHeight();
    void document.fonts?.ready.then(sendHeight);
    return () => { observer.disconnect(); media.removeEventListener('change', changed); };
  }, []);
  return <main ref={root} className={`selection selection--embed${wide ? ' selection--wide' : ''}`}><SduiShowcase wide={wide} /></main>;
}
