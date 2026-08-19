export function ComingSoon({ area, reason }: { area: string; reason: string }) {
  return (
    <div className="card">
      <h2>{area}</h2>
      <p style={{ color: "var(--muted)", lineHeight: 1.6 }}>
        This view isn&apos;t built yet - deliberately, not silently skipped. <strong>Why:</strong> {reason}
      </p>
    </div>
  );
}
