// Keeps a rendering bug in one page from blanking the whole app: the page
// says what went wrong and the rest (top bar, other pages) keeps working.
import { Component, type ReactNode } from "react";

type State = { error: Error | null };

export class PageErrorBoundary extends Component<{ children: ReactNode }, State> {
  override state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  override componentDidCatch(error: Error) {
    console.error("page failed to show", error);
  }

  override render() {
    if (!this.state.error) return this.props.children;
    return (
      <main>
        <section className="panel" role="alert">
          <h3>This page could not be shown</h3>
          <p className="muted">Something on it is in a shape the page did not expect. The record itself is safe.</p>
          <p className="small"><code>{this.state.error.message}</code></p>
          <button type="button" onClick={() => { window.location.reload(); }}>Reload</button>
        </section>
      </main>
    );
  }
}
