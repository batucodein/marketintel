import type { Metadata } from "next";
import { Fira_Code, Fira_Sans } from "next/font/google";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AuthProvider } from "@/lib/providers/auth-provider";
import { SWRProvider } from "@/lib/providers/swr-provider";
import "./globals.css";

const firaSans = Fira_Sans({
  variable: "--font-sans",
  subsets: ["latin"],
  weight: ["300", "400", "500", "600", "700"],
});

const firaCode = Fira_Code({
  variable: "--font-mono",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
});

export const metadata: Metadata = {
  title: "MarketIntel",
  description: "AI-powered B2B market intelligence for exporters",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className={`${firaSans.variable} ${firaCode.variable} h-full antialiased`}>
      <body className="min-h-full flex flex-col font-sans bg-background text-foreground">
        <AuthProvider>
          <SWRProvider>
            <TooltipProvider>{children}</TooltipProvider>
          </SWRProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
