import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "吃伴",
  description: "和朋友一起記錄飲食",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  // The app is used one-handed on a phone; let the browser account for the
  // home indicator and notch instead of us guessing.
  viewportFit: "cover",
  // Without this, iOS leaves `dvh` sized for the pre-keyboard viewport: the
  // composer stays put and the on-screen keyboard just covers whatever used
  // to be below it, so the browser's own "scroll the focused input into
  // view" opens a dead gap between the composer and the keyboard. This makes
  // `dvh` itself shrink for the keyboard, so the flex layout reflows and the
  // composer lands directly above it.
  interactiveWidget: "resizes-content",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="zh-Hant"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="bg-background text-foreground h-full overflow-hidden">
        {/* Mobile-first: a phone-width column that stays centred on desktop,
            exactly one viewport tall so each screen scrolls its own content. */}
        {/* overflow-y-auto so screens outside the app shell — login, register,
            onboarding, join — can still be scrolled when a short viewport or a
            large font puts their submit button below the fold. */}
        <div className="mx-auto flex h-dvh w-full max-w-md flex-col overflow-y-auto">
          {children}
        </div>
      </body>
    </html>
  );
}
