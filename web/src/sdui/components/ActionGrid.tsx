import { ActionControl } from '../actions';
import { SduiIcon } from '../icons';
import type { SduiComponentProps } from '../registry';
import type { ActionGridProps } from '../types';

/** Home `actions`: one round control per server action. */
export function ActionGrid({ props }: SduiComponentProps) {
  const p = props as unknown as ActionGridProps;
  return (
    <nav className="sdui-actions" aria-label="Ações">
      {p.items.map((item, index) => (
        <ActionControl
          key={`${item.label}-${index}`}
          action={item.action}
          className="sdui-actions__item"
          render={() => (
            <>
              <span className="sdui-actions__circle" aria-hidden="true">
                <SduiIcon name={item.icon} size={22} />
              </span>
              {item.label}
            </>
          )}
        />
      ))}
    </nav>
  );
}
