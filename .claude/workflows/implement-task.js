export const meta = {
  name: 'implement-task',
  description: 'Test-first duel on one task: the Tester writes failing tests, the Coder makes them pass, the Tester attacks',
  whenToUse: 'Implementing a TheHiveMCP ticket or spec item test first. Args: {ticket, task, items?, rounds?}',
  phases: [
    { title: 'Tests first', detail: 'the Tester writes failing tests from the task' },
    { title: 'Implement', detail: 'the Coder makes them pass, never touching a test' },
    { title: 'Attack', detail: 'the Tester looks for what breaks the code; the Coder answers' },
  ],
}

// args: {ticket: "DL-1234", task: "what to build, with its Done when", items?: ["SIL-001"], rounds?: 2}
// The two agents are .claude/agents/tester.md and coder.md; the hooks keep each to its files. They
// never talk directly: each one's questions go into the other's next prompt. The session that runs
// this workflow reviews, commits and pushes afterwards.

const a = args || {}
if (!a.task) throw new Error('implement-task needs args.task: what to build and its Done when')
const ticket = a.ticket || 'no ticket'
const items = (a.items || []).join(', ')
const rounds = Math.max(0, Math.min(a.rounds === undefined ? 2 : a.rounds, 4))

const TESTS = {
  type: 'object',
  properties: {
    tests: {
      type: 'array',
      items: {
        type: 'object',
        properties: { file: { type: 'string' }, name: { type: 'string' }, fails_because: { type: 'string' } },
        required: ['file', 'name', 'fails_because'],
      },
    },
    answers: { type: 'array', items: { type: 'string' }, description: "answers to the Coder's questions" },
    questions: { type: 'array', items: { type: 'string' }, description: 'questions for the Coder' },
    done: { type: 'boolean', description: 'true when you found nothing more that breaks the code' },
  },
  required: ['tests', 'questions', 'done'],
}

const CODE = {
  type: 'object',
  properties: {
    changed: { type: 'array', items: { type: 'string' } },
    passing: { type: 'boolean', description: 'every test passes, the new ones included' },
    blocked: { type: 'boolean', description: 'you wrote .cache/harness/BLOCKED.md' },
    answers: { type: 'array', items: { type: 'string' }, description: "answers to the Tester's questions" },
    questions: { type: 'array', items: { type: 'string' }, description: 'questions for the Tester' },
  },
  required: ['changed', 'passing', 'blocked', 'questions'],
}

const listTests = (t) => t.tests.map((x) => `- ${x.file}: ${x.name} (fails because ${x.fails_because})`).join('\n')
const relay = (label, lines) => (lines && lines.length ? `\n${label}:\n- ${lines.join('\n- ')}\n` : '')

const header = `Ticket ${ticket}${items ? `, spec items ${items}` : ''}.\nThe task:\n${a.task}\n`

phase('Tests first')
let tests = await agent(
  `${header}\nWrite the failing tests that prove this task, before any code exists. Check that each fails for the reason the task gives, and that the older tests still pass.`,
  { agentType: 'tester', label: 'tester: tests first', schema: TESTS },
)
if (!tests || tests.tests.length === 0) {
  return { ticket, outcome: 'no tests', note: 'the Tester wrote no failing test: the task may already be done, or be unclear', tests }
}

const history = [{ round: 0, tests: tests.tests }]
let code = null

for (let round = 0; round <= rounds; round++) {
  phase('Implement')
  code = await agent(
    `${header}\nThe Tester's failing tests:\n${listTests(tests)}\n${relay("The Tester's questions for you", tests.questions)}${relay("The Tester's answers to your questions", tests.answers)}\nMake them pass with the smallest change that does what the task asks.`,
    { agentType: 'coder', label: `coder: round ${round}`, schema: CODE },
  )
  if (!code) return { ticket, outcome: 'coder failed', history }
  history[history.length - 1].code = code
  if (code.blocked) return { ticket, outcome: 'blocked', note: 'see .cache/harness/BLOCKED.md', history }
  if (round === rounds) break

  phase('Attack')
  tests = await agent(
    `${header}\nThe Coder changed: ${code.changed.join(', ') || 'nothing'}; tests passing: ${code.passing}.${relay("The Coder's questions for you", code.questions)}${relay("The Coder's answers to your questions", code.answers)}\nAttack the implementation: write the failing tests for what still breaks it. Set done when you find nothing more.`,
    { agentType: 'tester', label: `tester: attack ${round + 1}`, schema: TESTS },
  )
  if (!tests || tests.done || tests.tests.length === 0) break
  history.push({ round: round + 1, tests: tests.tests })
}

log(`duel over: ${history.length} round(s), tests passing: ${code ? code.passing : false}`)
return {
  ticket,
  outcome: code && code.passing ? 'passing' : 'red',
  open_questions: [...((tests && tests.questions) || []), ...((code && code.questions) || [])],
  history,
}
