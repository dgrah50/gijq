---
name: bug-hunt
schedule: daily
maxProposals: 2
---
Audit the repository's non-test production source files for concrete, reproducible reliability bugs:
- null/undefined or nil dereferences
- off-by-one indexing, slicing, or boundary wrapping bugs
- incomplete input validation or non-atomic state mutations on error
- resource leaks or swallowed errors

Do NOT propose style nits, refactors, dependency upgrades, or changes to protected config/CI files.
For each finding, provide:
1. **File & Symbol:** exact path and function/method name
2. **Problem:** concise explanation of the failure mode
3. **Expected Behavior:** exact fix requirement
4. **Verification:** a concrete new test (do not edit existing tests) that compiles against `main` and fails today
