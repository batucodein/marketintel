"use client";

import { use } from "react";
import { ConversationThread } from "@/components/outreach/conversation-thread";

type PageProps = { params: Promise<{ groupId: string; conversationId: string }> };

export default function GroupConversationPage({ params }: PageProps) {
  const { groupId, conversationId } = use(params);
  return (
    <ConversationThread
      conversationId={conversationId}
      backHref={`/outreach/groups/${groupId}`}
    />
  );
}
