# System Audit 2026-03-29

| Page | Component | Function | Status | Note |
| --- | --- | --- | --- | --- |
| `/login` | auth form | wrong password reject | pass | inline error, stays on page |
| `/login` | auth form | valid password accept | pass | `123123` lands on `/work` |
| `/onboarding` | onboarding flow | route load and task path | pass | load and preview-run handoff both work |
| `/work` | launch form | start work through dev proxy | pass | authenticated browser POST accepted after proxy/auth fix |
| `/work` | launch form | active-root busy handling | pass | second launch while root active now returns a run and opens detail |
| `/work/[runId]` | run detail | graph, stream, inspector | pass | live stream attached; graph and inspector render |
| `/work/[runId]` | run detail | dismiss action | pass | interrupted run dismisses cleanly in live browser |
| `/team` | overview panel | active setup summary render | pass | active profile heading + team name line |
| `/team` | edit mode | open full editor | pass | editor, actions, topology render; team name label is clear |
| `/team` | profile switcher | switch active setup | pass | switch and hard reload stayed aligned |
| `/team` | setup actions | create setup | pass | browser create and auto-select work |
| `/team` | setup actions | copy setup | pass | clone and auto-select work |
| `/team` | setup actions | delete setup | pass | inactive profile delete works |
| `/team` | setup actions | import setup file | pass | imported draft stays visible; save is still required |
| `/team` | setup actions | export YAML | pass | browser export request returns 200 |
| `/team` | role topology | add role | pass | unsaved state and member count update |
| `/team` | role topology | remove role | pass | removing added role restores clean state |
| `/team` | member card | edit role text | pass | edited field can be saved |
| `/team` | member card | change base profile | pass | edited field can be saved |
| `/team` | member card | toggle tool authority | pass | chip toggle updates draft and save path |
| `/team` | member card | toggle delegation posture | pass | chip toggle updates draft and save path |
| `/team` | draft state | unsaved banner and discard | pass | banner shows and clears on discard |
| `/team` | navigation guard | leave with unsaved changes | pending | browser tool hits blocked confirm on unload path; verify manually or by test |
| `/team` | wording and hierarchy | active profile vs setup name clarity | pass | active profile now heads the page; team name is labeled separately |
| `/team` | responsive layout | mobile width and overflow | pass | 390px wide render has no horizontal overflow |
| `/automate` | index page | board load and schedule cards | pass | summary, running-now, and recent sections render |
| `/automate` | schedule form | default start time for every/at | pass | start time now pre-fills and submits cleanly |
| `/automate` | schedule form | create schedule submit | pass | first valid submission creates a schedule |
| `/automate` | schedule card | pause and resume | pass | button pair toggles the schedule state |
| `/automate` | schedule card | run now | pass | schedule run opens the new work detail |
| `/knowledge` | index page | empty/default state load | pass | stable after rebuild noise |
| `/knowledge` | filters and list | search, scope, limit, item drill-in | pending | needs one cleaner pass without browser-driver cross-talk |
| `/knowledge` | item lifecycle | create from runtime memory promotion | pass | work prompt created a real memory item |
| `/knowledge` | item lifecycle | edit content | pass | save edit persisted and refreshed labels |
| `/knowledge` | item lifecycle | forget with confirm | pass | confirm removed the item and restored empty state |
| `/recover` | status panels | route and runtime state load | pass | read-only load stable |
| `/recover` | route actions | deactivate and recover flows | pending | not deep-tested yet |
| `/conversations` | index page | list and detail navigation | pass | list and connector health render with live data |
| `/conversations` | index page | filters and pagination | pending | not deep-tested yet |
| `/conversations/[sessionId]` | detail page | route context, history, delivery retry | pass | history, route authority, and retry controls render |
| `/conversations/[sessionId]` | detail page | send message redirect | pass | send returns `run_id` and opens the new work run |
| `/conversations/[sessionId]` | detail page | busy banner and disabled send | pass | active run surfaces and blocks send |
| `/automate` | recent executions | recent occurrence list | pass | recent rows render once schedules exist |
| `/history` | history page | incidents, deliveries, retries | pass | load and run-link paths both render |
| `/settings` | settings page | auth, approvals, provider state | pass | password validation and machine save both pass |
