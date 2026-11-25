import type { Schemas } from "../api/client";
import { MicrosoftMark } from "./MicrosoftButton";
import "./sign-in.css";

/** How a team member signs in: "Microsoft" and/or "Password" (none while invited). */
export function SignInMethods({ methods }: { methods: Schemas["SignInMethod"][] }) {
  if (methods.length === 0) return null;
  return (
    <span className="sign-in-badges" aria-label={`Signs in with ${methods.map((m) => (m === "microsoft" ? "Microsoft" : "a password")).join(" or ")}`}>
      {methods.includes("microsoft") && <span className="badge sign-in-microsoft" title="Signs in with their Microsoft work account"><MicrosoftMark /> Microsoft</span>}
      {methods.includes("password") && <span className="badge sign-in-password" title="Signs in with a password">Password</span>}
    </span>
  );
}
