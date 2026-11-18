# baselinePlus — the n=20 reference run
November 17th 2026

Baseline plus is a larger run of MetBench that was inspired by the shortcomings of the original bench and the appearance of quick fixes. 
There were no major architecture overhauls or new tools granted to the models. This update focused on increasing the capabilities and reliability of what was already there.

The run came out at 81/240 graded passes (33.8%) with greeting at 40/40. This is about 10 times the pass rate of the original baseline. 
This run also gave us more data to see that llama and qwen are very different and strong at almost the opposite tasks.

These small models are finicky, when working directly with them you can guide them to the right answer however when left to their own devices they struggle to 
leverage the harness and get the job done. The changes in baselinePlus were meant to show how a few changes could greatly increase the reliability of these
models on simple tasks that should not be failing.

The 10x gains we see come from harness behavior not from prompt engineering and trying to reason with the model.

## Why this run happened

The original baseline was run with only an n=10 and the results were not all that exciting to do a larger test. Since then there have been plenty of smaller runs
with an n=5 which put the noise at +/-3/60 so a move of 2-3 passes was useless. So after adding a couple changes we wanted to run a more proper baseline with a 
larger n. This run was supposed to also show how some quick updates to a harness really benefit these small models that struggle without guidance. So we built 
the guidance into the harness.

## Setup / how to reproduce

If you want to reproduce this run yourself these are the important settings that were used. It was run on personal hardware, an NVIDIA 1080, so some duration may be 
different depending on your gear.

- run:

  ```bash
  go run ./cmd/metbench run \
    -models llama3.1:latest,qwen2.5-coder:7b \
    -n 20 \
    -host http://YOURHOST:11434 \
    -out bench/results/published/baselinePlus/runs.jsonl
  ```

- table gen:

  ```bash
  go run ./cmd/metbench report bench/results/published/baselinePlus/runs.jsonl > bench/results/published/baselinePlus/table.md
  ```

- date / duration: Nov 17 2026, about 1h30
- models: llama3.1:latest, qwen2.5-coder:7b
- commit: 382ee8836c6b15837f218bdbd7cd815c5d98421a
- prompt hash: bc4522a14fdb4cea13f3b8586673f57ba0475a55671fca240416280107ad368c
- ollama: 0.32.15 | temperature 0.2 | num_ctx 8192 | n=20
- 6 graded tasks x 2 models x 20 + greeting x 2 models x 20 = 280 rows

## Results at a glance

The results were very interesting, they showed huge leaps since the original baseline but also showed us clear next steps. The biggest revelation in the results is
the varied nature from model to model. Qwen and llama have almost opposite strengths and it shows in the results. But either way this shows a great improvement 
from the original baseline which only had 4/120 tests passing.

| Task | llama pass | qwen pass | llama applied | qwen applied | llama no-edit claim | qwen no-edit claim | tampered |
|---|---|---|---|---|---|---|---|
| add-bug | 19/20 | 4/20 | 19 | 4 | 0 | 16 | 0 |
| add-bug-descriptive | 20/20 | 12/20 | 20 | 12 | 0 | 8 | 0 |
| change-constant | 1/20 | 17/20 | 4 | 19 | 15 | 1 | 0 |
| discount-two-files | 0/20 | 0/20 | 14 | 8 | 0 | 11 | 6 |
| missing-function | 0/20 | 8/20 | 7 | 11 | 8 | 4 | 3 |
| two-bugs | 0/20 | 0/20 | 17 | 8 | 3 | 12 | 0 |
| greeting | n/a, 0 unnecessary | n/a, 0 unnecessary | - | - | - | - | - |

**Total graded: 81/240 (33.8%, 95% CI 28.1-39.9). Greeting: 40/40.**

**No edit claim:** the report counts rows that ended ok and left the workspace byte-for-byte unchanged.
There is also an issue with the model claiming success without calling edit_file which led to the creation of nudge that caught this case,
and sent a message to the model recommending edit_file.


## What changed since the baseline

A decent amount of things have changed since the baseline but none were large architectural changes or giving the model a lot more capabilities. The changes in 
the baseline were things like nudging the model with the error that the tool returned. Then realizing it always called read file again so changing the nudge
to recommend the model call edit file.

Smoke study progression (all n=5, 60 graded runs each): EvenBetterSysPrompt 9/60 -> withMatch.GO 8 -> Match.goRound2 11 -> NudgeV1 9 -> WholeMessageNormalize 8 -> NudgeV2 14 -> TaskFixes 21 -> RepairEscapes 26. Before these runs the original baseline was 4/120 at n=10.

Important to note the impact TaskFixes had because change-constant expected rounded value that rejected the correct answer. Missing function was missing the third row of a matrix as well. 

These were all run in small smoke tests but they show the general progression of changes and improvements to metrics. These runs saw changes to normalizeMessage 
to increase the plain-text to tool call coverage, the ability to edit a whole file for models in edit file, fix literal `\` and `n` instead of `\n` and white space anchors.

Here is a more complete list:
- read-then-edit prompt rule, tool definitions
- normalizeMessage for plain-text / alias tool calls
- match.go loose whitespace anchors
- whole-file content mode + guards
- nudge v1/v2 (quote the last tool error)
- two fixture bugs fixed (change-constant epsilon, missing-function matrix row)
- repaired literal backslash-n to `\n`

## What we learned

The main lesson is that these models are perfectly capable of solving these problems and have the answer they just fail at the packaging. Qwen 
sent a correct add.go in seven runs using {path, new_text} and was refused because old_text was missing. Llama wrote read_file(path = "gravity_test.go") into old_text seven times as if instructing itself. In both cases the guard was right and the model still did not comply. 
This is a harness problem and not a reasoning problem.

The second lesson is that prompt wording is the least reliable and most frustrating lever. Earlier smoke runs moved add-bug-descriptive from 7 
passing to only 1. No matter how little the tweak to a prompt was there were unseen consequences somewhere else. Especially when dealing with 
multiple models. A change that helps one can hurt another. 

The third lesson is editing a file and fixing the bug are different measures (an obvious one but still valuable). Llama applied 81 edits and only 40 passed. The problem is no longer claiming a fix without an edit but fixing the wrong thing.

Number four is the nudge has a ceiling on qwen. It fired in 76 out of 120 runs. 33 of the qwen runs that never called edit_file were nudged and still did not edit.
13 of them replied with <tool_response> as plain prose. More nudging is not the answer here but a more robust harness is.

The last lesson is context. 36 of 240 runs accumulated more than 8192 prompt tokens across the turn (mean about 14k). Of those only 5 passed. 
All 9 max_rounds endings are in that group. This appears to be a proxy for growing past the window. Although not fully tested it argues for 
compact search results and automatic compaction to help the model on a longer loop.

## The biggest finding: the two models split and building a better MetCode

The biggest takeaway is the variance per model. Llama does great on the add-bug and its descriptive sibling yet struggles on all the others only getting a single 
pass in change-constant. While on the other hand qwen does a great job in change-constant with 17/20 and even gets 8/20 in missing-function but struggles in the 
two add bugs. Of course it is not that simple and both models went 0/20 on discount-two-files and two-bugs but improving at those tasks is the next round of work.

This variance in models also reflects the changes that happened in baselinePlus. The first changes were attempting to change the system prompt and 
tool definitions to try and reason with these models in plain english. However that felt like whack a mole, one change would raise the score here but lower it 
somewhere else. The variance between models reflects that and tells us that global prompt changes will never work for every use case. Although we can refine our 
prompts by testing both models these changes have been unreliable and not where the gains are to be made. We need to build MetCode to be able to generalize to 
models and handle the shortcomings of not just a specific model's use case but all of them. These solutions need to be at the harness level to have a testable and 
reliable impact on the quality of MetCode instead of relying on these models to assume the correct solution. This directly aligns with the small model goal and 
getting the most out of these models.


## Failure shapes, not just pass rate

So yes these models behaved very differently but how did that actually show up in the benchmark? 

It showed up in how these models failed the benchmark, and similarly to baseline most cases are not that they could not reason the solution.

- llama: 81 edits applied, only 40 passed -> 41 applied-but-wrong; 1 run never called edit_file in 120
- qwen: 62 applied, 41 passed -> 21 applied-but-wrong; 39 runs never called edit_file in 120
- the nudge reached its limit on qwen: 76 nudges, 33 of the runs that never called edit_file still didn't

## Three failures worth telling a story about

1. qwen: 31 refusals across 28 runs. In 9 add-bug runs the file in new_text was complete and correct. The same shape in two-bugs still left best := 0, so the recovery helps the call shape, not the reasoning.

2. llama: the guard did its job. The failure is that llama treats old_text as a place to describe its next step rather than to copy file text. There were 17 old_text looks like a function call errors and 22 old_text not found errors in that cell.

3. two-bugs: 25/40 edits applied, and every one fixes the loop bound but leaves best := 0. The test case [-5, -2, -9] wants -2; with best := 0 it returns 0. No amount of tool-shape repair finds that — the model needs to see the failing test output.


## Limitations of this run

- No seed is included for controlling some randomness
- one turn only
- 19 rows ended outside normal completion: 10 timeouts (5 of them qwen on missing-function) and 9 max_rounds (all llama). These are harness/budget outcomes, not model-quality verdicts.
- only two models
- no code quality check

## What is next

Next steps are two cheap wins before any new run. First, recover {path, new_text} when new_text is a complete Go file. This hypothetically helps
7 add-bug failures turn green. Second after an edit lands, feed the go test output back into the same turn and let the model correct itself. 
Architecturally the idea is a verification callback on Agent, wired from task.TestCmd in the bench and from go test ./... in the CLI.

Test both the add-bug and two-bugs at n=20 because they are likely to signal change or improvements in this regard. 

Down the line:
- project map to efficiently store project file locations and context without flooding the context limit.
- add grep and sed abilities to models to give them more control
- automatic compaction
- option to run llama.cpp as well


## Data

- Raw runs: bench/results/published/baselinePlus/runs.jsonl
- Summary table: bench/results/published/baselinePlus/table.md
- Previous baseline for comparison: bench/results/published/baseline/README.md