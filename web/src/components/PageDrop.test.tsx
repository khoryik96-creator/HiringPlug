import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { usePageDrop } from "./PageDrop";

afterEach(() => { cleanup(); });

function Page({ onFiles, enabled = true }: { onFiles: (f: File[]) => void; enabled?: boolean }) {
  const drop = usePageDrop(".pdf", onFiles, enabled);
  return (
    <main {...drop.props}>
      {drop.overlay}
      <p>Anywhere</p>
      <div onDrop={(e) => { e.preventDefault(); }}>Its own box</div>
    </main>
  );
}
const cv = new File(["%PDF"], "cv.pdf", { type: "application/pdf" });
const files = { dataTransfer: { types: ["Files"], files: [cv] } };

describe("dropping files anywhere on a page", () => {
  it("takes files dropped anywhere, with an outline while they are dragged over", () => {
    const onFiles = vi.fn();
    render(<Page onFiles={onFiles} />);
    fireEvent.dragEnter(screen.getByText("Anywhere"), files);
    expect(screen.getByText("Drop the CVs to add candidates")).toBeTruthy();
    fireEvent.drop(screen.getByText("Anywhere"), files);
    expect(onFiles).toHaveBeenCalledWith([cv]);
    expect(screen.queryByText("Drop the CVs to add candidates")).toBeNull();
  });

  it("leaves files to a drop box on the page that took them", () => {
    const onFiles = vi.fn();
    render(<Page onFiles={onFiles} />);
    fireEvent.drop(screen.getByText("Its own box"), files);
    expect(onFiles).not.toHaveBeenCalled();
  });

  it("takes nothing while turned off (an import already open)", () => {
    const onFiles = vi.fn();
    render(<Page onFiles={onFiles} enabled={false} />);
    fireEvent.drop(screen.getByText("Anywhere"), files);
    expect(onFiles).not.toHaveBeenCalled();
  });
});
