interface ToastProps {
  text: string;
  onUndo?: () => void;
}

export function Toast({ text, onUndo }: ToastProps) {
  return (
    <div className="toast" role="status">
      <span>{text}</span>
      {onUndo ? (
        <button type="button" className="toast__undo" onClick={onUndo}>
          Desfazer
        </button>
      ) : null}
    </div>
  );
}
