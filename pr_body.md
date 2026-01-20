Show message count in search output to help agents and users understand when an email is part of a multi-message conversation.

## Problem

When searching Gmail via `gog gmail search`, the results show thread IDs but not the number of messages in each thread. This makes it difficult for AI agents (and humans) to determine if a message is a standalone email or part of a conversation.

**Example scenario:**
- User asks agent: "Are there any new emails from Claire?"
- Agent runs `gog gmail search 'newer_than:1d'`
- Results show "Meilleurs vœux 2026" from Claire
- Agent reads the email and sees it's a New Year's greeting
- **Missing:** The agent doesn't see that there are 6 messages in this thread with a back-and-forth conversation

## Solution

This PR adds message count to the search output:

**Before:**
```
ID  DATE  FROM  SUBJECT  LABELS
19b97acb5adefd83  Jan 20  Claire  Meilleurs vœux 2026  CATEGORY_PERSONAL,INBOX
```

**After:**
```
ID  DATE  FROM  SUBJECT  LABELS  THREAD
19b97acb5adefd83  Jan 20  Claire  Meilleurs vœux 2026  CATEGORY_PERSONAL,INBOX  [6 msgs]
```

Single-message emails show `-`, multi-message threads show `[X msgs]`.
The `LABELS` column is preserved for backward compatibility.

## Changes

1. **Added `MessageCount` field** to `threadItem` struct (for JSON output)
2. **Populated message count** from `len(thread.Messages)` in `fetchThreadDetails`
3. **Added new THREAD column** showing `[X msgs]` for threads, `-` for single messages
4. **Preserved LABELS column** for backward compatibility
5. **JSON output** includes `messageCount` field for programmatic access

## Prompt Context

This change was inspired by a conversation with an AI agent:

> I'm using gogcli from an AI agent and I was looking at recent emails. I searched for emails from today and found an email from Claire but I couldn't see that there were replies in the thread. The search results only showed the original email, not the replies. I ran `gog gmail get <msgId>` which only returned the first message. I should have used `gog gmail thread get <msgId>` to see the full thread.

The agent needed to run `gog gmail thread get <msgId>` to see the full conversation, but couldn't tell from the search results that it was necessary.

## Testing

- All existing Gmail tests pass
- Manual testing confirms:
  - Both LABELS and THREAD columns are displayed
  - Single messages show `-` in THREAD column
  - Multi-message threads show `[X msgs]` (e.g., `[6 msgs]`)
  - JSON output includes `messageCount` field
