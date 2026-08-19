import { ComingSoon } from "../../../components/ComingSoon";

export default function TenantsPage() {
  return (
    <ComingSoon
      area="Tenants"
      reason="tenants only exposes GET /v1/tenants as listMine (the caller's own memberships), with no admin-wide listing endpoint - the same gap GET /v1/users closed for users this phase, but adding it for every self-scoped module at once is out of scope here."
    />
  );
}
