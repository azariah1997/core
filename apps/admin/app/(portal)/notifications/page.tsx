import { ComingSoon } from "../../../components/ComingSoon";

export default function NotificationsPage() {
  return (
    <ComingSoon
      area="Notifications"
      reason="notifications only exposes GET /v1/notifications as listMine, plus per-user preferences/quiet-hours - no admin-wide delivery view exists yet to show, e.g., aggregate delivery/failure rates across users."
    />
  );
}
