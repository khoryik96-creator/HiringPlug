// Files dragged from the computer onto an element: it shows they can be
// dropped (dropping), and hands them over on the drop. Text or links dragged
// about are left to the browser.
import { type DragEvent, useState } from "react";

export function useFileDrop(enabled: boolean, onFiles: (files: File[]) => void) {
  const [dropping, setDropping] = useState(false);
  const hasFiles = (event: DragEvent) => event.dataTransfer.types.includes("Files");
  return {
    dropping,
    handlers: {
      onDragOver: (event: DragEvent) => {
        if (!enabled || !hasFiles(event)) return;
        event.preventDefault();
        setDropping(true);
      },
      onDragLeave: (event: DragEvent) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDropping(false);
      },
      onDrop: (event: DragEvent) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        setDropping(false);
        if (enabled) onFiles([...event.dataTransfer.files]);
      },
    },
  };
}
