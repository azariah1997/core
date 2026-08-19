export type NavItem = { label: string; href: string; live: boolean };

// Every one of the roadmap's 18 named visibility areas gets a real URL
// in this sidebar - live:true means it renders real data from core-api;
// live:false means it renders an honest "not built yet, here's why"
// page (components/ComingSoon.tsx), never fake data. Billing and AI
// Gateway aren't named in the roadmap's 18 but are cheap, real, and
// valuable, so they're included too.
export const NAV: { section: string; items: NavItem[] }[] = [
  {
    section: "Overview",
    items: [{ label: "Dashboard", href: "/", live: true }],
  },
  {
    section: "Identity & Access",
    items: [
      { label: "Users", href: "/users", live: true },
      { label: "Sessions & Devices", href: "/sessions", live: false },
      { label: "Roles & Permissions", href: "/permissions", live: true },
      { label: "Applications", href: "/applications", live: true },
      { label: "Tenants", href: "/tenants", live: false },
    ],
  },
  {
    section: "Social & Comms",
    items: [
      { label: "Relationships", href: "/relationships", live: false },
      { label: "Messages", href: "/messages", live: false },
      { label: "Notifications", href: "/notifications", live: false },
    ],
  },
  {
    section: "Platform",
    items: [
      { label: "Files", href: "/files", live: false },
      { label: "Jobs", href: "/jobs", live: true },
      { label: "Feature Flags", href: "/features", live: true },
      { label: "Configuration", href: "/configuration", live: true },
      { label: "Billing", href: "/billing", live: true },
      { label: "AI Gateway", href: "/ai", live: true },
    ],
  },
  {
    section: "Trust & Ops",
    items: [
      { label: "Moderation", href: "/moderation", live: true },
      { label: "Audit", href: "/audit", live: true },
      { label: "System Health", href: "/health", live: true },
      { label: "Deployments", href: "/deployments", live: false },
    ],
  },
];
