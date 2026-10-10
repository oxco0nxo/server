# Instructions for agents

- Use `make help` to find available development targets
- Before committing `.go` changes, run `make fmt` to format, and run `make lint` to lint
- Before committing `go.mod` changes, run `go mod tidy`
- Before committing new `.go` files, add the current year into the copyright header
- Before committing any files, remove all trailing whitespace from source code lines
- Add test code for newly added functions or new logic of old functions