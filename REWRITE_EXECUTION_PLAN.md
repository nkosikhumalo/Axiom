# Axiom Legacy Rewrite Execution Plan

This plan defines the implementation path for generating improved versions of legacy code while preserving behavior and keeping original source files unchanged.

## Operating Rules

- Generate all rewritten code under `.axiom/generated/` or a user-selected output directory.
- Never overwrite legacy source automatically.
- Require explicit approval before applying a patch.
- Refuse to claim equivalence when unsupported behavior affects the selected entry point.
- Validate every generated artifact by parsing, compiling, testing, and differential comparison when available.
- Keep assumptions, unsupported constructs, and source locations in the artifact manifest.

## Phase 1: Java-to-Java Method Rewrite

Status: in progress

### Goal

Given a selected Java method, generate an isolated Java class that preserves its package, signature, and supported method body.

### Steps

1. [x] Select a project, source scope, source file, and method through the interactive workflow.
2. [x] Resolve the selected method from the project index.
3. [x] Read and parse the selected method body from the original source file.
4. [x] Generate an isolated Java class containing the method signature and body.
5. [x] Write a reviewable patch beside the generated source.
6. [x] Record the generated artifact and source finding in `artifacts.json`.
7. [x] Expose generation through `--rewrite --approve-refactor`.
8. [x] Add interactive generation through `Generate an improved Java version`.
9. [ ] Preserve required imports, fields, constructors, and type dependencies.
10. [x] Compile generated source independently and report source-project failures separately.
11. [ ] Generate characterization tests for the rewritten method.
12. [ ] Compare legacy and generated behavior with differential tests.

### Completion Gate

A pure, self-contained Java method generates a readable Java implementation that compiles, passes characterization tests, and has no unsupported dependencies.

## Phase 2: Java Class-Aware Rewrite

Status: not started

1. Preserve imports used by the selected method.
2. Preserve required fields and field types.
3. Preserve constructors and initialization needed by the method.
4. Preserve annotations and visibility contracts.
5. Generate a renamed isolated class and an optional compatibility adapter.
6. Compile the generated class with its required project dependencies.

## Phase 3: Legacy Java Restructuring

Status: not started

1. Use `java-legacy` round0 as the original baseline.
2. Detect documented smells such as mixed persistence, networking, validation, and domain logic.
3. Generate focused collaborators for address, name, SSN, and persistence responsibilities.
4. Preserve the existing public API through adapters.
5. Generate characterization tests from the round0 behavior.
6. Compare generated output against later workshop rounds.

## Phase 4: C++ Legacy Rewrite

Status: in progress (indexing foundation)

1. [x] Index C++ headers, namespaces, and free functions.
2. [x] Resolve namespaces, header/source files, and namespaced free-function calls.
3. [ ] Resolve includes and cross-file calls.
4. [ ] Build a language-neutral representation for supported C++ constructs.
5. [ ] Generate modern C++ first, then Go where types and ownership are explicit.
6. [ ] Compile with the project toolchain.
7. [ ] Validate with tests and differential execution.

## Phase 5: PHP Legacy Rewrite

1. Index namespaces, classes, traits, functions, and methods.
2. Resolve includes and application calls.
3. Model dynamic behavior conservatively.
4. Generate modern PHP or a typed target representation.
5. Refuse generation when dynamic behavior cannot be preserved safely.

## Phase 6: Cross-Language Verification

1. Compare legacy and generated reachable entry points.
2. Expand pure dependencies into SMT.
3. Use characterization and differential tests for external or unsupported behavior.
4. Produce counterexamples when outputs differ.
5. Return `could_not_prove` when assumptions remain unresolved.

## Current Command

```bash
./axiom \
  --project /path/to/legacy-java-project \
  --entry 'com.example.Service.calculate(int)' \
  --refactor --rewrite --approve-refactor
```

Generated output is isolated under the selected output directory:

```text
.axiom/
  generated/java/
  patches/
  artifacts.json
```
