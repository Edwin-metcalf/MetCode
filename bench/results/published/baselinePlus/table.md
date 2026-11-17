| Model | Task | N | Pass | ok/max/fail/err | timeout/unreach | No-edit claims | Tampered | Calls/run | Tool errors | Edit fails mal/anc/oth | Unnecessary/run | Wall s (median, range) | Prompt tok/run |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| llama3.1:latest | add-bug | 20 | 19/20 | 19/0/0/0 | 1/0 | 0 | 0 | 3.9 | 18 (edit_file 18) | 18/0/0 | - | 9.9 (7.5-120.0) | 4613 |
| llama3.1:latest | add-bug-descriptive | 20 | 20/20 | 20/0/0/0 | 0/0 | 0 | 0 | 3.5 | 20 (edit_file 20) | 20/0/0 | - | 8.8 (6.7-12.7) | 3360 |
| llama3.1:latest | change-constant | 20 | 1/20 | 19/0/0/0 | 1/0 | 15 | 0 | 5.2 | 60 (edit_file 60) | 20/23/17 | - | 27.9 (10.2-124.9) | 9362 |
| llama3.1:latest | discount-two-files | 20 | 0/20 | 14/5/0/0 | 1/0 | 0 | 0 | 6.8 | 53 (edit_file 53) | 53/0/0 | - | 17.4 (10.6-147.5) | 8166 |
| llama3.1:latest | greeting | 20 | n/a | 20/0/0/0 | 0/0 | n/a | 0 | 0.0 | 0 | 0/0/0 | 0.0 | 1.4 (0.9-1.6) | 1261 |
| llama3.1:latest | missing-function | 20 | 0/20 | 15/4/0/0 | 1/0 | 8 | 0 | 6.5 | 61 (edit_file 60, read_file 1) | 28/32/0 | - | 31.3 (10.4-135.5) | 11107 |
| llama3.1:latest | two-bugs | 20 | 0/20 | 20/0/0/0 | 0/0 | 3 | 0 | 3.9 | 21 (edit_file 21) | 19/0/2 | - | 9.8 (7.5-12.6) | 3595 |
| qwen2.5-coder:7b | add-bug | 20 | 4/20 | 20/0/0/0 | 0/0 | 16 | 0 | 2.4 | 11 (edit_file 11) | 11/0/0 | - | 12.8 (3.5-21.4) | 6274 |
| qwen2.5-coder:7b | add-bug-descriptive | 20 | 12/20 | 20/0/0/0 | 0/0 | 8 | 0 | 2.2 | 4 (edit_file 4) | 4/0/0 | - | 12.4 (7.1-18.3) | 5964 |
| qwen2.5-coder:7b | change-constant | 20 | 17/20 | 20/0/0/0 | 0/0 | 1 | 0 | 2.0 | 1 (edit_file 1) | 1/0/0 | - | 21.6 (14.7-39.7) | 4940 |
| qwen2.5-coder:7b | discount-two-files | 20 | 0/20 | 19/0/0/0 | 1/0 | 11 | 6 | 1.9 | 0 | 0/0/0 | - | 22.6 (2.6-120.7) | 5489 |
| qwen2.5-coder:7b | greeting | 20 | n/a | 20/0/0/0 | 0/0 | n/a | 0 | 0.0 | 0 | 0/0/0 | 0.0 | 1.1 (1.1-1.4) | 1237 |
| qwen2.5-coder:7b | missing-function | 20 | 8/20 | 15/0/0/0 | 5/0 | 4 | 3 | 2.9 | 1 (update_plan 1) | 0/0/0 | - | 35.2 (18.8-142.6) | 6632 |
| qwen2.5-coder:7b | two-bugs | 20 | 0/20 | 20/0/0/0 | 0/0 | 12 | 0 | 2.5 | 15 (edit_file 15) | 15/0/0 | - | 12.2 (6.6-20.3) | 6307 |
