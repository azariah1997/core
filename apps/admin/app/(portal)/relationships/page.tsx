import { ComingSoon } from "../../../components/ComingSoon";

export default function RelationshipsPage() {
  return (
    <ComingSoon
      area="Relationships"
      reason="relationships only exposes GET /v1/relationships as listMine - there is no admin-wide or resource-scoped tuple browser, and building one usefully needs a resource-type-aware query builder rather than a flat table."
    />
  );
}
