"use client";

import { use } from "react";
import { ConversationThread } from "@/components/outreach/conversation-thread";

type PageProps = { params: Promise<{ conversationId: string }> };

export default function ConversationPage({ params }: PageProps) {
  const { conversationId } = use(params);
  return <ConversationThread conversationId={conversationId} backHref="/outreach" />;
}
