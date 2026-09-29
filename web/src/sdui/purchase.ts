import type { ProductItem, Screen } from './types';

/**
 * What the coded purchase form reads from the Investir envelope on view: the
 * product as the BFF shaped it, and the cash of `invest_summary` when that
 * section was served (it is omitted when account-sim is down).
 */
export interface PurchaseTarget {
  product: ProductItem;
  cash?: { display: string; cents: number };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isProduct(value: unknown, productID: string): value is ProductItem {
  return isRecord(value) && value.product_id === productID && typeof value.name === 'string';
}

/**
 * Finds a product by id in any component with a `products` list (rail or
 * list) and the cash of `invest_summary`. Web copies these values; it
 * computes none of them. Returns null when the screen has no such product.
 */
export function findPurchase(screen: Screen, productID: string): PurchaseTarget | null {
  let product: ProductItem | undefined;
  let cash: PurchaseTarget['cash'];
  for (const section of screen.sections) {
    for (const component of section.components) {
      const props = component.props;
      if (component.type === 'invest_summary' && typeof props.cash === 'string' && typeof props.cash_cents === 'number') {
        cash = { display: props.cash, cents: props.cash_cents };
      }
      if (!product && Array.isArray(props.products)) {
        product = props.products.find((item): item is ProductItem => isProduct(item, productID));
      }
    }
  }
  return product ? { product, cash } : null;
}
