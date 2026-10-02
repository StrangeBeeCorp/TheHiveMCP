export const meta = {
  name: 'implement-task',
  description: 'Test-first duel on one task: the Tester writes failing tests, the Coder makes them pass, the Tester attacks',
  whenToUse: 'Implementing a TheHiveMCP ticket or spec item test first. Args: {ticket, task, items?, rounds?}',
  phases: [
    { title: 'Tests first', detail: 'the Tester writes failing tests from the task' },
    { title: 'Implement', detail: 'the Coder makes them pass, never touching a test' },
    { title: 'Attack', detail: 'the Tester looks for what breaks the code; the Coder answers' },
    { title: 'Unseal', detail: 'the sealed tests move into the code; the Coder fixes what they catch' },
  ],
}

// args: {ticket: "DL-1234", task: "what to build, with its Done when", items?: ["SIL-001"], rounds?: 2}
// The two agents are .claude/agents/tester.md and coder.md; the hooks keep each to its files. They
// never talk directly: each one's questions go into the other's next prompt. With a ticket key, the
// Tester also seals tests under .claude/sealed/<ticket>/, which the Coder cannot read: at the end
// bin/harness unseal moves them into the code, and the Coder gets one round on what they catch. The
// session that runs this workflow reviews, commits and pushes afterwards.

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
    sealed: {
      type: 'array',
      items: { type: 'object', properties: { file: { type: 'string' }, name: { type: 'string' } }, required: ['file', 'name'] },
      description: 'the sealed tests, under .claude/sealed/<ticket>/',
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
const sealable = /^[A-Z][A-Z0-9]*-[0-9]+$/.test(ticket)
const sealing = sealable
  ? `\nAlso seal one to three tests the Coder will not see until the end: the cases an honest implementation of the task passes and one shaped to the visible tests would not. Write each under .claude/sealed/${ticket}/ at the path it will have in the repository (.claude/sealed/${ticket}/internal/tools/search/sealed_test.go), as a new file, and list them in sealed.`
  : ''

const UNSEALED = {
  type: 'object',
  properties: {
    unsealed: { type: 'array', items: { type: 'string' } },
    failing: TESTS.properties.tests,
  },
  required: ['unsealed', 'failing'],
}

phase('Tests first')
let tests = await agent(
  `${header}\nWrite the failing tests that prove this task, before any code exists. Check that each fails for the reason the task gives, and that the older tests still pass.${sealing}`,
  { agentType: 'tester', label: 'tester: tests first', schema: TESTS },
)
if (!tests || tests.tests.length === 0) {
  return { ticket, outcome: 'no tests', note: 'the Tester wrote no failing test: the task may already be done, or be unclear', tests }
}

const sealed = (sealable && tests.sealed) || []
const history = [{ round: 0, tests: tests.tests, sealed }]
let code = null
// Who spoke last: the Coder at the last round, or the Tester when an attack found nothing more.
let testerLast = false

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
  if (!tests) return { ticket, outcome: 'tester failed', note: 'the attacking Tester gave no answer: the code was not attacked', history }
  // New failing tests go to the Coder even when the Tester says it is done looking.
  if (tests.tests.length === 0) {
    testerLast = true
    break
  }
  history.push({ round: round + 1, tests: tests.tests })
}

// The last one to speak in the duel sent questions and answers the other never got. The other's
// questions reached it, and count as answered when it gave answers.
const lastAnswers = (testerLast ? tests.answers : code.answers) || []
const answered = lastAnswers.length > 0
const openQuestions = [...(testerLast || !answered ? tests.questions : []), ...(!testerLast || !answered ? code.questions : [])]
const duelCode = code

// The sealed tests must have moved into the code and pass: a sealed test that never ran is no pass.
let sealedPass = sealed.length === 0
if (sealed.length > 0 && code && code.passing) {
  phase('Unseal')
  const runSealed = (what) =>
    agent(
      `${header}\n${what} Run them (make test RUN=<their names>, or make test-integration RUN=… for those that need TheHive). List the sealed test files in the code, and every sealed test that fails, with why.`,
      { agentType: 'tester', label: 'tester: sealed tests', schema: UNSEALED },
    )
  let unsealed = await runSealed(
    `The duel is over and the visible tests pass. Run \`bin/harness unseal --ticket ${ticket}\`, which moves your sealed tests into the code.`,
  )
  history.push({ round: 'unseal', unsealed })

  if (!unsealed || unsealed.unsealed.length === 0) {
    return { ticket, outcome: 'unseal failed', note: `the sealed tests are not in the code: run bin/harness unseal --ticket ${ticket}`, history }
  }

  if (unsealed.failing.length > 0) {
    code = await agent(
      `${header}\nThe Tester's sealed tests are now in the code, and these fail:\n${listTests({ tests: unsealed.failing })}\nMake them pass with the smallest change that does what the task asks: they test the task, not your code.`,
      { agentType: 'coder', label: 'coder: sealed tests', schema: CODE },
    )
    history[history.length - 1].code = code
    unsealed = code && code.passing ? await runSealed('The Coder answered your sealed tests, which are in the code.') : null
    history.push({ round: 'sealed again', unsealed })
  }

  sealedPass = Boolean(unsealed && unsealed.failing.length === 0)
}

log(`duel over: ${history.length} round(s), tests passing: ${code ? code.passing : false}, sealed tests passing: ${sealedPass}`)
return {
  ticket,
  outcome: code && code.passing && sealedPass ? 'passing' : 'red',
  open_questions: [...openQuestions, ...(code && code !== duelCode ? code.questions : [])],
  last_answers: lastAnswers,
  history,
}
