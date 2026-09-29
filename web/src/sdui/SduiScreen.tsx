import { useEffect } from 'react';
import { Boundary } from './Boundary';
import { lookup } from './registry';
import type { Component, Screen, Slug } from './types';

/**
 * Web owns the grid: one column on the phone, two on desktop, and the span of
 * each section as drawn in OrlaApp.dc.html. A section not listed spans one.
 */
const WIDE_SECTIONS: Readonly<Record<Slug, readonly string[]>> = {
  home: ['moment', 'highlights'],
  investir: ['cash', 'highlights'],
  carteira: [],
  perfil: ['header', 'suitability', 'advisor'],
};

function spanOf(slug: Slug, section: string): 1 | 2 {
  return Object.hasOwn(WIDE_SECTIONS, slug) && WIDE_SECTIONS[slug].includes(section) ? 2 : 1;
}

function UnknownType({ type }: { type: string }) {
  useEffect(() => {
    console.error(`sdui: unknown component type ${type}`);
  }, [type]);
  return null;
}

function Slot({ component }: { component: Component }) {
  const Entry = lookup(component.type);
  if (!Entry) {
    return <UnknownType type={component.type} />;
  }
  return (
    <Boundary type={component.type} resetKey={component}>
      <Entry variant={component.variant} props={component.props} />
    </Boundary>
  );
}

/**
 * Renders a screen envelope: the heading, then each section in array order,
 * each component through the registry. Omitted sections render nothing.
 */
export function SduiScreen({ screen }: { screen: Screen }) {
  return (
    <div className="sdui-screen" data-slug={screen.slug} data-revision={screen.revision}>
      <div className="sdui-screen__head">
        <h1>{screen.title}</h1>
        {screen.subtitle ? <span>{screen.subtitle}</span> : null}
      </div>
      <div className="sdui-grid">
        {screen.sections.map((section, index) => (
          <div key={`${index}-${section.id}`} className="sdui-section" data-section={section.id} data-span={spanOf(screen.slug, section.id)}>
            {section.components.map((component, slot) => (
              <Slot key={`${slot}-${component.type}`} component={component} />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

/** The screen skeleton while the one screen request is in flight. */
export function SduiLoading() {
  return (
    <div className="sdui-loading">
      <span role="status">Montando sua tela…</span>
      <span className="sdui-skel sdui-skel--title" aria-hidden="true" />
      <span className="sdui-skel sdui-skel--moment" aria-hidden="true" />
      <span className="sdui-skel sdui-skel--wealth" aria-hidden="true" />
      <div className="sdui-skel__actions" aria-hidden="true">
        <span className="sdui-skel" />
        <span className="sdui-skel" />
        <span className="sdui-skel" />
        <span className="sdui-skel" />
      </div>
      <span className="sdui-skel sdui-skel--advisor" aria-hidden="true" />
      <p className="sdui-loading__note">Uma requisição só. Se passar do tempo, a tela mostra o que já chegou e cada seção atrasada cai na variante padrão.</p>
    </div>
  );
}
