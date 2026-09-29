import { Component, type ReactNode } from 'react';

interface BoundaryProps {
  /** The component type, for the report. */
  type: string;
  /** A new value clears a previous failure (a new envelope was rendered). */
  resetKey: unknown;
  children: ReactNode;
}

interface BoundaryState {
  failed: boolean;
  resetKey: unknown;
}

/**
 * Isolates one SDUI component: when it throws, its slot renders nothing, the
 * error is reported with console.error, and siblings keep rendering.
 */
export class Boundary extends Component<BoundaryProps, BoundaryState> {
  state: BoundaryState = { failed: false, resetKey: this.props.resetKey };

  static getDerivedStateFromError(): Partial<BoundaryState> {
    return { failed: true };
  }

  static getDerivedStateFromProps(props: BoundaryProps, state: BoundaryState): Partial<BoundaryState> | null {
    if (props.resetKey !== state.resetKey) {
      return { failed: false, resetKey: props.resetKey };
    }
    return null;
  }

  componentDidCatch(error: unknown) {
    console.error(`sdui: component ${this.props.type} failed`, error);
  }

  render() {
    return this.state.failed ? null : this.props.children;
  }
}
