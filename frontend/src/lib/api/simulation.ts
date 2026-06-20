import { apiFetch } from "./client";
import type { SequenceStep, AssistantMessage } from "../types/outreach";
import type { PersonaOption, Simulation } from "../types/simulation";

export function listPersonas(): Promise<{ personas: PersonaOption[] }> {
  return apiFetch("/outreach/simulations/personas");
}

export function listSimulations(): Promise<{ simulations: Simulation[] }> {
  return apiFetch("/outreach/simulations");
}

export function getSimulation(id: string): Promise<Simulation> {
  return apiFetch(`/outreach/simulations/${id}`);
}

export function runSimulation(body: {
  name: string;
  mode?: "interactive" | "quick";
  market_id: string;
  brand_id: string;
  lead_count: number;
  personas: string[];
  steps: SequenceStep[];
  on_positive_action?: string;
  on_negative_action?: string;
  // Per-tag reply guidance (tag → instruction) — persisted + editable mid-run
  // in interactive mode.
  playbook?: Record<string, string>;
  include_lessons?: boolean;
}): Promise<Simulation> {
  return apiFetch("/outreach/simulations", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function deleteSimulation(id: string): Promise<void> {
  return apiFetch(`/outreach/simulations/${id}`, { method: "DELETE" });
}

// --- Interactive stepping ---

export function advanceSimulation(id: string): Promise<Simulation> {
  return apiFetch(`/outreach/simulations/${id}/advance`, { method: "POST" });
}

export function approveSimLead(id: string, leadID: string): Promise<Simulation> {
  return apiFetch(`/outreach/simulations/${id}/leads/${leadID}/approve`, { method: "POST" });
}

export function dismissSimLead(id: string, leadID: string): Promise<Simulation> {
  return apiFetch(`/outreach/simulations/${id}/leads/${leadID}/dismiss`, { method: "POST" });
}

export function editSimDraft(
  id: string,
  leadID: string,
  draft: { subject: string; body: string },
): Promise<Simulation> {
  return apiFetch(`/outreach/simulations/${id}/leads/${leadID}/draft`, {
    method: "PATCH",
    body: JSON.stringify(draft),
  });
}

export function refineSimDraft(id: string, leadID: string, instruction: string): Promise<Simulation> {
  return apiFetch(`/outreach/simulations/${id}/leads/${leadID}/refine`, {
    method: "POST",
    body: JSON.stringify({ instruction }),
  });
}

export function getSimPlaybook(id: string): Promise<{ playbook: Record<string, string> }> {
  return apiFetch(`/outreach/simulations/${id}/playbook`);
}

export function setSimPlaybook(id: string, playbook: Record<string, string>): Promise<Simulation> {
  return apiFetch(`/outreach/simulations/${id}/playbook`, {
    method: "PUT",
    body: JSON.stringify({ playbook }),
  });
}

// --- Simulation agent (mirrors the group draft assistant) ---

export function getSimAssistant(id: string): Promise<{ messages: AssistantMessage[] }> {
  return apiFetch(`/outreach/simulations/${id}/assistant`);
}

export function sendSimAssistantMessage(id: string, message: string): Promise<AssistantMessage> {
  return apiFetch(`/outreach/simulations/${id}/assistant/messages`, {
    method: "POST",
    body: JSON.stringify({ message }),
  });
}

export function confirmSimAssistantMessage(
  id: string,
  messageID: string,
): Promise<{ updated: number; failed: number }> {
  return apiFetch(`/outreach/simulations/${id}/assistant/messages/${messageID}/confirm`, {
    method: "POST",
  });
}

export function dismissSimAssistantMessage(id: string, messageID: string): Promise<void> {
  return apiFetch(`/outreach/simulations/${id}/assistant/messages/${messageID}/dismiss`, {
    method: "POST",
  });
}
