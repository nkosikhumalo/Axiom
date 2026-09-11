package reporter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/nkosikhumalo/axiom/internal/solver"
)

const divider = "-------------------------------------------------------------"

var (
	passStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	failStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

// Report prints the verification result to stdout.
func Report(result *solver.Result, legacyFile, modernFile string) {
	fmt.Println()

	if !result.Sat {
		fmt.Println(passStyle.Render("  EQUIVALENCE VERIFIED"))
		fmt.Println(dimStyle.Render(divider))
		fmt.Printf("  Legacy : %s\n", legacyFile)
		fmt.Printf("  Modern : %s\n", modernFile)
		fmt.Println(dimStyle.Render(divider))
		fmt.Println(dimStyle.Render("  Z3 returned UNSAT — no counterexample exists."))
		fmt.Println(dimStyle.Render("  The rewrite is mathematically equivalent."))
		fmt.Println()
		return
	}

	fmt.Println(failStyle.Render("  EQUIVALENCE VERIFICATION FAILED"))
	fmt.Println(dimStyle.Render(divider))
	fmt.Println(warnStyle.Render("  Reason: Logic discrepancy detected between legacy and modern code."))
	fmt.Println()

	counterexample := parseModel(result.Model)
	if len(counterexample) > 0 {
		fmt.Println(labelStyle.Render("  Counterexample inputs (Z3 model):"))
		for _, v := range counterexample {
			fmt.Printf("    %s\n", v)
		}
	} else if result.Model != "" {
		fmt.Println(labelStyle.Render("  Z3 model:"))
		for _, line := range strings.Split(result.Model, "\n") {
			fmt.Printf("    %s\n", line)
		}
	}

	fmt.Println()
	fmt.Println(dimStyle.Render(divider))
	fmt.Println()
}

// parseModel extracts `(define-fun varName () Type value)` entries from the Z3 model
// and formats them as readable "varName = value" strings.
func parseModel(model string) []string {
	re := regexp.MustCompile(`\(define-fun\s+(\S+)\s+\(\)\s+\S+\s+`)
	matches := re.FindAllStringSubmatchIndex(model, -1)

	var result []string
	for _, match := range matches {
		name := model[match[2]:match[3]]
		valueStart := match[1]
		valueEnd := balancedExpressionEnd(model, valueStart)
		if valueEnd == -1 {
			continue
		}
		value := strings.TrimSpace(model[valueStart:valueEnd])
		// Skip internal Z3 names and function output definitions
		if strings.HasPrefix(name, "legacy_") || strings.HasPrefix(name, "modern_") {
			continue
		}
		result = append(result, fmt.Sprintf("%s = %s", name, value))
	}
	return result
}

func balancedExpressionEnd(model string, start int) int {
	depth := 0
	started := false
	for index := start; index < len(model); index++ {
		switch model[index] {
		case '(':
			depth++
			started = true
		case ')':
			if started {
				depth--
				if depth == 0 {
					return index + 1
				}
			} else {
				return index
			}
		case '\n':
			if !started {
				return index
			}
		}
	}
	if !started {
		lineEnd := strings.IndexByte(model[start:], '\n')
		if lineEnd >= 0 {
			return start + lineEnd
		}
		return len(model)
	}
	return -1
}
