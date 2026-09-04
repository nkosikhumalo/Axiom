package solver

import (
	"fmt"
	"os/exec"
	"strings"
)

// Result holds the Z3 solver response.
type Result struct {
	Sat       bool   // true = counterexample found (FAIL), false = UNSAT (PASS)
	Model     string // raw Z3 model output when SAT
	RawOutput string
}

// RunZ3 feeds the SMT-LIB2 query to Z3 via stdin and parses the response.
func RunZ3(query string) (*Result, error) {
	if _, err := exec.LookPath("z3"); err != nil {
		return nil, fmt.Errorf("z3 not found in PATH — install it with: brew install z3  or  sudo apt-get install z3")
	}

	cmd := exec.Command("z3", "-in")
	cmd.Stdin = strings.NewReader(query)

	out, err := cmd.Output()
	// Z3 exits 0 for both sat and unsat; non-zero usually means a parse error
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("z3 execution failed: %w", err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, fmt.Errorf("z3 returned empty output — check your SMT query")
	}

	result := &Result{RawOutput: raw}

	firstLine, rest, _ := strings.Cut(raw, "\n")
	firstLine = strings.TrimSpace(firstLine)

	switch firstLine {
	case "unsat":
		result.Sat = false
	case "sat":
		result.Sat = true
		result.Model = strings.TrimSpace(rest)
	default:
		return nil, fmt.Errorf("unexpected z3 response: %q\nFull output:\n%s", firstLine, raw)
	}

	return result, nil
}
