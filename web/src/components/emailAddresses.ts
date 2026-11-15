/** The addresses in a To, Cc or Bcc box, however they are separated. */
export const addresses = (text: string) => text.split(/[\s,;]+/).filter(Boolean);
