import { redirect } from "next/navigation";

// The standalone inbox is merged into the Email Groups experience (now titled
// "Inbox"). Anyone landing on /outreach goes straight there.
export default function OutreachPage() {
  redirect("/outreach/groups");
}
