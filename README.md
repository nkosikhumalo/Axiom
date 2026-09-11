# Axiom

> Mathematically prove 100% logic equivalence when rewriting legacy monoliths into modern languages.

Axiom is an automated formal verification engine built in Go. It solves the "Big Bang Rewrite Trap" — the multi-million-dollar danger of breaking production systems when migrating legacy codebases (C++, Java, PHP) to modern architectures.

Instead of relying on human-written unit tests that miss obscure edge cases, Axiom parses both legacy and modern code into Abstract Syntax Trees (ASTs), walks every execution branch symbolically, and uses Satisfiability Modulo Theories (SMT) to mathematically guarantee zero logic drift.

---

## Why Axiom?

| Standard Unit Testing | Axiom Formal Verification |
| --- | --- |
| Tests only static inputs human devs can think of | Tests every possible theoretical input simultaneously |
| Misses obscure edge cases & off-by-one bugs | Catches hidden bugs before code touches production |
| High maintenance cost per test file | Zero-code setup via AST static code analysis |
| Requires running runtime code with live DBs | 100% Static Analysis — zero execution risk |

---

## How It Works

```
AXIOM ENGINE PIPELINE

┌────────────────┐     ┌───────────────────────┐     ┌────────────────────────┐
│ Legacy Code    │ ──> │ go-tree-sitter Parser │ ──> │ Symbolic Path Walker   │
└────────────────┘     └───────────────────────┘     └───────────┬────────────┘
                                                                  │
                                                      Generates Math Formulas
                                                                  │
┌────────────────┐     ┌───────────────────────┐                 v
│ Modern Go Code │ ──> │ go-tree-sitter Parser │ ──> ┌────────────────────────┐
└────────────────┘     └───────────────────────┘     │ Z3 SMT Theorem Prover  │
                                                      └───────────┬────────────┘
                                                                  │
                                                       UNSAT (Pass) / SAT (Fail)
                                                                  │
                                                                  v
                                                      ┌────────────────────────┐
                                                      │  Counterexample Report │
                                                      └────────────────────────┘
```

### The 4-Step Verification Process

1. **AST Extraction** — Uses `go-tree-sitter` to parse multi-language source code into Abstract Syntax Trees, stripping away language syntax differences to isolate pure logic.

2. **Path Constraint Mapping** — Traverses conditional branches (`if`, `else`, `switch`) symbolically, constructing path constraints as algebraic logical statements.

3. **Equivalence Theorem Assertion** — Feeds both mathematical representations into Microsoft Research's Z3 SMT Solver, testing the formal equivalence theorem:

   ```
   For all x: f_legacy(x) = f_modern(x)
   ```

4. **Counterexample Isolation** — To prove this, Z3 attempts to satisfy the negation:

   ```
   There exists x: f_legacy(x) != f_modern(x)
   ```

   - **UNSAT (Unsatisfiable):** Proof complete. The rewrite is 100% safe.
   - **SAT (Satisfiable):** Logic drift detected. Axiom isolates the exact counterexample input that breaks the rewrite.

---

## Quick Start

### Prerequisites

- Go 1.22+
- Z3 SMT Solver CLI
  - macOS: `brew install z3`
  - Ubuntu/Debian: `sudo apt-get install z3`

### Installation

```bash
git clone https://github.com/your-username/axiom.git
cd axiom
go mod download
```

### Running a Verification Check

```bash
go run main.go --legacy=./examples/legacy_discount.cpp --modern=./examples/modern_discount.go
```

### Interactive Mode

Run Axiom without flags in a terminal to open the keyboard-driven workflow selector:

```bash
./axiom
```

Use the arrow keys and Enter to choose project analysis, refactoring, project verification, or single-file verification. Automated environments should continue using the flag-based commands because interactive mode requires a terminal.

To generate an isolated improved Java version of a selected method:

```bash
./axiom --project /path/to/project \
  --entry 'com.example.Service.calculate(int)' \
  --refactor --rewrite --approve-refactor
```

The generated source is written under `.axiom/generated/java/` and is never applied to the original project automatically.

---

## Real-World Verification Example

### Legacy C++ Code

```cpp
int calculateDiscount(int amount, int tier) {
    if (amount > 1000) {
        if (tier == 2) return amount - 100;
        return amount - 50;
    }
    return amount;
}
```

### Rewritten Go Code (with subtle off-by-one error)

```go
func CalculateDiscount(amount int, tier int) int {
    if amount >= 1000 { // BUG: Developer introduced >= instead of >
        if tier == 2 {
            return amount - 100
        }
        return amount - 50
    }
    return amount
}
```

### Axiom Console Output

```
[AXIOM] Parsing ASTs for legacy [C++] and modern [Go]... Done.
[AXIOM] Extracting Symbolic Path Constraints... Done.
[AXIOM] Invoking Z3 Theorem Prover...

EQUIVALENCE VERIFICATION FAILED
-----------------------------------------------------------------
Reason: Logic Discrepancy Detected at Boundary Conditions.

Counterexample Inputs Generated by Z3:
  amount = 1000
  tier   = 2

Execution Trace Divergence:
  Legacy Output : 1000  (No discount applied)
  Modern Output : 900   (Discount applied prematurely)

Root Cause: Boundary condition conflict at operator >= vs > on Line 2.
-----------------------------------------------------------------
```

---

## Built With

- [Go](https://go.dev/) — High concurrency, clean architecture
- [go-tree-sitter](https://github.com/smacker/go-tree-sitter) — Fast multi-language concrete syntax trees
- [Z3 SMT Solver](https://github.com/Z3Prover/z3) — Formal logic and mathematical theorem proving via SMT-LIB2

---

## Tech Stack

| Layer | Technology | Purpose |
| --- | --- | --- |
| Core Language | Go 1.22+ | High concurrency via goroutines, fast execution, single binary distribution |
| AST Parsing Engine | `smacker/go-tree-sitter` | Multi-language parsing (C++, Java, Go, PHP) into concrete syntax trees |
| SMT Solver Engine | Z3 SMT Solver (Microsoft) | Mathematical logic verification & counterexample solving via SMT-LIB2 IPC |
| CLI Framework | `spf13/cobra` | Command-line tool orchestration, command routing, and flag handling |
| Terminal UI & Diffs | `charmbracelet/lipgloss` | Structured terminal formatting, error styling, and counterexample diff rendering |

---

## Project Structure

```text
axiom/
├── cmd/
│   └── axiom/
│       └── main.go             # Application entry point & Cobra CLI setup
├── internal/
│   ├── ast/
│   │   ├── parser.go           # Tree-sitter initialization & language grammars
│   │   └── walker.go           # AST node traversal & syntax abstraction
│   ├── symbolic/
│   │   ├── constraint.go       # Branch mapping (if/else/switch path constraints)
│   │   └── expression.go       # AST-to-symbolic variable representations
│   ├── solver/
│   │   ├── smtlib.go           # SMT-LIB2 query builder and logic generator
│   │   └── z3.go               # Z3 CLI process execution & response parser
│   └── reporter/
│       └── reporter.go         # Counterexample formatting & line-by-line diffs
├── examples/                   # Test code pairs for verification runs
│   ├── legacy_discount.cpp
│   └── modern_discount.go
├── go.mod                      # Go module declaration
└── go.sum                      # Go dependency checksums
```

---

## Roadmap

- [x] SMT-LIB2 pipeline runner for Go
- [x] Basic condition branch symbolic expression mapping
- [ ] Multi-language Tree-sitter AST walker (C++, Java to Go)
- [ ] Loop unrolling & invariant extraction support
- [ ] GitHub Action runner for PR regression prevention

---

## License

Distributed under the MIT License. See `LICENSE` for more information.
