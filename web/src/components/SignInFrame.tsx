import type { ReactNode } from "react";

import "./sign-in.css";

/** The signed-out pages' frame: HyPlug's name over a card, and anything below it. */
export function SignInFrame({ children, after }: { children: ReactNode; after?: ReactNode }) {
  return (
    <main className="narrow sign-in">
      <h1 className="sign-in-brand">HyPlug</h1>
      <div className="sign-in-card">{children}</div>
      {after}
    </main>
  );
}
