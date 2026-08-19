import { ComingSoon } from "../../../components/ComingSoon";

export default function SessionsPage() {
  return (
    <ComingSoon
      area="Sessions & Devices"
      reason="devices has DeleteAll for privacy exports but no admin-facing list/revoke-one HTTP surface yet - core-api tracks device registrations, not live session state, so this page would need a new endpoint rather than just a new UI, which is out of scope for this phase."
    />
  );
}
