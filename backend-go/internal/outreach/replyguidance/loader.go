// Package replyguidance assembles the "standing reply guidance" block fed into
// the reply-draft prompt. Two sources, both keyed off the inbound reply's intent
// tags: the per-group playbook (authored) and per-brand learned lessons
// (remembered, matched by tags + sentiment).
package replyguidance

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const lessonLimit = 6

// Load returns the concatenated guidance text for drafting a reply on a
// conversation. campaignID may be nil (no playbook); brandID is the resolved
// sender profile. Returns "" when there is nothing to add. DB errors are
// logged (guidance silently vanishing is a debugging nightmare) but never
// block drafting.
func Load(ctx context.Context, pool *pgxpool.Pool, convID uuid.UUID, campaignID *uuid.UUID, brandID uuid.UUID) string {
	tags := convTags(ctx, pool, convID)
	bucket := sentimentBucket(ctx, pool, convID)

	var lines []string

	// 1. Per-group playbook: authored instructions for the reply's tags.
	if campaignID != nil && len(tags) > 0 {
		rows, err := pool.Query(ctx,
			`SELECT instruction FROM campaign_tag_guidance WHERE campaign_id=$1 AND tag = ANY($2)`,
			*campaignID, tags,
		)
		if err != nil {
			slog.Warn("replyguidance: playbook query failed", "campaign_id", *campaignID, "error", err)
		} else {
			for rows.Next() {
				var s string
				if rows.Scan(&s) == nil && strings.TrimSpace(s) != "" {
					lines = append(lines, "- "+SanitizeLine(s))
				}
			}
			rows.Close()
		}
	}

	// 2. Per-brand learned lessons: same sentiment bucket, and either untagged
	//    or sharing a tag with this reply — so a lesson never leaks onto a
	//    different kind of buyer answer. The 6 most recent matches are used,
	//    ordered oldest→newest so the newest lesson lands LAST in the prompt
	//    (most salient position — it wins on conflicts).
	rows, err := pool.Query(ctx,
		`SELECT instruction FROM (
		   SELECT instruction, created_at FROM brand_reply_lessons
		    WHERE sender_profile_id=$1 AND match_sentiment=$2
		      AND (cardinality(match_tags)=0 OR match_tags && $3)
		    ORDER BY created_at DESC LIMIT $4
		 ) recent ORDER BY created_at ASC`,
		brandID, bucket, tags, lessonLimit,
	)
	if err != nil {
		slog.Warn("replyguidance: lessons query failed", "brand_id", brandID, "error", err)
	} else {
		for rows.Next() {
			var s string
			if rows.Scan(&s) == nil && strings.TrimSpace(s) != "" {
				lines = append(lines, "- "+SanitizeLine(s))
			}
		}
		rows.Close()
	}

	return strings.Join(lines, "\n")
}

// SanitizeLine flattens user-authored guidance to a single line and neuters
// markdown section markers, so a multi-line lesson can't forge a new "## "
// prompt section and override the drafting rules.
func SanitizeLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	for strings.Contains(s, "##") {
		s = strings.ReplaceAll(s, "##", "#")
	}
	return s
}

func convTags(ctx context.Context, pool *pgxpool.Pool, convID uuid.UUID) []string {
	rows, err := pool.Query(ctx, `SELECT tag FROM conversation_tags WHERE conversation_id=$1`, convID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			out = append(out, t)
		}
	}
	return out
}

// sentimentBucket returns the conversation's last inbound 3-value sentiment
// (positive|neutral|negative), defaulting to neutral.
func sentimentBucket(ctx context.Context, pool *pgxpool.Pool, convID uuid.UUID) string {
	bucket := "neutral"
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(last_inbound_sentiment, 'neutral') FROM conversations WHERE id=$1`, convID,
	).Scan(&bucket)
	return bucket
}

// Lessons-write helper kept here so the conversation service doesn't hand-roll
// the insert. tags is the conversation's current tag set; bucket its sentiment.
func RememberLesson(ctx context.Context, pool *pgxpool.Pool, userID, brandID uuid.UUID, convID uuid.UUID, instruction string) error {
	tags := convTags(ctx, pool, convID)
	bucket := sentimentBucket(ctx, pool, convID)
	if tags == nil {
		tags = []string{}
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO brand_reply_lessons (user_id, sender_profile_id, instruction, match_tags, match_sentiment)
		 VALUES ($1,$2,$3,$4,$5)`,
		userID, brandID, instruction, tags, bucket,
	)
	if err != nil {
		return fmt.Errorf("remember lesson: %w", err)
	}
	return nil
}
