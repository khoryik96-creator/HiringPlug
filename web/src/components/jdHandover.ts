// A job description handed to the Add job form from elsewhere (an email's
// attachment): kept in memory until Jobs opens and takes it, once.
let pending: File | null = null;

export function handJd(file: File) { pending = file; }

/** The job description handed over, if any (it is then gone). */
export function takeJd(): File | null {
  const file = pending;
  pending = null;
  return file;
}
