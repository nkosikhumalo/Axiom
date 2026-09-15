# Axiom Issues and Implementation Plan

This document is the working backlog for expanding Axiom from a two-file equivalence prototype into a project-aware analysis, refactoring, and verification tool.

## Current Progress

- [x] Reject syntax-error ASTs before symbolic analysis.
- [x] Track unreachable returns, assignments, casts, ternaries, switches, and boolean values.
- [x] Select verification functions explicitly with `--legacy-function` and `--modern-function`.
- [x] Discover supported project files and write `dependency-graph.json`.
- [x] Generate a non-destructive Java `refactor-plan.md` proposal.
- [x] Index Java packages, classes, interfaces, constructors, overload signatures, and source locations.
- [x] Resolve imported typed Java method calls and classify unresolved external calls.
- [x] Resolve cross-file Java calls through imports, packages, receivers, inheritance, constructors, and overload signatures.
- [x] Resolve project entry points and restrict analysis to reachable symbols.
- [x] Model method summaries, side effects, exceptions, and explicit external-system assumptions.
- [x] Compare directly modelable reachable legacy and modern entries with Z3.
- [x] Expand pure reachable method summaries into project-level SMT.
- [ ] Model external assumptions soundly for `equivalent_under_assumptions` results.
- [x] Generate evidence-based Java refactoring findings with source locations.
- [x] Generate approved Java adapters, patches, and an artifact manifest without overwriting source.
- [ ] Extract internal method bodies into new collaborators instead of delegation adapters.
- [x] Compile generated Java adapters in an isolated `javac` output directory.
- [x] Detect Maven and Gradle project tests and record their statuses.
- [x] Record formatter and differential-validation limitations explicitly.
- [ ] Generate source patches or new Java files from an approved plan.

## Current Product Boundary

Axiom's current implementation:

- Parses one legacy file and one modern file for direct verification, and can index a project directory.
- Extracts paths from the first discovered function body.
- Models supported conditions, returns, assignments, casts, ternaries, and switches.
- Builds SMT-LIB expressions and asks Z3 whether outputs can differ.
- Prints a terminal pass/fail report.
- Writes a project dependency index and a non-destructive Java refactoring plan.

Axiom currently does not semantically resolve cross-file calls, generate source code, refactor Java automatically, compile generated code, or create reviewable patches.

## Product Modes

The CLI should eventually expose three explicit workflows:

```text
axiom analyze  --project ./legacy-app
axiom verify   --project ./legacy-app --entry com.example.DiscountService.calculate
axiom refactor --project ./legacy-app --entry com.example.DiscountService.calculate --language java
```

- `analyze` explains project structure, dependencies, unsupported constructs, and risks.
- `verify` compares behavior at a selected function or method.
- `refactor` proposes and optionally generates reviewable changes.

The default behavior must never overwrite a user's source code.

## Multi-File Legacy Projects

For a file connected to ten or more interconnected files, Axiom must:

1. Discover source files, packages, imports, includes, classes, interfaces, methods, and fields.
2. Parse every supported file and report syntax errors separately from semantic limitations.
3. Build a dependency graph rooted at a selected entry function or method.
4. Resolve calls, fields, constructors, inheritance, overloads, and shared state where possible.
5. Analyze reachable code recursively instead of treating cross-file calls as opaque variables.
6. Create summaries for large or external dependencies.
7. Mark database, network, filesystem, reflection, native, and concurrent behavior as explicit assumptions.
8. Report which files and symbols were analyzed, skipped, summarized, or unsupported.

The user must select the analysis boundary by a fully qualified symbol, for example:

```text
com.example.DiscountService.calculate(Customer, Order)
```

Axiom must not silently analyze the first function found when multiple functions or methods exist.

### Dependency Graph Example

```text
DiscountService.calculate
  +-- CustomerRepository.findCustomer
  |     +-- DatabaseClient
  +-- PricingRules.getDiscount
  +-- AuditLogger.log
```

Pure calculation methods can be symbolically expanded. External or side-effecting methods need contracts such as:

```text
findCustomer(id):
  returns a Customer when one exists
  may throw CustomerNotFoundException
  reads database state
```

Reports must distinguish:

- `Proven equivalent`
- `Equivalent under assumptions`
- `Not equivalent`
- `Could not prove`

## Java-to-Java Restructuring

Java must be a first-class restructuring target. Users should be able to keep Java while making the code clearer and more maintainable.

Potential transformations include:

- Extract long conditional blocks into named methods.
- Separate calculation logic from database access, logging, and messaging.
- Introduce domain classes around primitive data.
- Replace duplicated conditions with reusable predicates.
- Split large services into focused collaborators.
- Detect and isolate mutable shared state.
- Rename unclear variables and methods.
- Group related constants and rules.
- Introduce interfaces around external services.
- Generate characterization tests before changing behavior.

For example, a large method mixing validation, persistence, logging, and pricing could be restructured into:

```text
DiscountService.java
DiscountPolicy.java
DiscountContext.java
CustomerValidator.java
PricingRepository.java
```

The generated structure must preserve behavior and should preserve public APIs, package names, and serialization contracts when requested.

## Generated Artifacts

Every analysis or refactoring run should produce inspectable output under `.axiom/` or a user-selected directory:

```text
.axiom/
  dependency-graph.json
  analysis-report.json
  assumptions.json
  refactor-plan.md
  patches/
    001-extract-pricing-rules.patch
  generated/
    java/
      DiscountService.java
```

The tool should generate patches and new files first. Applying changes to the original project requires explicit user approval.

## Validation Requirements

Symbolic equivalence alone is not enough for safe restructuring. Generated code must be:

- Parsed without syntax errors.
- Compiled with the project's configured toolchain.
- Formatted with the project's formatter.
- Checked with the project's tests when available.
- Compared with characterization or differential tests when symbolic reasoning is incomplete.
- Reported with exact assumptions, unsupported constructs, and confidence levels.

Loops, exceptions, collections, floating point, strings, aliasing, reflection, concurrency, and I/O require dedicated models or explicit limitations. Axiom must return `Could not prove` when limitations prevent a sound conclusion; it must not report a false proof.

## Execution Order

The step-by-step implementation sequence is maintained in [EXECUTION_PLAN.md](EXECUTION_PLAN.md).
This file remains focused on the issue backlog, current limitations, and desired behavior.

## Guiding Rule

Axiom first explains the legacy system, then proposes changes, then generates reviewable code, and finally verifies the result.
