/**
 * Screen envelope and component props of the SDUI contract in
 * specs/http/bff.md ("Screens (phase 3)"). Every visible value is a display
 * string the BFF already formatted; web renders it as is.
 */

/** Screen slugs the BFF may serve and a navigate action may target. */
export const SLUGS = ['home', 'investir', 'carteira', 'perfil'] as const;
export type Slug = (typeof SLUGS)[number];

export type Tone = 'pos' | 'neg' | 'info' | 'gold' | 'neutral';

/** Panels a panel action may open. */
export type PanelTarget = 'deposit' | 'withdraw' | 'message' | 'complaint' | 'purchase';

export interface NavigateAction {
  type: 'navigate';
  label: string;
  target: Slug;
}

export interface PanelAction {
  type: 'panel';
  label: string;
  target: PanelTarget;
  product_id?: string;
}

export interface NoteAction {
  type: 'note';
  label: string;
  text: string;
}

export interface LinkAction {
  type: 'link';
  label: string;
  href: string;
}

export type Action = NavigateAction | PanelAction | NoteAction | LinkAction;

export interface Component {
  type: string;
  variant: string;
  props: Record<string, unknown>;
}

export interface Section {
  id: string;
  components: Component[];
}

export interface Omitted {
  id: string;
  type: string;
  reason: string;
}

export interface Screen {
  schema_version: number;
  slug: Slug;
  revision: string;
  title: string;
  subtitle?: string;
  sections: Section[];
  omitted: Omitted[];
}

/** Allocation row: `class` picks the color, `bar_width` (0–100) the width. */
export interface AllocationRow {
  class: 'stocks' | 'etfs' | 'fixed_income' | 'cash';
  label: string;
  share: string;
  bar_width: number;
}

export interface ActivityItem {
  icon?: string;
  title: string;
  meta: string;
  value?: string;
  tone?: 'pos' | 'neg' | 'neutral';
}

export interface MomentCardProps {
  kicker: string;
  title: string;
  body: string;
  meta?: string;
  tone: 'neg' | 'info' | 'gold' | 'neutral';
  icon?: string;
  action?: Action;
}

export interface WealthSummaryProps {
  total_label: string;
  total: string;
  cash_label: string;
  cash: string;
  cash_cents: number;
  allocation: AllocationRow[];
  day_change?: string;
  day_change_tone?: 'pos' | 'neg' | 'neutral';
}

export interface GridItem {
  label: string;
  icon?: string;
  action: Action;
}

export interface ActionGridProps {
  items: GridItem[];
}

export interface AdvisorCardProps {
  kicker: string;
  name: string;
  initials: string;
  meta: string;
  action?: Action;
}

export interface ActivityListProps {
  title: string;
  items: ActivityItem[];
  empty_text?: string;
}

/**
 * One catalog product as the BFF shaped it for the viewing customer.
 * `above_profile`, `badge`, and `warning` come from the BFF; web never
 * compares risk with the profile.
 */
export interface ProductItem {
  product_id: string;
  name: string;
  class_label: string;
  risk: number;
  risk_label: string;
  return_label: string;
  minimum: string;
  minimum_cents: number;
  above_profile: boolean;
  badge?: string;
  warning?: string;
  action: Action;
}

export interface InvestSummaryProps {
  cash_label: string;
  cash: string;
  cash_cents: number;
  profile_chip?: string;
}

export interface ProductRailProps {
  title: string;
  subtitle: string;
  products: ProductItem[];
}

export interface ProductListProps {
  title: string;
  products: ProductItem[];
}

/** One labelled value. `money` marks a value the eye toggle masks. */
export interface Stat {
  label: string;
  value: string;
  tone?: 'pos' | 'neg' | 'neutral';
  money?: boolean;
}

export interface PortfolioSummaryProps {
  total_label: string;
  total: string;
  stats: Stat[];
}

/** An allocation row with the class value as money. */
export interface BreakdownRow extends AllocationRow {
  value: string;
}

export interface AllocationBreakdownProps {
  title: string;
  rows: BreakdownRow[];
}

/** One position: `return` is the signed percentage the BFF computed. */
export interface PositionItem {
  product_id: string;
  name: string;
  applied: string;
  value: string;
  return: string;
  return_tone: 'pos' | 'neg' | 'neutral';
}

export interface PositionListProps {
  title: string;
  subtotal: string;
  applied_label: string;
  items: PositionItem[];
}

export interface ProfileHeaderProps {
  initials: string;
  name: string;
  subtitle: string;
  /** Left out when the registration read failed. */
  account?: string;
}

/** One investor profile level; `max_risk` comes from advisory, never from web. */
export interface ProfileLevel {
  key: string;
  label: string;
  description: string;
  limit: string;
  max_risk: number;
  current: boolean;
}

export interface ProfileScaleProps {
  title: string;
  subtitle: string;
  current_label: string;
  levels: ProfileLevel[];
  footer: string;
}

export interface ProfileField {
  label: string;
  value: string;
}

export interface ProfileFieldListProps {
  title: string;
  fields: ProfileField[];
  footnote: string;
}

export interface PreferenceOption {
  value: string;
  label: string;
}

export interface PreferenceListProps {
  title: string;
  theme: { label: string; hint: string };
  channel: { label: string; hint: string; value: string; options: PreferenceOption[] };
  beta: { label: string; hint: string; enabled: boolean };
}

/** What the preferences route stores: `channel` is `chat` or `email`. */
export interface Preferences {
  channel: string;
  beta: boolean;
}
