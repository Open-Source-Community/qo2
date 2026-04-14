#!/usr/bin/env python3
"""
Export user interview results as Markdown reports.
Usage: python3 scripts/export_markdown.py
Outputs one .md file per user in the exports/ directory.
"""

import sqlite3
import os

DIFFICULTY_LABELS = {1: "⭐ Easy", 2: "⭐⭐ Medium", 3: "⭐⭐⭐ Hard"}
STATUS_ICONS = {"pass": "✅", "fail": "❌", "skipped": "⏭️ Skipped"}


def export_markdown(db_path: str = "linux.db", output_dir: str = "exports"):
    if not os.path.exists(db_path):
        print(f"Error: Database file '{db_path}' not found.")
        return

    os.makedirs(output_dir, exist_ok=True)

    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    cur = conn.cursor()

    cur.execute("SELECT * FROM users ORDER BY user_id")
    users = cur.fetchall()

    if not users:
        print("No users found in the database.")
        return

    for user in users:
        uid   = user["user_id"]
        uname = user["name"]
        safe  = uname.replace(" ", "_")

        lines = []

        # ── Header ───────────────────────────────────────────────
        lines += [
            f"# 🧑‍💻 Interview Report: {uname}",
            "",
            "## 👤 Profile",
            "",
            f"| Field       | Value |",
            f"|-------------|-------|",
            f"| **Name**    | {uname} |",
            f"| **Email**   | {user['email'] or '—'} |",
            f"| **Phone**   | {user['phone'] or '—'} |",
            f"| **Year**    | {user['year'] or '—'} |",
            f"| **OSCian**  | {'Yes ✅' if user['oscian'] else 'No'} |",
            "",
        ]

        # ── Sessions ──────────────────────────────────────────────
        cur.execute(
            "SELECT * FROM sessions WHERE user_id = ? ORDER BY session_id",
            (uid,),
        )
        sessions = cur.fetchall()

        if not sessions:
            lines.append("> No sessions recorded for this user.\n")
        else:
            lines.append("## 📋 Sessions\n")

            for s_idx, ses in enumerate(sessions, 1):
                sid = ses["session_id"]
                lines += [
                    f"---",
                    f"### Session {s_idx} — {ses['time'] or 'Unknown date'}",
                    f"",
                    f"- **Score**: `{ses['score']}` &nbsp; **Result**: `{ses['result'] or '—'}`",
                    f"",
                ]

                # ── Submissions ───────────────────────────────────
                cur.execute(
                    """
                    SELECT
                        s.answer   AS user_answer,
                        s.score    AS points,
                        s.result   AS status,
                        q.text     AS question_text,
                        q.topic,
                        q.difficulty,
                        q.model_answer
                    FROM submissions s
                    JOIN questions q ON s.question_id = q.question_id
                    WHERE s.session_id = ?
                    ORDER BY s.submission_id
                    """,
                    (sid,),
                )
                subs = cur.fetchall()

                if not subs:
                    lines.append("*No submissions recorded for this session.*\n")
                    continue

                # Group by topic
                by_topic: dict[str, list] = {}
                for sub in subs:
                    topic = sub["topic"] or "General"
                    by_topic.setdefault(topic, []).append(sub)

                for topic, topic_subs in by_topic.items():
                    lines += [f"#### 📂 {topic}", ""]
                    for i, sub in enumerate(topic_subs, 1):
                        diff_label = DIFFICULTY_LABELS.get(sub["difficulty"], "Unknown")
                        raw_status = (sub["status"] or "fail").lower()
                        icon = STATUS_ICONS.get(raw_status, "❓")
                        points = sub["points"] or 0

                        lines += [
                            f"**Q{i}.** {sub['question_text']}",
                            f"",
                            f"| | |",
                            f"|---|---|",
                            f"| **Difficulty** | {diff_label} |",
                            f"| **User Answer** | `{sub['user_answer'] or '—'}` |",
                            f"| **Model Answer** | `{sub['model_answer'] or '—'}` |",
                            f"| **Result** | {icon} `{raw_status}` |",
                            f"| **Points** | `{points}` |",
                            f"",
                        ]

        # ── Write file ────────────────────────────────────────────
        filepath = os.path.join(output_dir, f"user_{uid}_{safe}.md")
        with open(filepath, "w", encoding="utf-8") as f:
            f.write("\n".join(lines))

        print(f"✅  Exported {uname!r}  →  {filepath}")

    conn.close()
    print("\nAll exports completed.")


if __name__ == "__main__":
    export_markdown()