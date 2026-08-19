import { redirect } from "next/navigation";
import { getSession } from "../../lib/session";
import { Sidebar } from "../../components/Sidebar";

export default async function PortalLayout({ children }: { children: React.ReactNode }) {
  const session = await getSession();
  if (!session) {
    // Middleware already redirects on a missing cookie; this catches the
    // "cookie present but expired" case each page's own data fetch would
    // otherwise only discover via a 401 deep inside a table.
    redirect("/login");
  }

  return (
    <div className="shell">
      <Sidebar />
      <main className="main">
        <div className="topbar">
          <div />
          <div className="row">
            <span className="who">{session.displayName}</span>
            <form action="/logout" method="post">
              <button className="btn small" type="submit">
                Sign out
              </button>
            </form>
          </div>
        </div>
        {children}
      </main>
    </div>
  );
}
