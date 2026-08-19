import Link from "next/link";

export default function PermissionsPage() {
  return (
    <div>
      <h1>Permissions model</h1>
      <p className="subtitle">
        Documentation, not a data table - the platform&apos;s access model spans two systems working together.
      </p>

      <div className="card" style={{ marginBottom: 16 }}>
        <h2>RBAC - roles (authz)</h2>
        <p>
          A small, fixed set of platform-wide roles (<code className="mono">platform.admin</code>,{" "}
          <code className="mono">support</code>, <code className="mono">moderator</code>) grant coarse capabilities
          that cut across modules - e.g. <code className="mono">platform.admin</code> is what
          <code className="mono"> IsPlatformAdmin</code> checks to authorize <code className="mono">GET /v1/users</code>,
          cross-user entitlement/usage lookups, and role management itself.{" "}
          <code className="mono">moderator</code> gates the Trust &amp; Safety case queue. Manage role grants from a
          user&apos;s <Link href="/users">detail page</Link>.
        </p>
      </div>

      <div className="card">
        <h2>ReBAC - relationships (permissions engine)</h2>
        <p>
          Fine-grained, resource-scoped access ("can user X edit document Y") is handled separately by the
          relationship-tuple permissions engine from earlier in the roadmap, not by authz&apos;s roles. That
          visibility surface is deferred in this portal - see{" "}
          <Link href="/relationships">Relationships</Link> - since browsing raw tuples usefully needs a
          resource-type-aware query builder rather than a generic table, which is out of scope for this phase.
        </p>
      </div>
    </div>
  );
}
