import type { ReactNode } from "react";

import "./sign-in.css";

/** The signed-out pages' frame: HiringPlug's name over a card, and anything below it. */
export function SignInFrame({ children, after }: { children: ReactNode; after?: ReactNode }) {
  return (
    <main className="narrow sign-in">
      <h1 className="sign-in-brand">HiringPlug</h1>
      <div className="sign-in-card">{children}</div>
      {after}
    </main>
  );
}
