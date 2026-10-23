## What and why / 改了什么、为什么

## Checklist / 检查项

- [ ] `make check` passes (Go vet + tests, web lint + typecheck + build)
- [ ] The agent is still send-only: no new dependencies, no command execution, no listening sockets, no response parsing
- [ ] New admin endpoints are registered only in `mountAdmin` and wrapped with `requireAdmin`
- [ ] Docs updated (English and 中文) if behaviour or configuration changed
