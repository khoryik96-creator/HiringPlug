// A value that follows another once it has stopped changing for `ms` (a
// search box: one search when typing pauses, not one per key press).
import { useEffect, useState } from "react";

export function useDebounced<T>(value: T, ms: number): T {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => { setSettled(value); }, ms);
    return () => { window.clearTimeout(t); };
  }, [value, ms]);
  return settled;
}
