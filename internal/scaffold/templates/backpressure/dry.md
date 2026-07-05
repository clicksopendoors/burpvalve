# DRY - Do Not Repeat Yourself

Purpose: prevent duplicate logic, copy-pasted branches, repeated validation, repeated API/client code, repeated prompt text, and parallel implementations of the same behavior.

The DRY subagent should check the staged feature for:

- copied code blocks that should be extracted or shared;
- duplicated conditionals, loops, validation rules, error handling, or mapping logic;
- repeated constants, strings, prompts, or magic numbers that need names or central definitions;
- new helpers that duplicate existing helpers;
- nearly identical tests that should use tables, fixtures, or shared builders;
- abstractions added too early that hide duplication instead of removing it.

The subagent should pass only when repetition is intentional, local, and justified, or when the implementation removes meaningful duplication without creating a worse abstraction.

If the condition is not met, attach a message naming the duplicated areas, the preferred extraction or consolidation, and whether the commit should be split before refactoring.
