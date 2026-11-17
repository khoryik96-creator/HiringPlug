// Two of a candidate's CVs side by side, in the CV & files tab: each with its
// own picker; what their details disagree on folded underneath.
import { useState } from "react";

import { type Document } from "../api/client";
import { CvDifferences } from "./CvDifferences";
import { CvPane, useDocument } from "./CvPane";

export function CvCompare({ docs, firstId }: { docs: Document[]; firstId: string }) {
  const [leftId, setLeft] = useState(firstId);
  const [rightId, setRight] = useState(() => (docs.find((d) => d.id !== firstId) ?? docs[0])?.id);
  const left = useDocument(leftId);
  const right = useDocument(rightId);
  return (
    <div className="cv-compare" role="region" aria-label="CVs side by side">
      <div className="compare">
        <CvPane docs={docs} doc={left.data} selected={leftId} onSelect={setLeft} />
        <CvPane docs={docs} doc={right.data} selected={rightId} onSelect={setRight} />
      </div>
      <details className="cv-compare-diff">
        <summary>What their details disagree on</summary>
        <CvDifferences a={left.data} b={right.data} />
      </details>
    </div>
  );
}
