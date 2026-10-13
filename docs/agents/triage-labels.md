# Triage roles

Keep these canonical names:
`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`.

Interpret each role using the Idea's current state and full conversation.
Use the following mapping.

| Role | Idea Store interpretation | Next step |
| --- | --- | --- |
| `needs-triage` | `idea:new` or an imported Idea without a state. | Read the context and identify scope and project. Let intake and assessment own the next lifecycle transition. |
| `needs-info` | `idea:clarifying` with an outstanding information request. | Obtain the missing answer. The owner resumes through Idea ready or Answer; a plain comment alone does not resume processing. |
| `ready-for-agent` | The next step in Investigation or Implementation is available to an agent. | Verify the required plan or spec. Confirm that the Idea and project are unpaused, dependencies are satisfied, budget is available and applicable code-slot ownership is established. The state alone does not establish readiness. |
| `ready-for-human` | A decision is needed in Clarifying or Needs Attention, or a research result or PR awaits owner review. | Name the specific offered action: answer, Retry, review the report, request rework or decide whether to merge. |
| `wontfix` | The owner explicitly declines further work. | Record the reason. Use owner-authorized Close when the current workflow offers it. |

Record a triage recommendation and its reason in an agent-authored comment.
Treat that comment as a dated assessment.
Re-evaluate it when the Idea changes.

Do not create workflow tags with these names.
Do not assign Store states mechanically from roles.
Do not force active, waiting or completed Ideas into a triage role.
Use `needs-info` in Clarifying when information is missing.
Use `ready-for-human` there when a decision on supplied options is needed.
Record the required human action explicitly.
Preserve the lifecycle's existing gates when recommending `ready-for-agent`.

`idea:done` records a reported result; it does not prove merge or deployment.
`idea:researched` records a research result awaiting owner review.
`idea:needs-attention` requires reading the diagnostic reason before choosing Retry.
`idea:closed` does not establish rejection; a completed split parent can use it.
`idea:abandoned` records a final PR closed without merge.
Use `wontfix` only for an explicit decision to decline work.
