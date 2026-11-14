interface ToastProps {
  text: string;
  /** One follow-up the advisor can take from the message, e.g. Desfazer or Tentar novamente. */
  action?: { label: string; run: () => void };
}

export function Toast({ text, action }: ToastProps) {
  return (
    <div className="toast" role="status">
      <span>{text}</span>
      {action ? (
        <button type="button" className="toast__undo" onClick={action.run}>
          {action.label}
        </button>
      ) : null}
    </div>
  );
}
