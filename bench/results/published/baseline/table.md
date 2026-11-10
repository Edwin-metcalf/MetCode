| Model | Task | N | Pass | ok/max/fail/err | timeout/unreach | No-edit claims | Tampered | Calls/run | Tool errors | Unnecessary/run | Wall s (median, range) | Prompt tok/run |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| llama3.1:latest | add-bug | 10 | 0/10 | 10/0/0/0 | 0/0 | 10 | 0 | 2.0 | 0 | - | 5.6 (5.1-7.1) | 2317 |
| llama3.1:latest | add-bug-descriptive | 10 | 3/10 | 10/0/0/0 | 0/0 | 1 | 0 | 4.4 | 20 | - | 9.1 (7.5-13.9) | 4087 |
| llama3.1:latest | change-constant | 10 | 0/10 | 10/0/0/0 | 0/0 | 4 | 0 | 2.4 | 16 | - | 4.6 (3.4-6.6) | 2425 |
| llama3.1:latest | discount-two-files | 10 | 0/10 | 10/0/0/0 | 0/0 | 10 | 0 | 2.0 | 0 | - | 6.6 (3.9-10.4) | 2561 |
| llama3.1:latest | greeting | 10 | n/a | 10/0/0/0 | 0/0 | n/a | 0 | 0.0 | 0 | 0.0 | 1.0 (0.5-1.4) | 1161 |
| llama3.1:latest | missing-function | 10 | 0/10 | 10/0/0/0 | 0/0 | 10 | 0 | 1.8 | 8 | - | 13.9 (6.9-22.1) | 2678 |
| llama3.1:latest | two-bugs | 10 | 0/10 | 10/0/0/0 | 0/0 | 10 | 0 | 2.2 | 0 | - | 6.8 (5.6-7.9) | 2609 |
| qwen2.5-coder:7b | add-bug | 10 | 0/10 | 10/0/0/0 | 0/0 | 10 | 0 | 1.3 | 0 | - | 5.6 (1.9-8.0) | 2834 |
| qwen2.5-coder:7b | add-bug-descriptive | 10 | 1/10 | 9/0/0/0 | 1/0 | 8 | 0 | 1.7 | 0 | - | 6.6 (3.6-121.8) | 3276 |
| qwen2.5-coder:7b | change-constant | 10 | 0/10 | 10/0/0/0 | 0/0 | 10 | 0 | 1.0 | 10 | - | 2.3 (2.0-2.7) | 2458 |
| qwen2.5-coder:7b | discount-two-files | 10 | 0/10 | 10/0/0/0 | 0/0 | 10 | 0 | 0.8 | 0 | - | 5.0 (1.2-7.8) | 2265 |
| qwen2.5-coder:7b | greeting | 10 | n/a | 10/0/0/0 | 0/0 | n/a | 0 | 0.0 | 0 | 0.0 | 1.1 (1.1-1.3) | 1135 |
| qwen2.5-coder:7b | missing-function | 10 | 0/10 | 10/0/0/0 | 0/0 | 10 | 0 | 0.2 | 0 | - | 1.9 (1.5-9.5) | 1437 |
| qwen2.5-coder:7b | two-bugs | 10 | 0/10 | 10/0/0/0 | 0/0 | 10 | 0 | 1.9 | 0 | - | 6.6 (3.8-10.0) | 3528 |
