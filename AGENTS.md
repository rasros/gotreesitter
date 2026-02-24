## Memory Safety (WSL2)
- NEVER run parallel bash commands
- NEVER use compound `cd && redirect` commands — split into separate steps
- Always pipe test/benchmark output to files, never stdout
- Run `go build` on ONE module at a time

## Exploration Limits
- Limit initial codebase exploration to 15 tool uses max
- Summarize findings before continuing
- Do NOT recursively explore replace-directive dependencies (e.g., gotreesitter)
  — ask first if deep exploration is needed

## Build Order
- Build gotreesitter first: `cd /home/draco/work/gotreesitter && go build ./...`
- Then build mane: `cd /home/draco/work/mane && go build ./...`
- Sequential only
