# Axiom Execution Plan

This is the step-by-step implementation plan for expanding Axiom into a project-aware analysis, Java restructuring, and behavioral verification tool.

## Operating Rules

- Work in small, testable slices.
- Preserve existing source files by default.
- Generate plans, patches, and new files into a separate output directory.
- Never claim equivalence when unsupported behavior affects the selected entry point.
- Every phase must end with focused tests and `go test ./...` before the next phase.
- Update this file as phases are completed.

## Phase 0: Stabilize the Current Code

Status: completed

1. Remove duplicate package declarations from edited test and implementation files.
2. Run `go test ./...` and `go vet ./...`.
3. Keep parser syntax-error rejection covered by tests.
4. Add tests for multiple functions, missing selected functions, and project indexing failures.
5. Update `ISSUES.md` so it remains a backlog and points here for execution order.

Completion gate: the complete test suite passes and malformed source produces a clear diagnostic.

## Phase 1: Build a Real Java Symbol Model

Status: completed

1. [x] Add package, class, interface, field, constructor, and method models.
2. [x] Record fully qualified names, parameter types, return types, visibility, and source metadata.
3. [x] Support Java inheritance and implemented interfaces as resolved relationships.
4. [x] Add Java fixtures covering packages, overloaded methods, constructors, and inheritance.
5. [x] Add tests for Java symbol extraction.

Completion gate: a Java project can be indexed with correct packages, classes, methods, fields, and signatures.

## Phase 2: Resolve Cross-File Symbols

Status: completed

1. [x] Resolve explicit imports to packages and source files.
2. [x] Resolve typed instance method calls and inherited methods.
3. [x] Resolve constructors and static method calls completely.
4. [x] Match overloaded methods using argument types, not only argument counts.
5. [x] Classify relationships as resolved, unresolved, external, or ambiguous.
6. [x] Replace all remaining best-effort name matching in the dependency graph.

Completion gate: a multi-file Java fixture produces correct call edges and reports unresolved calls explicitly.

## Phase 3: Add Project Entry-Point Resolution

Status: completed

1. Accept a fully qualified entry symbol.
2. Reject missing, ambiguous, and incompatible entry methods.
3. Compute only the dependency graph reachable from that entry point.
4. Report the selected entry and all included files and symbols.

Example:

```text
axiom analyze --project ./legacy-app \
  --entry com.example.DiscountService.calculate
```

Completion gate: analysis is limited to the selected method and its reachable dependencies.

## Phase 4: Add Method Summaries and Assumptions

Status: completed

1. [x] Classify methods as pure, stateful, external, or unsupported.
2. [x] Produce metadata summaries for pure methods and their dependencies.
3. [x] Represent external calls with explicit assumptions.
4. [x] Track exceptions, mutation, shared state, and side effects conservatively.
5. [x] Surface unsupported behavior instead of treating it as pure.

Completion gate: reports distinguish proven behavior, assumptions, and unsupported behavior.

## Phase 5: Add Project-Level Verification

Status: in progress

1. [x] Accept legacy and modern project roots.
2. [x] Resolve one entry symbol in each project.
3. [x] Validate compatible signatures and return types.
4. [x] Build reachable dependency graphs for both projects.
5. [x] Compare directly modelable entries with Z3 and refuse unsound dependency proofs.
6. [x] Report `Proven equivalent`, `Not equivalent`, and `Could not prove` outcomes with reasons.
7. [x] Expand pure reachable method summaries into the entry SMT model.
8. [ ] Produce `Equivalent under assumptions` only after assumption-aware modeling is implemented.

Example:

```text
axiom verify \
  --legacy-project ./legacy \
  --modern-project ./structured \
  --legacy-entry com.old.Discount.calculate \
  --modern-entry com.new.DiscountService.calculate
```

Completion gate: dependent files are analyzed without manually passing every file, and unresolved or unmodeled behavior cannot be reported as proven equivalent.

## Phase 6: Improve Java Refactoring Analysis

Status: completed

1. [x] Detect large methods, deep nesting, repeated conditions, excessive parameters, and mixed responsibilities.
2. [x] Detect persistence, logging, messaging, and business logic mixed in one method.
3. [x] Identify candidate extracted methods, classes, interfaces, and predicates.
4. [x] Include exact source locations and reasons in `refactor-plan.md`.
5. [x] Generate characterization-test recommendations.

Completion gate: plans contain specific methods, evidence, proposed structure, and review warnings.

## Phase 7: Generate Java Files and Patches

Status: completed

1. [x] Generate reviewable Java adapters and patches only after `--approve-refactor`.
2. [x] Generate behavior-preserving delegation implementations while preserving selected method signatures.
3. [x] Write output under `.axiom/generated/` and `.axiom/patches/`.
4. [x] Include source finding and line metadata in the artifact manifest and generated scaffold.
5. [x] Never overwrite the original project by default.

Completion gate: generated files are reviewable and isolated from the legacy source tree. Deeper internal method extraction remains a future transformation.

## Phase 8: Compile and Validate Generated Code

Status: completed

1. [x] Detect Maven, Gradle, or plain Java projects.
2. [x] Compile generated Java artifacts with `javac` in an isolated classes directory.
3. [x] Run optional `google-java-format` validation when installed.
4. [x] Run Maven or Gradle project tests when a build tool is detected.
5. [x] Record characterization/differential validation as explicitly skipped when no project harness exists.
6. [x] Record compilation, formatting, test, and differential statuses in `artifacts.json`.

Completion gate: generated Java compilation and available project validation are recorded explicitly; missing project-specific differential harnesses cannot be mistaken for a proof.

## Phase 9: Improve Reports and CI

Status: in progress

1. [x] Add JSON and SARIF output.
2. [x] Include source locations, dependency edges, assumptions, unsupported constructs, and counterexamples.
3. [x] Add CI-friendly exit codes.
4. [ ] Add explicit approval for applying patches.
5. [x] Add GitHub Action integration.

Completion gate: CI can consume reports and fail safely on counterexamples or unvalidated generated code.

## Immediate Next Action

Complete Phase 0, then begin Phase 1 with Java symbol extraction and fixtures. Do not begin code generation until project symbols, call resolution, and validation gates are reliable.
