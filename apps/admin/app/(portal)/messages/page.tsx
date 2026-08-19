import { ComingSoon } from "../../../components/ComingSoon";

export default function MessagesPage() {
  return (
    <ComingSoon
      area="Messages"
      reason="messaging only exposes conversations/messages scoped to the caller's own membership, with no admin-wide listing - and message content is user-private data an admin console shouldn't surface without a specific support-case reason, which argues for a scoped lookup tool rather than a browsable table."
    />
  );
}
