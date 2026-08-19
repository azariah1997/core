import { ApiError } from "../lib/api";

export function ApiErrorBox({ error }: { error: unknown }) {
  if (error instanceof ApiError) {
    return (
      <div className="error-box">
        {error.status} {error.code ? `(${error.code})` : ""} — {error.message}
      </div>
    );
  }
  return <div className="error-box">{error instanceof Error ? error.message : "Something went wrong."}</div>;
}
