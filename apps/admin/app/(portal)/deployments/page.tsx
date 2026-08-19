import { ComingSoon } from "../../../components/ComingSoon";

export default function DeploymentsPage() {
  return (
    <ComingSoon
      area="Deployments"
      reason="deployment/release visibility is Phase 29's territory (CI/CD, Helm releases, rollout status) - nothing in the platform tracks deployments as data yet, so this page would have nothing real to show until that phase lands."
    />
  );
}
