# The Small Model Project Baseline

MetCode now has a full benchmarking harness for evaluating Code
combined with small models. The first published large run of the harness acts as this baseline. 
This baseline highlights a lot of the pain points when working with small models. These findings are the 
first organized step in making MetCode more and more reliable.

## The reason behind building MetBench

It became more and more clear that we needed some way to properly evaluate the ability of MetCode and the 
7-8B parameter models. Especially a way to evaluate them that was repeatable consistent and easy to scale.
Therefore we built out MetBench. The value of having an evaluation harness lets us diagnose the issues and 
quirks fo these models to better understand what the next step should be in improving. 

Past standardizing evaluation the harness also saves us time and manual work. Instead of typing in the same 
prompt 10 times and recording the output we now can simply add the task and run it as many times as we
would like. This benchmarking harness was built to be adaptable, whether that means adding tasks or evaluation fields, it is incredibly easy
to change the harness so to identify new areas of concern.

## The baseline findings

The main issue we came across is how much direction these models need. When testing manually it was easy to
eventually get the model to do what you wanted. Whether that was editing a file or running a command. However a lot
of times you had to be very explicit about what you wanted the model to do. The harness found that with
little input (or no continuous conversational input) the models struggled to do what was asked. We found 
that even when given the tool calls to edit files, run commands, read files that agents still struggled to 
invoke the right calls. The baseline was run on two models: qwen2.5-coder:7b and llama3.1:latest. The model
choice direclty impacted the performance with llama tending to attempt to make more tool calls while qwen 
simply did not make the call.

### What we found

When working one on one in MetCode the errors are much less apparent as if the assistant does not properly
edit the file the first time you can simply reprompt it and be more specific. However in bench mark or an 
agentic non supervised scenario, the shortcomings are obvious. The only task that had successes was 
add-bug-descriptive which between the two models passed 4/20 times. Next to the base add-bug which never
passed is intersting. It shows how the slight difference in prompt increased the pass rate. The
descriptive prompt simply also stated "Use the read_file tool to understand the files, and then call the 
edit_file tool to make the edit." 

This highlights some of the issues of small models and the goals of MetCode. These models do not call tools 
very reliably. Sometimes they use different key words, or return the call in plain text, or wrapped in a 
markdown code block. A logical fix would be to be more precise, add more in-depth tool definitons,
and write more complicated prompts. That is where you hit the other issue: context limits. The whole 
point of MetCode and working with these small models is to run them on older or cheaper hardware. Therefore
the goal is to make these models a reliable and helpful tool on 8GB of VRAM. The tasks for this baseline were 
mostly simple with a single input prompt and tool calls only going up to an average of 4.4 per run at the 
highest case(llama3.1, add-bug-descriptive). That same example used an average of 4087 tokens a run eating up half the context window. 

Therefore it is incredibly important to consider these tradeoffs and build MetCode to be able to support 
these models in both a reliability but also efficiency manner.

### Data and etc

#### Results at a glance

| Task | llama3.1 pass | qwen2.5-coder pass | llama no-edit claims | qwen no-edit claims | llama calls/run | qwen calls/run |
|---|---|---|---|---|---|---|
| add-bug | 0/10 | 0/10 | 10 | 10 | 2.0 | 1.3 |
| add-bug-descriptive | 3/10 | 1/10 | 1 | 8 | 4.4 | 1.7 |
| change-constant | 0/10 | 0/10 | 4 | 10 | 2.4 | 1.0 |
| discount-two-files | 0/10 | 0/10 | 10 | 10 | 2.0 | 0.8|
| missing-function | 0/10 | 0/10 | 10 | 10 | 1.8 | 0.2 |
| two-bugs | 0/10 | 0/10 | 10 | 10 | 2.2 | 1.9 |
| greeting | na | na | na | na | 0.0 | 0.0|

**No edit claim** is the model returning text or output saying that it did the correct thing
but did not actually edit the file. Ie a models final reply says the fix was made, but no edit_file call was applied.

#### Findings
- Almost nothing passed, the only case that did pass was add-bug-descriptive.
    - Models needed the extra nudge to pass.
- Claims without edits were the dominant failure.
    - 103 of 120 coding runs claimed a fix without out applying one.
    - 45 of 60 for llama3.1 and 58 of 60 for qwen.
    - In these cases the model described a solution but no edit was made. This almost matters more than the passing rate because if MetCode can gracefully handle these and turn them into passes then these models will become much more useful.
- Applying an edit did not guarantee passing
    - llama3.1 applied an edit in 15 of 70 runs but only passed 3. Therefore 12 applied edits failed, qwen applied 1 edit that succeeded.
    - The gap could be caused by returning a malformed edit_file call. This could mean new key words MetCode was not prepared to handle.
- Tool errors clustered in three tasks for llam3.1
    - 20 erros on add-bug-descriptive, 16 on change constant, and 8 on missing function. Qwen only had 10 and they were all on change-constant.
- Prompt wording affected outcome
    - Although currently not a very impressive number the passing prompts on add-bug-descriptive show that with some more explaining you can wrestle the right answer out of these models. 
- Tampering of test
    - We made sure to check if the models edited the test and 0 out of 140 runs modified the test files
- One Timeout
    - The timeout happened with qwen running add-bug-descriptive, 121.8s max hit.
- Context cost 
    - Average tokens per task ranged 1,100 to 3,500 across two models.
    - The heaviest run, llama3.1 on add-bug-descriptive, used 4,087 tokens against a 8192 window.

#### Limitations
- 10 runs per cell is enough to see large gaps 0 vs 10 but not small differences such as 3/10 vs 1/10.
- Only two models, both 7-8B parameters and only one prompt set.
- One commit, so these numbers describe f723d28.
- Grading depends on go test output. A passing test suite is the only criteria, does not measure code quality

### What's next?
Planned fixes, in rough order:
- Better tool call parsing, strengthen normalize.go 
- Detect a claimed fix with no edit and retry once with a nudge
- Tighten tool-use instruction in the base prompt.
- Build a file index so agent can find files without repeated list_dir.
- Improve ability to grep and find the correct lines to then replace.

### Data
- Raw runs: ```bench/results/published/baseline/baseline.jsonl```
- Summary table: ```bench/results/published/baseline/table.md```
- Commit to check out for reproduction: ```f723d28```