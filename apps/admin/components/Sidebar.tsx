"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { NAV } from "./nav";

export function Sidebar() {
  const pathname = usePathname();
  return (
    <div className="sidebar">
      <div className="brand">🕹️ Core Platform</div>
      <nav>
        {NAV.map((group) => (
          <div key={group.section}>
            <div className="section-label">{group.section}</div>
            {group.items.map((item) => {
              const active = item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
              return (
                <Link key={item.href} href={item.href} className={active ? "active" : ""}>
                  {item.label}
                  {!item.live && <span style={{ opacity: 0.5 }}> · soon</span>}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>
      <div className="footer">Admin Portal · Phase 25</div>
    </div>
  );
}
