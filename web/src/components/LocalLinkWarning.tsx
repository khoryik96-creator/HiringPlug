// A link to "localhost" opens only on the computer running HyPlug: sent
// to anyone else it leads nowhere. Email windows say so before sending.

const LOCAL = /https?:\/\/(?:localhost|127(?:\.\d{1,3}){3}|0\.0\.0\.0|\[::1\])(?=[:/?#"'\s<]|$)/i;

/** Whether text (or HTML) holds a link that works only on this computer. */
export function hasLocalLink(text: string): boolean {
  return LOCAL.test(text);
}

export function LocalLinkWarning({ html }: { html: string }) {
  if (!hasLocalLink(html)) return null;
  return (
    <p className="warning small local-link" role="alert">
      This email has a link that only works on this computer (“localhost”): the people you send it to cannot open it.
      HyPlug needs its real address first (on a server, or this computer’s address on the office network); then make the link again.
    </p>
  );
}
