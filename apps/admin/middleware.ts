import { NextRequest, NextResponse } from "next/server";

const COOKIE_NAME = "core_admin_session";
const PUBLIC_PATHS = ["/login"];

// Redirect-if-unauthenticated at the edge, before any page renders -
// middleware can only read the cookie's presence (Edge runtime can't
// easily reuse the server-only session module's JSON parsing the same
// way page code does), so it's a coarse "is there a session cookie at
// all" gate; each page's own data fetching (lib/api.ts) still enforces
// the real check by needing a non-expired token to call core-api with
// at all - a request with an expired-but-present cookie fails there
// with a clear 401, not a silent wrong-data render.
export function middleware(request: NextRequest) {
  const isPublic = PUBLIC_PATHS.some((p) => request.nextUrl.pathname.startsWith(p));
  const hasSession = request.cookies.has(COOKIE_NAME);

  if (!isPublic && !hasSession) {
    const url = request.nextUrl.clone();
    url.pathname = "/login";
    return NextResponse.redirect(url);
  }
  if (isPublic && hasSession) {
    const url = request.nextUrl.clone();
    url.pathname = "/";
    return NextResponse.redirect(url);
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
