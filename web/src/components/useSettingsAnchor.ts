import { useCallback } from "react";

function settingsAnchor() {
  return window.location.hash.split("#")[2];
}

function revealSetting(element: HTMLElement) {
  const disclosure = element.closest("details");
  if (disclosure) disclosure.open = true;
  element.focus({ preventScroll: true });
  if (typeof element.scrollIntoView === "function") element.scrollIntoView({ block: "start" });
}

export function focusCurrentSetting() {
  const id = settingsAnchor();
  const element = id && document.getElementById(id);
  if (element) revealSetting(element);
}

/** Scroll when the target mounts, including settings that wait for a query. */
export function useSettingsAnchor(id?: string) {
  const selected = Boolean(id && settingsAnchor() === id);
  return useCallback((element: HTMLElement | null) => {
    if (!selected || !element) return;
    revealSetting(element);
  }, [selected]);
}
