// The implement-task workflow, run against scripted agents: what each role is asked, in what
// order, what reaches the other one, and how the duel ends. No model runs here: make test-workflows
// runs it in the pinned Node image.
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const source = await readFile(new URL('./implement-task.js', import.meta.url), 'utf8')
const AsyncFunction = Object.getPrototypeOf(async () => {}).constructor
const workflow = new AsyncFunction('args', 'agent', 'phase', 'log', source.replace('export const meta', 'const meta'))

// duel runs the workflow with args. Each agent call is answered by script[label], a value or a
// function of the prompt; calls lists every call made, in order.
async function duel(args, script) {
  const calls = []
  const agent = async (prompt, opts) => {
    calls.push({ label: opts.label, role: opts.agentType, prompt })
    assert.ok(opts.label in script, `no scripted answer for "${opts.label}"`)
    const answer = script[opts.label]
    return typeof answer === 'function' ? answer(prompt) : answer
  }
  const result = await workflow(args, agent, () => {}, () => {})
  return { result, calls, labels: calls.map((c) => c.label) }
}

const task = { ticket: 'DL-1', task: 'report an empty create as an error' }
const failing = (name) => ({ file: 'internal/tools/manage/create_test.go', name, fails_because: 'the create reports a success' })
const tests = (names, extra = {}) => ({ tests: names.map(failing), questions: [], done: false, ...extra })
const code = (extra = {}) => ({ changed: ['internal/tools/manage/create.go'], passing: true, blocked: false, questions: [], ...extra })

test('the tests come first, and each role gets the questions and answers of the other', async () => {
  const { result, calls, labels } = await duel(task, {
    'tester: tests first': tests(['TestEmptyCreate'], { questions: ['Is a 204 empty too?'] }),
    'coder: round 0': code({ questions: ['Which hint?'], answers: ['A 204 is empty too.'] }),
    'tester: attack 1': tests([], { done: true, answers: ['Say where the value goes.'] }),
  })

  assert.deepEqual(labels, ['tester: tests first', 'coder: round 0', 'tester: attack 1'])
  assert.deepEqual(calls.map((c) => c.role), ['tester', 'coder', 'tester'])
  assert.match(calls[0].prompt, /report an empty create as an error/)
  assert.match(calls[1].prompt, /TestEmptyCreate \(fails because the create reports a success\)/)
  assert.match(calls[1].prompt, /The Tester's questions for you:\n- Is a 204 empty too\?/)
  assert.match(calls[2].prompt, /The Coder's questions for you:\n- Which hint\?/)
  assert.match(calls[2].prompt, /The Coder's answers to your questions:\n- A 204 is empty too\./)
  assert.equal(result.outcome, 'passing')
  assert.deepEqual(result.open_questions, [], "the Tester answered the Coder's question")
  assert.deepEqual(result.last_answers, ['Say where the value goes.'], 'the answer no Coder got is returned')
})

test("an attack's new failing tests go back to the Coder, even when the Tester is done looking", async () => {
  const { result, calls, labels } = await duel(task, {
    'tester: tests first': tests(['TestEmptyCreate']),
    'coder: round 0': code(),
    'tester: attack 1': tests(['TestEmptyCreateInAnAlert'], { done: true, answers: ['Yes, an alert too.'] }),
    'coder: round 1': code(),
    'tester: attack 2': tests([], { done: true }),
  })

  assert.deepEqual(labels, ['tester: tests first', 'coder: round 0', 'tester: attack 1', 'coder: round 1', 'tester: attack 2'])
  assert.match(calls[3].prompt, /TestEmptyCreateInAnAlert/)
  assert.match(calls[3].prompt, /The Tester's answers to your questions:\n- Yes, an alert too\./)
  assert.equal(result.outcome, 'passing')
  assert.equal(result.history.length, 2)
})

test('the duel stops at its last round, red when the code still fails', async () => {
  const { result, labels } = await duel({ ...task, rounds: 1 }, {
    'tester: tests first': tests(['TestEmptyCreate']),
    'coder: round 0': code(),
    'tester: attack 1': tests(['TestEmptyCreateInAnAlert'], { questions: ['And for a file?'] }),
    'coder: round 1': code({ passing: false, questions: ['Is a file observable in scope?'] }),
  })

  assert.deepEqual(labels, ['tester: tests first', 'coder: round 0', 'tester: attack 1', 'coder: round 1'])
  assert.equal(result.outcome, 'red')
  assert.deepEqual(result.open_questions, ['And for a file?', 'Is a file observable in scope?'])
})

test('rounds are capped at four attacks', async () => {
  const script = { 'tester: tests first': tests(['TestA']) }
  for (let r = 0; r <= 4; r++) script[`coder: round ${r}`] = code()
  for (let r = 1; r <= 4; r++) script[`tester: attack ${r}`] = tests([`TestAttack${r}`])

  const { labels } = await duel({ ...task, rounds: 99 }, script)
  assert.equal(labels.filter((l) => l.startsWith('coder')).length, 5)
  assert.equal(labels.at(-1), 'coder: round 4')
})

test('a blocked Coder, a Coder or an attacking Tester that fails to answer, and a Tester with no test each end the duel', async () => {
  let run = await duel(task, { 'tester: tests first': tests(['TestA']), 'coder: round 0': code({ blocked: true }) })
  assert.equal(run.result.outcome, 'blocked')
  assert.deepEqual(run.labels, ['tester: tests first', 'coder: round 0'])

  run = await duel(task, { 'tester: tests first': tests(['TestA']), 'coder: round 0': null })
  assert.equal(run.result.outcome, 'coder failed')

  run = await duel(task, { 'tester: tests first': tests([]) })
  assert.equal(run.result.outcome, 'no tests')
  assert.deepEqual(run.labels, ['tester: tests first'], 'no Coder without a failing test')

  run = await duel(task, { 'tester: tests first': tests(['TestA']), 'coder: round 0': code(), 'tester: attack 1': null })
  assert.equal(run.result.outcome, 'tester failed', 'an attack that never ran is no pass')
  assert.equal(run.result.history.length, 1)
})

test('a duel needs a task', async () => {
  await assert.rejects(duel({ ticket: 'DL-1' }, {}), /needs args.task/)
})

// Sealed tests: with a ticket key the Tester also writes tests the Coder never sees; once the duel
// is over they move into the code and must pass.
const sealedTest = { file: '.claude/sealed/DL-1/internal/tools/manage/sealed_test.go', name: 'TestSealedEmptyCreate' }
const sealedDuel = (sealedAnswers, coderOnSealed) => {
  let run = 0
  return {
    'tester: tests first': tests(['TestEmptyCreate'], { sealed: [sealedTest] }),
    'coder: round 0': code(),
    'tester: attack 1': tests([], { done: true }),
    'tester: sealed tests': () => sealedAnswers[run++],
    'coder: sealed tests': coderOnSealed,
  }
}

test('with a ticket key the Tester seals tests, which the Coder never sees, and they run once the duel is over', async () => {
  const { result, calls, labels } = await duel(task, sealedDuel([{ unsealed: ['internal/tools/manage/sealed_test.go'], failing: [] }]))

  assert.deepEqual(labels, ['tester: tests first', 'coder: round 0', 'tester: attack 1', 'tester: sealed tests'])
  assert.match(calls[0].prompt, /under \.claude\/sealed\/DL-1\//)
  assert.match(calls[3].prompt, /bin\/harness unseal --ticket DL-1/)
  for (const c of calls.filter((x) => x.role === 'coder')) assert.doesNotMatch(c.prompt, /sealed|TestSealedEmptyCreate/i)
  assert.equal(result.outcome, 'passing')
})

test('a sealed test that fails gets the Coder one round, and runs again', async () => {
  const failingSealed = { ...sealedTest, fails_because: 'an alert parent is not tried' }
  const { result, calls, labels } = await duel(
    task,
    sealedDuel([{ unsealed: ['sealed_test.go'], failing: [failingSealed] }, { unsealed: ['sealed_test.go'], failing: [] }], code()),
  )

  assert.deepEqual(labels.slice(3), ['tester: sealed tests', 'coder: sealed tests', 'tester: sealed tests'])
  assert.match(calls[4].prompt, /TestSealedEmptyCreate \(fails because an alert parent is not tried\)/)
  assert.equal(result.outcome, 'passing')
})

test('sealed tests that did not move into the code, or still fail, leave the duel red', async () => {
  let run = await duel(task, sealedDuel([{ unsealed: [], failing: [] }]))
  assert.equal(run.result.outcome, 'unseal failed')

  const failingSealed = { ...sealedTest, fails_because: 'still' }
  const stillFailing = { unsealed: ['sealed_test.go'], failing: [failingSealed] }
  run = await duel(task, sealedDuel([stillFailing, stillFailing], code()))
  assert.equal(run.result.outcome, 'red')

  run = await duel(task, sealedDuel([stillFailing], code({ passing: false })))
  assert.equal(run.result.outcome, 'red', 'the Coder could not make them pass')
  assert.equal(run.labels.filter((l) => l === 'tester: sealed tests').length, 1, 'nothing to run again')
})

test('without a ticket key nothing is sealed, and with the visible tests red nothing is unsealed', async () => {
  let run = await duel({ task: task.task }, {
    'tester: tests first': tests(['TestEmptyCreate'], { sealed: [sealedTest] }),
    'coder: round 0': code(),
    'tester: attack 1': tests([], { done: true }),
  })
  assert.doesNotMatch(run.calls[0].prompt, /\.claude\/sealed/)
  assert.equal(run.result.outcome, 'passing', 'the sealed list is ignored without a ticket')

  run = await duel({ ...task, rounds: 0 }, {
    'tester: tests first': tests(['TestEmptyCreate'], { sealed: [sealedTest] }),
    'coder: round 0': code({ passing: false }),
  })
  assert.deepEqual(run.labels, ['tester: tests first', 'coder: round 0'], 'no unseal while the visible tests fail')
  assert.equal(run.result.outcome, 'red')
})
