package uiact

import (
	"context"
	"encoding/json"
	"fmt"
)

type Receipt struct {
	Verified bool
}

func Run(ctx context.Context, r Runner, p Program, t Tree) (Receipt, error) {
	script, err := Render(p, t)
	if err != nil {
		return Receipt{}, err
	}

	out, err := r.Run(ctx, script)
	if err != nil {
		return Receipt{}, classify(err.Error(), fmt.Errorf("uiact: %s refused the program: %w", t.App, err))
	}

	var result struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return Receipt{}, fmt.Errorf("uiact: could not read back what happened (output %q): %w",
			truncate(string(out), 200), err)
	}
	if !result.OK {
		return Receipt{}, classify(result.Error, fmt.Errorf("uiact: the window refused the program: %s", result.Error))
	}
	return Receipt{Verified: result.Verified}, nil
}
