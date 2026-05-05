"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import { Inbox, Users, Settings, Mail, Megaphone, Workflow, CheckSquare } from "lucide-react";

const SUB_NAV = [
  { href: "/outreach", label: "Inbox", icon: Inbox, exact: true },
  { href: "/outreach/campaigns", label: "Campaigns", icon: Megaphone },
  { href: "/outreach/sequences", label: "Sequences", icon: Workflow },
  { href: "/outreach/tasks", label: "Tasks", icon: CheckSquare },
  { href: "/outreach/contacts", label: "Contacts", icon: Users },
  { href: "/outreach/settings/profile", label: "Sender profile", icon: Mail },
  { href: "/outreach/settings/channels", label: "Channels", icon: Settings },
];

export default function OutreachLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2 border-b border-border pb-2 -mx-4 px-4 lg:-mx-6 lg:px-6 overflow-x-auto">
        {SUB_NAV.map((item) => {
          const active = item.exact ? pathname === item.href : pathname.startsWith(item.href);
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors whitespace-nowrap",
                active
                  ? "bg-blue-50 text-blue-800"
                  : "text-muted-foreground hover:bg-accent hover:text-foreground",
              )}
            >
              <item.icon className="h-4 w-4" />
              {item.label}
            </Link>
          );
        })}
      </div>
      <div>{children}</div>
    </div>
  );
}
