// Package agentcore holds the provider-agnostic ReAct loop shared by every
// tool-calling assistant (the group draft agent and the simulation agent). It
// only uses prompt+system strings via ai.CompleteJSON and parses JSON out — no
// provider's native function-calling API — so switching LLM providers is a
// task→provider remap and nothing here changes. Each caller supplies its own
// tool dispatcher + action validator and keeps its own persistence/confirm flow.
package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// Dispatch runs one read-only tool and returns its result rendered as text.
type Dispatch func(name string, args json.RawMessage) string

// Validate returns the proposed action only if it's well-formed, else nil.
type Validate func(*prompts.AssistantAction) *prompts.AssistantAction

const maxSteps = 6
const toolResultMax = 1500

// RunTurn executes the bounded loop for one user turn and returns the final
// reply (a graceful fallback is substituted if the model never answers) and the
// validated proposed action. ctx must already carry the user id (ai.WithUserID).
// onTool, if set, is called with each tool name as it runs. logLabel identifies
// the caller in the degraded-path warning log.
func RunTurn(ctx context.Context, router *ai.Router, base prompts.AgentTurnInput, dispatch Dispatch, validate Validate, onTool func(name string), logLabel string) (string, *prompts.AssistantAction) {
	var (
		toolLog strings.Builder
		reply   string
		action  *prompts.AssistantAction
		lastRaw string
		aiErr   error
	)
	for step := 0; step < maxSteps; step++ {
		in := base
		in.ToolLog = strings.TrimSpace(toolLog.String())
		in.LastStep = step == maxSteps-1
		p := prompts.BuildAgentPrompt(in)
		raw, _, err := router.CompleteJSON(ctx, "assistant_chat", p.Prompt, p.System, 0)
		if err != nil {
			// The model emitted something that wasn't clean JSON (stray prose,
			// truncation, fences). Don't give up — nudge once and retry, exactly
			// like the parse-failure path below. Only bail on the final step.
			aiErr = err
			if in.LastStep {
				break
			}
			toolLog.WriteString("NOTE: your last response could not be parsed. Respond with ONLY the JSON object {reply, tool, action} — no prose, fences, or text before or after it.\n\n")
			continue
		}
		lastRaw = string(raw)
		var out prompts.AgentTurnResult
		if json.Unmarshal(raw, &out) != nil {
			if in.LastStep {
				break
			}
			toolLog.WriteString("NOTE: your last response was not valid JSON. Respond with ONLY the JSON object {reply, tool, action}.\n\n")
			continue
		}
		if !in.LastStep && out.Tool != nil && strings.TrimSpace(out.Tool.Name) != "" {
			if onTool != nil {
				onTool(out.Tool.Name)
			}
			result := dispatch(out.Tool.Name, out.Tool.Args)
			fmt.Fprintf(&toolLog, "TOOL %s(%s)\n→ %s\n\n",
				out.Tool.Name, strings.TrimSpace(string(out.Tool.Args)), truncate(result, toolResultMax))
			continue
		}
		reply = strings.TrimSpace(out.Reply)
		if validate != nil {
			action = validate(out.Action)
		} else {
			action = out.Action
		}
		if reply == "" && action == nil && !in.LastStep {
			toolLog.WriteString("NOTE: you returned an empty reply. You now have enough — write a direct answer to the user in \"reply\" (if there's no data for something, say so plainly).\n\n")
			continue
		}
		break
	}
	if reply == "" {
		if action != nil {
			reply = "Here's what I'd do — confirm to apply."
		} else {
			reply = "I couldn't pull that together just now — mind rephrasing, or ask me about your drafts, replies, leads, or playbook?"
			slog.Warn("agent: empty final reply", "label", logLabel, "ai_err", aiErr, "last_raw", truncate(lastRaw, 600))
		}
	}
	return reply, action
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
