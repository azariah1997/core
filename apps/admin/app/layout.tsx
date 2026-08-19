import "./globals.css";

export const metadata = {
  title: "Core Platform Admin",
  description: "Control-plane UI for the Core Platform",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
