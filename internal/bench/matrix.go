package bench

import (
	"encoding/json"
	"fmt"
	"io"
)

type MatrixConfig struct {
	Models    []string
	N         int
	Host      string
	GitCommit string
}

// RunMatrix runs every model x task x repeat, writing one JSON line per run to w
// progress is called after each run (may be nil)
func RunMatrix(tasks []Task, mc MatrixConfig, w io.Writer, progress func(done, total int, r Result)) error {

	if mc.N < 1 || len(mc.Models) == 0 || len(tasks) == 0 {
		return fmt.Errorf("nothing to run: %d models, %d tasks, n=%d", len(mc.Models), len(tasks), mc.N)
	}
	total := len(mc.Models) * len(tasks) * mc.N
	done := 0

	for _, model := range mc.Models {
		for _, task := range tasks {
			for i := 0; i < mc.N; i++ {
				res, err := Run(task, Config{Model: model, Host: mc.Host, RunIndex: i, GitCommit: mc.GitCommit})
				if err != nil {
					return fmt.Errorf("%s / %s / run %d: %w", model, task.Name, i, err)
				}

				err = json.NewEncoder(w).Encode(res)
				if err != nil {
					return err
				}

				done += 1
				if progress != nil {
					progress(done, total, res)
				}

				// no request go through typo or dead server
				//the row above is kept as evidence stop before writing more junk
				if res.Ended == "model_error" && res.PromptTokens == 0 {
					return fmt.Errorf("%s: the first request failed, so no model call completed (check the model name and host)", model)
				}
			}
		}
	}

	return nil
}
