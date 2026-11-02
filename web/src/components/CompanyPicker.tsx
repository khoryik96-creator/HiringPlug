import { api, unwrap } from "../api/client";
import { companiesKey } from "../api/keys";
import { type Picked, SearchPicker } from "./SearchPicker";

export type PickedCompany = Picked;

/** Choose a company by typing part of its name. */
export function CompanyPicker(props: {
  id?: string;
  value: PickedCompany | null;
  onChange: (company: PickedCompany | null) => void;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
}) {
  return (
    <SearchPicker
      {...props}
      queryKey={companiesKey}
      placeholder="Type to search companies"
      noMatch="No company matches. Add it under Companies first."
      search={async (q) =>
        unwrap(await api.GET("/v1/companies", { params: { query: { q, limit: 8 } } })).items.map((c) => ({ id: c.id, name: c.name, detail: c.city }))}
    />
  );
}
